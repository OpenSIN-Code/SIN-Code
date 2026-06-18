// SPDX-License-Identifier: MIT
// Purpose: context compaction modes agentic|basic|off (issue #378).
// CompactionMode is a higher-level user-facing selector that maps onto the
// existing CompactionStrategy enum defined in compaction.go. It does not
// replace ContextCompactionMode (the lower-level off|deterministic|llm|hybrid
// selector); it is an orthogonal, simpler 3-state control surface.
package agentloop

import (
	"fmt"
	"strings"
)

// CompactionMode selects how aggressively context is compacted.
type CompactionMode int

const (
	// CompactionModeAgentic keeps user prompts, tool results, and
	// verification evidence while summarising intermediate reasoning.
	CompactionModeAgentic CompactionMode = iota
	// CompactionModeBasic summarises the oldest messages and keeps the
	// last PreserveLastN turns verbatim.
	CompactionModeBasic
	// CompactionModeOff disables automatic compaction entirely.
	CompactionModeOff
)

// String makes CompactionMode satisfy fmt.Stringer.
func (m CompactionMode) String() string {
	switch m {
	case CompactionModeAgentic:
		return "agentic"
	case CompactionModeBasic:
		return "basic"
	case CompactionModeOff:
		return "off"
	default:
		return "unknown"
	}
}

// ParseCompactionMode normalises user input. Empty/off/none/disabled map to
// Off; unknown values return a typed error.
func ParseCompactionMode(s string) (CompactionMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "agentic", "agent", "smart":
		return CompactionModeAgentic, nil
	case "basic", "simple":
		return CompactionModeBasic, nil
	case "off", "none", "disabled", "":
		return CompactionModeOff, nil
	}
	return CompactionModeOff, fmt.Errorf("agentloop: unknown CompactionMode %q", s)
}

// CompactionModeConfig bundles a CompactionMode with its budget parameters.
type CompactionModeConfig struct {
	Mode          CompactionMode
	ThresholdPct  float64
	MaxTokens     int
	PreserveLastN int
}

// DefaultCompactionModeConfig returns the production-default configuration:
// agentic mode, 70% threshold, 8000-token budget, 20 verbatim tail turns.
func DefaultCompactionModeConfig() CompactionModeConfig {
	return CompactionModeConfig{
		Mode:          CompactionModeAgentic,
		ThresholdPct:  0.7,
		MaxTokens:     8000,
		PreserveLastN: 20,
	}
}

// ShouldCompact reports whether compaction should fire given used tokens out
// of a max budget. Off mode never compacts; a non-positive max never
// compacts; otherwise compaction fires when usage exceeds ThresholdPct of
// max. A non-positive threshold defaults to 0.7.
func (cfg CompactionModeConfig) ShouldCompact(used, max int) bool {
	if cfg.Mode == CompactionModeOff {
		return false
	}
	if max <= 0 {
		return false
	}
	threshold := cfg.ThresholdPct
	if threshold <= 0 {
		threshold = 0.7
	}
	return float64(used) > float64(max)*threshold
}

// PickStrategy maps the configured mode onto the existing CompactionStrategy
// enum from compaction.go. Agentic maps to Selective (keep tool results,
// drop prose); Basic maps to Hybrid (summarise old + keep recent verbatim);
// Off falls back to the default strategy (it is never invoked because
// ShouldCompact returns false for Off).
func (cfg CompactionModeConfig) PickStrategy(used, max int) CompactionStrategy {
	switch cfg.Mode {
	case CompactionModeAgentic:
		return CompactionSelective
	case CompactionModeBasic:
		return CompactionHybrid
	default:
		return DefaultCompactionStrategy()
	}
}
