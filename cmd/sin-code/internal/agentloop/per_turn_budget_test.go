// SPDX-License-Identifier: MIT
// Purpose: tests for per-turn thinking/token budget enforcement
// (issue #375). The PerTurnBudget struct accumulates per-turn
// reasoning+token charges and exposes a PreFlight hook so the loop
// can refuse to send the next provider call once the cap is breached.
//
// Race-clean: the struct is mutex-guarded and exercised under
// `go test -race` (mandate M7).
package agentloop

import (
	"errors"
	"sync"
	"testing"
)

// 1. Zero-budget budget is unenforced and lets any charge through.
func TestPerTurnBudget_ZeroBudget_IsUnlimited(t *testing.T) {
	p := NewPerTurnBudget(0, 0)
	if p.IsEnforced() {
		t.Fatal("zero budget should not be enforced")
	}
	if err := p.Charge(1_000_000, 1_000_000); err != nil {
		t.Fatalf("unlimited charge should not error: %v", err)
	}
	if r := p.ThinkingRemaining(); r != -1 {
		t.Errorf("Remaining on unlimited: got %d, want -1", r)
	}
	if r := p.TokensRemaining(); r != -1 {
		t.Errorf("TokensRemaining on unlimited: got %d, want -1", r)
	}
}

// 2. Thinking cap trips when charge exceeds it; tokens remain free.
func TestPerTurnBudget_ThinkingCap_Trips(t *testing.T) {
	p := NewPerTurnBudget(200, 0)
	if err := p.Charge(150, 100); err != nil {
		t.Fatalf("charge under cap should not error: %v", err)
	}
	overErr := p.Charge(60, 100)
	if !errors.Is(overErr, ErrPerTurnBudgetExceeded) {
		t.Fatalf("charge over thinking cap: want ErrPerTurnBudgetExceeded, got %v", overErr)
	}
	// Token cap is 0 (unlimited) so the error narrative must NOT
	// mention tokens — guards against a "both tripped" bug.
	if overErr != nil && containsString(overErr.Error(), "tokens ") {
		t.Errorf("error should not mention tokens (cap is unlimited): %v", overErr)
	}
	if p.ThinkingUsed() != 210 {
		t.Errorf("per-turn thinking used: got %d, want 210", p.ThinkingUsed())
	}
}

// 3. Token cap trips independently of thinking cap.
func TestPerTurnBudget_TokenCap_Trips(t *testing.T) {
	p := NewPerTurnBudget(0, 500)
	if err := p.Charge(0, 400); err != nil {
		t.Fatalf("under cap should not error: %v", err)
	}
	if err := p.Charge(0, 200); !errors.Is(err, ErrPerTurnBudgetExceeded) {
		t.Fatalf("charge over token cap: want ErrPerTurnBudgetExceeded, got %v", err)
	}
	if p.TokensUsed() != 600 {
		t.Errorf("per-turn tokens used: got %d, want 600", p.TokensUsed())
	}
}

// 4. Reset on turn boundary zeroes per-turn counters but lifetime
// accumulators survive (TUI bar requirement, issue #375).
func TestPerTurnBudget_Reset_KeepsLifetime(t *testing.T) {
	p := NewPerTurnBudget(500, 1000)
	_ = p.Charge(100, 200)
	_ = p.Charge(150, 300)
	if p.ThinkingUsed() != 250 || p.TokensUsed() != 500 {
		t.Fatalf("per-turn before reset: thinking=%d tokens=%d", p.ThinkingUsed(), p.TokensUsed())
	}
	if p.ThinkingAllTime() != 250 || p.TokensAllTime() != 500 {
		t.Fatalf("all-time before reset: thinking=%d tokens=%d", p.ThinkingAllTime(), p.TokensAllTime())
	}
	p.Reset()
	if p.ThinkingUsed() != 0 || p.TokensUsed() != 0 {
		t.Errorf("after reset, per-turn should be 0: thinking=%d tokens=%d", p.ThinkingUsed(), p.TokensUsed())
	}
	if p.ThinkingAllTime() != 250 || p.TokensAllTime() != 500 {
		t.Errorf("after reset, all-time should survive: thinking=%d tokens=%d",
			p.ThinkingAllTime(), p.TokensAllTime())
	}
	if err := p.PreFlight(); err != nil {
		t.Errorf("after reset, PreFlight should pass: %v", err)
	}
}

