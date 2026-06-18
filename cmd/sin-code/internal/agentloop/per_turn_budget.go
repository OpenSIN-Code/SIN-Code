// SPDX-License-Identifier: MIT
// Purpose: per-turn thinking-budget and token-budget enforcement (issue
// #375). PerTurnBudget accumulates thinking-token and output-token usage
// within a single agent turn and reports when either budget is exceeded.
// A budget of 0 means unlimited for that dimension. Thread-safe (M7).
package agentloop

import "sync"

// PerTurnBudget tracks thinking-token and output-token spend for a single
// agent turn. A limit of 0 means unlimited for that dimension.
type PerTurnBudget struct {
	ThinkingTokens int
	TokenTokens    int

	mu           sync.Mutex
	usedThinking int
	usedTokens   int
}

// NewPerTurnBudget returns a budget that enforces up to thinking
// thinking-tokens and tokens output-tokens per turn. Either value may be
// 0 to disable enforcement for that dimension.
func NewPerTurnBudget(thinking, tokens int) *PerTurnBudget {
	return &PerTurnBudget{ThinkingTokens: thinking, TokenTokens: tokens}
}

// AddThinking accumulates n thinking tokens against the budget.
func (b *PerTurnBudget) AddThinking(n int) {
	b.mu.Lock()
	b.usedThinking += n
	b.mu.Unlock()
}

// AddTokens accumulates n output tokens against the budget.
func (b *PerTurnBudget) AddTokens(n int) {
	b.mu.Lock()
	b.usedTokens += n
	b.mu.Unlock()
}

// ThinkingExceeded reports whether the thinking-token budget has been
// surpassed. A limit of 0 (unlimited) never exceeds.
func (b *PerTurnBudget) ThinkingExceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ThinkingTokens <= 0 {
		return false
	}
	return b.usedThinking > b.ThinkingTokens
}

// TokenExceeded reports whether the output-token budget has been
// surpassed. A limit of 0 (unlimited) never exceeds.
func (b *PerTurnBudget) TokenExceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.TokenTokens <= 0 {
		return false
	}
	return b.usedTokens > b.TokenTokens
}

// Reset clears accumulated usage so the budget can be reused for a new turn.
func (b *PerTurnBudget) Reset() {
	b.mu.Lock()
	b.usedThinking = 0
	b.usedTokens = 0
	b.mu.Unlock()
}

// Stats returns the accumulated thinking-token and output-token usage.
func (b *PerTurnBudget) Stats() (thinking, tokens int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.usedThinking, b.usedTokens
}
