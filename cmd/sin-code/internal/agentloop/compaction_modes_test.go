// SPDX-License-Identifier: MIT
// Purpose: unit tests for CompactionMode / CompactionModeConfig (issue #378).
package agentloop

import (
	"testing"
)

func TestCompactionMode_String(t *testing.T) {
	cases := []struct {
		mode CompactionMode
		want string
	}{
		{CompactionModeAgentic, "agentic"},
		{CompactionModeBasic, "basic"},
		{CompactionModeOff, "off"},
		{CompactionMode(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Fatalf("CompactionMode(%d).String() = %q, want %q", c.mode, got, c.want)
		}
	}
}

func TestParseCompactionMode_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want CompactionMode
	}{
		{"agentic", CompactionModeAgentic},
		{"AGENT", CompactionModeAgentic},
		{" smart ", CompactionModeAgentic},
		{"basic", CompactionModeBasic},
		{"SIMPLE", CompactionModeBasic},
		{"off", CompactionModeOff},
		{"none", CompactionModeOff},
		{"disabled", CompactionModeOff},
		{"", CompactionModeOff},
	}
	for _, c := range cases {
		got, err := ParseCompactionMode(c.in)
		if err != nil {
			t.Fatalf("ParseCompactionMode(%q) unexpected err: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseCompactionMode(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseCompactionMode_Invalid(t *testing.T) {
	if _, err := ParseCompactionMode("turbo"); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestDefaultCompactionModeConfig(t *testing.T) {
	cfg := DefaultCompactionModeConfig()
	if cfg.Mode != CompactionModeAgentic {
		t.Fatalf("Mode = %v, want agentic", cfg.Mode)
	}
	if cfg.ThresholdPct != 0.7 {
		t.Fatalf("ThresholdPct = %v, want 0.7", cfg.ThresholdPct)
	}
	if cfg.MaxTokens != 8000 {
		t.Fatalf("MaxTokens = %d, want 8000", cfg.MaxTokens)
	}
	if cfg.PreserveLastN != 20 {
		t.Fatalf("PreserveLastN = %d, want 20", cfg.PreserveLastN)
	}
}

func TestShouldCompact_OffNeverCompacts(t *testing.T) {
	cfg := CompactionModeConfig{Mode: CompactionModeOff, ThresholdPct: 0.7, MaxTokens: 1000}
	if cfg.ShouldCompact(99999, 1000) {
		t.Fatal("off mode must never compact")
	}
}

func TestShouldCompact_Threshold(t *testing.T) {
	cfg := CompactionModeConfig{Mode: CompactionModeBasic, ThresholdPct: 0.7, MaxTokens: 1000}
	if cfg.ShouldCompact(600, 1000) {
		t.Fatal("600/1000 at 0.7 should not compact")
	}
	if !cfg.ShouldCompact(701, 1000) {
		t.Fatal("701/1000 at 0.7 should compact")
	}
	// Non-positive max never compacts.
	if cfg.ShouldCompact(701, 0) {
		t.Fatal("max<=0 should not compact")
	}
	// Non-positive threshold defaults to 0.7.
	cfg2 := CompactionModeConfig{Mode: CompactionModeBasic, ThresholdPct: 0, MaxTokens: 1000}
	if cfg2.ShouldCompact(600, 1000) {
		t.Fatal("default threshold 0.7: 600 should not compact")
	}
	if !cfg2.ShouldCompact(701, 1000) {
		t.Fatal("default threshold 0.7: 701 should compact")
	}
}

func TestPickStrategy_Agentic(t *testing.T) {
	cfg := DefaultCompactionModeConfig()
	if got := cfg.PickStrategy(8000, 10000); got != CompactionSelective {
		t.Fatalf("agentic PickStrategy = %v, want selective", got)
	}
}

func TestPickStrategy_BasicAndOff(t *testing.T) {
	basic := CompactionModeConfig{Mode: CompactionModeBasic}
	if got := basic.PickStrategy(8000, 10000); got != CompactionHybrid {
		t.Fatalf("basic PickStrategy = %v, want hybrid", got)
	}
	off := CompactionModeConfig{Mode: CompactionModeOff}
	if got := off.PickStrategy(8000, 10000); got != DefaultCompactionStrategy() {
		t.Fatalf("off PickStrategy = %v, want default", got)
	}
}
