// SPDX-License-Identifier: MIT
// Purpose: roundtrip tests for context-compaction config keys wired
// through cmd/sin-code/internal (the canonical config package).
//
// Verifies that each of the new agentloop.context_compaction /
// agentloop.compaction_trigger / agentloop.context_window / etc. keys
// round-trips through defaultConfig → getConfigValueFrom →
// setConfigValueIn → applyMap, and that validateConfig accepts valid
// values while flagging invalid ones.
//
// File location deviation from the spec: the original mandate placed
// this test at cmd/sin-code/internal/config/config_compaction_test.go,
// but the unexported helpers (defaultConfig, setConfigValueIn, etc.)
// are not package-visible. Keeping the test in package internal keeps it
// colocated with the surface it covers.
package internal

import (
	"strings"
	"testing"
)

// TestCompactionConfigRoundtrip verifies each new compaction-mode key is
// present in defaultConfig and survives a default → set → get roundtrip
// without mutation.
func TestCompactionConfigRoundtrip(t *testing.T) {
	cases := []struct {
		key      string
		wantDef  string
		validSet []string // values that setConfigValueIn must accept
		invalid  []string // values that setConfigValueIn must reject
	}{
		{
			key:      "agentloop.context_compaction",
			wantDef:  "off",
			validSet: []string{"off", "deterministic", "llm", "hybrid"},
			invalid:  []string{"half", "yes", ""},
		},
		{
			key:      "agentloop.compaction_trigger",
			wantDef:  "tokens",
			validSet: []string{"turns", "tokens", "both"},
			invalid:  []string{"off", "99"},
		},
		{
			key:      "agentloop.context_window",
			wantDef:  "0",
			validSet: []string{"0", "4096", "32768"},
			invalid:  []string{"-1"},
		},
		{
			key:      "agentloop.compaction_preserve_evidence",
			wantDef:  "true",
			validSet: []string{"true", "false"},
			invalid:  []string{},
		},
		{
			key:      "agentloop.compaction_recent_turns",
			wantDef:  "4",
			validSet: []string{"1", "4", "12"},
			invalid:  []string{"0", "-3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			cfg := defaultConfig()
			got, err := getConfigValueFrom(tc.key, cfg)
			if err != nil {
				t.Fatalf("getConfigValueFrom(%q): %v", tc.key, err)
			}
			if got != tc.wantDef {
				t.Errorf("default for %q: got %q want %q", tc.key, got, tc.wantDef)
			}
			for _, v := range tc.validSet {
				if err := setConfigValueIn(tc.key, v, &cfg); err != nil {
					t.Errorf("setConfigValueIn(%q, %q) rejected valid value: %v", tc.key, v, err)
				}
				back, err := getConfigValueFrom(tc.key, cfg)
				if err != nil {
					t.Errorf("getConfigValueFrom after set(%q, %q): %v", tc.key, v, err)
				}
				if back != v {
					t.Errorf("roundtrip %q %q→%q→%q", tc.key, v, back, back)
				}
			}
			for _, v := range tc.invalid {
				if err := setConfigValueIn(tc.key, v, &cfg); err == nil {
					t.Errorf("setConfigValueIn(%q, %q) accepted invalid value (it should reject)", tc.key, v)
				}
			}
		})
	}
}

// TestCompactionConfigApplyMap verifies the applyMap path also picks up
// the new keys. applyMap is the entry point for TOML parsing; a key
// missing from its switch silently drops the user's value, so this
// regression test catches that.
func TestCompactionConfigApplyMap(t *testing.T) {
	cfg := defaultConfig()
	applyMap(&cfg, map[string]string{
		"agentloop.context_compaction":            "hybrid",
		"agentloop.compaction_trigger":            "both",
		"agentloop.context_window":                "16384",
		"agentloop.compaction_preserve_evidence":   "false",
		"agentloop.compaction_recent_turns":       "8",
	})
	if cfg.AgentLoopContextCompaction != "hybrid" {
		t.Errorf("applyMap context_compaction: got %q want hybrid", cfg.AgentLoopContextCompaction)
	}
	if cfg.AgentLoopCompactionTrigger != "both" {
		t.Errorf("applyMap compaction_trigger: got %q want both", cfg.AgentLoopCompactionTrigger)
	}
	if cfg.AgentLoopContextWindow != 16384 {
		t.Errorf("applyMap context_window: got %d want 16384", cfg.AgentLoopContextWindow)
	}
	if cfg.AgentLoopCompactionPreserveEvidence {
		t.Errorf("applyMap context_preserve_evidence: should be false")
	}
	if cfg.AgentLoopCompactionRecentTurns != 8 {
		t.Errorf("applyMap context_recent_turns: got %d want 8", cfg.AgentLoopCompactionRecentTurns)
	}
}

// TestCompactionConfigPairs verifies the new keys show up in
// configPairs() — the canonical "config show" formatter. A key missing
// from configPairs would be invisible to operators.
func TestCompactionConfigPairs(t *testing.T) {
	pairs := configPairs(defaultConfig(), false)
	want := []string{
		"agentloop.context_compaction",
		"agentloop.compaction_trigger",
		"agentloop.context_window",
		"agentloop.compaction_preserve_evidence",
		"agentloop.compaction_recent_turns",
	}
	gotSet := make(map[string]struct{}, len(pairs))
	for _, p := range pairs {
		gotSet[p.Key] = struct{}{}
	}
	for _, k := range want {
		if _, ok := gotSet[k]; !ok {
			t.Errorf("configPairs missing key %q (full list: %s)", k, renderPairs(pairs))
		}
	}
}

func renderPairs(pairs []configPair) string {
	var b strings.Builder
	for _, p := range pairs {
		b.WriteString(p.Key + " ")
	}
	return b.String()
}

// TestValidateCompactionConfig checks that validateConfig flags invalid
// values for the new keys.
func TestValidateCompactionConfig(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SinCodeConfig)
		want   string // expected substring in the issue message; empty = no issue
	}{
		{
			name: "valid hybrid",
			mutate: func(c *SinCodeConfig) {
				c.AgentLoopContextCompaction = "hybrid"
				c.AgentLoopCompactionTrigger = "both"
				c.AgentLoopContextWindow = 16384
				c.AgentLoopCompactionRecentTurns = 6
			},
		},
		{
			name: "invalid mode",
			mutate: func(c *SinCodeConfig) {
				c.AgentLoopContextCompaction = "invalid-mode"
			},
			want: "agentloop.context_compaction must be",
		},
		{
			name: "invalid trigger",
			mutate: func(c *SinCodeConfig) {
				c.AgentLoopCompactionTrigger = "tokens-too"
			},
			want: "agentloop.compaction_trigger must be",
		},
		{
			name: "negative context window",
			mutate: func(c *SinCodeConfig) {
				c.AgentLoopContextWindow = -10
			},
			want: "agentloop.context_window must be",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			tc.mutate(&cfg)
			issues := validateConfig(cfg)
			if tc.want == "" {
				for _, iss := range issues {
					if strings.Contains(iss, "context_compaction") ||
						strings.Contains(iss, "compaction_trigger") ||
						strings.Contains(iss, "context_window") ||
						strings.Contains(iss, "compaction_recent_turns") {
						t.Errorf("unexpected compaction issue: %q (all: %v)", iss, issues)
					}
				}
				return
			}
			found := false
			for _, iss := range issues {
				if strings.Contains(iss, tc.want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("validateConfig did not flag %q (issues: %v)", tc.want, issues)
			}
		})
	}
}
