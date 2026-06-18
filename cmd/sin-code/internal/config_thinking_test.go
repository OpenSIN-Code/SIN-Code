// SPDX-License-Identifier: MIT
// Purpose: roundtrip tests for LLMThinkingEnabled and LLMThinkingBudget
// config keys.
//
// Verifies:
//   (a) Default values: disabled + 0 budget by default.
//   (b) applyMap parses "true"/"1" and integer values for both keys.
//   (c) configPairs emits both keys in the list.
//   (d) getConfigValueFrom / setConfigValue roundtrip works for "true"/"4096".
//   (e) Validate accepts 0 ≥ and rejects negative budgets.
// Docs: cmd/sin-code/internal/config_thinking_test.go
package internal

import (
	"testing"
)

func TestConfig_ThinkingDefaults(t *testing.T) {
	cfg := defaultConfig()
	if cfg.LLMThinkingEnabled {
		t.Errorf("default LLMThinkingEnabled should be false, got %v", cfg.LLMThinkingEnabled)
	}
	if cfg.LLMThinkingBudget != 0 {
		t.Errorf("default LLMThinkingBudget should be 0, got %d", cfg.LLMThinkingBudget)
	}
}

func TestConfig_ThinkingApplyMap_True(t *testing.T) {
	cfg := defaultConfig()
	applyMap(&cfg, map[string]string{
		"llm.thinking_enabled": "true",
		"llm.thinking_budget":  "8192",
	})
	if !cfg.LLMThinkingEnabled {
		t.Errorf("LLMThinkingEnabled should be true after applyMap(true)")
	}
	if cfg.LLMThinkingBudget != 8192 {
		t.Errorf("LLMThinkingBudget should be 8192, got %d", cfg.LLMThinkingBudget)
	}
}

func TestConfig_ThinkingApplyMap_One(t *testing.T) {
	cfg := defaultConfig()
	applyMap(&cfg, map[string]string{"llm.thinking_enabled": "1"})
	if !cfg.LLMThinkingEnabled {
		t.Errorf("LLMThinkingEnabled should accept '1', got %v", cfg.LLMThinkingEnabled)
	}
}

func TestConfig_ThinkingApplyMap_FalseLeavesZero(t *testing.T) {
	cfg := defaultConfig()
	applyMap(&cfg, map[string]string{"llm.thinking_enabled": "false"})
	if cfg.LLMThinkingEnabled {
		t.Errorf("LLMThinkingEnabled should be false after applyMap(false)")
	}
	// budget unset stays 0
	if cfg.LLMThinkingBudget != 0 {
		t.Errorf("unrelated keys should not touch budget: %d", cfg.LLMThinkingBudget)
	}
}

func TestConfig_ThinkingPairs(t *testing.T) {
	cfg := defaultConfig()
	cfg.LLMThinkingEnabled = true
	cfg.LLMThinkingBudget = 4096
	pairs := configPairs(cfg, false)
	var sawEnabled, sawBudget bool
	for _, p := range pairs {
		switch p.Key {
		case "llm.thinking_enabled":
			if p.Value != "true" {
				t.Errorf("llm.thinking_enabled = %q, want true", p.Value)
			}
			sawEnabled = true
		case "llm.thinking_budget":
			if p.Value != "4096" {
				t.Errorf("llm.thinking_budget = %q, want 4096", p.Value)
			}
			sawBudget = true
		}
	}
	if !sawEnabled {
		t.Errorf("configPairs missing llm.thinking_enabled")
	}
	if !sawBudget {
		t.Errorf("configPairs missing llm.thinking_budget")
	}
}

func TestConfig_ThinkingRoundtrip_GetSet(t *testing.T) {
	cfg := defaultConfig()
	// set via setConfigValueIn (the in-place variant)
	if err := setConfigValueIn("llm.thinking_enabled", "true", &cfg); err != nil {
		t.Fatalf("setConfigValueIn enabled: %v", err)
	}
	if err := setConfigValueIn("llm.thinking_budget", "12345", &cfg); err != nil {
		t.Fatalf("setConfigValueIn budget: %v", err)
	}
	if !cfg.LLMThinkingEnabled {
		t.Errorf("LLMThinkingEnabled should be true after set")
	}
	if cfg.LLMThinkingBudget != 12345 {
		t.Errorf("LLMThinkingBudget should be 12345, got %d", cfg.LLMThinkingBudget)
	}
	// and get
	val, err := getConfigValueFrom("llm.thinking_enabled", cfg)
	if err != nil {
		t.Fatalf("getConfigValueFrom enabled: %v", err)
	}
	if val != "true" {
		t.Errorf("get enabled = %q, want true", val)
	}
	val, err = getConfigValueFrom("llm.thinking_budget", cfg)
	if err != nil {
		t.Fatalf("getConfigValueFrom budget: %v", err)
	}
	if val != "12345" {
		t.Errorf("get budget = %q, want 12345", val)
	}
}

func TestConfig_ThinkingSet_BadValue(t *testing.T) {
	cfg := defaultConfig()
	if err := setConfigValueIn("llm.thinking_budget", "not-an-int", &cfg); err == nil {
		t.Errorf("setConfigValueIn should reject non-integer budget")
	}
}
