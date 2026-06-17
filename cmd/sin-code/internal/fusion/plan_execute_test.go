// SPDX-License-Identifier: MIT
// Purpose: Race-safety and correctness tests for the plan+execute
// tournament (issue #321, M7). All tests must pass under
// `go test -race -count=1`.
package fusion

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProviderPool_Get_All(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	})
	got := pool.Get(nil)
	if len(got) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(got))
	}
}

func TestProviderPool_Get_Filtered(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	})
	got := pool.Get([]string{"a", "c"})
	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(got))
	}
	if got[0].Name != "a" || got[1].Name != "c" {
		t.Errorf("expected a,c; got %s,%s", got[0].Name, got[1].Name)
	}
}

func TestSimpleArbiter_PickPlan_LongestWins(t *testing.T) {
	a := &SimpleArbiter{}
	plans := []PlanCandidate{
		{Model: "short", Plan: "do stuff"},
		{Model: "detailed", Plan: "step 1: read\nstep 2: edit\nstep 3: test"},
		{Model: "medium", Plan: "read, edit, test"},
	}
	best, err := a.PickPlan(plans)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.Model != "detailed" {
		t.Errorf("expected 'detailed' (longest), got %q", best.Model)
	}
	if best.Plan == "" {
		t.Error("expected non-empty plan")
	}
}

func TestSimpleArbiter_PickPlan_Empty(t *testing.T) {
	a := &SimpleArbiter{}
	_, err := a.PickPlan(nil)
	if err == nil {
		t.Fatal("expected error for no candidates")
	}
}

func TestSimpleArbiter_PickResult_VerifiedWins(t *testing.T) {
	a := &SimpleArbiter{}
	results := []ResultCandidate{
		{Model: "unverified-long", Output: strings.Repeat("x", 1000), Verified: false},
		{Model: "verified-short", Output: "correct", Verified: true},
	}
	best, err := a.PickResult(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !best.Verified || best.Model != "verified-short" {
		t.Errorf("expected verified 'verified-short', got verified=%v model=%q", best.Verified, best.Model)
	}
}

func TestSimpleArbiter_PickResult_NoVerified_LongestWins(t *testing.T) {
	a := &SimpleArbiter{}
	results := []ResultCandidate{
		{Model: "short", Output: "x", Verified: false},
		{Model: "long", Output: strings.Repeat("y", 100), Verified: false},
	}
	best, err := a.PickResult(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.Model != "long" {
		t.Errorf("expected 'long', got %q", best.Model)
	}
}

func TestPlanExecuteTournament_Plan_ParallelBest(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{
		{Name: "weak"}, {Name: "strong"}, {Name: "medium"},
	})
	tournament := NewPlanExecuteTournament(pool)
	tournament.PerProviderTimeout = 5 * time.Second
	tournament.PlanFunc = func(ctx context.Context, prov ProviderConfig, prompt string) (string, error) {
		switch prov.Name {
		case "weak":
			return "plan: do it", nil
		case "strong":
			return "plan: step 1\nstep 2\nstep 3\nstep 4\nstep 5", nil
		case "medium":
			return "plan: step 1\nstep 2", nil
		}
		return "", errors.New("unknown")
	}

	best, err := tournament.Plan(context.Background(), "build a feature", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.Model != "strong" {
		t.Errorf("expected 'strong' (longest plan), got %q", best.Model)
	}
}

func TestPlanExecuteTournament_Execute_ParallelBest(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	})
	tournament := NewPlanExecuteTournament(pool)
	tournament.PerProviderTimeout = 5 * time.Second
	plan := &BestPlan{Plan: "do the thing", Model: "strong"}
	tournament.ExecuteFunc = func(ctx context.Context, prov ProviderConfig, p *BestPlan) (string, error) {
		switch prov.Name {
		case "a":
			return "partial output", nil
		case "b":
			return "full correct output with all tests passing", nil
		case "c":
			return "wrong", nil
		}
		return "", errors.New("unknown")
	}
	tournament.VerifyFunc = func(ctx context.Context, output string) bool {
		return strings.Contains(output, "correct")
	}

	best, err := tournament.Execute(context.Background(), plan, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !best.Verified {
		t.Error("expected verified result")
	}
	if best.Model != "b" {
		t.Errorf("expected 'b' (verified), got %q", best.Model)
	}
}

func TestPlanExecuteTournament_Plan_AllFail(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{
		{Name: "a"}, {Name: "b"},
	})
	tournament := NewPlanExecuteTournament(pool)
	tournament.PlanFunc = func(ctx context.Context, prov ProviderConfig, prompt string) (string, error) {
		return "", errors.New("boom")
	}

	_, err := tournament.Plan(context.Background(), "task", nil)
	if err == nil {
		t.Fatal("expected error when all plan providers fail")
	}
}

func TestPlanExecuteTournament_Execute_AllFail(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{
		{Name: "a"}, {Name: "b"},
	})
	tournament := NewPlanExecuteTournament(pool)
	tournament.ExecuteFunc = func(ctx context.Context, prov ProviderConfig, p *BestPlan) (string, error) {
		return "", errors.New("boom")
	}

	_, err := tournament.Execute(context.Background(), &BestPlan{Plan: "x"}, nil)
	if err == nil {
		t.Fatal("expected error when all execute providers fail")
	}
}

func TestPlanExecuteTournament_NilPlan_Error(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{{Name: "a"}})
	tournament := NewPlanExecuteTournament(pool)
	tournament.ExecuteFunc = func(ctx context.Context, prov ProviderConfig, p *BestPlan) (string, error) {
		return "x", nil
	}
	_, err := tournament.Execute(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
}

func TestPlanExecuteTournament_NoProviders_Error(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{{Name: "a"}})
	tournament := NewPlanExecuteTournament(pool)
	tournament.PlanFunc = func(ctx context.Context, prov ProviderConfig, prompt string) (string, error) {
		return "x", nil
	}
	_, err := tournament.Plan(context.Background(), "task", []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for no matching providers")
	}
}

func TestPlanExecuteTournament_RaceSafe(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	})
	tournament := NewPlanExecuteTournament(pool)
	tournament.PerProviderTimeout = 5 * time.Second
	var callCount atomic.Int32
	tournament.PlanFunc = func(ctx context.Context, prov ProviderConfig, prompt string) (string, error) {
		callCount.Add(1)
		time.Sleep(time.Duration(10) * time.Millisecond)
		return "plan from " + prov.Name, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = tournament.Plan(context.Background(), "task", nil)
		}()
	}
	wg.Wait()
	if callCount.Load() < 10 {
		t.Errorf("expected at least 10 plan calls, got %d", callCount.Load())
	}
}

func TestPlanExecuteTournament_PerProviderTimeout(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{
		{Name: "slow"}, {Name: "fast"},
	})
	tournament := NewPlanExecuteTournament(pool)
	tournament.PerProviderTimeout = 50 * time.Millisecond
	tournament.PlanFunc = func(ctx context.Context, prov ProviderConfig, prompt string) (string, error) {
		if prov.Name == "slow" {
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		return "plan from " + prov.Name, nil
	}

	best, err := tournament.Plan(context.Background(), "task", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.Model != "fast" {
		t.Errorf("expected 'fast' (slow timed out), got %q", best.Model)
	}
}
