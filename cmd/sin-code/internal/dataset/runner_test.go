// SPDX-License-Identifier: MIT
// Purpose: Tests for Dataset Runner
package dataset

import (
	"context"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
)

func TestRunnerInit(t *testing.T) {
	cfg := RunnerConfig{
		Headless:       true,
		TimeoutPerCase: 30 * time.Second,
		RetryOnFailure: true,
		MaxRetries:     2,
	}

	runner := NewRunner(cfg)
	if runner == nil {
		t.Fatal("Runner is nil")
	}
	if len(runner.Results()) != 0 {
		t.Error("Expected empty results initially")
	}
}

func TestRunDataset(t *testing.T) {
	ds := &Dataset{
		Name: "test-suite",
		TestCases: []TestCase{
			{
				ID:       "tc-1",
				Category: "basic",
				Prompt:   "Write hello world",
				Expected: Expected{
					MustContain: []string{"hello"},
				},
				Constraints: Constraints{
					MaxTurns:       3,
					TimeoutSeconds: 10,
				},
			},
		},
	}

	cfg := RunnerConfig{
		Headless:       true,
		TimeoutPerCase: 30 * time.Second,
	}

	runner := NewRunner(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := runner.Run(ctx, ds)
	if err != nil {
		t.Logf("Run completed with: %v (expected for mock)", err)
	}

	results := runner.Results()
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestConstraintValidationInRunner(t *testing.T) {
	ds := &Dataset{
		Name: "constraint-test",
		TestCases: []TestCase{
			{
				ID:       "ct-1",
				Category: "constraints",
				Prompt:   "test",
				Constraints: Constraints{
					MustUseTools: []string{"code_gen"},
					MaxTurns:     2,
				},
				Expected: Expected{
					MustContain: []string{"test"},
				},
			},
		},
	}

	cfg := RunnerConfig{
		TimeoutPerCase: 15 * time.Second,
	}

	runner := NewRunner(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := runner.Run(ctx, ds)
	if err != nil {
		t.Logf("Run returned: %v (OK)", err)
	}
}

func TestTimeoutHandling(t *testing.T) {
	ds := &Dataset{
		Name: "timeout-test",
		TestCases: []TestCase{
			{
				ID:       "to-1",
				Category: "timeout",
				Prompt:   "this might take too long",
				Constraints: Constraints{
					TimeoutSeconds: 1, // Very short timeout
				},
				Expected: Expected{
					MustContain: []string{"ok"},
				},
			},
		},
	}

	cfg := RunnerConfig{
		TimeoutPerCase: 2 * time.Second,
	}

	runner := NewRunner(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := runner.Run(ctx, ds)
	// Should complete (not panic) even if timeout occurs
	if err != nil {
		t.Logf("Timeout handling OK: %v", err)
	}
}

func TestRetryOnFailure(t *testing.T) {
	ds := &Dataset{
		Name: "retry-test",
		TestCases: []TestCase{
			{
				ID:       "retry-1",
				Category: "retry",
				Prompt:   "test prompt",
				Expected: Expected{
					MustContain: []string{"ok"},
				},
			},
		},
	}

	cfg := RunnerConfig{
		RetryOnFailure: true,
		MaxRetries:     3,
		TimeoutPerCase: 10 * time.Second,
	}

	runner := NewRunner(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := runner.Run(ctx, ds)
	if err != nil {
		t.Logf("Retry test completed with: %v", err)
	}

	results := runner.Results()
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestResultsStorage(t *testing.T) {
	cfg := RunnerConfig{
		TimeoutPerCase: 10 * time.Second,
	}

	runner := NewRunner(cfg)

	// Simulate storing multiple results
	for i := 0; i < 5; i++ {
		result := &RunResult{
			TestCaseID: "test-" + string(rune(i+'0')),
			Passed:     i%2 == 0,
		}
		runner.results = append(runner.results, result)
	}

	results := runner.Results()
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	if passed != 3 {
		t.Errorf("Expected 3 passed, got %d", passed)
	}
}

func TestJudgeIntegration(t *testing.T) {
	ds := &Dataset{
		Name: "judge-test",
		TestCases: []TestCase{
			{
				ID:       "judge-1",
				Category: "judge",
				Prompt:   "test",
				Expected: Expected{
					MustContain: []string{"test"},
				},
			},
		},
	}

	cfg := RunnerConfig{
		TimeoutPerCase: 10 * time.Second,
	}

	judge := eval.NewJudge("mock") // Mock judge
	runner := NewRunner(cfg)
	runner.judge = judge

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := runner.Run(ctx, ds)
	if err != nil {
		t.Logf("Judge integration test: %v", err)
	}

	results := runner.Results()
	if len(results) == 0 {
		t.Error("Expected results from judge integration")
	}
}

func TestMultipleTestCases(t *testing.T) {
	ds := &Dataset{
		Name: "multi-test",
		TestCases: []TestCase{
			{
				ID:       "mt-1",
				Category: "cat1",
				Prompt:   "prompt1",
				Expected: Expected{MustContain: []string{"test1"}},
			},
			{
				ID:       "mt-2",
				Category: "cat2",
				Prompt:   "prompt2",
				Expected: Expected{MustContain: []string{"test2"}},
			},
			{
				ID:       "mt-3",
				Category: "cat3",
				Prompt:   "prompt3",
				Expected: Expected{MustContain: []string{"test3"}},
			},
		},
	}

	cfg := RunnerConfig{
		TimeoutPerCase: 10 * time.Second,
	}

	runner := NewRunner(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := runner.Run(ctx, ds)
	if err != nil {
		t.Logf("Multi test run: %v", err)
	}

	results := runner.Results()
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
}

func BenchmarkRunnerExecution(b *testing.B) {
	ds := &Dataset{
		Name: "bench-test",
		TestCases: []TestCase{
			{
				ID:       "bench-1",
				Category: "perf",
				Prompt:   "test",
				Expected: Expected{MustContain: []string{"ok"}},
			},
		},
	}

	cfg := RunnerConfig{
		TimeoutPerCase: 10 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runner := NewRunner(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = runner.Run(ctx, ds)
		cancel()
	}
}
