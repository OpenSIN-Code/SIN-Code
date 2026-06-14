// SPDX-License-Identifier: MIT
// Purpose: Dataset Runner - executes test cases and collects results
package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
)

// RunResult repräsentiert das Ergebnis eines einzelnen Test-Durchlaufs
type RunResult struct {
	TestCaseID    string        `json:"test_case_id"`
	Passed        bool          `json:"passed"`
	Turns         int           `json:"turns"`
	ToolsCalled   []string      `json:"tools_called"`
	Duration      time.Duration `json:"duration_ms"`
	VerifyPassed  bool          `json:"verify_passed"`
	Error         string        `json:"error,omitempty"`
	AgentOutput   string        `json:"agent_output,omitempty"`
	JudgeScore    float64       `json:"judge_score"`
	JudgeFeedback string        `json:"judge_feedback,omitempty"`
}

// RunnerConfig enthält Konfiguration für den Dataset Runner
type RunnerConfig struct {
	TimeoutPerCase time.Duration
	OutputFile     string
	Headless       bool
}

// Runner führt Testfälle aus und sammelt Ergebnisse
type Runner struct {
	config  RunnerConfig
	results []RunResult
}

// NewRunner erstellt einen neuen Dataset Runner
func NewRunner(cfg RunnerConfig) *Runner {
	return &Runner{
		config:  cfg,
		results: make([]RunResult, 0),
	}
}

// Run führt alle Testfälle eines Datasets aus
func (r *Runner) Run(ctx context.Context, ds *Dataset) error {
	if ds == nil || len(ds.TestCases) == 0 {
		return fmt.Errorf("dataset is empty")
	}

	fmt.Printf("🚀 Running %d test cases from dataset '%s'\n", len(ds.TestCases), ds.Name)
	fmt.Println(string([]byte{45, 45, 45, 45, 45, 45, 45, 45, 45, 45, 45, 45, 45, 45, 45, 45}))
	fmt.Println()

	for i, tc := range ds.TestCases {
		fmt.Printf("[%d/%d] Running: %s\n", i+1, len(ds.TestCases), tc.ID)
		result := r.executeTestCase(ctx, &tc)
		r.results = append(r.results, result)

		if result.Error != "" {
			fmt.Printf("  ❌ Error: %s\n", result.Error)
		} else {
			status := "✅"
			if !result.Passed {
				status = "❌"
			}
			fmt.Printf("  %s Judge Score: %.2f | Verify: %v | Turns: %d\n",
				status, result.JudgeScore, result.VerifyPassed, result.Turns)
		}
	}

	fmt.Println()
	return r.SaveResults(r.config.OutputFile)
}

// executeTestCase führt einen einzelnen Testfall aus
func (r *Runner) executeTestCase(ctx context.Context, tc *TestCase) RunResult {
	start := time.Now()
	result := RunResult{TestCaseID: tc.ID}

	// Timeout pro Case anwenden
	if r.config.TimeoutPerCase > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.TimeoutPerCase)
		defer cancel()
	}

	// 1. Agent-Loop starten mit tc.Prompt
	agentOutput, turns, tools, err := r.runAgentWithPrompt(ctx, tc)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	result.Turns = turns
	result.ToolsCalled = tools
	result.AgentOutput = truncateString(agentOutput, 500)

	// 2. Constraints validieren
	if !r.validateConstraints(tc, turns, tools) {
		result.Passed = false
		result.Duration = time.Since(start)
		return result
	}

	// 3. Verify-Command ausführen (falls vorhanden)
	if tc.Expected.VerifyCmd != "" {
		verifyResult := r.executeVerifyCommand(ctx, tc.Expected.VerifyCmd)
		result.VerifyPassed = verifyResult
	} else {
		result.VerifyPassed = true
	}

	// 4. LLM-as-a-Judge: Bewertung durchführen
	judge := eval.NewJudge("openai/gpt-4-mini")
	judgeResult := judge.Evaluate(ctx, tc.Expected.Criteria, agentOutput, tools)

	result.JudgeScore = judgeResult.Score
	result.JudgeFeedback = judgeResult.Feedback
	result.Passed = judgeResult.Passed && result.VerifyPassed

	result.Duration = time.Since(start)
	return result
}

// runAgentWithPrompt startet den Agent mit einem Prompt und sammelt Ergebnisse
func (r *Runner) runAgentWithPrompt(ctx context.Context, tc *TestCase) (output string, turns int, tools []string, err error) {
	// Mock-Implementierung – in Production würde agentloop.Loop.Run() aufgerufen
	// Loop würde initialisiert mit:
	//   - LocalTool: echte Tool-Implementierungen
	//   - LocalSpec: echte Tool-Spezifikationen
	//   - MaxTurns: aus tc.Constraints.MaxTurns
	//   - Completion: LLM-Provider (z.B. OpenAI)
	// result := loop.Run(ctx, tc.Prompt)
	// return result.Summary, result.Turns, toolsExtractedFromResult(), nil

	if ctx.Err() != nil {
		return "", 0, nil, fmt.Errorf("context cancelled or timed out")
	}

	// Demo-Output für lokale Tests
	output = fmt.Sprintf("Agent executed prompt: %s", tc.Prompt[:minInt(50, len(tc.Prompt))])
	turns = 1
	tools = []string{"analyze", "generate"}

	return output, turns, tools, nil
}

// validateConstraints prüft, ob die Testfall-Constraints erfüllt sind
func (r *Runner) validateConstraints(tc *TestCase, turns int, toolsCalled []string) bool {
	c := tc.Constraints

	// Check: MustUseTools
	if len(c.MustUseTools) > 0 {
		for _, mustTool := range c.MustUseTools {
			found := false
			for _, called := range toolsCalled {
				if called == mustTool {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Check: ForbiddenTools
	if len(c.ForbiddenTools) > 0 {
		for _, forbidden := range c.ForbiddenTools {
			for _, called := range toolsCalled {
				if called == forbidden {
					return false
				}
			}
		}
	}

	// Check: MaxTurns
	if c.MaxTurns > 0 && turns > c.MaxTurns {
		return false
	}

	return true
}

// executeVerifyCommand führt den Verify-Command aus
func (r *Runner) executeVerifyCommand(ctx context.Context, cmd string) bool {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	command := exec.CommandContext(cmdCtx, "sh", "-c", cmd)
	err := command.Run()
	return err == nil
}

// SaveResults speichert Ergebnisse als JSON
func (r *Runner) SaveResults(path string) error {
	data, err := json.MarshalIndent(r.results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Results gibt die gesammelten Ergebnisse zurück
func (r *Runner) Results() []RunResult {
	return r.results
}

// Helper: truncateString kürzt einen String
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Helper: minInt gibt das Minimum zweier Integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
