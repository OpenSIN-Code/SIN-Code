// SPDX-License-Identifier: MIT
package tui

import (
	"sync"
	"time"
)

// TypewriterConfig controls the typewriter streaming effect.
type TypewriterConfig struct {
	Enabled     bool // master switch
	CharsPerSec int  // reveal speed (0 = instant/no effect)
	BatchSize   int  // chars per tick (1 = true typewriter, 4 = smooth)
}

// DefaultTypewriterConfig returns sensible defaults.
func DefaultTypewriterConfig() TypewriterConfig {
	return TypewriterConfig{
		Enabled:     true,
		CharsPerSec: 200, // fast enough to not lag, slow enough to see motion
		BatchSize:   3,
	}
}

// TypewriterBuffer progressively reveals streamed text with a typewriter
// effect. It stores the full text but only exposes a growing prefix to the
// renderer.
type TypewriterBuffer struct {
	mu       sync.Mutex
	full     string    // complete text received so far
	revealed int       // how many chars of full are visible
	cfg      TypewriterConfig
	lastTick time.Time
}

// NewTypewriterBuffer creates a buffer with the given configuration.
func NewTypewriterBuffer(cfg TypewriterConfig) *TypewriterBuffer {
	return &TypewriterBuffer{
		cfg:      cfg,
		lastTick: time.Now(),
	}
}

// Append adds a streamed chunk to the full text.
func (tb *TypewriterBuffer) Append(text string) {
	if text == "" {
		return
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.full += text
}

// Tick advances the revealed count based on elapsed time since the last tick.
// The tick interval should match streamTickInterval (250ms) from update.go.
func (tb *TypewriterBuffer) Tick() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.revealed >= len(tb.full) {
		return
	}
	if !tb.cfg.Enabled || tb.cfg.CharsPerSec <= 0 {
		tb.revealed = len(tb.full)
		tb.lastTick = time.Now()
		return
	}
	elapsed := time.Since(tb.lastTick)
	charsToAdd := int(elapsed.Seconds() * float64(tb.cfg.CharsPerSec))
	batch := tb.cfg.BatchSize
	if batch < 1 {
		batch = 1
	}
	if charsToAdd < batch {
		charsToAdd = batch
	}
	tb.revealed += charsToAdd
	if tb.revealed > len(tb.full) {
		tb.revealed = len(tb.full)
	}
	tb.lastTick = time.Now()
}

// Visible returns the current visible prefix of the full text.
func (tb *TypewriterBuffer) Visible() string {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.revealed >= len(tb.full) {
		return tb.full
	}
	if tb.revealed < 0 {
		return ""
	}
	return tb.full[:tb.revealed]
}

// Full returns the complete text regardless of reveal progress.
func (tb *TypewriterBuffer) Full() string {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.full
}

// Complete jumps to the end, revealing everything.
func (tb *TypewriterBuffer) Complete() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.revealed = len(tb.full)
	tb.lastTick = time.Now()
}

// Reset clears the buffer for the next response.
func (tb *TypewriterBuffer) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.full = ""
	tb.revealed = 0
	tb.lastTick = time.Now()
}

// IsComplete returns true when all text has been revealed.
func (tb *TypewriterBuffer) IsComplete() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if len(tb.full) == 0 {
		return false
	}
	return tb.revealed >= len(tb.full)
}

// Config returns the current configuration.
func (tb *TypewriterBuffer) Config() TypewriterConfig {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.cfg
}

// SetConfig updates the configuration.
func (tb *TypewriterBuffer) SetConfig(cfg TypewriterConfig) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.cfg = cfg
}

// RevealedCount returns the number of characters currently revealed.
func (tb *TypewriterBuffer) RevealedCount() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.revealed
}

// Len returns the total length of the full text.
func (tb *TypewriterBuffer) Len() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return len(tb.full)
}
