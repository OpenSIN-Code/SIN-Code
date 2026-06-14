// SPDX-License-Identifier: MIT
// Purpose: coverage tests for permission_defaults.go.
package internal

import (
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

func TestPermissionDefaultRules(t *testing.T) {
	rules := DefaultPermissionRules()
	if len(rules) == 0 {
		t.Fatal("expected default rules")
	}
	hasRead := false
	for _, r := range rules {
		if r.Tool == "sin_read" && r.Policy == "allow" {
			hasRead = true
		}
	}
	if !hasRead {
		t.Errorf("expected sin_read allow rule, got %+v", rules)
	}
}

func TestPermissionRulesForAgent(t *testing.T) {
	cfg := orchestrator.AgentConfig{
		ToolsDeny:  []string{"sin_write", "sin_edit"},
		ToolsAllow: []string{"sin_read", "sckg_*"},
	}
	rules := RulesForAgent(cfg)
	if len(rules) < 4 {
		t.Fatalf("expected agent rules, got %d", len(rules))
	}
	// Agent-specific deny/allow rules are prepended before defaults.
	checks := []struct {
		idx    int
		tool   string
		policy string
	}{
		{0, "sin_write", "deny"},
		{1, "sin_edit", "deny"},
		{2, "sin_read", "allow"},
		{3, "sckg_*", "allow"},
	}
	for _, c := range checks {
		if rules[c.idx].Tool != c.tool || rules[c.idx].Policy != c.policy {
			t.Errorf("rule %d: expected %s=%s, got %s=%s",
				c.idx, c.tool, c.policy, rules[c.idx].Tool, rules[c.idx].Policy)
		}
	}
}

func TestPermissionLoadEffectiveAgent(t *testing.T) {
	cfg, source, err := LoadEffectiveAgent("coder")
	if err != nil {
		t.Fatalf("expected default coder agent: %v", err)
	}
	if source != "default" {
		t.Errorf("expected source default, got %q", source)
	}
	if cfg.Name != "coder" {
		t.Errorf("expected name coder, got %q", cfg.Name)
	}

	_, _, err = LoadEffectiveAgent("does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}
