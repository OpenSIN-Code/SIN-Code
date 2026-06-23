// SPDX-License-Identifier: MIT
// Purpose: closed-set compaction modes (issue #378) — agentic | basic | off.
// Matches Cline's three-way switch from a SOTA coding agent. Distinct from
// the existing string-typed ContextCompactionMode (compaction_types.go —
// "off|deterministic|llm|hybrid"): CompactionMode is an int with exactly
// three values and a small config struct that picks one of the existing
// CompactionStrategy values. Both can coexist in the same package because
// the contexts are different — the legacy string-mode enum is wired into
// the byte-stable ctx.compactor (issue #172), this int-mode is the public
// API that the `--compaction-mode` CLI flag and the TUI indicator route
// through. The mapping keeps the Compactor.Compact() / Compact2() entry
// points untouched.
//
// Defaults mirror the documented SOTA values:
//
//	Mode        ThresholdPct  MaxTokens  PreserveLastN  Strategy
//	agentic     0.70          8000       20             CompactionHybrid
//	basic       0.80          8000       20             CompactionTruncate
//	off         1.00          8000       20             CompactionSummarize (doc-only)
//
// Off is a "no-op" mode: it returns (false, _strategy) from ShouldCompact /
// PickStrategy so callers don't fire the compactor. The strategy that
// PickStrategy returns when Mode==Off is documented as a sentinel only;
// the documented contract is "callers MUST gate on cfg.Mode != Off
// BEFORE consulting PickStrategy" — see ShouldCompact below.
package agentloop

import "strings"

// CompactionMode is the public, closed-set enum of compaction modes
// surfaced to operators (issue #378). Three values, never zero — the
// zero value is treated as Off (legacy compatibility).
type CompactionMode int

const (
	// CompactionModeOff disables every proactive compaction call. The
	// loop's legacy issue #278 turn-count heuristic still applies if
	// compactor.Threshold > 0; this mode only blocks the new mode-based
	// path.
	CompactionModeOff CompactionMode = iota
	// CompactionModeAgentic is the SOTA smart mode: it keeps user
	// prompts, tool results, and verification evidence and summarises
	// the middle. Picks CompactionHybrid.
	CompactionModeAgentic
	// CompactionModeBasic drops everything older than the last 20
	// verbatim turns. Picks CompactionTruncate.
	CompactionModeBasic
)

// ParseCompactionMode normalises user input. Aliases are case-folded;
// unknown strings default to Off (legacy carry-over) and surface a
// non-nil error so the CLI can warn. The empty value maps to Off too.
func ParseCompactionMode(s string) (CompactionMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off", "none", "disabled", "default":
		return CompactionModeOff, nil
	case "agentic", "smart":
		return CompactionModeAgentic, nil
	case "basic", "truncate", "keep-last-n":
		return CompactionModeBasic, nil
	}
	return CompactionModeOff, errUnknownCompactionMode(s)
}

// String returns the canonical, lowercase mode name. The zero value
// renders as "off" so log output stays deterministic.
func (m CompactionMode) String() string {
	switch m {
	case CompactionModeAgentic:
		return "agentic"
	case CompactionModeBasic:
		return "basic"
	case CompactionModeOff:
		return "off"
	default:
		return "off"
	}
}

// errUnknownCompactionMode is the canonical error returned when a
// caller passes a string the closed set does not recognise.
var errUnknownCompactionMode = func(s string) error {
	return parseError("agentloop: unknown compaction mode \"" + s + "\"")
}

// parseError is a tiny package-local helper so we can prefix our own
// error type without importing "errors" into the public surface.
type parseError string

func (e parseError) Error() string { return string(e) }

// CompactionModeConfig is the operator-facing shape exposed to the
// chat-loop, the TUI, and the CLI flag (issue #378). All fields are
// validated by Normalize; zero-values are filled with safe defaults so
// downstream code never re-checks.
type CompactionModeConfig struct {
	// Mode selects which compaction mode is currently active. Required.
	Mode CompactionMode
	// ThresholdPct is the fraction of the cap at which compaction fires.
	// Default for Agentic is 0.70, for Basic is 0.80, for Off is 1.00.
	ThresholdPct float64
	// MaxTokens is the effective token cap the compactor tries to fit
	// the trimmed conversation into. Zero defaults to 8000 so the
	// downstream compactor settings stay aligned with the legacy path.
	MaxTokens int
	// PreserveLastN is the number of recent turns the trimmed slice
	// keeps verbatim. Default 20 (matches Cline's basic-mode default).
	PreserveLastN int
}

// DefaultCompactionModeConfig returns the safe defaults for the
// Off mode (matches the legacy single-gate behavior byte-for-byte).
// Callers that opt into Agentic/Basic via the CLI switch should
// call ModeSpecificDefaults instead.
func DefaultCompactionModeConfig() CompactionModeConfig {
	return CompactionModeConfig{
		Mode:          CompactionModeOff,
		ThresholdPct:  1.00,
		MaxTokens:     8000,
		PreserveLastN: 20,
	}
}

