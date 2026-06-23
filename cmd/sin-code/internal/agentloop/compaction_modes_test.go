// SPDX-License-Identifier: MIT
// Purpose: tests for the closed-set CompactionMode enum (issue #378). The
// struct is dependency-free (M2-compatible) so the 12 tests below pin
// its public contract end-to-end without spinning up a real Compactor.
package agentloop

import (
	"errors"
	"strings"
	"testing"
)

// 1. String() renders the canonical lowercase name for every value;
// unknown ints fall through to "off" so logs stay deterministic.
func TestCompactionMode_String(t *testing.T) {
	cases := []struct {
		m    CompactionMode
		want string
	}{
		{CompactionModeOff, "off"},
		{CompactionModeAgentic, "agentic"},
		{CompactionModeBasic, "basic"},
		{CompactionMode(99), "off"},
	}
	for _, tc := range cases {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("CompactionMode(%d).String(): got %q, want %q", int(tc.m), got, tc.want)
		}
	}
}

// 2. ParseCompactionMode returns the right value for every alias and
// surfaces an error for unknown strings (CLI can warn).
func TestCompactionMode_ParseAliases(t *testing.T) {
	type tc struct {
		in   string
		want CompactionMode
	}
	cases := []tc{
		{"", CompactionModeOff},
		{"off", CompactionModeOff},
		{"OFF", CompactionModeOff},
		{"none", CompactionModeOff},
		{"disabled", CompactionModeOff},
		{"default", CompactionModeOff},
		{"agentic", CompactionModeAgentic},
		{"Agentic", CompactionModeAgentic},
		{"smart", CompactionModeAgentic},
		{"basic", CompactionModeBasic},
		{"keep-last-n", CompactionModeBasic},
	}
	for _, c := range cases {
		got, err := ParseCompactionMode(c.in)
		if err != nil {
			t.Errorf("ParseCompactionMode(%q): unexpected err %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseCompactionMode(%q): got %v, want %v", c.in, got, c.want)
		}
	}
	// Unknown strings MUST error so the CLI can warn without guessing.
	_, err := ParseCompactionMode("nonsense")
	if err == nil {
		t.Error("ParseCompactionMode(\"nonsense\"): want non-nil error")
	}
}

// 3. Agentic ⇒ Hybrid (keeps important context, summarises the middle).
func TestCompactionMode_PickStrategy_Agentic_Hybrid(t *testing.T) {
	cfg := ModeSpecificDefaults(CompactionModeAgentic)
	if got := cfg.PickStrategy(0, 8000); got != CompactionHybrid {
		t.Errorf("agentic PickStrategy: got %v, want Hybrid", got)
	}
}

// 4. Basic ⇒ Truncate (keep last N verbatim, drop the rest).
func TestCompactionMode_PickStrategy_Basic_Truncate(t *testing.T) {
	cfg := ModeSpecificDefaults(CompactionModeBasic)
	if got := cfg.PickStrategy(0, 8000); got != CompactionTruncate {
		t.Errorf("basic PickStrategy: got %v, want Truncate", got)
	}
}

// 5. Off is a documented no-op — ShouldCompact NEVER fires.
func TestCompactionMode_ShouldCompact_Off_AlwaysFalse(t *testing.T) {
	cfg := ModeSpecificDefaults(CompactionModeOff)
	cases := []struct{ used, max int }{
		{0, 8000},
		{7999, 8000},
		{8000, 8000},
		{16000, 8000},
		{1_000_000, 8000},
	}
	for _, c := range cases {
		if cfg.ShouldCompact(c.used, c.max) {
			t.Errorf("off-mode ShouldCompact(%d,%d): want false, got true", c.used, c.max)
		}
	}
}

// 6. Agentic threshold is 70% so ShouldCompact fires strictly above
// 70% of max. Matches the legacy ShouldCompact(>strict) contract in
// compaction.go:157 so the byte-stable snapshot stays consistent.
func TestCompactionMode_ShouldCompact_Agentic_Threshold70(t *testing.T) {
	cfg := ModeSpecificDefaults(CompactionModeAgentic)
	if cfg.ShouldCompact(5600, 8000) {
		t.Errorf("70%% threshold should NOT fire at exactly 70%% (strict >)")
	}
	if !cfg.ShouldCompact(5601, 8000) {
		t.Errorf("70%% threshold SHOULD fire just above 70%% (~70.01%%)")
	}
	if !cfg.ShouldCompact(8000, 8000) {
		t.Errorf("100%% usage SHOULD fire")
	}
}

// 7. Basic threshold is 80% so ShouldCompact fires strictly above
// 80% of max.
func TestCompactionMode_ShouldCompact_Basic_Threshold80(t *testing.T) {
	cfg := ModeSpecificDefaults(CompactionModeBasic)
	if cfg.ShouldCompact(6400, 8000) {
		t.Errorf("80%% threshold should NOT fire at exactly 80%% (strict >)")
	}
	if !cfg.ShouldCompact(6401, 8000) {
		t.Errorf("80%% threshold SHOULD fire just above 80%% (~80.01%%)")
	}
}

