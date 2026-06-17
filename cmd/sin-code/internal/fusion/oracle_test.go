// SPDX-License-Identifier: MIT
// Purpose: Oracle-mode tournament tests (issue #344). All tests pass under
// `go test -race -count=1` (mandate M7).
package fusion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// fakeOracleJudge returns a verdict that ranks candidates by the number of
// times the marker string appears in their output. This simulates a judge
// that can distinguish correct from buggy outputs.
func fakeOracleJudge(marker string) OracleJudgeFn {
	return func(ctx context.Context, prompt string, candidates []Candidate) (OracleVerdict, error) {
		scores := make(map[string]OracleScore, len(candidates))
		best := ""
		bestScore := -1
		for _, c := range candidates {
			count := 0
			for i := 0; i+len(marker) <= len(c.Output); i++ {
				if c.Output[i:i+len(marker)] == marker {
					count++
				}
			}
			score := OracleScore{
				Correctness:  count,
				Completeness: count,
				Risk:         0,
				Total:        count * 2,
				Reasoning:    fmt.Sprintf("marker count = %d", count),
			}
			scores[c.Provider] = score
			if score.Total > bestScore {
				bestScore = score.Total
				best = c.Provider
			}
		}
		return OracleVerdict{WinnerProvider: best, Scores: scores, Reasoning: "marker-based ranking"}, nil
	}
}

func makeOracleRunFunc(providers map[string]*fakeProvider) RunFunc {
	return func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		fp := providers[prov.Name]
		if fp == nil {
			return nil, errors.New("unknown provider: " + prov.Name)
		}
		fp.calls.Add(1)
		select {
		case <-time.After(fp.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &agentloop.Result{
			SessionID: sess.ID,
			Summary:   fp.output,
			Verified:  false,
			Turns:     1,
			Tokens:    fp.tokens,
		}, nil
	}
}

func TestOracleTournament_JudgePicksCorrect(t *testing.T) {
	providers := map[string]*fakeProvider{
		"correct": {name: "correct", delay: 10 * time.Millisecond, output: "CORRECT answer", tokens: 100},
		"buggy":   {name: "buggy", delay: 10 * time.Millisecond, output: "wrong answer", tokens: 100},
	}

	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "correct"}, {Name: "buggy"}},
		RunFunc:   makeOracleRunFunc(providers),
		ForkFunc:  makeForkFunc(),
		Mode:      ModeOracle,
		OracleJudge: fakeOracleJudge("CORRECT"),
		MinQuorum: 2,
		Workspace: "/test/ws",
		Prompt:    "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected winner, got error: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected winner, got nil")
	}
	if result.Winner.Provider != "correct" {
		t.Errorf("expected winner 'correct', got %q", result.Winner.Provider)
	}
	if result.AllFailed {
		t.Error("expected AllFailed=false")
	}
	if len(result.Losers) != 1 {
		t.Errorf("expected 1 loser, got %d", len(result.Losers))
	}
	if result.Losers[0].VerifyResult.Report != "oracle judge loser" {
		t.Errorf("expected loser report 'oracle judge loser', got %q", result.Losers[0].VerifyResult.Report)
	}
}

