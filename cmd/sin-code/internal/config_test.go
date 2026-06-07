// SPDX-License-Identifier: MIT
// Purpose: Unit tests for config.go (expanded: roundtrip, list, path, init, multi-value).
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
}

func TestConfig_Path(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
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
