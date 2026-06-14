// SPDX-License-Identifier: MIT
// Purpose: Dataset Runner for executing evaluation test cases
package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// RunResult speichert das Ergebnis eines Test-Laufs
type RunResult struct {
	TestCaseID    string        `json:"test_case_id"`
	SessionID     string        `json:"session_id"`
	Success       bool          `json:"success"`
	Turns         int           `json:"turns"`
	Duration      time.Duration `json:"duration"`
	ToolsUsed     []string      `json:"tools_used"`
	VerifyPassed  bool          `json:"verify_passed"`
	Error         string        `json:"error,omitempty"`
	JudgeScore    float64       `json:"judge_score,omitempty"`
	JudgeFeedback string        `json:"judge_feedback,omitempty"`
}

// RunnerConfig konfiguriert den Dataset-Runner
type RunnerConfig struct {
	ProfileName    string
	VerifyCommand  string
	HeadlessMode   bool
	TimeoutPerCase time.Duration
}

// Runner führt TestCases aus einem Dataset aus
type Runner struct {
	config  RunnerConfig
	results []RunResult
}

// NewRunner erstellt einen neuen Dataset-Runner
func NewRunner(config RunnerConfig) *Runner {
	if config.TimeoutPerCase == 0 {
		config.TimeoutPerCase = 5 * time.Minute
	}
	return &Runner{
		config:  config,
		results: []RunResult{},
	}
}

// Run führt alle TestCases aus einem Dataset aus
func (r *Runner) Run(ctx context.Context, ds *Dataset) ([]RunResult, error) {
	fmt.Printf("Starting evaluation of dataset: %s (v%s)\n", ds.Name, ds.Version)
	fmt.Printf("Total test cases: %d\n\n", len(ds.TestCases))

	for i, tc := range ds.TestCases {
		fmt.Printf("[%d/%d] Running test case: %s\n", i+1, len(ds.TestCases), tc.ID)

		result := RunResult{
			TestCaseID: tc.ID,
			ToolsUsed:  []string{},
		}

		// Timeout pro TestCase
		testCtx, cancel := context.WithTimeout(ctx, r.config.TimeoutPerCase)

		// Simulierte Ausführung
		// In der echten Implementierung würde hier der Agent Loop aufgerufen
		startTime := time.Now()
		err := r.executeTestCase(testCtx, &tc)
		duration := time.Since(startTime)

		cancel()

		if err != nil {
			result.Success = false
			result.Error = err.Error()
			fmt.Printf("  FAILED: %v\n", err)
		} else {
			result.Success = true
			fmt.Printf("  PASSED\n")
		}

		result.Duration = duration
		result.Turns = 1 // Placeholder

		r.results = append(r.results, result)
	}

	fmt.Printf("\nEvaluation complete. %d/%d tests passed.\n", 
		r.passCount(), len(ds.TestCases))

	return r.results, nil
}

// executeTestCase führt einen einzelnen TestCase aus
func (r *Runner) executeTestCase(ctx context.Context, tc *TestCase) error {
	// Constraint-Validierung
	if tc.Constraints.TimeoutSeconds > 0 {
		// Hier würde die Timeout-Logik implementiert
	}

	// Forbidden Tools Check
	if len(tc.Constraints.ForbiddenTools) > 0 {
		// Hier würde die Tool-Restriction überprüft
	}

	// In echter Implementierung:
	// 1. Agent Loop mit TestCase.Prompt starten
	// 2. Constraint-Validierung durchführen
	// 3. Verify Command ausführen wenn nötig
	// 4. LLM-as-a-Judge aufrufen für Expected-Validierung

	return nil
}

// passCount gibt die Anzahl bestandener Tests zurück
func (r *Runner) passCount() int {
	count := 0
	for _, result := range r.results {
		if result.Success {
			count++
		}
	}
	return count
}

// SaveResults speichert die Test-Ergebnisse als JSON
func (r *Runner) SaveResults(path string) error {
	data, err := json.MarshalIndent(r.results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write results file: %w", err)
	}

	return nil
}

// Results gibt die gesammelten Ergebnisse zurück
func (r *Runner) Results() []RunResult {
	return r.results
}