func TestOracleTournament_AllRunToCompletion(t *testing.T) {
	var runCount atomic.Int32
	providers := map[string]*fakeProvider{
		"slow":  {name: "slow", delay: 200 * time.Millisecond, output: "CORRECT", tokens: 100},
		"fast":  {name: "fast", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
	}

	runFunc := func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		fp := providers[prov.Name]
		if fp == nil {
			return nil, errors.New("unknown provider: " + prov.Name)
		}
		fp.calls.Add(1)
		runCount.Add(1)
		select {
		case <-time.After(fp.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &agentloop.Result{SessionID: sess.ID, Summary: fp.output, Tokens: fp.tokens}, nil
	}

	tournament := &Tournament{
		Providers:   []ProviderConfig{{Name: "slow"}, {Name: "fast"}},
		RunFunc:     runFunc,
		ForkFunc:    makeForkFunc(),
		Mode:        ModeOracle,
		OracleJudge: fakeOracleJudge("CORRECT"),
		MinQuorum:   2,
		Workspace:   "/test/ws",
		Prompt:      "do the thing",
		SourceSessionID: "src-1",
	}

	_, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected winner, got error: %v", err)
	}
	if runCount.Load() != 2 {
		t.Errorf("expected both providers to run, got %d", runCount.Load())
	}
}

func TestOracleTournament_CostCeiling(t *testing.T) {
	providers := map[string]*fakeProvider{
		"expensive": {name: "expensive", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 10_000_000},
	}

	tournament := &Tournament{
		Providers:   []ProviderConfig{{Name: "expensive", InputPer1M: 5.0, OutputPer1M: 5.0}},
		RunFunc:     makeOracleRunFunc(providers),
		ForkFunc:    makeForkFunc(),
		Mode:        ModeOracle,
		OracleJudge: fakeOracleJudge("CORRECT"),
		MinQuorum:   1,
		MaxCostUSD:  0.01,
		Workspace:   "/test/ws",
		Prompt:      "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if !errors.Is(err, ErrCostCeilingExceeded) {
		t.Fatalf("expected ErrCostCeilingExceeded, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result on cost exceed")
	}
	if result.TotalCostUSD <= 0 {
		t.Error("expected positive cost on cost exceed")
	}
}

func TestOracleTournament_JudgeError_AllFailed(t *testing.T) {
	providers := map[string]*fakeProvider{
		"a": {name: "a", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
	}

	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "a"}},
		RunFunc:   makeOracleRunFunc(providers),
		ForkFunc:  makeForkFunc(),
		Mode:      ModeOracle,
		OracleJudge: func(ctx context.Context, prompt string, candidates []Candidate) (OracleVerdict, error) {
			return OracleVerdict{}, errors.New("judge unavailable")
		},
		MinQuorum: 1,
		Workspace: "/test/ws",
		Prompt:    "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when judge fails")
	}
	if !errors.Is(err, ErrAllProvidersFailed) && !strings.Contains(err.Error(), "judge failed") {
		t.Fatalf("expected judge/all-failed error, got: %v", err)
	}
	if result == nil || !result.AllFailed {
		t.Error("expected AllFailed=true")
	}
}

func TestOracleTournament_RaceSafety(t *testing.T) {
	for i := 0; i < 20; i++ {
		providers := map[string]*fakeProvider{
			"a": {name: "a", delay: time.Duration(i+1) * time.Millisecond, output: "CORRECT", tokens: 100 * (i + 1)},
			"b": {name: "b", delay: time.Duration(20-i) * time.Millisecond, output: "CORRECT", tokens: 200},
		}

		tournament := &Tournament{
			Providers:   []ProviderConfig{{Name: "a"}, {Name: "b"}},
			RunFunc:     makeOracleRunFunc(providers),
			ForkFunc:    makeForkFunc(),
			Mode:        ModeOracle,
			OracleJudge: fakeOracleJudge("CORRECT"),
			MinQuorum:   2,
			Workspace:   "/test/ws",
			Prompt:      "do the thing",
			SourceSessionID: "src-1",
		}

		result, err := tournament.Run(context.Background())
		if err != nil {
			t.Fatalf("iter %d: unexpected error: %v", i, err)
		}
		if result == nil || result.Winner == nil {
			t.Fatalf("iter %d: expected winner", i)
		}
	}
}

