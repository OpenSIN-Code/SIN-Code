package internal

import (
	"testing"
)

func TestConfig_CompactionDefaults(t *testing.T) {
	cfg := defaultConfig()
	if cfg.AgentLoopContextCompaction != "off" {
		t.Errorf("default ContextCompaction should be 'off', got %q", cfg.AgentLoopContextCompaction)
	}
	if cfg.AgentLoopCompactionTrigger != "tokens" {
		t.Errorf("default CompactionTrigger should be 'tokens', got %q", cfg.AgentLoopCompactionTrigger)
	}
	if cfg.AgentLoopCompactionMaxTokens != 8000 {
		t.Errorf("default CompactionMaxTokens should be 8000, got %d", cfg.AgentLoopCompactionMaxTokens)
	}
	if cfg.AgentLoopContextWindow != 0 {
		t.Errorf("default ContextWindow should be 0, got %d", cfg.AgentLoopContextWindow)
	}
	if !cfg.AgentLoopCompactionPreserveEvidence {
		t.Error("default CompactionPreserveEvidence should be true")
	}
	if cfg.AgentLoopCompactionRecentTurns != 4 {
		t.Errorf("default CompactionRecentTurns should be 4, got %d", cfg.AgentLoopCompactionRecentTurns)
	}
}

func TestConfig_CompactionParseContextCompactionMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"off", "off"},
		{"none", "off"},
		{"default", "off"},
		{"deterministic", "deterministic"},
		{"det", "deterministic"},
		{"llm", "llm"},
		{"hybrid", "hybrid"},
		{"", ""},
		{"bogus", ""},
		{"DETERMINISTIC", "deterministic"},
	}
	for _, tc := range tests {
		got := parseContextCompactionMode(tc.input)
		if tc.want == "" {
			if got != nil {
				t.Errorf("parseContextCompactionMode(%q) = %v, want nil", tc.input, got)
			}
		} else if got == nil {
			t.Errorf("parseContextCompactionMode(%q) = nil, want %q", tc.input, tc.want)
		} else if *got != tc.want {
			t.Errorf("parseContextCompactionMode(%q) = %q, want %q", tc.input, *got, tc.want)
		}
	}
}

func TestConfig_CompactionParseCompactionTrigger(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"turns", "turns"},
		{"messages", "turns"},
		{"tokens", "tokens"},
		{"both", "both"},
		{"any", "both"},
		{"", ""},
		{"bogus", ""},
		{"TURNS", "turns"},
		{"TOKENS", "tokens"},
	}
	for _, tc := range tests {
		got := parseCompactionTrigger(tc.input)
		if tc.want == "" {
			if got != nil {
				t.Errorf("parseCompactionTrigger(%q) = %v, want nil", tc.input, got)
			}
		} else if got == nil {
			t.Errorf("parseCompactionTrigger(%q) = nil, want %q", tc.input, tc.want)
		} else if *got != tc.want {
			t.Errorf("parseCompactionTrigger(%q) = %q, want %q", tc.input, *got, tc.want)
		}
	}
}

func TestConfig_CompactionApplyMap(t *testing.T) {
	cfg := defaultConfig()
	applyMap(&cfg, map[string]string{
		"agentloop.context_compaction":      "deterministic",
		"agentloop.compaction_trigger":      "both",
		"agentloop.compaction_max_tokens":   "4096",
		"agentloop.context_window":          "16384",
		"agentloop.compaction_preserve_evidence": "false",
		"agentloop.compaction_recent_turns": "8",
	})
	if cfg.AgentLoopContextCompaction != "deterministic" {
		t.Errorf("ContextCompaction = %q, want deterministic", cfg.AgentLoopContextCompaction)
	}
	if cfg.AgentLoopCompactionTrigger != "both" {
		t.Errorf("CompactionTrigger = %q, want both", cfg.AgentLoopCompactionTrigger)
	}
	if cfg.AgentLoopCompactionMaxTokens != 4096 {
		t.Errorf("CompactionMaxTokens = %d, want 4096", cfg.AgentLoopCompactionMaxTokens)
	}
	if cfg.AgentLoopContextWindow != 16384 {
		t.Errorf("ContextWindow = %d, want 16384", cfg.AgentLoopContextWindow)
	}
	if cfg.AgentLoopCompactionPreserveEvidence {
		t.Error("CompactionPreserveEvidence should be false")
	}
	if cfg.AgentLoopCompactionRecentTurns != 8 {
		t.Errorf("CompactionRecentTurns = %d, want 8", cfg.AgentLoopCompactionRecentTurns)
	}
}

