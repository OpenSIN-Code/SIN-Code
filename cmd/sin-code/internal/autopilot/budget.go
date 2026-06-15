// SPDX-License-Identifier: MIT
// Purpose: bounded-autonomy watchdog (mandate M4). Hard wall-clock and
// experiment caps that deterministically stop the autonomous loop.
package autopilot

import (
	"fmt"
	"sync"
	"time"
)

// Budget enforces the two hard limits of bounded autonomy.
type Budget struct {
	mu             sync.Mutex
	deadline       time.Time
	maxExperiments int
	used           int
	startedAt      time.Time
}

// NewBudget creates a budget with a wall-clock and experiment cap.
func NewBudget(minutes, maxExperiments int) *Budget {
	now := time.Now()
	return &Budget{
		deadline:       now.Add(time.Duration(minutes) * time.Minute),
		maxExperiments: maxExperiments,
		startedAt:      now,
	}
}

// testBudgetForceConsumeFail is set by coverage tests to force Consume to
// return false without actually advancing the experiment counter.
var testBudgetForceConsumeFail bool

// StopReason explains why the loop must end ("" means keep going).
func (b *Budget) StopReason() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maxExperiments > 0 && b.used >= b.maxExperiments {
		return fmt.Sprintf("experiment cap reached (%d)", b.maxExperiments)
	}
	if time.Now().After(b.deadline) {
		return fmt.Sprintf("time budget exhausted (%s)", time.Since(b.startedAt).Round(time.Second))
	}
	return ""
}

// CanContinue reports whether another experiment is allowed.
func (b *Budget) CanContinue() bool { return b.StopReason() == "" }

// Consume records that one experiment was started. Returns false if the
// experiment cap was already hit (caller must not start the experiment).
func (b *Budget) Consume() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if testBudgetForceConsumeFail {
		return false
	}
	if b.maxExperiments > 0 && b.used >= b.maxExperiments {
		return false
	}
	b.used++
	return true
}

// Remaining returns time and experiment headroom for status reporting.
func (b *Budget) Remaining() (time.Duration, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := time.Until(b.deadline)
	if d < 0 {
		d = 0
	}
	left := b.maxExperiments - b.used
	if left < 0 {
		left = 0
	}
	return d, left
}

// Used returns how many experiments have been consumed.
func (b *Budget) Used() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

// Elapsed returns wall-clock time since the budget started.
func (b *Budget) Elapsed() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return time.Since(b.startedAt)
}
