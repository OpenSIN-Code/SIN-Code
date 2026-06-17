// SPDX-License-Identifier: MIT
// Purpose: coverage tests for permission_defaults.go.
package internal

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
)

func TestPermissionDefaultRules(t *testing.T) {
	rules := DefaultPermissionRules()
	if len(rules) == 0 {
		t.Fatal("expected default rules")
	}
	hasRead := false
	hasProfileRender := false
	for _, r := range rules {
		if r.Tool == "sin_read" && r.Policy == "allow" {
			hasRead = true
		}
		// v3.18.0 (issue #175): profile renderer surfaced to agents.
		// The render verb touches per-agent dotdirs so must default
		// to "ask"; show/list/verify are pure reads.
		if r.Tool == "profile__render" && r.Policy == "ask" {
			hasProfileRender = true
		}
	}
	if !hasRead {
		t.Errorf("expected sin_read allow rule, got %+v", rules)
	}
	if !hasProfileRender {
		t.Errorf("expected profile__render ask rule (issue #175), got %+v", rules)
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

// TestAgentProfilesAllowFullCatalog verifies that the bundled agent profiles
// (fireworks, qwen-relay) expose the full SIN tool surface and all registered
// MCP prefixes while leaving destructive SIN builtins at the default "ask"
// tier (issue #249).
func TestAgentProfilesAllowFullCatalog(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	repoRoot := filepath.Join(filepath.Dir(self), "..", "..", "..")
	profiles := []string{
		filepath.Join(repoRoot, "profiles", "fireworks.toml"),
		filepath.Join(repoRoot, "profiles", "qwen-relay.toml"),
	}
	for _, path := range profiles {
		var cfg orchestrator.AgentConfig
		if _, err := toml.DecodeFile(path, &cfg); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		rules := RulesForAgent(cfg)
		eng := permission.New(rules)

		mustAllow := []string{
			"sin_test",
			"sin_git_log",
			"sin_security_scan",
			"websearch__search",
			"browser__navigate",
		}
		for _, tool := range mustAllow {
			if got := eng.Check(tool); got != permission.Allow {
				t.Errorf("%s: %s expected Allow, got %s", path, tool, got)
			}
		}

		mustAsk := []string{
			"sin_bash",
			"sin_git_commit",
			"sin_test_generate",
			"sin_browser_navigate",
		}
		for _, tool := range mustAsk {
			if got := eng.Check(tool); got != permission.Ask {
				t.Errorf("%s: %s expected Ask, got %s", path, tool, got)
			}
		}
	}
}