func TestConfig_CompactionPairs(t *testing.T) {
	cfg := defaultConfig()
	cfg.AgentLoopContextCompaction = "llm"
	cfg.AgentLoopCompactionTrigger = "turns"
	cfg.AgentLoopCompactionMaxTokens = 12000
	cfg.AgentLoopContextWindow = 24000
	cfg.AgentLoopCompactionPreserveEvidence = false
	cfg.AgentLoopCompactionRecentTurns = 2
	pairs := configPairs(cfg, false)
	m := make(map[string]string)
	for _, p := range pairs {
		m[p.Key] = p.Value
	}
	if m["agentloop.context_compaction"] != "llm" {
		t.Errorf("context_compaction = %q, want llm", m["agentloop.context_compaction"])
	}
	if m["agentloop.compaction_trigger"] != "turns" {
		t.Errorf("compaction_trigger = %q, want turns", m["agentloop.compaction_trigger"])
	}
	if m["agentloop.compaction_max_tokens"] != "12000" {
		t.Errorf("compaction_max_tokens = %q, want 12000", m["agentloop.compaction_max_tokens"])
	}
	if m["agentloop.context_window"] != "24000" {
		t.Errorf("context_window = %q, want 24000", m["agentloop.context_window"])
	}
	if m["agentloop.compaction_preserve_evidence"] != "false" {
		t.Errorf("compaction_preserve_evidence = %q, want false", m["agentloop.compaction_preserve_evidence"])
	}
	if m["agentloop.compaction_recent_turns"] != "2" {
		t.Errorf("compaction_recent_turns = %q, want 2", m["agentloop.compaction_recent_turns"])
	}
}

func TestConfig_CompactionGetSetRoundtrip(t *testing.T) {
	cfg := defaultConfig()
	err := setConfigValueIn("agentloop.context_compaction", "llm", &cfg)
	if err != nil {
		t.Fatalf("set context_compaction: %v", err)
	}
	got, err := getConfigValueFrom("agentloop.context_compaction", cfg)
	if err != nil {
		t.Fatalf("get context_compaction: %v", err)
	}
	if got != "llm" {
		t.Errorf("get context_compaction = %q, want llm", got)
	}

	err = setConfigValueIn("agentloop.compaction_max_tokens", "9999", &cfg)
	if err != nil {
		t.Fatalf("set compaction_max_tokens: %v", err)
	}
	got, err = getConfigValueFrom("agentloop.compaction_max_tokens", cfg)
	if err != nil {
		t.Fatalf("get compaction_max_tokens: %v", err)
	}
	if got != "9999" {
		t.Errorf("get compaction_max_tokens = %q, want 9999", got)
	}

	err = setConfigValueIn("agentloop.compaction_preserve_evidence", "false", &cfg)
	if err != nil {
		t.Fatalf("set compaction_preserve_evidence: %v", err)
	}
	got, err = getConfigValueFrom("agentloop.compaction_preserve_evidence", cfg)
	if err != nil {
		t.Fatalf("get compaction_preserve_evidence: %v", err)
	}
	if got != "false" {
		t.Errorf("get compaction_preserve_evidence = %q, want false", got)
	}

	err = setConfigValueIn("agentloop.compaction_recent_turns", "2", &cfg)
	if err != nil {
		t.Fatalf("set compaction_recent_turns: %v", err)
	}
	got, err = getConfigValueFrom("agentloop.compaction_recent_turns", cfg)
	if err != nil {
		t.Fatalf("get compaction_recent_turns: %v", err)
	}
	if got != "2" {
		t.Errorf("get compaction_recent_turns = %q, want 2", got)
	}
}

func TestConfig_CompactionValidate(t *testing.T) {
	cfg := defaultConfig()
	errs := validateConfig(cfg)
	for _, e := range errs {
		if e == `agentloop.context_compaction must be 'off', 'deterministic', 'llm', or 'hybrid', got "off"` {
			t.Errorf("off should be valid, got rejection: %s", e)
		}
		if e == `agentloop.compaction_trigger must be 'turns', 'tokens', or 'both', got "tokens"` {
			t.Errorf("tokens should be valid, got rejection: %s", e)
		}
	}
}

func TestConfig_CompactionValidateInvalid(t *testing.T) {
	cfg := defaultConfig()
	cfg.AgentLoopContextCompaction = "bogus"
	errs := validateConfig(cfg)
	found := false
	for _, e := range errs {
		if e == `agentloop.context_compaction must be 'off', 'deterministic', 'llm', or 'hybrid', got "bogus"` {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validateConfig to reject bogus mode, got %v", errs)
	}
}
