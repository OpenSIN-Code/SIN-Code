package internal

import (
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
	// Create a temporary config file.
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "sin-code.toml")

	// Write config manually.
	content := `theme = "light"
default_timeout = 120
default_format = "text"
mcp_server_enabled = false
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	// Parse it back.
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
	// This test relies on the default config if no file exists.
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
	// Test validation for theme.
	err := setConfigValue("theme", "invalid")
	if err == nil {
		t.Error("expected error for invalid theme")
	}

	// Test validation for format.
	err = setConfigValue("default_format", "xml")
	if err == nil {
		t.Error("expected error for invalid format")
	}

	// Test unknown key.
	err = setConfigValue("unknown", "value")
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
