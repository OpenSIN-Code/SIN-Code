// SPDX-License-Identifier: MIT
// Purpose: Unit tests for config.go (expanded: roundtrip, list, path, init, multi-value,
// new config subsystem: show, validate, deep merge, atomic writes, masking, namespaced keys).
// Docs: config.doc.md
package internal

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig_DefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Theme != "dark" {
		t.Errorf("expected theme 'dark', got %q", cfg.Theme)
	}
	if cfg.DefaultTimeout != 60 {
		t.Errorf("expected timeout 60, got %d", cfg.DefaultTimeout)
	}
	if cfg.DefaultFormat != "json" {
		t.Errorf("expected format 'json', got %q", cfg.DefaultFormat)
	}
	if !cfg.MCPServerEnabled {
		t.Error("expected MCP server enabled by default")
	}
	if cfg.LLMMaxTokens != 8192 {
		t.Errorf("expected llm.max_tokens 8192, got %d", cfg.LLMMaxTokens)
	}
	if cfg.AgentVerifyMode != "poc" {
		t.Errorf("expected agent.verify_mode 'poc', got %q", cfg.AgentVerifyMode)
	}
}

func TestConfig_ConfigDir(t *testing.T) {
	dir := configDir()
	if dir == "" {
		t.Fatal("configDir returned empty string")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	if !contains(dir, ".config") {
		t.Errorf("expected path to contain '.config', got %q", dir)
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "sin-code.toml")

	content := `theme = "light"
default_timeout = 120
default_format = "text"
mcp_server_enabled = false
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read temp config: %v", err)
	}

	if !contains(string(data), "light") {
		t.Error("expected config to contain 'light' theme")
	}
	if !contains(string(data), "120") {
		t.Error("expected config to contain timeout 120")
	}
}

func TestConfig_GetConfigValue(t *testing.T) {
	val, err := getConfigValue("theme")
	if err != nil {
		t.Fatalf("getConfigValue(theme): %v", err)
	}
	if val != "dark" && val != "light" {
		t.Errorf("expected theme to be 'dark' or 'light', got %q", val)
	}

	_, err = getConfigValue("unknown_key")
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestConfig_SetConfigValue_Validation(t *testing.T) {
	err := setConfigValue("theme", "invalid")
	if err == nil {
		t.Error("expected error for invalid theme")
	}

	err = setConfigValue("default_format", "xml")
	if err == nil {
		t.Error("expected error for invalid format")
	}

	err = setConfigValue("unknown", "value")
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestConfig_GetSetRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	initCfg := defaultConfig()
	if err := saveConfig(initCfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := setConfigValue("theme", "light"); err != nil {
		t.Fatalf("setConfigValue(theme, light): %v", err)
	}
	val, err := getConfigValue("theme")
	if err != nil {
		t.Fatalf("getConfigValue(theme): %v", err)
	}
	if val != "light" {
		t.Errorf("expected 'light', got %q", val)
	}

	if err := setConfigValue("default_timeout", "30"); err != nil {
		t.Fatalf("setConfigValue(default_timeout, 30): %v", err)
	}
	val, err = getConfigValue("default_timeout")
	if err != nil {
		t.Fatalf("getConfigValue(default_timeout): %v", err)
	}
	if val != "30" {
		t.Errorf("expected '30', got %q", val)
	}

	if err := setConfigValue("default_format", "text"); err != nil {
		t.Fatalf("setConfigValue(default_format, text): %v", err)
	}
	val, err = getConfigValue("default_format")
	if err != nil {
		t.Fatalf("getConfigValue(default_format): %v", err)
	}
	if val != "text" {
		t.Errorf("expected 'text', got %q", val)
	}

	if err := setConfigValue("mcp_server_enabled", "false"); err != nil {
		t.Fatalf("setConfigValue(mcp_server_enabled, false): %v", err)
	}
	val, err = getConfigValue("mcp_server_enabled")
	if err != nil {
		t.Fatalf("getConfigValue(mcp_server_enabled): %v", err)
	}
	if val != "false" {
		t.Errorf("expected 'false', got %q", val)
	}
}

func TestConfig_List(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	err := configListCmd.RunE(configListCmd, []string{})
	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("configListCmd.RunE: %v", err)
	}
	output := buf.String()
	if !contains(output, "Configuration directory") {
		t.Error("expected output to contain 'Configuration directory'")
	}
	if !contains(output, "theme") {
		t.Error("expected output to contain 'theme'")
	}
	if !contains(output, "default_timeout") {
		t.Error("expected output to contain 'default_timeout'")
	}
	if !contains(output, "default_format") {
		t.Error("expected output to contain 'default_format'")
	}
	if !contains(output, "mcp_server_enabled") {
		t.Error("expected output to contain 'mcp_server_enabled'")
	}
	if !contains(output, "llm.base_url") {
		t.Error("expected output to contain 'llm.base_url'")
	}
}

func TestConfig_Path(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	err := configPathCmd.RunE(configPathCmd, []string{})
	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("configPathCmd.RunE: %v", err)
	}
	expected := filepath.Join(tmpDir, ".config", "sin") + "\n"
	got := buf.String()
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestConfig_Init(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	err := configInitCmd.RunE(configInitCmd, []string{})
	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("configInitCmd.RunE: %v", err)
	}
	output := buf.String()
	if !contains(output, "Created default configuration") {
		t.Error("expected init output to mention 'Created default configuration'")
	}

	cfgPath := filepath.Join(tmpDir, ".config", "sin", "sin-code.toml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	content := string(data)
	if !contains(content, "dark") {
		t.Error("expected default config to contain 'dark' theme")
	}
	if !contains(content, "mcp_server_enabled = true") {
		t.Error("expected default config to contain 'mcp_server_enabled = true'")
	}
	if !contains(content, "llm.base_url") {
		t.Error("expected default config to contain 'llm.base_url'")
	}
	if !contains(content, "agent.verify_mode") {
		t.Error("expected default config to contain 'agent.verify_mode'")
	}
}

func TestConfig_MultipleValuesPersistAndReload(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := setConfigValue("theme", "light"); err != nil {
		t.Fatalf("set theme: %v", err)
	}
	if err := setConfigValue("default_timeout", "90"); err != nil {
		t.Fatalf("set timeout: %v", err)
	}
	if err := setConfigValue("default_format", "text"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if err := setConfigValue("mcp_server_enabled", "false"); err != nil {
		t.Fatalf("set mcp: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected theme 'light', got %q", cfg.Theme)
	}
	if cfg.DefaultTimeout != 90 {
		t.Errorf("expected timeout 90, got %d", cfg.DefaultTimeout)
	}
	if cfg.DefaultFormat != "text" {
		t.Errorf("expected format 'text', got %q", cfg.DefaultFormat)
	}
	if cfg.MCPServerEnabled {
		t.Error("expected MCP server disabled")
	}
}

func TestConfig_InvalidKeyGet(t *testing.T) {
	_, err := getConfigValue("foobar")
	if err == nil {
		t.Error("expected error for unknown key in getConfigValue")
	}
	if !contains(fmt.Sprintf("%v", err), "unknown config key") {
		t.Errorf("expected 'unknown config key' in error, got %v", err)
	}
}

func TestConfig_InvalidKeySet(t *testing.T) {
	err := setConfigValue("foobar", "baz")
	if err == nil {
		t.Error("expected error for unknown key in setConfigValue")
	}
	if !contains(fmt.Sprintf("%v", err), "unknown config key") {
		t.Errorf("expected 'unknown config key' in error, got %v", err)
	}
}

func TestConfig_InvalidTimeout(t *testing.T) {
	err := setConfigValue("default_timeout", "notanumber")
	if err == nil {
		t.Error("expected error for non-numeric timeout")
	}
}

func TestConfig_LoadMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig on missing file should not error: %v", err)
	}
	if cfg.Theme != "dark" {
		t.Errorf("expected default theme 'dark', got %q", cfg.Theme)
	}
	if cfg.DefaultTimeout != 60 {
		t.Errorf("expected default timeout 60, got %d", cfg.DefaultTimeout)
	}
}

func TestConfig_LoadWithComments(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `# this is a comment
theme = "light"

# another comment
default_timeout = 45
default_format = "json"
mcp_server_enabled = true
`
	cfgPath := filepath.Join(cfgDir, "sin-code.toml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected theme 'light', got %q", cfg.Theme)
	}
	if cfg.DefaultTimeout != 45 {
		t.Errorf("expected timeout 45, got %d", cfg.DefaultTimeout)
	}
}

func TestConfig_ConfigDirEmptyOnHomeError(t *testing.T) {
	t.Setenv("HOME", "")
	dir := configDir()
	_ = dir
}

func TestConfig_LoadConfigReadError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "sin-code.toml")
	if err := os.WriteFile(cfgPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chmod(cfgPath, 0000)
	defer os.Chmod(cfgPath, 0644)

	_, err := loadConfig()
	if err == nil {
		t.Error("expected error for unreadable config file")
	}
}

func TestConfig_SaveConfigMkdirError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.Chmod(readOnlyDir, 0555)
	defer os.Chmod(readOnlyDir, 0755)

	t.Setenv("HOME", readOnlyDir)
	cfg := defaultConfig()
	err := saveConfig(cfg)
	if err == nil {
		t.Error("expected error for unwritable config dir")
	}
}

func TestConfig_InitConfigSaveError(t *testing.T) {
	readOnlyDir := t.TempDir()
	os.Chmod(readOnlyDir, 0555)
	defer os.Chmod(readOnlyDir, 0755)

	t.Setenv("HOME", readOnlyDir)
	err := initConfig()
	if err == nil {
		t.Error("expected error when saveConfig fails in initConfig")
	}
}

func TestConfig_LoadConfigEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `
theme = "light"

invalid_line_without_equals
default_format = "text"
`
	cfgPath := filepath.Join(cfgDir, "sin-code.toml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected theme 'light', got %q", cfg.Theme)
	}
	if cfg.DefaultFormat != "text" {
		t.Errorf("expected format 'text', got %q", cfg.DefaultFormat)
	}
	if cfg.DefaultTimeout != 60 {
		t.Errorf("expected default timeout 60 for missing key, got %d", cfg.DefaultTimeout)
	}
}

func TestConfig_SetConfigValueSaveError(t *testing.T) {
	readOnlyDir := t.TempDir()
	os.Chmod(readOnlyDir, 0555)
	defer os.Chmod(readOnlyDir, 0755)

	t.Setenv("HOME", readOnlyDir)
	err := setConfigValue("theme", "dark")
	if err == nil {
		t.Error("expected error when saveConfig fails in setConfigValue")
	}
}

func TestConfig_GetConfigAllKeys(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}

	keys := []string{"theme", "default_timeout", "default_format", "mcp_server_enabled"}
	for _, key := range keys {
		val, err := getConfigValue(key)
		if err != nil {
			t.Errorf("getConfigValue(%q) error: %v", key, err)
		}
		if val == "" {
			t.Errorf("expected non-empty value for %q", key)
		}
	}
}

func TestConfig_SetGetMcpEnabledTrue(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}

	if err := setConfigValue("mcp_server_enabled", "true"); err != nil {
		t.Fatalf("setConfigValue(mcp_server_enabled, true): %v", err)
	}
	val, err := getConfigValue("mcp_server_enabled")
	if err != nil {
		t.Fatalf("getConfigValue(mcp_server_enabled): %v", err)
	}
	if val != "true" {
		t.Errorf("expected 'true', got %q", val)
	}
}

func TestConfig_SetConfigValueOutput(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	err := setConfigValue("theme", "dark")
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !contains(out, "Set theme") {
		t.Errorf("expected output to mention 'Set theme', got %q", out)
	}
}

func TestConfig_MaskSecret(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"short", "***"},
		{"my-super-secret-key", "my-s...-key"},
		{"12345678", "***"},
		{"123456789", "1234...6789"},
	}
	for _, c := range cases {
		got := maskSecret(c.in)
		if got != c.want {
			t.Errorf("maskSecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfig_SetGetNewKeys(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}

	if err := setConfigValue("llm.api_key", "sk-1234567890abcdef"); err != nil {
		t.Fatalf("set llm.api_key: %v", err)
	}
	val, err := getConfigValue("llm.api_key")
	if err != nil {
		t.Fatalf("get llm.api_key: %v", err)
	}
	if val != "sk-1...cdef" {
		t.Errorf("expected masked api key, got %q", val)
	}

	if err := setConfigValue("permissions.tools_allow", "sin_*,sckg_*"); err != nil {
		t.Fatalf("set tools_allow: %v", err)
	}
	val, err = getConfigValue("permissions.tools_allow")
	if err != nil {
		t.Fatalf("get tools_allow: %v", err)
	}
	if val != "sin_*,sckg_*" {
		t.Errorf("expected tools_allow list, got %q", val)
	}

	if err := setConfigValue("agent.max_turns", "120"); err != nil {
		t.Fatalf("set agent.max_turns: %v", err)
	}
	val, err = getConfigValue("agent.max_turns")
	if err != nil {
		t.Fatalf("get agent.max_turns: %v", err)
	}
	if val != "120" {
		t.Errorf("expected '120', got %q", val)
	}
}

func TestConfig_ValidateValid(t *testing.T) {
	issues := validateConfig(defaultConfig())
	if len(issues) != 0 {
		t.Errorf("expected valid default config, got issues: %v", issues)
	}
}

func TestConfig_ValidateInvalid(t *testing.T) {
	cfg := defaultConfig()
	cfg.Theme = "blue"
	cfg.DefaultTimeout = -1
	cfg.DefaultFormat = "yaml"
	cfg.LLMMaxTokens = 0
	cfg.LLMTemperature = 3.0
	cfg.AgentVerifyMode = "fast"
	cfg.AgentMaxTurns = -5

	issues := validateConfig(cfg)
	if len(issues) == 0 {
		t.Fatal("expected validation issues")
	}
	want := []string{
		"theme", "default_timeout", "default_format", "llm.max_tokens",
		"llm.temperature", "agent.verify_mode", "agent.max_turns",
	}
	for _, w := range want {
		found := false
		for _, iss := range issues {
			if contains(iss, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected issue containing %q, got %v", w, issues)
		}
	}
}

func TestConfig_DeepMergeProjectOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	user := `theme = "dark"
agent.verify_mode = "poc"
llm.max_tokens = 4096
`
	if err := os.WriteFile(filepath.Join(cfgDir, "sin-code.toml"), []byte(user), 0644); err != nil {
		t.Fatal(err)
	}

	projDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(filepath.Join(projDir, ".sin-code"), 0755); err != nil {
		t.Fatal(err)
	}
	proj := `theme = "light"
agent.max_turns = 50
`
	if err := os.WriteFile(filepath.Join(projDir, ".sin-code", "config.toml"), []byte(proj), 0644); err != nil {
		t.Fatal(err)
	}

	// Change into project directory so project config path resolves.
	oldWd, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(oldWd)

	cfg, err := loadMergedConfig()
	if err != nil {
		t.Fatalf("loadMergedConfig: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected project theme override 'light', got %q", cfg.Theme)
	}
	if cfg.AgentVerifyMode != "poc" {
		t.Errorf("expected user verify_mode 'poc' to remain, got %q", cfg.AgentVerifyMode)
	}
	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("expected user llm.max_tokens 4096 to remain, got %d", cfg.LLMMaxTokens)
	}
	if cfg.AgentMaxTurns != 50 {
		t.Errorf("expected project agent.max_turns 50, got %d", cfg.AgentMaxTurns)
	}
}

func TestConfig_DeepMergeProjectDoesNotUnsetBool(t *testing.T) {
	// Project config that does not mention mcp_server_enabled should not
	// disable the user default.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	user := `theme = "dark"
mcp_server_enabled = true
`
	if err := os.WriteFile(filepath.Join(cfgDir, "sin-code.toml"), []byte(user), 0644); err != nil {
		t.Fatal(err)
	}

	projDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(filepath.Join(projDir, ".sin-code"), 0755); err != nil {
		t.Fatal(err)
	}
	proj := `theme = "light"
`
	if err := os.WriteFile(filepath.Join(projDir, ".sin-code", "config.toml"), []byte(proj), 0644); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(oldWd)

	cfg, err := loadMergedConfig()
	if err != nil {
		t.Fatalf("loadMergedConfig: %v", err)
	}
	if !cfg.MCPServerEnabled {
		t.Error("expected mcp_server_enabled to remain true when not mentioned in project config")
	}
	if cfg.Theme != "light" {
		t.Errorf("expected project theme override, got %q", cfg.Theme)
	}
}

func TestConfig_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "sin-code.toml")
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty config file after atomic save")
	}
	// No tmp files should remain.
	matches, err := filepath.Glob(filepath.Join(cfgDir, "sin-code.toml.tmp*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("expected no leftover tmp files, got %v", matches)
	}
}

func TestConfig_ShowMasked(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.LLMAPIKey = "sk-abcdef1234567890"
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	err := configShowCmd.RunE(configShowCmd, []string{})
	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("configShowCmd: %v", err)
	}
	out := buf.String()
	if contains(out, cfg.LLMAPIKey) {
		t.Errorf("expected api key to be masked, got %q", out)
	}
	if !contains(out, "sk-a...7890") {
		t.Errorf("expected masked api key in output, got %q", out)
	}
}

func TestConfig_ShowPlain(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.LLMAPIKey = "sk-abcdef1234567890"
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	configShowCmd.Flags().Set("plain", "true")
	err := configShowCmd.RunE(configShowCmd, []string{})
	configShowCmd.Flags().Set("plain", "false")
	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("configShowCmd: %v", err)
	}
	out := buf.String()
	if !contains(out, cfg.LLMAPIKey) {
		t.Errorf("expected plain api key in output, got %q", out)
	}
}

func TestConfig_ShowJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	configShowCmd.Flags().Set("json", "true")
	err := configShowCmd.RunE(configShowCmd, []string{})
	configShowCmd.Flags().Set("json", "false")
	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("configShowCmd: %v", err)
	}
	out := buf.String()
	if !contains(out, "\"theme\"") {
		t.Errorf("expected JSON theme key, got %q", out)
	}
	if !contains(out, "\"llm\"") {
		t.Errorf("expected JSON llm section, got %q", out)
	}
}

func TestConfig_ShowTOML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	configShowCmd.Flags().Set("toml", "true")
	err := configShowCmd.RunE(configShowCmd, []string{})
	configShowCmd.Flags().Set("toml", "false")
	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("configShowCmd: %v", err)
	}
	out := buf.String()
	if !contains(out, "llm.base_url") {
		t.Errorf("expected TOML llm.base_url, got %q", out)
	}
}

func TestConfig_ValidateCommand(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	err := configValidateCmd.RunE(configValidateCmd, []string{})
	w.Close()
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("configValidateCmd: %v", err)
	}
	if !contains(buf.String(), "Configuration is valid") {
		t.Errorf("expected valid message, got %q", buf.String())
	}
}

func TestConfig_ValidateCommandInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Theme = "invalid"
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	err := configValidateCmd.RunE(configValidateCmd, []string{})
	if err == nil {
		t.Fatal("expected validation error for invalid theme")
	}
}

func TestConfig_ExpandedRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `theme = "light"
default_timeout = 120
default_format = "text"
mcp_server_enabled = false
llm.base_url = "https://example.com/v1"
llm.api_key = "secret-key"
llm.model = "gpt-4"
llm.max_tokens = 4096
llm.temperature = 0.5
agent.verify_mode = "oracle"
agent.max_turns = 100
agent.headless = true
agent.yolo = true
permissions.tools_allow = "sin_*,sckg_*"
permissions.tools_deny = "dangerous_*"
paths.mcp_config = "./mcp.json"
paths.skills_dir = "./skills"
`
	if err := os.WriteFile(filepath.Join(cfgDir, "sin-code.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected theme light, got %q", cfg.Theme)
	}
	if cfg.DefaultTimeout != 120 {
		t.Errorf("expected timeout 120, got %d", cfg.DefaultTimeout)
	}
	if cfg.LLMBaseURL != "https://example.com/v1" {
		t.Errorf("expected llm base url, got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMAPIKey != "secret-key" {
		t.Errorf("expected api key, got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMModel != "gpt-4" {
		t.Errorf("expected model, got %q", cfg.LLMModel)
	}
	if cfg.LLMMaxTokens != 4096 {
		t.Errorf("expected max tokens 4096, got %d", cfg.LLMMaxTokens)
	}
	if cfg.LLMTemperature != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", cfg.LLMTemperature)
	}
	if cfg.AgentVerifyMode != "oracle" {
		t.Errorf("expected verify oracle, got %q", cfg.AgentVerifyMode)
	}
	if cfg.AgentMaxTurns != 100 {
		t.Errorf("expected max turns 100, got %d", cfg.AgentMaxTurns)
	}
	if !cfg.AgentHeadless {
		t.Error("expected headless true")
	}
	if !cfg.AgentYolo {
		t.Error("expected yolo true")
	}
	if len(cfg.ToolsAllow) != 2 || cfg.ToolsAllow[0] != "sin_*" {
		t.Errorf("expected tools allow, got %v", cfg.ToolsAllow)
	}
	if len(cfg.ToolsDeny) != 1 || cfg.ToolsDeny[0] != "dangerous_*" {
		t.Errorf("expected tools deny, got %v", cfg.ToolsDeny)
	}
	if cfg.PathsMCPConfig != "./mcp.json" {
		t.Errorf("expected mcp config path, got %q", cfg.PathsMCPConfig)
	}
	if cfg.PathsSkillsDir != "./skills" {
		t.Errorf("expected skills dir, got %q", cfg.PathsSkillsDir)
	}
}