func TestOracleTournament_PositionBias(t *testing.T) {
	// Same output must receive the same score regardless of candidate slot.
	var mu sync.Mutex
	providerOrder := []string{}

	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "first"}, {Name: "second"}, {Name: "third"}},
		RunFunc: func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
			mu.Lock()
			providerOrder = append(providerOrder, prov.Name)
			mu.Unlock()
			return &agentloop.Result{SessionID: sess.ID, Summary: "SAME OUTPUT", Tokens: 100}, nil
		},
		ForkFunc: makeForkFunc(),
		Mode:     ModeOracle,
		OracleJudge: func(ctx context.Context, prompt string, candidates []Candidate) (OracleVerdict, error) {
			scores := make(map[string]OracleScore, len(candidates))
			for _, c := range candidates {
				scores[c.Provider] = OracleScore{Correctness: 7, Completeness: 7, Risk: 2, Total: 12, Reasoning: "same"}
			}
			// Deterministic tie-break picks first by name.
			return OracleVerdict{WinnerProvider: "first", Scores: scores, Reasoning: "tie by score"}, nil
		},
		MinQuorum: 3,
		Workspace: "/test/ws",
		Prompt:    "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected winner, got error: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected winner")
	}
	if len(providerOrder) != 3 {
		t.Errorf("expected 3 providers to run, got %d", len(providerOrder))
	}
}

func TestOracleTournament_NilOracleJudge(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "a"}},
		RunFunc:   makeOracleRunFunc(map[string]*fakeProvider{}),
		ForkFunc:  makeForkFunc(),
		Mode:      ModeOracle,
		MinQuorum: 1,
	}

	_, err := tournament.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil OracleJudge")
	}
}

func TestParseOracleVerdict_JSON(t *testing.T) {
	candidates := []Candidate{
		{Provider: "a", Output: "good"},
		{Provider: "b", Output: "bad"},
	}
	raw := `{
		"scores": {
			"candidate-0": {"correctness_score": 9, "completeness_score": 8, "risk_score": 2, "reasoning": "r0"},
			"candidate-1": {"correctness_score": 4, "completeness_score": 4, "risk_score": 6, "reasoning": "r1"}
		},
		"winner_candidate_id": "candidate-0",
		"reasoning": "a is better"
	}`

	verdict, err := parseOracleVerdict(raw, candidates, 10)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if verdict.WinnerProvider != "a" {
		t.Errorf("expected winner a, got %q", verdict.WinnerProvider)
	}
	if verdict.Scores["a"].Total != 9+8+(10-2) {
		t.Errorf("expected total score 25, got %d", verdict.Scores["a"].Total)
	}
	if verdict.Scores["b"].Total != 4+4+(10-6) {
		t.Errorf("expected total score 12, got %d", verdict.Scores["b"].Total)
	}
}

func TestParseOracleVerdict_MarkdownBlock(t *testing.T) {
	candidates := []Candidate{{Provider: "a", Output: "good"}}
	raw := "```json\n" + `{"scores": {"candidate-0": {"correctness_score": 10, "completeness_score": 10, "risk_score": 0, "reasoning": "r"}}, "winner_candidate_id": "candidate-0", "reasoning": "best"}` + "\n```"

	verdict, err := parseOracleVerdict(raw, candidates, 10)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if verdict.WinnerProvider != "a" {
		t.Errorf("expected winner a, got %q", verdict.WinnerProvider)
	}
}

func TestParseOracleVerdict_ClampScores(t *testing.T) {
	candidates := []Candidate{{Provider: "a", Output: "good"}}
	raw := `{"scores": {"candidate-0": {"correctness_score": 99, "completeness_score": -5, "risk_score": 200, "reasoning": "r"}}, "winner_candidate_id": "candidate-0", "reasoning": "best"}`

	verdict, err := parseOracleVerdict(raw, candidates, 10)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if verdict.Scores["a"].Correctness != 10 {
		t.Errorf("expected clamped correctness 10, got %d", verdict.Scores["a"].Correctness)
	}
	if verdict.Scores["a"].Completeness != 0 {
		t.Errorf("expected clamped completeness 0, got %d", verdict.Scores["a"].Completeness)
	}
	if verdict.Scores["a"].Risk != 10 {
		t.Errorf("expected clamped risk 10, got %d", verdict.Scores["a"].Risk)
	}
}

func TestLLMOracleJudge_NotConfigured(t *testing.T) {
	j := NewLLMOracleJudge(nil, "")
	_, err := j.Judge(context.Background(), "prompt", []Candidate{{Provider: "a", Output: "x"}})
	if err == nil {
		t.Fatal("expected error for unconfigured judge")
	}
}

