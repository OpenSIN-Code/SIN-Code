// SPDX-License-Identifier: MIT
// Purpose: Tests for Oracle-mode tournament (issue #344). All tests must
// pass under `go test -race -count=1` (mandate M7).
package fusion

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// makeJudgeFn returns a JudgeFunc that scores attempts according to the
// provided mapping (model → score). All scores include reasoning.
func makeJudgeFn(scores map[string]float64) JudgeFunc {
	return func(ctx context.Context, attempts []Attempt) ([]ModelScore, string, error) {
		out := make([]ModelScore, len(attempts))
		for i, a := range attempts {
			out[i] = ModelScore{
				Model:     a.Model,
				Score:     scores[a.Model],
				Reasoning: fmt.Sprintf("judge: %s scored %.2f", a.Model, scores[a.Model]),
			}
		}
		return out, "overall: evaluated all candidates", nil
	}
}

func makeOraclePool() *ProviderPool {
	return NewProviderPool([]ProviderConfig{
		{Name: "model-a"},
		{Name: "model-b"},
		{Name: "model-c"},
	})
}

// TestOracleTournament_JudgeSelectsBestOutput verifies the judge picks
// the highest-scoring output as the winner.
func TestOracleTournament_JudgeSelectsBestOutput(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "judge-model")
	ot.JudgeFn = makeJudgeFn(map[string]float64{
		"model-a": 0.5,
		"model-b": 0.9,
		"model-c": 0.3,
	})
	attempts := []Attempt{
		{Model: "model-a", Output: "output-a", Verified: true},
		{Model: "model-b", Output: "output-b", Verified: true},
		{Model: "model-c", Output: "output-c", Verified: true},
	}
	result, err := ot.Run(context.Background(), &Task{Prompt: "test"}, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Winner != "model-b" {
		t.Fatalf("expected winner model-b, got %s", result.Winner)
	}
	if result.Score != 0.9 {
		t.Fatalf("expected score 0.9, got %f", result.Score)
	}
	if result.FallbackUsed {
		t.Fatal("expected no fallback")
	}
}

// TestOracleTournament_JudgeExcludesSelf verifies the judge model's own
// output is excluded from evaluation.
func TestOracleTournament_JudgeExcludesSelf(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "model-a")
	ot.JudgeFn = makeJudgeFn(map[string]float64{
		"model-a": 1.0,
		"model-b": 0.5,
		"model-c": 0.3,
	})
	attempts := []Attempt{
		{Model: "model-a", Output: "self-output", Verified: true},
		{Model: "model-b", Output: "output-b", Verified: true},
		{Model: "model-c", Output: "output-c", Verified: true},
	}
	result, err := ot.Run(context.Background(), &Task{Prompt: "test"}, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Winner == "model-a" {
		t.Fatal("judge model must not win (self-exclusion)")
	}
	if len(result.AllScores) != 2 {
		t.Fatalf("expected 2 scores (self excluded), got %d", len(result.AllScores))
	}
}

// TestOracleTournament_JudgeRequiresReasoning verifies missing reasoning
// triggers fallback.
func TestOracleTournament_JudgeRequiresReasoning(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "judge")
	ot.JudgeFn = func(ctx context.Context, attempts []Attempt) ([]ModelScore, string, error) {
		return []ModelScore{
			{Model: "model-a", Score: 0.8, Reasoning: ""},
			{Model: "model-b", Score: 0.5, Reasoning: "has reasoning"},
		}, "test", nil
	}
	attempts := []Attempt{
		{Model: "model-a", Output: "aaa", Verified: true},
		{Model: "model-b", Output: "bb", Verified: true},
	}
	result, err := ot.Run(context.Background(), &Task{}, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.FallbackUsed {
		t.Fatal("expected fallback when reasoning missing")
	}
}

// TestOracleTournament_ScoreRangeValidation verifies out-of-range scores
// trigger fallback.
func TestOracleTournament_ScoreRangeValidation(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "judge")
	ot.JudgeFn = func(ctx context.Context, attempts []Attempt) ([]ModelScore, string, error) {
		return []ModelScore{
			{Model: "model-a", Score: 1.5, Reasoning: "over range"},
			{Model: "model-b", Score: 0.5, Reasoning: "ok"},
		}, "test", nil
	}
	attempts := []Attempt{
		{Model: "model-a", Output: "aaa", Verified: true},
		{Model: "model-b", Output: "bb", Verified: true},
	}
	result, err := ot.Run(context.Background(), &Task{}, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.FallbackUsed {
		t.Fatal("expected fallback for out-of-range score")
	}
}

