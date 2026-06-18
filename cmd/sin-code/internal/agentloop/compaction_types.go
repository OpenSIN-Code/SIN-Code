// SPDX-License-Identifier: MIT
// Package agentloop - context compaction types (CompactInput / CompactResult /
// ContextCompactionMode / CompactionTrigger / CompactorConfig). See
// compaction.go for the implementation, this file holds the public surface
// only.
package agentloop

import "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"

// ContextCompactionMode selects the compaction algorithm.
// Empty == "off". Closed set: off | deterministic | llm | hybrid.
type ContextCompactionMode string

const (
	ContextCompactionOff           ContextCompactionMode = "off"
	ContextCompactionDeterministic ContextCompactionMode = "deterministic"
	ContextCompactionLLM           ContextCompactionMode = "llm"
	ContextCompactionHybrid        ContextCompactionMode = "hybrid"
)

// ParseContextCompactionMode normalises user input. Empty/dotted variants
// map to Off; unknown values return a typed error.
func ParseContextCompactionMode(s string) (ContextCompactionMode, error) {
	switch toLowerTrim(s) {
	case "off", "none", "disabled", "", "default":
		return ContextCompactionOff, nil
	case "deterministic", "det":
		return ContextCompactionDeterministic, nil
	case "llm", "summarize":
		return ContextCompactionLLM, nil
	case "hybrid", "llm+deterministic":
		return ContextCompactionHybrid, nil
	}
	return ContextCompactionOff, errUnknownMode(s)
}

// String makes ContextCompactionMode satisfy fmt.Stringer.
func (m ContextCompactionMode) String() string {
	if m == "" {
		return string(ContextCompactionOff)
	}
	return string(m)
}

// IsLossy reports whether the mode produces sidecar-snapshot-worthy output.
func (m ContextCompactionMode) IsLossy() bool {
	switch m {
	case ContextCompactionLLM, ContextCompactionHybrid:
		return true
	}
	return false
}

// CompactionTrigger decides when ShouldCompact returns true.
type CompactionTrigger string

const (
	CompactionTriggerTurns  CompactionTrigger = "turns"
	CompactionTriggerTokens CompactionTrigger = "tokens"
	CompactionTriggerBoth   CompactionTrigger = "both"
)

// ParseCompactionTrigger normalises user input.
func ParseCompactionTrigger(s string) (CompactionTrigger, error) {
	switch toLowerTrim(s) {
	case "turns", "messages":
		return CompactionTriggerTurns, nil
	case "tokens":
		return CompactionTriggerTokens, nil
	case "", "both", "any", "default":
		return CompactionTriggerBoth, nil
	}
	return CompactionTriggerBoth, errUnknownTrigger(s)
}

// String makes CompactionTrigger satisfy fmt.Stringer.
func (t CompactionTrigger) String() string {
	if t == "" {
		return string(CompactionTriggerBoth)
	}
	return string(t)
}

// CompactorConfig is the wired shape the loopbuilder passes.
type CompactorConfig struct {
	Mode             ContextCompactionMode
	Trigger          CompactionTrigger
	Threshold        float64
	ContextWindow    int
	MaxTokens        int
	PreserveEvidence bool
	RecentTurns      int
}

// DefaultCompactorConfig returns the safe default config (Mode=off so
// the legacy single-gate behavior is preserved byte-for-byte).
func DefaultCompactorConfig() CompactorConfig {
	return CompactorConfig{
		Mode:             ContextCompactionOff,
		Trigger:          CompactionTriggerBoth,
		Threshold:        0.8,
		ContextWindow:    0,
		MaxTokens:        8000,
		PreserveEvidence: true,
		RecentTurns:      4,
	}
}

// Normalize fills zero-value fields with safe defaults and clamps bad
// inputs so downstream code never has to re-validate.
func (c *CompactorConfig) Normalize() {
	if c.Mode == "" {
		c.Mode = ContextCompactionOff
	}
	if c.Trigger == "" {
		c.Trigger = CompactionTriggerBoth
	}
	if c.Threshold <= 0 {
		c.Threshold = 0.8
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 8000
	}
	if c.RecentTurns <= 0 {
		c.RecentTurns = 4
	}
}

// CompactInput is the request payload for CompactInput(ctx, input).
type CompactInput struct {
	Messages        []session.Message
	EvidenceIndices map[int]bool
	Strategy        CompactionStrategy
	Mode            ContextCompactionMode
	MaxTokens       int
	SessionID       string
}

// CompactResult is the structured response from CompactInput.
type CompactResult struct {
	Kept         []session.Message
	Dropped      []session.Message
	Summary      string
	SnapshotID   string
	TokensBefore int
	TokensAfter  int
	Mode         ContextCompactionMode
}