// 5. PreFlight blocks the next provider call once a previous turn
// exhausted the budget (cut-off BEFORE sending, issue #375).
func TestPerTurnBudget_PreFlight_BlocksAfterExhaust(t *testing.T) {
	p := NewPerTurnBudget(100, 0)
	_ = p.Charge(60, 0)
	if err := p.PreFlight(); err != nil {
		t.Fatalf("PreFlight under cap should pass: %v", err)
	}
	_ = p.Charge(50, 0) // cumulative 110 > 100
	if err := p.PreFlight(); !errors.Is(err, ErrPerTurnBudgetExceeded) {
		t.Fatalf("PreFlight after excess: want ErrPerTurnBudgetExceeded, got %v", err)
	}
	// Reset restores PreFlight pass and is the loop's turn-boundary
	// signal that the cap is no longer exceeded.
	p.Reset()
	if err := p.PreFlight(); err != nil {
		t.Errorf("PreFlight after reset should pass: %v", err)
	}
}

// 6. Negative charges are clamped so a buggy provider payload cannot
// poison the accumulator.
func TestPerTurnBudget_Charge_NegativeClampedToZero(t *testing.T) {
	p := NewPerTurnBudget(100, 100)
	_ = p.Charge(-9999, -9999)
	if p.ThinkingUsed() != 0 || p.TokensUsed() != 0 {
		t.Errorf("negative charge should be clamped: thinking=%d tokens=%d",
			p.ThinkingUsed(), p.TokensUsed())
	}
}

// 7. Remaining can drop to zero but never negative.
func TestPerTurnBudget_Remaining_BoundedLow(t *testing.T) {
	p := NewPerTurnBudget(50, 0)
	_ = p.Charge(80, 0) // 30 over
	r := p.ThinkingRemaining()
	if r != 0 {
		t.Errorf("Remaining after overshoot: got %d, want 0", r)
	}
}

// 8. Nil receiver is a safe no-op so loops without per-turn budgets
// stay free of nil-deref crashes (mandate M7).
func TestPerTurnBudget_NilSafe(t *testing.T) {
	var p *PerTurnBudget
	if p.IsEnforced() {
		t.Error("nil IsEnforced should be false")
	}
	if err := p.Charge(99, 99); err != nil {
		t.Errorf("nil Charge should be no-op: %v", err)
	}
	if err := p.PreFlight(); err != nil {
		t.Errorf("nil PreFlight should pass: %v", err)
	}
	p.Reset()
	if p.ThinkingUsed() != 0 || p.TokensUsed() != 0 {
		t.Errorf("nil Read should be 0: thinking=%d tokens=%d", p.ThinkingUsed(), p.TokensUsed())
	}
}

// 9. Concurrent chargers from many goroutines do not lose updates and
// the final all-time sum is exact (race-clean under -race).
func TestPerTurnBudget_RaceSafe_ConcurrentCharges(t *testing.T) {
	p := NewPerTurnBudget(0, 0)
	const goroutines = 64
	const perG = 100
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				_ = p.Charge(1, 1)
			}
		}()
	}
	wg.Wait()
	want := goroutines * perG
	if got := p.ThinkingAllTime(); got != want {
		t.Errorf("thinking all-time after concurrency: got %d, want %d", got, want)
	}
	if got := p.TokensAllTime(); got != want {
		t.Errorf("tokens all-time after concurrency: got %d, want %d", got, want)
	}
}

// 10. ErrPerTurnBudgetExceeded wraps the loop's "thinking budget
// exhausted" substrate cleanly so future hooks/ledger code can match
// it with errors.Is without re-implementing the string.
func TestPerTurnBudget_Error_MatchByErrorsIs(t *testing.T) {
	p := NewPerTurnBudget(10, 0)
	err := p.Charge(11, 0)
	if !errors.Is(err, ErrPerTurnBudgetExceeded) {
		t.Fatalf("expected errors.Is match, got %v", err)
	}
}

// strings_notInErr returns true when the given error message does not
// contain the substring (used by test #2 to assert the error narrative
// does not falsely mention the unlimited dimension).
func strings_notInErr(err error, sub string) bool {
	if err == nil {
		return true
	}
	return !containsString(err.Error(), sub)
}

func containsString(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
