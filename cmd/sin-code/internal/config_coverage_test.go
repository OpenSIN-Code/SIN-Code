// SPDX-License-Identifier: MIT
// Purpose: Additional coverage tests for config.go (st-cov1): command error paths,
// load/save edge cases, and full key coverage for get/set helpers.
// Docs: config.doc.md
package internal

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// makeUnreadableConfig creates a user config file that loadConfig cannot read.
func makeUnreadableConfig(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "sin-code.toml")
	if err := os.WriteFile(cfgPath, []byte(`theme = "dark"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(cfgPath, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(cfgPath, 0o644) })
	}
}

func TestConfig_CmdGetSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}
	_ = captureStdout(t)
	err := configGetCmd.RunE(configGetCmd, []string{"theme"})
	if err != nil {
		t.Fatalf("configGetCmd: %v", err)
	}
}

func TestConfig_CmdGetError(t *testing.T) {
	makeUnreadableConfig(t)
	_ = captureStdout(t)
	err := configGetCmd.RunE(configGetCmd, []string{"theme"})
	if err == nil {
		t.Fatal("expected error from configGetCmd when loadConfig fails")
	}
}

func TestConfig_CmdSetSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}
	_ = captureStdout(t)
	err := configSetCmd.RunE(configSetCmd, []string{"theme", "light"})
	if err != nil {
		t.Fatalf("configSetCmd: %v", err)
	}
}

func TestConfig_CmdListError(t *testing.T) {
	makeUnreadableConfig(t)
	_ = captureStdout(t)
	err := configListCmd.RunE(configListCmd, []string{})
	if err == nil {
		t.Fatal("expected error from configListCmd when loadMergedConfig fails")
	}
}

func TestConfig_CmdShowError(t *testing.T) {
	makeUnreadableConfig(t)
	_ = captureStdout(t)
	err := configShowCmd.RunE(configShowCmd, []string{})
	if err == nil {
		t.Fatal("expected error from configShowCmd when loadMergedConfig fails")
	}
}

func TestConfig_CmdValidateError(t *testing.T) {
	makeUnreadableConfig(t)
	_ = captureStdout(t)
	err := configValidateCmd.RunE(configValidateCmd, []string{})
	if err == nil {
		t.Fatal("expected error from configValidateCmd when loadMergedConfig fails")
	}
}

func TestConfig_LoadMergedConfigUserError(t *testing.T) {
	makeUnreadableConfig(t)
	_, err := loadMergedConfig()
	if err == nil {
		t.Fatal("expected loadMergedConfig to error on unreadable user config")
	}
}

func TestConfig_LoadMergedConfigProjectMerge(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "sin-code.toml"), []byte("theme = \"dark\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(filepath.Join(projDir, ".sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, ".sin-code", "config.toml"), []byte("theme = \"light\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWd, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(oldWd)

	cfg, err := loadMergedConfig()
	if err != nil {
		t.Fatalf("loadMergedConfig: %v", err)
	}
	if cfg.Theme != "light" {
		t.Errorf("expected project theme override, got %q", cfg.Theme)
	}
}

func TestConfig_LoadMergedConfigProjectError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based unreadable project config test is Unix-specific")
	}
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "sin-code.toml"), []byte("theme = \"dark\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(filepath.Join(projDir, ".sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	projCfg := filepath.Join(projDir, ".sin-code", "config.toml")
	if err := os.WriteFile(projCfg, []byte("theme = \"light\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(projCfg, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(projCfg, 0o644) })

	oldWd, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(oldWd)

	_, err := loadMergedConfig()
	if err == nil {
		t.Fatal("expected loadMergedConfig to error on unreadable project config")
	}
}

func TestConfig_SetConfigValueLoadError(t *testing.T) {
	makeUnreadableConfig(t)
	_ = captureStdout(t)
	err := setConfigValue("theme", "light")
	if err == nil {
		t.Fatal("expected setConfigValue to error when loadConfig fails")
	}
}

func TestConfig_SaveConfigTempWriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based write test is Unix-specific")
	}
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(cfgDir, 0o755) })
	err := saveConfig(defaultConfig())
	if err == nil {
		t.Fatal("expected error writing temp config")
	}
}

func TestConfig_SaveConfigRenameError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfgDir := filepath.Join(tmpDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a directory at the target path so os.Rename fails.
	cfgPath := filepath.Join(cfgDir, "sin-code.toml")
	if err := os.MkdirAll(cfgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = captureStdout(t)
	err := saveConfig(defaultConfig())
	if err == nil {
		t.Fatal("expected rename error")
	}
}

func TestConfig_GetConfigValueError(t *testing.T) {
	makeUnreadableConfig(t)
	_ = captureStdout(t)
	_, err := getConfigValue("theme")
	if err == nil {
		t.Fatal("expected getConfigValue to error")
	}
}

func TestConfig_GetConfigValueFromAllKeys(t *testing.T) {
	cfg := SinCodeConfig{
		Theme: "dark", DefaultTimeout: 60, DefaultFormat: "json",
		MCPServerEnabled: true, LLMBaseURL: "http://example.com", LLMAPIKey: "sk-secret",
		LLMModel: "m", LLMMaxTokens: 100, LLMTemperature: 0.5,
		AgentVerifyMode: "poc", AgentMaxTurns: 80, AgentHeadless: true, AgentYolo: true,
		ToolsAllow: []string{"a", "b"}, ToolsDeny: []string{"c"},
		PathsMCPConfig: "./mcp.json", PathsSkillsDir: "./skills",
	}
	cases := map[string]string{
		"theme":                   "dark",
		"default_timeout":         "60",
		"default_format":          "json",
		"mcp_server_enabled":      "true",
		"llm.base_url":            "http://example.com",
		"llm.api_key":             "sk-s...cret",
		"llm.model":               "m",
		"llm.max_tokens":          "100",
		"llm.temperature":         "0.5",
		"agent.verify_mode":       "poc",
		"agent.max_turns":         "80",
		"agent.headless":          "true",
		"agent.yolo":              "true",
		"permissions.tools_allow": "a,b",
		"permissions.tools_deny":  "c",
		"paths.mcp_config":        "./mcp.json",
		"paths.skills_dir":        "./skills",
	}
	for key, want := range cases {
		got, err := getConfigValueFrom(key, cfg)
		if err != nil {
			t.Fatalf("getConfigValueFrom(%q): %v", key, err)
		}
		if got != want {
			t.Errorf("getConfigValueFrom(%q) = %q, want %q", key, got, want)
		}
	}
	_, err := getConfigValueFrom("unknown", cfg)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestConfig_SetConfigValueInAllKeys(t *testing.T) {
	cfg := defaultConfig()
	cases := []struct {
		key, val string
		check    func() bool
	}{
		{"theme", "light", func() bool { return cfg.Theme == "light" }},
		{"default_timeout", "120", func() bool { return cfg.DefaultTimeout == 120 }},
		{"default_format", "text", func() bool { return cfg.DefaultFormat == "text" }},
		{"mcp_server_enabled", "true", func() bool { return cfg.MCPServerEnabled }},
		{"llm.base_url", "http://x", func() bool { return cfg.LLMBaseURL == "http://x" }},
		{"llm.api_key", "k", func() bool { return cfg.LLMAPIKey == "k" }},
		{"llm.model", "m", func() bool { return cfg.LLMModel == "m" }},
		{"llm.max_tokens", "100", func() bool { return cfg.LLMMaxTokens == 100 }},
		{"llm.temperature", "0.5", func() bool { return cfg.LLMTemperature == 0.5 }},
		{"agent.verify_mode", "oracle", func() bool { return cfg.AgentVerifyMode == "oracle" }},
		{"agent.max_turns", "100", func() bool { return cfg.AgentMaxTurns == 100 }},
		{"agent.headless", "true", func() bool { return cfg.AgentHeadless }},
		{"agent.yolo", "true", func() bool { return cfg.AgentYolo }},
		{"permissions.tools_allow", "a,b", func() bool { return len(cfg.ToolsAllow) == 2 && cfg.ToolsAllow[0] == "a" && cfg.ToolsAllow[1] == "b" }},
		{"permissions.tools_deny", "c", func() bool { return len(cfg.ToolsDeny) == 1 && cfg.ToolsDeny[0] == "c" }},
		{"paths.mcp_config", "./m.json", func() bool { return cfg.PathsMCPConfig == "./m.json" }},
		{"paths.skills_dir", "./s", func() bool { return cfg.PathsSkillsDir == "./s" }},
	}
	for _, c := range cases {
		if err := setConfigValueIn(c.key, c.val, &cfg); err != nil {
			t.Fatalf("setConfigValueIn(%q, %q): %v", c.key, c.val, err)
		}
		if !c.check() {
			t.Errorf("setConfigValueIn(%q, %q) did not update field", c.key, c.val)
		}
	}
}

func TestConfig_SetConfigValueInErrors(t *testing.T) {
	cfg := defaultConfig()
	cases := []struct {
		key, val string
	}{
		{"theme", "blue"},
		{"default_timeout", "0"},
		{"default_timeout", "x"},
		{"default_format", "xml"},
		{"llm.max_tokens", "0"},
		{"llm.max_tokens", "x"},
		{"llm.temperature", "-1"},
		{"llm.temperature", "x"},
		{"agent.verify_mode", "fast"},
		{"agent.max_turns", "0"},
		{"agent.max_turns", "x"},
		{"unknown", "value"},
	}
	for _, c := range cases {
		err := setConfigValueIn(c.key, c.val, &cfg)
		if err == nil {
			t.Errorf("expected error for setConfigValueIn(%q, %q)", c.key, c.val)
		}
	}
}

func TestConfig_SetConfigValueInTemperatureRange(t *testing.T) {
	cfg := defaultConfig()
	if err := setConfigValueIn("llm.temperature", "2.0", &cfg); err != nil {
		t.Fatalf("expected temperature 2.0 to be valid: %v", err)
	}
	if err := setConfigValueIn("llm.temperature", "2.1", &cfg); err == nil {
		t.Fatal("expected temperature 2.1 to be invalid")
	}
}

func TestConfig_ApplyMapFull(t *testing.T) {
	cfg := defaultConfig()
	m := map[string]string{
		"theme":                   "light",
		"default_timeout":         "120",
		"default_format":          "text",
		"mcp_server_enabled":      "true",
		"llm.base_url":            "http://x",
		"llm.api_key":             "k",
		"llm.model":               "m",
		"llm.max_tokens":          "100",
		"llm.temperature":         "0.5",
		"agent.verify_mode":       "oracle",
		"agent.max_turns":         "100",
		"agent.headless":          "true",
		"agent.yolo":              "true",
		"permissions.tools_allow": "[a,b]",
		"permissions.tools_deny":  "[c]",
		"paths.mcp_config":        "./m.json",
		"paths.skills_dir":        "./s",
	}
	applyMap(&cfg, m)
	if cfg.Theme != "light" || cfg.DefaultTimeout != 120 || cfg.DefaultFormat != "text" || !cfg.MCPServerEnabled {
		t.Errorf("applyMap did not set basic fields: %+v", cfg)
	}
	if cfg.LLMBaseURL != "http://x" || cfg.LLMAPIKey != "k" || cfg.LLMModel != "m" || cfg.LLMMaxTokens != 100 || cfg.LLMTemperature != 0.5 {
		t.Errorf("applyMap did not set LLM fields: %+v", cfg)
	}
	if cfg.AgentVerifyMode != "oracle" || cfg.AgentMaxTurns != 100 || !cfg.AgentHeadless || !cfg.AgentYolo {
		t.Errorf("applyMap did not set agent fields: %+v", cfg)
	}
	if len(cfg.ToolsAllow) != 2 || len(cfg.ToolsDeny) != 1 || cfg.PathsMCPConfig != "./m.json" || cfg.PathsSkillsDir != "./s" {
		t.Errorf("applyMap did not set permission/path fields: %+v", cfg)
	}
}

func TestConfig_SplitList(t *testing.T) {
	if got := splitList(""); len(got) != 0 {
		t.Errorf("splitList(\"\") = %v, want empty", got)
	}
	if got := splitList("a, b ,, c"); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("splitList trimmed unexpectedly: %v", got)
	}
}
