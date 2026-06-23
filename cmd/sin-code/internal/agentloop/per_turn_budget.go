// SPDX-License-Identifier: MIT
// Purpose: per-turn thinking and token budget enforcement (issue #375).
// Distinct from the per-RUN MaxTokens / ThinkingBudgetPerRequest caps that
// the loop already accumulates: this package tracks *per single LLM turn*
// usage and refuses to emit the next response once a non-zero cap is
// breached. Race-clean (mandate M7).
package agentloop

import (
	"errors"
	"fmt"
	"sync"
)

// ErrPerTurnBudgetExceeded is returned by Charge when the per-turn
// accumulator crosses either non-zero budget. The accumulation ALWAYS
// happens first so the counters stay accurate even when the caller bails
// on the error.
var ErrPerTurnBudgetExceeded = errors.New("agentloop: per-turn budget exceeded")

// PerTurnBudget tracks reasoning-token and total-token consumption for a
// single LLM turn. Configure once per Run with NewPerTurnBudget(thinking,
// tokens); call Reset at every turn boundary; call Charge after each
// provider response.
//
// Zero-valued budget fields are unlimited for that dimension so callers
// can opt into one or both halves independently. Nil-safe: methods on a
// nil receiver are no-ops so loops without per-turn enforcement stay
// byte-identical to legacy behaviour and never nil-deref.
type PerTurnBudget struct {
	mu             sync.Mutex
	thinkingBudget int
	tokenBudget    int
	thinkingUsed   int
	tokensUsed     int
	thinkingAllTime int
	tokensAllTime   int
}

// NewPerTurnBudget creates a per-turn budget with the given caps. Zero or
// negative means unlimited for that dimension.
func NewPerTurnBudget(thinkingBudget, tokenBudget int) *PerTurnBudget {
	if thinkingBudget < 0 {
		thinkingBudget = 0
	}
	if tokenBudget < 0 {
		tokenBudget = 0
	}
	return &PerTurnBudget{
		thinkingBudget: thinkingBudget,
		tokenBudget:    tokenBudget,
	}
}

// Reset zeroes the per-turn accumulators. Lifetime counters are preserved
// so dashboards can render "thinking used this session so far" alongside
// the current-turn readout.
func (p *PerTurnBudget) Reset() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.thinkingUsed = 0
	p.tokensUsed = 0
	p.mu.Unlock()
}

// Charge records thinking and total-token usage from the most recent
// provider response. It always increments the accumulators first (so
// failure paths still report accurate over-budget totals) then returns
// ErrPerTurnBudgetExceeded when either non-zero cap is exceeded.
//
// Negative inputs are clamped to zero so a buggy provider payload cannot
// corrupt the accumulators.
func (p *PerTurnBudget) Charge(thinkingTokens, totalTokens int) error {
	if p == nil {
		return nil
	}
	if thinkingTokens < 0 {
		thinkingTokens = 0
	}
	if totalTokens < 0 {
		totalTokens = 0
	}
	p.mu.Lock()
	p.thinkingUsed += thinkingTokens
	p.tokensUsed += totalTokens
	p.thinkingAllTime += thinkingTokens
	p.tokensAllTime += totalTokens
	exceeded := false
	var detail string
	if p.thinkingBudget > 0 && p.thinkingUsed > p.thinkingBudget {
		exceeded = true
		detail = fmt.Sprintf("thinking %d > %d", p.thinkingUsed, p.thinkingBudget)
	}
	if p.tokenBudget > 0 && p.tokensUsed > p.tokenBudget {
		exceeded = true
		if detail != "" {
			detail += "; "
		}
		detail += fmt.Sprintf("tokens %d > %d", p.tokensUsed, p.tokenBudget)
	}
	p.mu.Unlock()
	if exceeded {
		return fmt.Errorf("%w: %s", ErrPerTurnBudgetExceeded, detail)
	}
	return nil
}

// PreFlight reports ErrPerTurnBudgetExceeded when the per-turn
// accumulators from a previous turn already exceeded the cap. It is the
// "cut off BEFORE sending" check (issue #375 acceptance criterion): the
// loop calls PreFlight immediately before invoking the provider so a
// prior turn that burned the budget blocks the next provider call for
// free — no wire round-trip wasted on a guaranteed over-budget request.
//
// PreFlight does NOT mutate any counter.
func (p *PerTurnBudget) PreFlight() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.thinkingBudget > 0 && p.thinkingUsed > p.thinkingBudget {
		return fmt.Errorf("%w: thinking %d > %d",
			ErrPerTurnBudgetExceeded, p.thinkingUsed, p.thinkingBudget)
	}
	if p.tokenBudget > 0 && p.tokensUsed > p.tokenBudget {
		return fmt.Errorf("%w: tokens %d > %d",
			ErrPerTurnBudgetExceeded, p.tokensUsed, p.tokenBudget)
	}
	return nil
}

// IsEnforced reports whether any non-zero cap is wired. The loop skips
// PreFlight/Charge when this returns false so the no-budget path stays
// zero-cost.
func (p *PerTurnBudget) IsEnforced() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.thinkingBudget > 0 || p.tokenBudget > 0
}

// ThinkingUsed returns the per-turn reasoning tokens consumed so far.
func (p *PerTurnBudget) ThinkingUsed() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.thinkingUsed
}

// TokensUsed returns the per-turn total tokens consumed so far.
func (p *PerTurnBudget) TokensUsed() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokensUsed
}

// ThinkingAllTime returns the lifetime thinking-token accumulator
// (preserved across Reset calls).
func (p *PerTurnBudget) ThinkingAllTime() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.thinkingAllTime
}

// TokensAllTime returns the lifetime total-token accumulator (preserved
// across Reset calls).
func (p *PerTurnBudget) TokensAllTime() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokensAllTime
}

// ThinkingRemaining returns the per-turn thinking budget remaining. -1
// means unlimited (no cap configured). 0 means on-cap; >0 means under.
func (p *PerTurnBudget) ThinkingRemaining() int {
	if p == nil {
		return -1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.thinkingBudget <= 0 {
		return -1
	}
	r := p.thinkingBudget - p.thinkingUsed
	if r < 0 {
		r = 0
	}
	return r
}

// TokensRemaining returns the per-turn token budget remaining. -1 means
// unlimited (no cap configured).
func (p *PerTurnBudget) TokensRemaining() int {
	if p == nil {
		return -1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tokenBudget <= 0 {
		return -1
	}
	r := p.tokenBudget - p.tokensUsed
	if r < 0 {
		r = 0
	}
	return r
}
