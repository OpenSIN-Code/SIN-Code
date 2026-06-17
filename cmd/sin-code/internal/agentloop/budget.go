// SPDX-License-Identifier: MIT
// Purpose: Budget enforcement — token and cost limits per session and
// project (issue #320). The Budget struct accumulates token and USD
// spend and exposes level/remaining/percent queries so the agent loop
// can stop before overspending. Thread-safe (mandate M7).
package agentloop

import (
	"errors"
	"fmt"
	"sync"
)

// BudgetLevel classifies how much of the budget has been consumed.
type BudgetLevel int

const (
	// BudgetGreen means usage is below 60%.
	BudgetGreen BudgetLevel = iota
	// BudgetYellow means usage is between 60% and 90% (inclusive).
	BudgetYellow
	// BudgetRed means usage exceeds 90%.
	BudgetRed
)

func (l BudgetLevel) String() string {
	switch l {
	case BudgetGreen:
		return "green"
	case BudgetYellow:
		return "yellow"
	case BudgetRed:
		return "red"
	default:
		return "unknown"
	}
}

// ErrBudgetExhausted is returned by Consume when either the token or cost
// limit has been exceeded.
var ErrBudgetExhausted = errors.New("agentloop: budget exhausted")

// Budget enforces token and cost limits for a session or project. A zero
// limit (maxTokens == 0 or maxCostUSD == 0) means unlimited for that
// dimension — only non-zero limits are enforced.
//
// All methods are safe for concurrent use (mandate M7).
type Budget struct {
	mu         sync.Mutex
	maxTokens  int
	maxCostUSD float64
	usedTokens int
	usedCost   float64
}

// NewBudget creates a Budget with the given limits. Zero means unlimited
// for that dimension.
func NewBudget(maxTokens int, maxCostUSD float64) *Budget {
	return &Budget{
		maxTokens:  maxTokens,
		maxCostUSD: maxCostUSD,
	}
}

// Consume records token and cost usage. It always adds the usage to the
// running totals (so tracking remains accurate even when over budget) and
// then returns ErrBudgetExhausted if either non-zero limit is exceeded.
func (b *Budget) Consume(tokens int, cost float64) error {
	if b == nil {
		return nil
	}
	if tokens < 0 {
		tokens = 0
	}
	if cost < 0 {
		cost = 0
	}
	b.mu.Lock()
	b.usedTokens += tokens
	b.usedCost += cost
	exceeded := false
	var detail string
	if b.maxTokens > 0 && b.usedTokens > b.maxTokens {
		exceeded = true
		detail = fmt.Sprintf("tokens %d > %d", b.usedTokens, b.maxTokens)
	}
	if b.maxCostUSD > 0 && b.usedCost > b.maxCostUSD {
		exceeded = true
		if detail != "" {
			detail += "; "
		}
		detail += fmt.Sprintf("cost $%.4f > $%.4f", b.usedCost, b.maxCostUSD)
	}
	b.mu.Unlock()
	if exceeded {
		return fmt.Errorf("%w: %s", ErrBudgetExhausted, detail)
	}
	return nil
}

// Remaining returns the remaining token and cost budget. For unlimited
// dimensions (limit == 0) the remaining value is -1, signalling
// "unlimited" to the caller.
func (b *Budget) Remaining() (int, float64) {
	if b == nil {
		return -1, -1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	remTokens := -1
	if b.maxTokens > 0 {
		remTokens = b.maxTokens - b.usedTokens
		if remTokens < 0 {
			remTokens = 0
		}
	}
	remCost := -1.0
	if b.maxCostUSD > 0 {
		remCost = b.maxCostUSD - b.usedCost
		if remCost < 0 {
			remCost = 0
		}
	}
	return remTokens, remCost
}

// Percent returns the fraction of the budget that has been consumed,
// expressed as a value between 0.0 and 1.0+. When both limits are zero
// (unlimited), it returns 0. When only one dimension has a limit, that
// dimension's percentage is used. When both have limits, the higher of
// the two is returned — the caller should act on the most-consumed
// dimension.
func (b *Budget) Percent() float64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var tokPct, costPct float64
	if b.maxTokens > 0 {
		tokPct = float64(b.usedTokens) / float64(b.maxTokens)
	}
	if b.maxCostUSD > 0 {
		costPct = b.usedCost / b.maxCostUSD
	}
	if tokPct >= costPct {
		return tokPct
	}
	return costPct
}

// IsExhausted reports whether either non-zero limit has been exceeded.
func (b *Budget) IsExhausted() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maxTokens > 0 && b.usedTokens > b.maxTokens {
		return true
	}
	if b.maxCostUSD > 0 && b.usedCost > b.maxCostUSD {
		return true
	}
	return false
}

// Level returns the budget level based on Percent(): Green < 60%,
// Yellow 60–90%, Red > 90%.
func (b *Budget) Level() BudgetLevel {
	if b == nil {
		return BudgetGreen
	}
	pct := b.Percent()
	switch {
	case pct > 0.9:
		return BudgetRed
	case pct >= 0.6:
		return BudgetYellow
	default:
		return BudgetGreen
	}
}

// Reset zeroes the accumulated usage. Limits are preserved.
func (b *Budget) Reset() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.usedTokens = 0
	b.usedCost = 0
	b.mu.Unlock()
}

// UsedTokens returns the total tokens consumed so far.
func (b *Budget) UsedTokens() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.usedTokens
}

// UsedCost returns the total USD cost consumed so far.
func (b *Budget) UsedCost() float64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.usedCost
}

// MaxTokens returns the configured token limit (0 = unlimited).
func (b *Budget) MaxTokens() int {
	if b == nil {
		return 0
	}
	return b.maxTokens
}

// MaxCostUSD returns the configured cost limit (0 = unlimited).
func (b *Budget) MaxCostUSD() float64 {
	if b == nil {
		return 0
	}
	return b.maxCostUSD
}
