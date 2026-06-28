// SPDX-License-Identifier: MIT
// Purpose: comprehensive tests for the config subpackage, covering Load,
// DeepMerge, Show (masking), Validate, Get, Set, List, and Path.
// All file operations use t.TempDir() — never touches the real config dir.
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

// setupHome creates a temp HOME directory and returns it.
// The caller is responsible for any config file creation inside it.
func setupHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

// setupConfigDir creates the config directory under a temp HOME and returns
// the path to the user config file (sin-code.toml).
func setupConfigDir(t *testing.T) string {
	t.Helper()
	home := setupHome(t)
	cfgDir := filepath.Join(home, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	return filepath.Join(cfgDir, "sin-code.toml")
}

// writeUserConfig writes the given content to the user config file.
func writeUserConfig(t *testing.T, content string) {
	t.Helper()
	path := setupConfigDir(t)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}
}

// contains is a local helper to avoid importing strings just for one call.
func contains(s, sub string) bool { return strings.Contains(s, sub) }

// ─── 1. TestLoad_DefaultPath ────────────────────────────────────────────────

func TestLoad_DefaultPath(t *testing.T) {
	writeUserConfig(t, `theme = "light"
default_timeout = 120
llm.base_url = "https://my-llm.example.com/v1"
`)

	cfg, err := LoadMergedConfig()
	if err != nil {
		t.Fatalf("LoadMergedConfig: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected theme 'light', got %q", cfg.Theme)
	}
	if cfg.DefaultTimeout != 120 {
		t.Errorf("expected default_timeout 120, got %d", cfg.DefaultTimeout)
	}
	if cfg.LLMBaseURL != "https://my-llm.example.com/v1" {
		t.Errorf("expected custom base_url, got %q", cfg.LLMBaseURL)
	}
}

// ─── 2. TestLoad_CustomPath ─────────────────────────────────────────────────

func TestLoad_CustomPath(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "my-config.toml")
	content := `theme = "light"
default_timeout = 45
llm.model = "gpt-4o"
`
	if err := os.WriteFile(custom, []byte(content), 0o644); err != nil {
		t.Fatalf("write custom config: %v", err)
	}

	cfg, err := LoadFromFile(custom)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected theme 'light', got %q", cfg.Theme)
	}
	if cfg.DefaultTimeout != 45 {
		t.Errorf("expected timeout 45, got %d", cfg.DefaultTimeout)
	}
	if cfg.LLMModel != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", cfg.LLMModel)
	}
}

// ─── 3. TestLoad_NoFile ─────────────────────────────────────────────────────

func TestLoad_NoFile(t *testing.T) {
	// Set HOME to a temp dir with no config file.
	setupHome(t)

	cfg, err := LoadMergedConfig()
	if err != nil {
		t.Fatalf("LoadMergedConfig with no file should not error: %v", err)
	}
	// Should return defaults.
	if cfg.Theme != "dark" {
		t.Errorf("expected default theme 'dark', got %q", cfg.Theme)
	}
	if cfg.DefaultTimeout != 60 {
		t.Errorf("expected default timeout 60, got %d", cfg.DefaultTimeout)
	}
	if cfg.LLMMaxTokens != 8192 {
		t.Errorf("expected default max_tokens 8192, got %d", cfg.LLMMaxTokens)
	}
	if cfg.AgentVerifyMode != "poc" {
		t.Errorf("expected default verify_mode 'poc', got %q", cfg.AgentVerifyMode)
	}
}

// ─── 4. TestDeepMerge_UserProject ───────────────────────────────────────────

