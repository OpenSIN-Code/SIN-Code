// SPDX-License-Identifier: MIT
// Purpose: LLM-as-a-Judge for SIN-Code eval (issue #75, M2). Prompts
// a generic OpenAI-compatible LLM via internal/llm.Client with a
// deterministic JSON-shaped response that callers can parse.
//
// The judge relies on the LLM returning ONE JSON object with the
// canonical JudgeResult schema. We do NOT prescribe a JSON-schema
// `response_format` parameter because the upstream
// internal/llm.ChatRequest struct has no such field today; we
// simply demand JSON in the prompt and parse strictly. If the LLM
// returns prose we return an error and let the caller decide.
//
// Docs: judge.doc.md
package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

// JudgeResult is the canonical shape returned by every LLM judge. It
// also populates every RunResult.Judge* field via dataset.Runner.
type JudgeResult struct {
	Pass     bool               `json:"pass"`
	Score    float64            `json:"score"`
	Reason   string             `json:"reason"`
	Feedback string             `json:"feedback,omitempty"`
	Criteria map[string]float64 `json:"criteria,omitempty"`
}

// MinPassScore is the default threshold below which a JudgeResult is
// marked Fail. Configurable per Judge instance.
const MinPassScore = 0.7

// JudgeConfig configures one Judge. Zero values are filled with
// safe defaults in NewJudge.
type JudgeConfig struct {
	Model        string
	Temperature  float64
	MinPassScore float64
	Strict       bool
	MaxTokens    int
}

// Trajectory is the minimal record the judge needs to evaluate one
// agent run. Built by the dataset runner after each case completes.
type Trajectory struct {
	Prompt         string   `json:"prompt"`
	Turns          int      `json:"turns"`
	ToolsUsed      []string `json:"tools_used"`
	VerifyPassed   bool     `json:"verify_passed"`
	FinalOutput    string   `json:"final_output"`
	SessionID      string   `json:"session_id"`
	Duration       string   `json:"duration"`
	CustomCriteria string   `json:"custom_criteria,omitempty"`
}

// Judge wraps llm.Client with a configuration. Safe for concurrent
// calls (the underlying http.Client is too).
type Judge struct {
	cfg    JudgeConfig
	client *llm.Client
}

// NewJudge constructs a Judge. Missing required fields surface as an
// error — the caller almost always wants to know which field.
func NewJudge(cfg JudgeConfig, client *llm.Client) (*Judge, error) {
	if client == nil {
		return nil, errors.New("eval: llm client is nil")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("eval: judge model is empty")
	}
	if cfg.MinPassScore == 0 {
		cfg.MinPassScore = MinPassScore
	}
	if cfg.MinPassScore < 0 || cfg.MinPassScore > 1 {
		return nil, fmt.Errorf("eval: MinPassScore must be in [0,1], got %v", cfg.MinPassScore)
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.3
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 512
	}
	return &Judge{cfg: cfg, client: client}, nil
}

// Evaluate runs one trajectory through the LLM and returns a
// parsed JudgeResult. The Pass field is recomputed from Score and
// the configured MinPassScore so a strict mode flip happens at the
// call site, not inside the model.
func (j *Judge) Evaluate(ctx context.Context, traj Trajectory) (*JudgeResult, error) {
	if ctx == nil {
		return nil, errors.New("eval: nil context")
	}
	resp, err := j.client.Chat(ctx, llm.ChatRequest{
		Model:       j.cfg.Model,
		Temperature: j.cfg.Temperature,
		MaxTokens:   j.cfg.MaxTokens,
		Messages: []llm.Message{
			{Role: "system", Content: j.buildSystemPrompt(traj)},
			{Role: "user", Content: j.buildUserPrompt(traj)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("eval: llm call: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("eval: llm returned no choices")
	}
	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	// Strip surrounding ```json fences if the model added them.
	raw = stripJSONFence(raw)
	var result JudgeResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("eval: parse judge response: %w (raw: %q)", err, raw)
	}
	// Force Pass based on the configured threshold to override any
	// model-side LLM drift that ignores the prompt instruction.
	result.Pass = result.Score >= j.cfg.MinPassScore
	return &result, nil
}

// EvaluateBatch is the convenience loop over a slice. Errors short-
// circuit; callers that want partial results should call Evaluate
// individually.
func (j *Judge) EvaluateBatch(ctx context.Context, trajs []Trajectory) ([]*JudgeResult, error) {
	out := make([]*JudgeResult, len(trajs))
	for i, t := range trajs {
		r, err := j.Evaluate(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("eval: trajectory[%d]: %w", i, err)
		}
		out[i] = r
	}
	return out, nil
}

// buildSystemPrompt returns the role + rules the judge must follow.
// Kept compact (≤ 600 tokens typical) so cheap models follow it.
func (j *Judge) buildSystemPrompt(_ Trajectory) string {
	var sb strings.Builder
	sb.WriteString(`Du bist ein strenger, deterministischer Evaluator für KI-Agenten-Trajektorien (LLM-as-a-Judge).
Bewerte die folgende Trajektorie objektiv und antworte NUR mit einem JSON-Objekt (kein Markdown, kein Code-Block).

Schema (exakt):
{"pass": <bool>, "score": <float 0.0–1.0>, "reason": "<kurz>", "feedback": "<konstruktiv>", "criteria": {"goal": <0–1>, "tools": <0–1>, "reasoning": <0–1>, "verify": <0–1>}}

Gewichtung:
- Goal (40%): Wurde das im Prompt gesteckte Ziel vollständig + korrekt erreicht?
- Tools (25%): Wurden die richtigen Tools effizient genutzt (kein unnötiger Call)?
- Reasoning (25%): War die Schlussfolgerungskette logisch und nachvollziehbar?
- Verify (10%): Wurde das Verify-Gate ausgelöst und bestanden?`)
	if j.cfg.Strict {
		sb.WriteString("\n\nSTRICT MODE: Score >= 0.9 erforderlich für pass=true. Jede Verletzung eines Constraints oder Verified=false gibt ≥ 0.1 Abzug.")
	}
	return sb.String()
}

// buildUserPrompt carries the trajectory payload. We bound each
// field so a runaway transcript never bloats the prompt past the
// model's context window.
func (j *Judge) buildUserPrompt(traj Trajectory) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Prompt: %s\n", truncate(traj.Prompt, 1000))
	if len(traj.ToolsUsed) > 0 {
		fmt.Fprintf(&sb, "Tools used: %s\n", strings.Join(traj.ToolsUsed, ", "))
	}
	fmt.Fprintf(&sb, "Turns: %d  Duration: %s  Verify passed: %v\n", traj.Turns, traj.Duration, traj.VerifyPassed)
	fmt.Fprintf(&sb, "Final output: %s\n", truncate(traj.FinalOutput, 4000))
	if traj.CustomCriteria != "" {
		fmt.Fprintf(&sb, "Custom criteria: %s\n", traj.CustomCriteria)
	}
	return sb.String()
}

// stripJSONFence removes ```json ... ``` fences the LLM sometimes
// wraps the response in.
func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i > 0 {
			s = s[i+1:]
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	return strings.TrimSpace(s)
}

// truncate clamps a string to n runes, appending "…(truncated)" if
// truncated. Stays in the same package so buildUserPrompt + any
// future prompt builder share the helper.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}