// ModeSpecificDefaults returns the SOTA defaulted config for the
// requested mode. Off returns DefaultCompactionModeConfig() for
// exact legacy parity.
func ModeSpecificDefaults(m CompactionMode) CompactionModeConfig {
	cfg := DefaultCompactionModeConfig()
	cfg.Mode = m
	switch m {
	case CompactionModeAgentic:
		cfg.ThresholdPct = 0.70
	case CompactionModeBasic:
		cfg.ThresholdPct = 0.80
	case CompactionModeOff:
		// already 1.00
	}
	return cfg
}

// Normalize fills zero fields with their documented defaults for the
// currently-set Mode. Threshold pct is clamped to [0,1] so a bad user
// value never produces a non-firing or always-firing compactor.
// PreserveLastN is clamped to >= 1 so the compactor always retains at
// least the most recent message. MaxTokens is clamped to >= 1 too.
// Idempotent — safe to call repeatedly.
func (cfg *CompactionModeConfig) Normalize() {
	specific := ModeSpecificDefaults(cfg.Mode)
	if cfg.ThresholdPct <= 0 {
		cfg.ThresholdPct = specific.ThresholdPct
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = specific.MaxTokens
	}
	if cfg.PreserveLastN <= 0 {
		cfg.PreserveLastN = specific.PreserveLastN
	}
	if cfg.ThresholdPct > 1.0 {
		cfg.ThresholdPct = 1.0
	}
	if cfg.PreserveLastN < 1 {
		cfg.PreserveLastN = 1
	}
	if cfg.MaxTokens < 1 {
		cfg.MaxTokens = 1
	}
}

// ShouldCompact reports whether the operator-configured threshold says
// "compact now" given the current usage (used) and the configured cap
// (max). The closed-set contract is:
//
//   - Mode == Off ⇒ always returns false (no proactive compaction).
//   - Mode == Agentic / Basic ⇒ returns (used / max) > ThresholdPct.
//
// A non-positive max is treated as "unknown" and yields false so the
// caller never divides by zero. Negative usage is clamped to zero so
// a buggy provider payload can't drive a false-positive compact.
func (cfg CompactionModeConfig) ShouldCompact(used, max int) bool {
	if cfg.Mode == CompactionModeOff {
		return false
	}
	cfg = enforceNormalized(cfg)
	if max <= 0 {
		return false
	}
	if used < 0 {
		used = 0
	}
	pct := float64(used) / float64(max)
	return pct > cfg.ThresholdPct
}

// PickStrategy returns the CompactionStrategy the compactor should run
// when ShouldCompact is true for the same (used, max) pair. The closed
// set:
//
//   - Mode == Agentic ⇒ CompactionHybrid
//   - Mode == Basic   ⇒ CompactionTruncate
//   - Mode == Off     ⇒ CompactionSummarize (DOC ONLY: callers MUST
//     gate on cfg.Mode != CompactionModeOff BEFORE calling
//     PickStrategy; the legacy CompactionStrategy value is returned for
//     the threshold-log signal but must not be passed to compactor.Compact.)
//
// The output is byte-stable per `(cfg, used, max)` pair so the system
// can pin snapshots in golden tests (issue #172 stability contract).
func (cfg CompactionModeConfig) PickStrategy(used, max int) CompactionStrategy {
	cfg = enforceNormalized(cfg)
	switch cfg.Mode {
	case CompactionModeAgentic:
		return CompactionHybrid
	case CompactionModeBasic:
		return CompactionTruncate
	case CompactionModeOff:
		// Documented sentinel; see comment above. Compactor.Compact must
		// not be invoked in this case — ShouldCompact gates it.
		return CompactionSummarize
	default:
		return CompactionHybrid
	}
}

// IsLossy mirrors the existing ContextCompactionMode.IsLossy() so the
// TUI and ledger see a single shape. Agentic → lossy (Hybrid
// summarises the middle). Basic → lossless (Truncate drops only the
// oldest turns, never rewrites surviving ones). Off → lossless.
func (m CompactionMode) IsLossy() bool {
	return m == CompactionModeAgentic
}

// enforceNormalized returns a copy of cfg with zero fields filled in.
// We never mutate the receiver's Mode here so callers that branched on
// cfg.Mode get the same answer; ShouldCompact / PickStrategy both call
// this to read defaults without surprising the caller.
func enforceNormalized(cfg CompactionModeConfig) CompactionModeConfig {
	if cfg.Mode == 0 {
		cfg.Mode = CompactionModeOff
	}
	cfg.Normalize()
	return cfg
}