// 8. Zero / negative inputs don't divide by zero or go negative.
func TestCompactionMode_ShouldCompact_EdgeInputs(t *testing.T) {
	cfg := ModeSpecificDefaults(CompactionModeAgentic)
	if cfg.ShouldCompact(100, 0) {
		t.Error("max=0 should always return false")
	}
	if cfg.ShouldCompact(-100, 8000) {
		t.Error("negative used should be clamped to 0 → return false")
	}
	// At 100% usage vs 70% threshold, strict-`>` fires.
	if !cfg.ShouldCompact(1, 1) {
		t.Errorf("1/1=100%% vs 70%% threshold: want true (strict >)")
	}
}

// 9. Normalize fills zero-fields with mode-specific defaults AND
// clamps bad inputs so downstream code never has to re-validate.
func TestCompactionModeConfig_Normalize(t *testing.T) {
	// All zero → mode-specific defaults (for Agentic).
	cfg := CompactionModeConfig{Mode: CompactionModeAgentic}
	cfg.Normalize()
	if cfg.ThresholdPct < 0.69 || cfg.ThresholdPct > 0.71 {
		t.Errorf("agentic normalized ThresholdPct: got %.3f, want ~0.70", cfg.ThresholdPct)
	}
	if cfg.MaxTokens != 8000 {
		t.Errorf("agentic normalized MaxTokens: got %d, want 8000", cfg.MaxTokens)
	}
	if cfg.PreserveLastN != 20 {
		t.Errorf("agentic normalized PreserveLastN: got %d, want 20", cfg.PreserveLastN)
	}

	// Mode switching re-defaults blank fields too.
	cfg = CompactionModeConfig{Mode: CompactionModeBasic}
	cfg.Normalize()
	if cfg.ThresholdPct < 0.79 || cfg.ThresholdPct > 0.81 {
		t.Errorf("basic normalized ThresholdPct: got %.3f, want ~0.80", cfg.ThresholdPct)
	}

	// Bad inputs get clamped.
	cfg = CompactionModeConfig{
		Mode:          CompactionModeAgentic,
		ThresholdPct:  2.5,        // way too high
		MaxTokens:     -10,        // negative
		PreserveLastN: -3,         // negative
	}
	cfg.Normalize()
	if cfg.ThresholdPct > 1.0 {
		t.Errorf("ThresholdPct clamp: got %.3f, want <= 1.0", cfg.ThresholdPct)
	}
	if cfg.MaxTokens < 1 {
		t.Errorf("MaxTokens clamp: got %d, want >= 1", cfg.MaxTokens)
	}
	if cfg.PreserveLastN < 1 {
		t.Errorf("PreserveLastN clamp: got %d, want >= 1", cfg.PreserveLastN)
	}
}

// 10. IsLossy mirrors the existing ContextCompactionMode.IsLossy() so
// the TUI / ledger see one shape per mode.
func TestCompactionMode_IsLossy(t *testing.T) {
	if !CompactionModeAgentic.IsLossy() {
		t.Error("agentic should be lossy (Hybrid summarises the middle)")
	}
	if CompactionModeBasic.IsLossy() {
		t.Error("basic should NOT be lossy (Truncate drops but does not rewrite)")
	}
	if CompactionModeOff.IsLossy() {
		t.Error("off should NOT be lossy")
	}
}

// 11. PickStrategy output is byte-stable per (cfg, used, max) pair
// so the eval-comparator can pin golden snapshots (issue #172).
func TestCompactionModeConfig_PickStrategy_ByteStable(t *testing.T) {
	cfg := ModeSpecificDefaults(CompactionModeAgentic)
	cfg.Normalize()
	a := cfg.PickStrategy(5601, 8000)
	b := cfg.PickStrategy(5601, 8000)
	if a != b {
		t.Errorf("agentic PickStrategy not byte-stable: a=%v b=%v", a, b)
	}
	cfg2 := ModeSpecificDefaults(CompactionModeBasic)
	cfg2.Normalize()
	c := cfg2.PickStrategy(6401, 8000)
	d := cfg2.PickStrategy(6401, 8000)
	if c != d {
		t.Errorf("basic PickStrategy not byte-stable: c=%v d=%v", c, d)
	}
	if a == c {
		t.Errorf("agentic and basic should pick DIFFERENT strategies: got %v vs %v", a, c)
	}
}

// 12. Unknown input strings surface an errors.Is-tagable error so
// the CLI surface can hang a wrapping layer without losing fidelity.
func TestCompactionMode_ParseUnknown_IsErrorsCheckable(t *testing.T) {
	_, err := ParseCompactionMode("BOGUS-MODE")
	if err == nil {
		t.Fatal("expected error for unknown input")
	}
	if !strings.Contains(err.Error(), "BOGUS-MODE") {
		t.Errorf("error should mention the bad input: %v", err)
	}
	// Avoid aliasing budget errors (M2 clean types).
	if errors.Is(err, ErrPerTurnBudgetExceeded) {
		t.Error("compaction-mode parse errors must NOT alias budget errors (M2 clean types)")
	}
}
