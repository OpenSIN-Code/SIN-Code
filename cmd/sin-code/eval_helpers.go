// SPDX-License-Identifier: MIT
// Purpose: Eval helper functions — stub run override, LLM client/judge
// wiring, trace exporter parsing, and workspace root derivation.
// Extracted from eval_cmd.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: consolidate when eval is refactored
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	sinctrace "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
)

// ── helpers ─────────────────────────────────────────────────────────

// stubRunOverride is the offline / CI Loop.RunOverride. It echoes a
// canonical "stub" marker as the agent's final output so structural
// rules in the dataset (contains / avoids / max_turns) have something
// to check without a real LLM. The contents are predictable so unit
// tests can assert exact bytes. Verified=true because the stub is
// treated as a successful loop by the runner; dataset-level rules
// (keywords, max_turns) still cause pass/fail flips.
func stubRunOverride(_ context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
	out := "stub echo: " + prompt
	return &agentloop.Result{
		SessionID: sess.ID,
		Summary:   out,
		Verified:  true,
		Turns:     1,
	}, nil
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or reserved follow-up is implemented
// newLLMClientFor returns a *llm.Client. Centralised so the cmd
// package keeps a single import path for the LLM bridge.
func newLLMClientFor(endpoint, apiKey string) *llm.Client { //nolint:unused // reserved for follow-up
	return llm.NewClient(endpoint, apiKey)
}

// mustParseExporter returns ExporterKind, falling back to noop for
// unknown values with a stderr warning. We don't return the error to
// keep the RunE flow linear; the unknown kind is not fatal.
func mustParseExporter(s string) sinctrace.ExporterKind {
	kind, err := sinctrace.ParseExporter(s)
	if err != nil {
		var ue *sinctrace.UnknownExporterError
		if errors.As(err, &ue) {
			fmt.Fprintf(os.Stderr, "warn: %s — using noop\n", ue.Error())
		}
		return sinctrace.ExporterNoop
	}
	return kind
}

// buildEvalCompletion constructs a chat-completion func for the agent
// loop when --use-model is in effect (issue #261). The client honours
// llm.base_url / llm.api_key / llm.model from the merged config, with
// env vars LLM_API_KEY / LLM_MODEL overriding transparently.
func buildEvalCompletion() (func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error), error) {
	cfg, err := internal.LoadMergedConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.LLMAPIKey)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY env or llm.api_key config required")
	}
	baseURL := strings.TrimSpace(cfg.LLMBaseURL)
	if baseURL == "" {
		baseURL = "https://integrate.api.nvidia.com/v1"
	}
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	if model == "" {
		model = strings.TrimSpace(cfg.LLMModel)
	}
	if model == "" {
		return nil, fmt.Errorf("LLM_MODEL env or llm.model config required")
	}
	client := llm.NewClient(baseURL, apiKey)
	maxTokens := cfg.LLMMaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	temperature := cfg.LLMTemperature
	if temperature == 0 {
		temperature = 0.2
	}
	return agentloop.NewProviderCompletion(client, model, maxTokens, temperature), nil
}

// applyJudge runs the LLM-as-a-Judge over results and writes the
// score/feedback back into each RunResult. Returns an error when the
// judge can't even be constructed (no API key, no model). The CLI
// caller turns that into a stderr warning but keeps the JSON report.
func applyJudge(ctx context.Context, results []dataset.RunResult, model, endpoint, keyEnv string) error {
	apiKey := os.Getenv(keyEnv)
	if apiKey == "" {
		return fmt.Errorf("judge: env %s is empty", keyEnv)
	}
	client := newLLMClientFor(endpoint, apiKey)
	judge, err := eval.NewJudge(eval.JudgeConfig{Model: model}, client)
	if err != nil {
		return fmt.Errorf("judge: build: %w", err)
	}
	for i := range results {
		if !results[i].Success {
			continue
		}
		traj := eval.Trajectory{
			Prompt:       results[i].TestCaseID,
			Turns:        results[i].Turns,
			ToolsUsed:    results[i].ToolsUsed,
			VerifyPassed: results[i].VerifyPassed,
			FinalOutput:  results[i].FinalOutput,
			SessionID:    results[i].SessionID,
			Duration:     results[i].Duration.String(),
		}
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		r, err := judge.Evaluate(cctx, traj)
		cancel()
		if err != nil {
			// Per-result errors don't kill the batch.
			results[i].JudgeFeedback = "judge error: " + err.Error()
			continue
		}
		results[i].JudgeScore = r.Score
		results[i].JudgeFeedback = r.Feedback
		if !r.Pass {
			results[i].Success = false
			if results[i].Error == "" {
				results[i].Error = "judge failed"
			}
		}
	}
	return nil
}

// workspaceRoot derives a reasonable workspace for the agent loop
// when a dataset lives under evals/<file>. Picks the parent-of-parent
// so the agent can read evals/ without chasing its own tails.
func workspaceRoot(datasetPath string) string {
	abs, err := filepath.Abs(datasetPath)
	if err != nil {
		return "."
	}
	dir := filepath.Dir(abs)
	if filepath.Base(dir) == "evals" {
		return filepath.Dir(dir)
	}
	return dir
}
