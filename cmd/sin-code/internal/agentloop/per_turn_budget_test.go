// SPDX-License-Identifier: MIT
// Purpose: unit tests for PerTurnBudget (issue #375, M7 race-clean).
package agentloop

import (
	"sync"
	"testing"
)

func TestNewPerTurnBudget_Defaults(t *testing.T) {
	b := NewPerTurnBudget(1000, 2000)
	if b.ThinkingRemaining() != 1000 {
		t.Fatalf("ThinkingRemaining = %d, want 1000", b.ThinkingRemaining())
	}
	if b.TokensRemaining() != 2000 {
		t.Fatalf("TokensRemaining = %d, want 2000", b.TokensRemaining())
	}
	th, tk := b.Stats()
	if th != 0 || tk != 0 {
		t.Fatalf("Stats() = (%d,%d), want (0,0)", th, tk)
	}
}

func TestPerTurnBudget_AddAndStats(t *testing.T) {
	b := NewPerTurnBudget(1000, 2000)
	b.AddThinking(300)
	b.AddTokens(500)
	b.AddThinking(100)
	b.AddTokens(200)
	th, tk := b.Stats()
	if th != 400 {
		t.Fatalf("thinking = %d, want 400", th)
	}
	if tk != 700 {
		t.Fatalf("tokens = %d, want 700", tk)
	}
}

func TestPerTurnBudget_ThinkingExceeded(t *testing.T) {
	b := NewPerTurnBudget(1000, 2000)
	b.AddThinking(800)
	if b.ThinkingExceeded() {
		t.Fatal("should not exceed at 800/1000")
	}
	b.AddThinking(300)
	if !b.ThinkingExceeded() {
		t.Fatal("should exceed at 1100/1000")
	}
	if b.TokenExceeded() {
		t.Fatal("tokens should not exceed")
	}
}

func TestPerTurnBudget_TokenExceeded(t *testing.T) {
	b := NewPerTurnBudget(1000, 2000)
	b.AddTokens(2001)
	if !b.TokenExceeded() {
		t.Fatal("tokens should exceed at 2001/2000")
	}
	if b.ThinkingExceeded() {
		t.Fatal("thinking should not exceed")
	}
}

func TestPerTurnBudget_UnlimitedAndReset(t *testing.T) {
	b := NewPerTurnBudget(0, 0)
	b.AddThinking(1 << 20)
	b.AddTokens(1 << 20)
	if b.ThinkingExceeded() {
		t.Fatal("unlimited thinking should never exceed")
	}
	if b.TokenExceeded() {
		t.Fatal("unlimited tokens should never exceed")
	}
	b.Reset()
	th, tk := b.Stats()
	if th != 0 || tk != 0 {
		t.Fatalf("after Reset Stats = (%d,%d), want (0,0)", th, tk)
	}
}

func TestPerTurnBudget_Concurrent(t *testing.T) {
	b := NewPerTurnBudget(10000, 10000)
	var wg sync.WaitGroup
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				b.AddThinking(1)
				b.AddTokens(1)
				_ = b.ThinkingExceeded()
				_ = b.TokenExceeded()
				_, _ = b.Stats()
			}
		}()
	}
	wg.Wait()
	th, tk := b.Stats()
	if th != 5000 {
		t.Fatalf("thinking = %d, want 5000", th)
	}
	if tk != 5000 {
		t.Fatalf("tokens = %d, want 5000", tk)
	}
}
