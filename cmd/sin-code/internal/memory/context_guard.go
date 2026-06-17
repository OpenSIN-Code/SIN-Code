// SPDX-License-Identifier: MIT
// Purpose: context exhaustion warnings and auto-compaction trigger
// (issue #350). The ContextGuard monitors token usage against a
// budget and produces colour-coded status levels. When usage reaches
// Orange (80%+), compaction should be triggered; at Yellow (60%+)
// a warning is surfaced. Thread-safe (mandate M7).
package memory

import (
	"fmt"
	"strings"
	"sync"
)

// GuardLevel is the colour-coded context usage level.
type GuardLevel int

const (
	GuardGreen  GuardLevel = iota // < 60% — plenty of headroom
	GuardYellow                   // 60–80% — warn user
	GuardOrange                   // 80–95% — compact now
	GuardRed                      // > 95% — critical, stop adding
)

func (l GuardLevel) String() string {
	switch l {
	case GuardGreen:
		return "green"
	case GuardYellow:
		return "yellow"
	case GuardOrange:
		return "orange"
	case GuardRed:
		return "red"
	default:
		return "unknown"
	}
}

const (
	greenYellowBoundary  = 0.60
	yellowOrangeBoundary = 0.80
	orangeRedBoundary    = 0.95
)

// ContextGuard monitors context token usage and determines when
// warnings or compaction should be triggered.
type ContextGuard struct {
	mu        sync.RWMutex
	maxTokens int
	used      int
}

// NewContextGuard creates a guard with the given token budget.
func NewContextGuard(maxTokens int) *ContextGuard {
	if maxTokens <= 0 {
		maxTokens = 1
	}
	return &ContextGuard{maxTokens: maxTokens}
}

// Update sets the current token usage count.
func (g *ContextGuard) Update(used int) {
	if used < 0 {
		used = 0
	}
	g.mu.Lock()
	g.used = used
	g.mu.Unlock()
}

// ratio returns the current usage ratio (0.0 – 1.0+).
func (g *ContextGuard) ratio() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.maxTokens <= 0 {
		return 1.0
	}
	return float64(g.used) / float64(g.maxTokens)
}

// Level returns the current guard level based on usage ratio.
func (g *ContextGuard) Level() GuardLevel {
	r := g.ratio()
	switch {
	case r < greenYellowBoundary:
		return GuardGreen
	case r < yellowOrangeBoundary:
		return GuardYellow
	case r < orangeRedBoundary:
		return GuardOrange
	default:
		return GuardRed
	}
}

// ShouldCompact returns true when the level is Orange or Red —
// the agent should trigger auto-compaction.
func (g *ContextGuard) ShouldCompact() bool {
	lvl := g.Level()
	return lvl == GuardOrange || lvl == GuardRed
}

// ShouldWarn returns true when the level is Yellow or above —
// the user should be warned about context pressure.
func (g *ContextGuard) ShouldWarn() bool {
	lvl := g.Level()
	return lvl != GuardGreen
}

// Message returns a human-readable status string with usage
// percentage and recommended action.
func (g *ContextGuard) Message() string {
	g.mu.RLock()
	used, max := g.used, g.maxTokens
	g.mu.RUnlock()
	pct := 0
	if max > 0 {
		pct = used * 100 / max
	}
	lvl := g.Level()
	var b strings.Builder
	fmt.Fprintf(&b, "[")
	filled := pct / 10
	for i := 0; i < 10; i++ {
		if i < filled {
			b.WriteRune('█')
		} else {
			b.WriteRune('░')
		}
	}
	fmt.Fprintf(&b, "] %d%% (%s)", pct, lvl.String())
	switch lvl {
	case GuardGreen:
		b.WriteString(" — context healthy")
	case GuardYellow:
		b.WriteString(" — consider trimming conversation")
	case GuardOrange:
		b.WriteString(" — auto-compaction recommended")
	case GuardRed:
		b.WriteString(" — context nearly exhausted, compact immediately")
	}
	return b.String()
}

// Used returns the current token usage count.
func (g *ContextGuard) Used() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.used
}

// Max returns the token budget.
func (g *ContextGuard) Max() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.maxTokens
}
