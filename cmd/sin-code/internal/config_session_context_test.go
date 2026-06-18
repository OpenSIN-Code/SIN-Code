// SPDX-License-Identifier: MIT
// Purpose: config tests for the session-start context injection flag (issue #379).
package internal

import (
	"strings"
	"testing"
)

func TestConfig_SessionContextEnabled_Default(t *testing.T) {
	cfg := defaultConfig()
	if !cfg.AgentLoopSessionContextEnabled {
		t.Error("expected agentloop.session_context.enabled to default to true")
	}
}

func TestConfig_SessionContextEnabled_GetSet(t *testing.T) {
	cfg := defaultConfig()
	if err := setConfigValueIn("agentloop.session_context.enabled", "false", &cfg); err != nil {
		t.Fatalf("set false: %v", err)
	}
	if cfg.AgentLoopSessionContextEnabled {
		t.Error("expected flag to be false after set")
	}
	got, err := getConfigValueFrom("agentloop.session_context.enabled", cfg)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "false" {
		t.Errorf("expected get to return 'false', got %q", got)
	}
	if err := setConfigValueIn("agentloop.session_context.enabled", "true", &cfg); err != nil {
		t.Fatalf("set true: %v", err)
	}
	if !cfg.AgentLoopSessionContextEnabled {
		t.Error("expected flag to be true after set")
	}
}

func TestConfig_SessionContextEnabled_Render(t *testing.T) {
	cfg := defaultConfig()
	rendered := renderConfigTOML(cfg)
	if !strings.Contains(rendered, "agentloop.session_context.enabled") {
		t.Errorf("rendered config missing agentloop.session_context.enabled:\n%s", rendered)
	}
}
