// SPDX-License-Identifier: MIT
// Purpose: tests for `sin-code fusion` subcommands (issue #290).
// All tests are read-only: no API calls, no side effects, no DB writes.
package main

import (
	"bytes"
	"strings"
	"testing"
)

// isolateConfig sets HOME to a temp dir so LoadMergedConfig uses defaults
// instead of picking up the developer's real ~/.config/sin/sin-code.toml.
func isolateConfig(t *testing.T) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
}

func TestFusionStatusRuns(t *testing.T) {
	isolateConfig(t)
	cmd := NewFusionCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fusion status: %v", err)
	}
	s := out.String()
	for _, want := range []string{"Enabled", "Verify gate mode", "Tournament mode", "Providers", "Max cost"} {
		if !strings.Contains(s, want) {
			t.Errorf("status output missing %q:\n%s", want, s)
		}
	}
}

func TestFusionConfigShowsAllKeys(t *testing.T) {
	isolateConfig(t)
	cmd := NewFusionCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"config"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fusion config: %v", err)
	}
	s := out.String()
	for _, key := range []string{
		"fusion.enabled",
		"fusion.providers",
		"fusion.max_cost_usd",
		"fusion.min_quorum",
		"fusion.per_provider_timeout_s",
		"fusion.difficulty_gate",
		"fusion.oracle_mode",
	} {
		if !strings.Contains(s, key) {
			t.Errorf("config output missing %q:\n%s", key, s)
		}
	}
	if !strings.Contains(s, "Environment overrides") {
		t.Errorf("config output missing env var section:\n%s", s)
	}
	for _, env := range []string{"SIN_EVALUATOR_MODEL", "SIN_EVALUATOR_BASE_URL", "FIREWORKS_API_KEY"} {
		if !strings.Contains(s, env) {
			t.Errorf("config output missing env var %q:\n%s", env, s)
		}
	}
}

func TestFusionProvidersListsDefaultModels(t *testing.T) {
	isolateConfig(t)
	cmd := NewFusionCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"providers"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fusion providers: %v", err)
	}
	s := out.String()
	for _, name := range []string{
		"minimax-m3",
		"kimi-k2p7-code-fast",
		"kimi-k2p7-code",
		"deepseek-v4-pro",
		"qwen-3p7-plus",
		"glm-5p2",
	} {
		if !strings.Contains(s, name) {
			t.Errorf("providers output missing %q:\n%s", name, s)
		}
	}
	if !strings.Contains(s, "Base URL:") {
		t.Errorf("providers output missing Base URL:\n%s", s)
	}
}

func TestFusionStatusDefaultDisabled(t *testing.T) {
	isolateConfig(t)
	cmd := NewFusionCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fusion status: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "Enabled:              false") {
		t.Errorf("expected fusion disabled by default:\n%s", s)
	}
}

func TestFusionCommandRegistersSubcommands(t *testing.T) {
	cmd := NewFusionCmd()
	subs := map[string]bool{}
	for _, c := range cmd.Commands() {
		subs[c.Name()] = true
	}
	for _, want := range []string{"status", "config", "providers"} {
		if !subs[want] {
			t.Errorf("missing subcommand %q (have %v)", want, subs)
		}
	}
}
