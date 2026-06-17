// SPDX-License-Identifier: MIT
// Purpose: unit tests for the Budget enforcer (issue #320, M7).
package agentloop

import (
	"errors"
	"sync"
	"testing"
)

func TestBudget_Consume_UnderLimit_NoError(t *testing.T) {
	b := NewBudget(10000, 5.0)
	if err := b.Consume(3000, 1.0); err != nil {
		t.Fatalf("expected no error under limit, got: %v", err)
	}
	remTok, remCost := b.Remaining()
	if remTok != 7000 {
		t.Errorf("remaining tokens: got %d, want 7000", remTok)
	}
	if remCost != 4.0 {
		t.Errorf("remaining cost: got %.2f, want 4.0", remCost)
	}
}

func TestBudget_Consume_OverTokenLimit_Error(t *testing.T) {
	b := NewBudget(1000, 5.0)
	if err := b.Consume(1500, 0.5); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted, got: %v", err)
	}
	if !b.IsExhausted() {
		t.Error("expected IsExhausted=true after exceeding token limit")
	}
}

func TestBudget_Consume_OverCostLimit_Error(t *testing.T) {
	b := NewBudget(10000, 0.10)
	if err := b.Consume(100, 0.20); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted, got: %v", err)
	}
	if !b.IsExhausted() {
		t.Error("expected IsExhausted=true after exceeding cost limit")
	}
}

func TestBudget_Unlimited_NoError(t *testing.T) {
	b := NewBudget(0, 0)
	if err := b.Consume(9_999_999, 9_999.0); err != nil {
		t.Fatalf("unlimited budget should never error, got: %v", err)
	}
	if b.IsExhausted() {
		t.Error("unlimited budget should not be exhausted")
	}
	remTok, remCost := b.Remaining()
	if remTok != -1 || remCost != -1 {
		t.Errorf("unlimited remaining should be -1, -1; got %d, %.2f", remTok, remCost)
	}
}

func TestBudget_Percent(t *testing.T) {
	b := NewBudget(1000, 10.0)
	b.Consume(300, 0)
	if pct := b.Percent(); pct < 0.29 || pct > 0.31 {
		t.Errorf("percent after 300/1000 tokens: got %.2f, want ~0.30", pct)
	}
	b.Consume(0, 5.0)
	if pct := b.Percent(); pct < 0.49 || pct > 0.51 {
		t.Errorf("percent after $5/$10 cost: got %.2f, want ~0.50", pct)
	}
}

func TestBudget_Level(t *testing.T) {
	b := NewBudget(1000, 10.0)
	b.Consume(200, 0)
	if b.Level() != BudgetGreen {
		t.Errorf("at 20%%: got %s, want green", b.Level())
	}
	b.Reset()
	b.Consume(650, 0)
	if b.Level() != BudgetYellow {
		t.Errorf("at 65%%: got %s, want yellow", b.Level())
	}
	b.Reset()
	b.Consume(950, 0)
	if b.Level() != BudgetRed {
		t.Errorf("at 95%%: got %s, want red", b.Level())
	}
}

func TestBudget_Reset(t *testing.T) {
	b := NewBudget(1000, 5.0)
	b.Consume(500, 2.5)
	b.Reset()
	if b.IsExhausted() {
		t.Error("after reset, budget should not be exhausted")
	}
	if tok := b.UsedTokens(); tok != 0 {
		t.Errorf("after reset, used tokens: got %d, want 0", tok)
	}
	if cost := b.UsedCost(); cost != 0 {
		t.Errorf("after reset, used cost: got %.2f, want 0", cost)
	}
	if lvl := b.Level(); lvl != BudgetGreen {
		t.Errorf("after reset, level: got %s, want green", lvl)
	}
}

func TestBudget_Remaining_OverLimit_Zero(t *testing.T) {
	b := NewBudget(1000, 5.0)
	b.Consume(2000, 10.0)
	remTok, remCost := b.Remaining()
	if remTok != 0 {
		t.Errorf("remaining tokens after overshoot: got %d, want 0", remTok)
	}
	if remCost != 0 {
		t.Errorf("remaining cost after overshoot: got %.2f, want 0", remCost)
	}
}

func TestBudget_NilSafe(t *testing.T) {
	var b *Budget
	if err := b.Consume(100, 1.0); err != nil {
		t.Errorf("nil Consume should be no-op, got: %v", err)
	}
	if b.IsExhausted() {
		t.Error("nil IsExhausted should be false")
	}
	if b.Level() != BudgetGreen {
		t.Error("nil Level should be green")
	}
	if pct := b.Percent(); pct != 0 {
		t.Errorf("nil Percent should be 0, got %.2f", pct)
	}
	tok, cost := b.Remaining()
	if tok != -1 || cost != -1 {
		t.Errorf("nil Remaining should be -1, -1; got %d, %.2f", tok, cost)
	}
	b.Reset()
}

func TestBudget_RaceSafe(t *testing.T) {
	b := NewBudget(100000, 100.0)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Consume(100, 0.1)
			_ = b.IsExhausted()
			_ = b.Level()
			_, _ = b.Remaining()
			_ = b.Percent()
		}()
	}
	wg.Wait()
	tok := b.UsedTokens()
	if tok != 20000 {
		t.Errorf("after 200×100 consume, used tokens: got %d, want 20000", tok)
	}
}