// TestOracleTournament_AllZeroScoresRejected verifies all-zero scores
// trigger fallback.
func TestOracleTournament_AllZeroScoresRejected(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "judge")
	ot.JudgeFn = makeJudgeFn(map[string]float64{
		"model-a": 0.0,
		"model-b": 0.0,
	})
	attempts := []Attempt{
		{Model: "model-a", Output: "aaa", Verified: true},
		{Model: "model-b", Output: "bb", Verified: true},
	}
	result, err := ot.Run(context.Background(), &Task{}, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.FallbackUsed {
		t.Fatal("expected fallback for all-zero scores")
	}
}

// TestOracleTournament_AllOneScoresRejected verifies all-one scores
// trigger fallback (judge rubber-stamping).
func TestOracleTournament_AllOneScoresRejected(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "judge")
	ot.JudgeFn = makeJudgeFn(map[string]float64{
		"model-a": 1.0,
		"model-b": 1.0,
	})
	attempts := []Attempt{
		{Model: "model-a", Output: "aaa", Verified: true},
		{Model: "model-b", Output: "bb", Verified: true},
	}
	result, err := ot.Run(context.Background(), &Task{}, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.FallbackUsed {
		t.Fatal("expected fallback for all-one scores")
	}
}

// TestOracleTournament_FallbackOnJudgeFailure verifies judge error
// triggers deterministic fallback.
func TestOracleTournament_FallbackOnJudgeFailure(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "judge")
	ot.JudgeFn = func(ctx context.Context, attempts []Attempt) ([]ModelScore, string, error) {
		return nil, "", errors.New("judge API unavailable")
	}
	attempts := []Attempt{
		{Model: "model-a", Output: "short", Verified: true},
		{Model: "model-b", Output: "much longer output", Verified: true},
	}
	result, err := ot.Run(context.Background(), &Task{}, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.FallbackUsed {
		t.Fatal("expected fallback on judge error")
	}
	if result.Winner != "model-b" {
		t.Fatalf("expected model-b (longest verified), got %s", result.Winner)
	}
}

// TestOracleTournament_DeterministicTieBreak verifies equal scores are
// tie-broken by model name (alphabetical).
func TestOracleTournament_DeterministicTieBreak(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "judge")
	ot.JudgeFn = makeJudgeFn(map[string]float64{
		"model-b": 0.7,
		"model-a": 0.7,
	})
	attempts := []Attempt{
		{Model: "model-b", Output: "b", Verified: true},
		{Model: "model-a", Output: "a", Verified: true},
	}
	result, err := ot.Run(context.Background(), &Task{}, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Winner != "model-a" {
		t.Fatalf("expected model-a (alphabetical tie-break), got %s", result.Winner)
	}
}

// TestOracleTournament_ConcurrentSafety verifies concurrent Run calls
// do not corrupt state (mandate M7).
func TestOracleTournament_ConcurrentSafety(t *testing.T) {
	pool := makeOraclePool()
	ot := NewOracleTournament(pool, "judge")
	ot.JudgeFn = makeJudgeFn(map[string]float64{
		"model-a": 0.5,
		"model-b": 0.8,
		"model-c": 0.3,
	})
	attempts := []Attempt{
		{Model: "model-a", Output: "a", Verified: true},
		{Model: "model-b", Output: "b", Verified: true},
		{Model: "model-c", Output: "c", Verified: true},
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := ot.Run(context.Background(), &Task{Prompt: "concurrent"}, attempts)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result.Winner != "model-b" {
				t.Errorf("expected model-b, got %s", result.Winner)
			}
		}()
	}
	wg.Wait()
}

// TestValidateJudge_ValidScores verifies valid scores pass validation.
func TestValidateJudge_ValidScores(t *testing.T) {
	ot := NewOracleTournament(makeOraclePool(), "judge")
	scores := []ModelScore{
		{Model: "a", Score: 0.8, Reasoning: "good"},
		{Model: "b", Score: 0.5, Reasoning: "ok"},
		{Model: "c", Score: 0.2, Reasoning: "weak"},
	}
	if err := ot.ValidateJudge(scores); err != nil {
		t.Fatalf("expected valid, got error: %v", err)
	}
}

// TestOracleTournament_NoEligibleAttempts verifies all-judge-model
// attempts return ErrNoEligibleAttempts.
func TestOracleTournament_NoEligibleAttempts(t *testing.T) {
	ot := NewOracleTournament(makeOraclePool(), "model-a")
	ot.JudgeFn = makeJudgeFn(map[string]float64{"model-a": 0.5})
	attempts := []Attempt{
		{Model: "model-a", Output: "only self", Verified: true},
	}
	_, err := ot.Run(context.Background(), &Task{}, attempts)
	if !errors.Is(err, ErrNoEligibleAttempts) {
		t.Fatalf("expected ErrNoEligibleAttempts, got: %v", err)
	}
}