func TestDeepMerge_UserProject(t *testing.T) {
	home := setupHome(t)
	cfgDir := filepath.Join(home, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userCfg := `theme = "dark"
agent.verify_mode = "poc"
llm.max_tokens = 4096
`
	if err := os.WriteFile(filepath.Join(cfgDir, "sin-code.toml"), []byte(userCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a project directory with a .sin-code/config.toml override.
	projDir := filepath.Join(home, "myproject")
	if err := os.MkdirAll(filepath.Join(projDir, ".sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	projCfg := `theme = "light"
agent.max_turns = 50
`
	if err := os.WriteFile(filepath.Join(projDir, ".sin-code", "config.toml"), []byte(projCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// Chdir into the project so projectConfigPath() resolves correctly.
	oldWd, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(oldWd)

	cfg, err := LoadMergedConfig()
	if err != nil {
		t.Fatalf("LoadMergedConfig: %v", err)
	}

	// Project overrides user.
	if cfg.Theme != "light" {
		t.Errorf("expected project override theme 'light', got %q", cfg.Theme)
	}
	// User-only keys should remain.
	if cfg.AgentVerifyMode != "poc" {
		t.Errorf("expected user verify_mode 'poc' to persist, got %q", cfg.AgentVerifyMode)
	}
	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("expected user llm.max_tokens 4096 to persist, got %d", cfg.LLMMaxTokens)
	}
	// Project-only keys should be present.
	if cfg.AgentMaxTurns != 50 {
		t.Errorf("expected project agent.max_turns 50, got %d", cfg.AgentMaxTurns)
	}
}

// ─── 5. TestDeepMerge_NestedKeys ────────────────────────────────────────────

func TestDeepMerge_NestedKeys(t *testing.T) {
	home := setupHome(t)
	cfgDir := filepath.Join(home, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userCfg := `llm.base_url = "https://user-llm.example.com/v1"
llm.api_key = "user-secret-key"
llm.max_tokens = 4096
`
	if err := os.WriteFile(filepath.Join(cfgDir, "sin-code.toml"), []byte(userCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	projDir := filepath.Join(home, "project")
	if err := os.MkdirAll(filepath.Join(projDir, ".sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Project overrides llm.api_key but NOT llm.base_url or llm.max_tokens.
	projCfg := `llm.api_key = "project-secret-key"
`
	if err := os.WriteFile(filepath.Join(projDir, ".sin-code", "config.toml"), []byte(projCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(oldWd)

	cfg, err := LoadMergedConfig()
	if err != nil {
		t.Fatalf("LoadMergedConfig: %v", err)
	}

	// Project overrides one nested key.
	if cfg.LLMAPIKey != "project-secret-key" {
		t.Errorf("expected project api_key override, got %q", cfg.LLMAPIKey)
	}
	// User keys that project did not mention should remain.
	if cfg.LLMBaseURL != "https://user-llm.example.com/v1" {
		t.Errorf("expected user base_url to persist, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("expected user max_tokens to persist, got %d", cfg.LLMMaxTokens)
	}
}

// ─── 6. TestShow_SecretMasking ──────────────────────────────────────────────

func TestShow_SecretMasking(t *testing.T) {
	cfg := SinCodeConfig{
		LLMAPIKey: "sk-abcdef1234567890",
	}

	pairs := Pairs(cfg, true) // mask = true
	var apiKeyPair string
	for _, p := range pairs {
		if p.Key == "llm.api_key" {
			apiKeyPair = p.Value
		}
	}
	if apiKeyPair == "" {
		t.Fatal("expected llm.api_key in pairs")
	}
	if contains(apiKeyPair, cfg.LLMAPIKey) {
		t.Errorf("expected api_key to be masked, got %q", apiKeyPair)
	}
	if !contains(apiKeyPair, "...") {
		t.Errorf("expected masked api_key with '...', got %q", apiKeyPair)
	}
}

// ─── 7. TestShow_Plain ──────────────────────────────────────────────────────

func TestShow_Plain(t *testing.T) {
	cfg := SinCodeConfig{
		LLMAPIKey: "sk-abcdef1234567890",
	}

	pairs := Pairs(cfg, false) // mask = false (plain)
	var apiKeyPair string
	for _, p := range pairs {
		if p.Key == "llm.api_key" {
			apiKeyPair = p.Value
		}
	}
	if apiKeyPair != cfg.LLMAPIKey {
		t.Errorf("expected plain api_key %q, got %q", cfg.LLMAPIKey, apiKeyPair)
	}
}

// ─── 8. TestValidate_ValidConfig ────────────────────────────────────────────

func TestValidate_ValidConfig(t *testing.T) {
	// Use table-driven approach for multiple valid configs.
	cases := []struct {
		name string
		mut  func(*SinCodeConfig)
	}{
		{"defaults", func(c *SinCodeConfig) {}},
		{"light theme", func(c *SinCodeConfig) { c.Theme = "light" }},
		{"oracle mode", func(c *SinCodeConfig) { c.AgentVerifyMode = "oracle" }},
		{"high max_turns", func(c *SinCodeConfig) { c.AgentMaxTurns = 200 }},
		{"terse style", func(c *SinCodeConfig) { c.LLMStyle = "terse" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mut(&cfg)
			issues := Validate(cfg)
			if len(issues) != 0 {
				t.Errorf("expected no issues, got: %v", issues)
			}
		})
	}
}

// ─── 9. TestValidate_InvalidKey ─────────────────────────────────────────────

func TestValidate_InvalidKey(t *testing.T) {
	// Validate catches invalid enum-like field values.
	cases := []struct {
		name   string
		mut    func(*SinCodeConfig)
		expect string // substring expected in issues
	}{
		{"invalid theme", func(c *SinCodeConfig) { c.Theme = "blue" }, "theme"},
		{"invalid format", func(c *SinCodeConfig) { c.DefaultFormat = "yaml" }, "default_format"},
		{"invalid verify_mode", func(c *SinCodeConfig) { c.AgentVerifyMode = "fast" }, "agent.verify_mode"},
		{"invalid style", func(c *SinCodeConfig) { c.LLMStyle = "loud" }, "llm.style"},
		{"invalid conflict_check", func(c *SinCodeConfig) { c.WorktreeConflictCheck = "yes" }, "worktree.conflict_check"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mut(&cfg)
			issues := Validate(cfg)
			if len(issues) == 0 {
				t.Fatalf("expected validation issues for %s", tc.name)
			}
			found := false
			for _, iss := range issues {
				if contains(iss, tc.expect) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected issue containing %q, got: %v", tc.expect, issues)
			}
		})
	}
}

// ─── 10. TestValidate_InvalidValue ──────────────────────────────────────────

func TestValidate_InvalidValue(t *testing.T) {
	// Validate catches out-of-range numeric values.
	cases := []struct {
		name   string
		mut    func(*SinCodeConfig)
		expect string
	}{
		{"negative timeout", func(c *SinCodeConfig) { c.DefaultTimeout = -1 }, "default_timeout"},
		{"zero max_tokens", func(c *SinCodeConfig) { c.LLMMaxTokens = 0 }, "llm.max_tokens"},
		{"temperature too high", func(c *SinCodeConfig) { c.LLMTemperature = 3.0 }, "llm.temperature"},
		{"negative max_turns", func(c *SinCodeConfig) { c.AgentMaxTurns = -5 }, "agent.max_turns"},
		{"coverage over 100", func(c *SinCodeConfig) { c.TestCoverageThreshold = 150 }, "test.coverage_threshold"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mut(&cfg)
			issues := Validate(cfg)
			if len(issues) == 0 {
				t.Fatalf("expected validation issues for %s", tc.name)
			}
			found := false
			for _, iss := range issues {
				if contains(iss, tc.expect) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected issue containing %q, got: %v", tc.expect, issues)
			}
		})
	}
}

// ─── 11. TestGet_ExistingKey ────────────────────────────────────────────────

func TestGet_ExistingKey(t *testing.T) {
	cfg := SinCodeConfig{
		Theme:          "light",
		DefaultTimeout: 90,
		LLMModel:       "gpt-4o",
		AgentMaxTurns:  50,
	}

	cases := []struct {
		key  string
		want string
	}{
		{"theme", "light"},
		{"default_timeout", "90"},
		{"llm.model", "gpt-4o"},
		{"agent.max_turns", "50"},
		{"mcp_server_enabled", "false"},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			val, err := GetFrom(tc.key, cfg)
			if err != nil {
				t.Fatalf("GetFrom(%q): %v", tc.key, err)
			}
			if val != tc.want {
				t.Errorf("GetFrom(%q) = %q, want %q", tc.key, val, tc.want)
			}
		})
	}
}

// ─── 12. TestGet_NonExistentKey ─────────────────────────────────────────────

func TestGet_NonExistentKey(t *testing.T) {
	cfg := SinCodeConfig{}

	cases := []struct {
		key string
	}{
		{"foobar"},
		{"unknown.key"},
		{"llm.nonexistent"},
		{""},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			_, err := GetFrom(tc.key, cfg)
			if err == nil {
				t.Errorf("expected error for unknown key %q, got nil", tc.key)
			}
			if !contains(err.Error(), "unknown config key") {
				t.Errorf("expected 'unknown config key' in error, got %v", err)
			}
		})
	}
}

// ─── 13. TestSet_NewKey ─────────────────────────────────────────────────────

func TestSet_NewKey(t *testing.T) {
	// Start with a zero-value config and set keys that were at their zero value.
	cfg := Default()

	cases := []struct {
		key, val string
		check    func(c SinCodeConfig) bool
	}{
		{"theme", "light", func(c SinCodeConfig) bool { return c.Theme == "light" }},
		{"default_timeout", "30", func(c SinCodeConfig) bool { return c.DefaultTimeout == 30 }},
		{"llm.model", "claude-3", func(c SinCodeConfig) bool { return c.LLMModel == "claude-3" }},
		{"agent.max_turns", "100", func(c SinCodeConfig) bool { return c.AgentMaxTurns == 100 }},
		{"mcp_server_enabled", "false", func(c SinCodeConfig) bool { return !c.MCPServerEnabled }},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			if err := SetIn(tc.key, tc.val, &cfg); err != nil {
				t.Fatalf("SetIn(%q, %q): %v", tc.key, tc.val, err)
			}
			if !tc.check(cfg) {
				t.Errorf("SetIn(%q, %q) did not update config correctly", tc.key, tc.val)
			}
		})
	}
}

// ─── 14. TestSet_ExistingKey ────────────────────────────────────────────────

func TestSet_ExistingKey(t *testing.T) {
	cfg := Default()

	// First set theme to "light".
	if err := SetIn("theme", "light", &cfg); err != nil {
		t.Fatalf("first SetIn(theme, light): %v", err)
	}
	if cfg.Theme != "light" {
		t.Fatalf("expected 'light', got %q", cfg.Theme)
	}

	// Overwrite with "dark".
	if err := SetIn("theme", "dark", &cfg); err != nil {
		t.Fatalf("second SetIn(theme, dark): %v", err)
	}
	if cfg.Theme != "dark" {
		t.Errorf("expected overwrite to 'dark', got %q", cfg.Theme)
	}

	// Overwrite default_timeout multiple times.
	for _, v := range []string{"10", "20", "30", "120"} {
		if err := SetIn("default_timeout", v, &cfg); err != nil {
			t.Fatalf("SetIn(default_timeout, %s): %v", v, err)
		}
		if got, _ := GetFrom("default_timeout", cfg); got != v {
			t.Errorf("after SetIn(default_timeout, %s), GetFrom returned %q", v, got)
		}
	}
}

// ─── 15. TestList ───────────────────────────────────────────────────────────

func TestList(t *testing.T) {
	cfg := Default()
	pairs := Pairs(cfg, true)

	// Build a map of keys for easy lookup.
	keyMap := make(map[string]string)
	for _, p := range pairs {
		keyMap[p.Key] = p.Value
	}

	// Verify that essential keys are present.
	expectedKeys := []string{
		"theme",
		"default_timeout",
		"default_format",
		"mcp_server_enabled",
		"llm.base_url",
		"llm.api_key",
		"llm.model",
		"llm.max_tokens",
		"llm.temperature",
		"llm.style",
		"agent.verify_mode",
		"agent.max_turns",
		"agentloop.required_tools",
		"agentloop.forbidden_tools",
		"permissions.tools_allow",
		"permissions.tools_deny",
		"paths.mcp_config",
		"test.coverage_threshold",
		"test.timeout_seconds",
		"fusion.enabled",
		"worktree.conflict_check",
	}

	for _, key := range expectedKeys {
		if _, ok := keyMap[key]; !ok {
			t.Errorf("expected key %q in Pairs output, not found", key)
		}
	}

	// Verify values match the default config.
	if keyMap["theme"] != "dark" {
		t.Errorf("expected theme 'dark', got %q", keyMap["theme"])
	}
	if keyMap["default_format"] != "json" {
		t.Errorf("expected default_format 'json', got %q", keyMap["default_format"])
	}
	if keyMap["agent.verify_mode"] != "poc" {
		t.Errorf("expected verify_mode 'poc', got %q", keyMap["agent.verify_mode"])
	}
}

// ─── 16. TestPath ───────────────────────────────────────────────────────────

func TestPath(t *testing.T) {
	dir := setupHome(t)

	got := Path()
	expected := filepath.Join(dir, ".config", "sin")
	if got != expected {
		t.Errorf("Path() = %q, want %q", got, expected)
	}
}

// ─── Extra: TestSetIn_RejectsInvalidValues ──────────────────────────────────

func TestSetIn_RejectsInvalidValues(t *testing.T) {
	cases := []struct {
		key, val, desc string
	}{
		{"theme", "blue", "invalid theme"},
		{"default_format", "xml", "invalid format"},
		{"default_timeout", "notanumber", "non-numeric timeout"},
		{"llm.max_tokens", "0", "zero max_tokens"},
		{"llm.temperature", "5.0", "temperature out of range"},
		{"agent.verify_mode", "fast", "invalid verify_mode"},
		{"llm.style", "loud", "invalid style"},
		{"agent.max_turns", "-1", "negative max_turns"},
		{"worktree.conflict_check", "yes", "invalid conflict_check"},
		{"foobar", "value", "unknown key"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := Default()
			err := SetIn(tc.key, tc.val, &cfg)
			if err == nil {
				t.Errorf("expected error for %s (key=%q val=%q), got nil", tc.desc, tc.key, tc.val)
			}
		})
	}
}

// ─── Extra: TestMaskSecret ──────────────────────────────────────────────────

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"short", "***"},
		{"12345678", "***"},
		{"123456789", "1234...6789"},
		{"sk-abcdef1234567890", "sk-a...7890"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := MaskSecret(tc.in)
			if got != tc.want {
				t.Errorf("MaskSecret(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ─── Extra: TestSet_PersistsToDisk ──────────────────────────────────────────

func TestSet_PersistsToDisk(t *testing.T) {
	// Set up a config dir with a default config.
	path := setupConfigDir(t)
	cfg := Default()
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Set a value (persists to disk).
	if err := Set("theme", "light"); err != nil {
		t.Fatalf("Set(theme, light): %v", err)
	}

	// Verify it persisted by reading the file.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !contains(string(data), "light") {
		t.Errorf("expected 'light' in saved config file, got:\n%s", string(data))
	}

	// Verify via Get.
	val, err := Get("theme")
	if err != nil {
		t.Fatalf("Get(theme): %v", err)
	}
	if val != "light" {
		t.Errorf("expected 'light', got %q", val)
	}
}

// ─── Extra: TestLoadFromFile_NonExistent ────────────────────────────────────

func TestLoadFromFile_NonExistent(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.toml")

	cfg, err := LoadFromFile(missing)
	if err != nil {
		t.Fatalf("LoadFromFile with missing file should not error: %v", err)
	}
	// Should return defaults.
	if cfg.Theme != "dark" {
		t.Errorf("expected default theme 'dark', got %q", cfg.Theme)
	}
}

// ─── Extra: TestPairs_SortedByKey ───────────────────────────────────────────

func TestPairs_SortedByKey(t *testing.T) {
	cfg := Default()
	pairs := Pairs(cfg, true)

	for i := 1; i < len(pairs); i++ {
		if pairs[i-1].Key > pairs[i].Key {
			t.Errorf("Pairs not sorted: %q > %q at index %d", pairs[i-1].Key, pairs[i].Key, i)
		}
	}
}
