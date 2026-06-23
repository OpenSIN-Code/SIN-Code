// SPDX-License-Identifier: MIT
// Purpose: per-turn thinking and token budget enforcement (issue #375).
// The agent loop tracks how many reasoning tokens a single LLM turn
// consumes and refuses to send the next request when the configured
// per-turn cap (thinking or total) is exceeded. Distinct from the
// per-RUN caps already wired into Loop.ThinkingBudgetPerRequest and
// Loop.MaxTokens (issues #151 / first thinking-budget PR).
//
// Thread-safe (mandate M7): all mutations are mutex-guarded so callers
// can hold a single PerTurnBudget across concurrent sub-agent goroutines
// if they opt to share it.
package agentloop

import (
	"errors"
	"fmt"
	"sync"
)

// ErrPerTurnBudgetExceeded is returned by Charge when the accumulated
// per-turn usage exceeds either non-zero budget. The accumulation ALWAYS
// happens first so the cumulative-used counters stay accurate even when
// the caller bails on the error.
var ErrPerTurnBudgetExceeded = errors.New("agentloop: per-turn budget exceeded")

// PerTurnBudget tracks reasoning-token and total-token consumption for a
// single LLM turn. Configure once per run with NewPerTurnBudget; reset at
// every turn boundary with Reset; charge after each provider response.
//
// Zero-valued budget fields are unlimited for that dimension so callers
// can opt into one or both halves independently. Nil-safe: methods on a
// nil receiver are no-ops so loops without per-turn enforcement stay
// byte-identical to legacy behavior.
type PerTurnBudget struct {
	mu             sync.Mutex
	ThinkingBudget int // 0 = unlimited reasoning-token cap per turn
	TokenBudget    int // 0 = unlimited prompt+completion token cap per turn
	thinkingUsed   int // accumulated this turn (after the most recent Reset)
	tokensUsed     int // accumulated this turn (after the most recent Reset)
	// Totals are lifetime counters, never reset by Reset(). Useful for
	// debug logging and the TUI "thinking budget bar" (issue #375).
	thinkingAllTime int
	tokensAllTime   int
}

// NewPerTurnBudget creates a budget with the given per-turn caps. Zero
// means unlimited for that dimension.
func NewPerTurnBudget(thinkingBudget, tokenBudget int) *PerTurnBudget {
	if thinkingBudget < 0 {
		thinkingBudget = 0
	}
	if tokenBudget < 0 {
		tokenBudget = 0
	}
	return &PerTurnBudget{
		ThinkingBudget: thinkingBudget,
		TokenBudget:    tokenBudget,
	}
}

// Reset zeroes the per-turn accumulators. Both AllTime counters are
// preserved so the TUI can render lifetime usage across the session
// (issue #375 acceptance criterion: "TUI shows thinking budget bar").
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
// provider response. It always increments the accumulators (so the
// failure path still reports accurate "over-budget" totals) and returns
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
	if p.ThinkingBudget > 0 && p.thinkingUsed > p.ThinkingBudget {
		exceeded = true
		detail = fmt.Sprintf("thinking %d > %d", p.thinkingUsed, p.ThinkingBudget)
	}
	if p.TokenBudget > 0 && p.tokensUsed > p.TokenBudget {
		exceeded = true
		if detail != "" {
			detail += "; "
		}
		detail += fmt.Sprintf("tokens %d > %d", p.tokensUsed, p.TokenBudget)
	}
	p.mu.Unlock()
	if exceeded {
		return fmt.Errorf("%w: %s", ErrPerTurnBudgetExceeded, detail)
	}
	return nil
}

// PreFlight returns ErrPerTurnBudgetExceeded when the accumulators from
// a previous turn already exceeded the cap. It is the "cut off BEFORE
// sending" hook (issue #375): the loop calls PreFlight immediately before
// invoking the provider, so a prior turn that burned the budget blocks
// the next provider call for free — no wire round-trip wasted.
//
// PreFlight does NOT mutate any counter.
func (p *PerTurnBudget) PreFlight() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ThinkingBudget > 0 && p.thinkingUsed > p.ThinkingBudget {
		return fmt.Errorf("%w: thinking %d > %d", ErrPerTurnBudgetExceeded, p.thinkingUsed, p.ThinkingBudget)
	}
	if p.TokenBudget > 0 && p.tokensUsed > p.TokenBudget {
		return fmt.Errorf("%w: tokens %d > %d", ErrPerTurnBudgetExceeded, p.tokensUsed, p.TokenBudget)
	}
	return nil
}

// IsEnforced reports whether any non-zero cap is wired. The loop never
// calls PreFlight/Charge when this returns false so the no-budget path
// stays zero-cost.
func (p *PerTurnBudget) IsEnforced() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ThinkingBudget > 0 || p.TokenBudget > 0
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

// TokensAllTime returns the lifetime total-token accumulator
// (preserved across Reset calls).
func (p *PerTurnBudget) TokensAllTime() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokensAllTime
}

// ThinkingRemaining returns the per-turn thinking budget remaining.
// -1 == unlimited (no cap configured). 0 means on-cap; >0 means under.
func (p *PerTurnBudget) ThinkingRemaining() int {
	if p == nil {
		return -1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ThinkingBudget <= 0 {
		return -1
	}
	r := p.ThinkingBudget - p.thinkingUsed
	if r < 0 {
		r = 0
	}
	return r
}

// TokensRemaining returns the per-turn token budget remaining. Same
// -1 == unlimited convention as ThinkingRemaining.
func (p *PerTurnBudget) TokensRemaining() int {
	if p == nil {
		return -1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.TokenBudget <= 0 {
		return -1
	}
	r := p.TokenBudget - p.tokensUsed
	if r < 0 {
		r = 0
	}
	return r
}
