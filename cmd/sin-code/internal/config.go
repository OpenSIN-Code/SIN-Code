// SPDX-License-Identifier: MIT
// Purpose: unified configuration management for sin-code. Supports user-level
// config (~/.config/sin/sin-code.toml), project-level override
// (./.sin-code/config.toml), deep merge, atomic writes, secret masking,
// and validation.
// Docs: config.doc.md
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// SinCodeConfig is the unified configuration model. Fields are flat with
// namespaced keys (e.g. llm.base_url) for simple TOML-like parsing without
// adding a parser dependency.
type SinCodeConfig struct {
	Theme            string   `toml:"theme"`
	DefaultTimeout   int      `toml:"default_timeout"`
	DefaultFormat    string   `toml:"default_format"`
	MCPServerEnabled bool     `toml:"mcp_server_enabled"`
	LLMBaseURL       string   `toml:"llm.base_url"`
	LLMAPIKey        string   `toml:"llm.api_key"`
	LLMModel         string   `toml:"llm.model"`
	LLMMaxTokens     int      `toml:"llm.max_tokens"`
	LLMTemperature   float64  `toml:"llm.temperature"`
	AgentVerifyMode  string   `toml:"agent.verify_mode"`
	AgentMaxTurns    int      `toml:"agent.max_turns"`
	AgentHeadless    bool     `toml:"agent.headless"`
	AgentYolo        bool     `toml:"agent.yolo"`
	ToolsAllow       []string `toml:"permissions.tools_allow"`
	ToolsDeny        []string `toml:"permissions.tools_deny"`
	PathsMCPConfig   string   `toml:"paths.mcp_config"`
	PathsSkillsDir   string   `toml:"paths.skills_dir"`
}

func defaultConfig() SinCodeConfig {
	return SinCodeConfig{
		Theme:            "dark",
		DefaultTimeout:   60,
		DefaultFormat:    "json",
		MCPServerEnabled: true,
		LLMBaseURL:       "https://integrate.api.nvidia.com/v1",
		LLMAPIKey:        "",
		LLMModel:         "",
		LLMMaxTokens:     8192,
		LLMTemperature:   0.0,
		AgentVerifyMode:  "poc",
		AgentMaxTurns:    80,
		AgentHeadless:    false,
		AgentYolo:        false,
		ToolsAllow:       []string{},
		ToolsDeny:        []string{},
		PathsMCPConfig:   filepath.Join("~", ".sin-code", "mcp.json"),
		PathsSkillsDir:   "",
	}
}

// ConfigCmd is the root `sin-code config` command.
var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "View and manage sin-code configuration",
	Long: `Manage sin-code configuration files and settings.

Configuration files:
  ~/.config/sin/sin-code.toml    User configuration (defaults)
  ./.sin-code/config.toml         Project configuration (overrides user)

Subcommands:
  config init               Create default configuration files
  config show               Show the merged configuration
  config validate           Validate the merged configuration
  config get <key>          Get a configuration value
  config set <key> <value>  Set a configuration value
  config list               List all configuration values
  config path               Show configuration directory path`,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		val, err := getConfigValue(key)
		if err != nil {
			return err
		}
		fmt.Println(val)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value in the user config file",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]
		return setConfigValue(key, value)
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadMergedConfig()
		if err != nil {
			return err
		}
		fmt.Printf("Configuration directory: %s\n", configDir())
		fmt.Printf("Project config:          %s\n", projectConfigPath())
		fmt.Println()
		for _, kv := range configPairs(cfg, true) {
			fmt.Printf("%-24s = %s\n", kv.Key, kv.Value)
		}
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration directory path",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(configDir())
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default configuration files",
	RunE: func(cmd *cobra.Command, args []string) error {
		return initConfig()
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the merged configuration",
	Long:  `Prints the merged user + project configuration. Secrets are masked by default.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		tomlOut, _ := cmd.Flags().GetBool("toml")
		plain, _ := cmd.Flags().GetBool("plain")
		cfg, err := loadMergedConfig()
		if err != nil {
			return err
		}
		mask := !plain
		if jsonOut {
			return showJSON(cfg, mask)
		}
		if tomlOut {
			return showTOML(cfg, mask)
		}
		for _, kv := range configPairs(cfg, mask) {
			fmt.Printf("%-24s = %s\n", kv.Key, kv.Value)
		}
		return nil
	},
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the merged configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadMergedConfig()
		if err != nil {
			return err
		}
		issues := validateConfig(cfg)
		if len(issues) == 0 {
			fmt.Println("✓ Configuration is valid")
			return nil
		}
		fmt.Println("✗ Configuration issues:")
		for _, iss := range issues {
			fmt.Printf("  - %s\n", iss)
		}
		return fmt.Errorf("config validation failed (%d issues)", len(issues))
	},
}

type configPair struct {
	Key   string
	Value string
}

func init() {
	ConfigCmd.AddCommand(configGetCmd)
	ConfigCmd.AddCommand(configSetCmd)
	ConfigCmd.AddCommand(configListCmd)
	ConfigCmd.AddCommand(configPathCmd)
	ConfigCmd.AddCommand(configInitCmd)
	ConfigCmd.AddCommand(configShowCmd)
	ConfigCmd.AddCommand(configValidateCmd)

	configShowCmd.Flags().Bool("json", false, "Output as JSON")
	configShowCmd.Flags().Bool("toml", false, "Output as TOML")
	configShowCmd.Flags().Bool("plain", false, "Do not mask secrets")
}

// ─── Config file paths ────────────────────────────────────────────────────

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "sin")
}

func userConfigPath() string {
	return filepath.Join(configDir(), "sin-code.toml")
}

func projectConfigPath() string {
	return filepath.Join(".", ".sin-code", "config.toml")
}

// ─── Load / save / merge ────────────────────────────────────────────────────

func loadMergedConfig() (SinCodeConfig, error) {
	cfg := defaultConfig()
	user, err := loadConfigFrom(userConfigPath())
	if err != nil {
		return cfg, err
	}
	cfg = mergeConfig(cfg, user.Raw)
	proj, err := loadConfigFrom(projectConfigPath())
	if err != nil && !os.IsNotExist(err) {
		return cfg, err
	}
	if err == nil {
		cfg = mergeConfig(cfg, proj.Raw)
	}
	return cfg, nil
}

func loadConfig() (SinCodeConfig, error) {
	cfr, err := loadConfigFrom(userConfigPath())
	return cfr.Cfg, err
}

type configFileResult struct {
	Cfg SinCodeConfig
	Raw map[string]string
}

func loadConfigFrom(path string) (configFileResult, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return configFileResult{Cfg: cfg, Raw: nil}, nil
		}
		return configFileResult{Cfg: cfg, Raw: nil}, fmt.Errorf("read config %s: %w", path, err)
	}
	m := parseConfigRaw(string(data))
	applyMap(&cfg, m)
	return configFileResult{Cfg: cfg, Raw: m}, nil
}

// parseConfigRaw returns a flat map of key→value from a simple line-based
// config file. Comments start with '#', empty lines are ignored. Values are
// stripped of surrounding quotes; arrays are left as comma-separated text.
func parseConfigRaw(text string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		out[key] = val
	}
	return out
}

func mergeConfig(base SinCodeConfig, override map[string]string) SinCodeConfig {
	// Deep merge: project config overrides user config, and only keys that are
	// actually present in the file take effect. This prevents zero-value booleans
	// from silently disabling settings.
	applyMap(&base, override)
	return base
}

func saveConfig(cfg SinCodeConfig) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	path := userConfigPath()
	content := renderConfigTOML(cfg)

	// Atomic write: write to a temp file in the same directory, then rename.
	// This keeps readers from seeing a half-written file.
	tmp := path + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	fmt.Printf("✅ Saved configuration to %s\n", path)
	return nil
}

func renderConfigTOML(cfg SinCodeConfig) string {
	return fmt.Sprintf(`# sin-code configuration
# Generated by 'sin-code config init'. Edit manually or use 'sin-code config set'.

# TUI theme: "dark" or "light"
theme = %q

# Default timeout for long-running commands (seconds)
default_timeout = %d

# Default output format: "text" or "json"
default_format = %q

# Enable MCP server by default when running 'sin-code serve'
mcp_server_enabled = %v

llm.base_url = %q
llm.api_key = %q
llm.model = %q
llm.max_tokens = %d
llm.temperature = %v

agent.verify_mode = %q
agent.max_turns = %d
agent.headless = %v
agent.yolo = %v

permissions.tools_allow = %q
permissions.tools_deny = %q

paths.mcp_config = %q
paths.skills_dir = %q
`, cfg.Theme, cfg.DefaultTimeout, cfg.DefaultFormat, cfg.MCPServerEnabled,
		cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMMaxTokens, cfg.LLMTemperature,
		cfg.AgentVerifyMode, cfg.AgentMaxTurns, cfg.AgentHeadless, cfg.AgentYolo,
		strings.Join(cfg.ToolsAllow, ","), strings.Join(cfg.ToolsDeny, ","),
		cfg.PathsMCPConfig, cfg.PathsSkillsDir)
}

func initConfig() error {
	cfg := defaultConfig()
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("Created default configuration at %s\n", userConfigPath())
	fmt.Printf("   Theme: %s\n", cfg.Theme)
	fmt.Printf("   Default timeout: %d seconds\n", cfg.DefaultTimeout)
	fmt.Printf("   Default format: %s\n", cfg.DefaultFormat)
	fmt.Printf("   MCP server enabled: %v\n", cfg.MCPServerEnabled)
	fmt.Printf("   LLM base URL: %s\n", cfg.LLMBaseURL)
	fmt.Printf("   Agent verify mode: %s\n", cfg.AgentVerifyMode)
	fmt.Println()
	fmt.Println("Tip: Use 'sin-code config set theme light' to switch themes.")
	return nil
}

// ─── Get / set / pairs ──────────────────────────────────────────────────────

func getConfigValue(key string) (string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return "", err
	}
	return getConfigValueFrom(key, cfg)
}

func getConfigValueFrom(key string, cfg SinCodeConfig) (string, error) {
	switch key {
	case "theme":
		return cfg.Theme, nil
	case "default_timeout":
		return fmt.Sprintf("%d", cfg.DefaultTimeout), nil
	case "default_format":
		return cfg.DefaultFormat, nil
	case "mcp_server_enabled":
		return fmt.Sprintf("%v", cfg.MCPServerEnabled), nil
	case "llm.base_url":
		return cfg.LLMBaseURL, nil
	case "llm.api_key":
		return maskSecret(cfg.LLMAPIKey), nil
	case "llm.model":
		return cfg.LLMModel, nil
	case "llm.max_tokens":
		return fmt.Sprintf("%d", cfg.LLMMaxTokens), nil
	case "llm.temperature":
		return fmt.Sprintf("%v", cfg.LLMTemperature), nil
	case "agent.verify_mode":
		return cfg.AgentVerifyMode, nil
	case "agent.max_turns":
		return fmt.Sprintf("%d", cfg.AgentMaxTurns), nil
	case "agent.headless":
		return fmt.Sprintf("%v", cfg.AgentHeadless), nil
	case "agent.yolo":
		return fmt.Sprintf("%v", cfg.AgentYolo), nil
	case "permissions.tools_allow":
		return strings.Join(cfg.ToolsAllow, ","), nil
	case "permissions.tools_deny":
		return strings.Join(cfg.ToolsDeny, ","), nil
	case "paths.mcp_config":
		return cfg.PathsMCPConfig, nil
	case "paths.skills_dir":
		return cfg.PathsSkillsDir, nil
	default:
		return "", fmt.Errorf("unknown config key: %q", key)
	}
}

func setConfigValue(key, value string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := setConfigValueIn(key, value, &cfg); err != nil {
		return err
	}
	return saveConfig(cfg)
}

func setConfigValueIn(key, value string, cfg *SinCodeConfig) error {
	switch key {
	case "theme":
		if value != "dark" && value != "light" {
			return fmt.Errorf("theme must be 'dark' or 'light', got %q", value)
		}
		cfg.Theme = value
	case "default_timeout":
		v, err := strconv.Atoi(value)
		if err != nil || v <= 0 {
			return fmt.Errorf("default_timeout must be a positive integer, got %q", value)
		}
		cfg.DefaultTimeout = v
	case "default_format":
		if value != "text" && value != "json" {
			return fmt.Errorf("default_format must be 'text' or 'json', got %q", value)
		}
		cfg.DefaultFormat = value
	case "mcp_server_enabled":
		cfg.MCPServerEnabled = value == "true" || value == "1"
	case "llm.base_url":
		cfg.LLMBaseURL = value
	case "llm.api_key":
		cfg.LLMAPIKey = value
	case "llm.model":
		cfg.LLMModel = value
	case "llm.max_tokens":
		v, err := strconv.Atoi(value)
		if err != nil || v <= 0 {
			return fmt.Errorf("llm.max_tokens must be a positive integer, got %q", value)
		}
		cfg.LLMMaxTokens = v
	case "llm.temperature":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || v < 0 || v > 2 {
			return fmt.Errorf("llm.temperature must be between 0 and 2, got %q", value)
		}
		cfg.LLMTemperature = v
	case "agent.verify_mode":
		if value != "off" && value != "poc" && value != "oracle" {
			return fmt.Errorf("agent.verify_mode must be 'off', 'poc', or 'oracle', got %q", value)
		}
		cfg.AgentVerifyMode = value
	case "agent.max_turns":
		v, err := strconv.Atoi(value)
		if err != nil || v <= 0 {
			return fmt.Errorf("agent.max_turns must be a positive integer, got %q", value)
		}
		cfg.AgentMaxTurns = v
	case "agent.headless":
		cfg.AgentHeadless = value == "true" || value == "1"
	case "agent.yolo":
		cfg.AgentYolo = value == "true" || value == "1"
	case "permissions.tools_allow":
		cfg.ToolsAllow = splitList(value)
	case "permissions.tools_deny":
		cfg.ToolsDeny = splitList(value)
	case "paths.mcp_config":
		cfg.PathsMCPConfig = value
	case "paths.skills_dir":
		cfg.PathsSkillsDir = value
	default:
		return fmt.Errorf("unknown config key: %q", key)
	}
	fmt.Printf("✅ Set %s = %q\n", key, value)
	return nil
}

func configPairs(cfg SinCodeConfig, mask bool) []configPair {
	apiKey := cfg.LLMAPIKey
	if mask {
		apiKey = maskSecret(apiKey)
	}
	pairs := []configPair{
		{"theme", cfg.Theme},
		{"default_timeout", fmt.Sprintf("%d", cfg.DefaultTimeout)},
		{"default_format", cfg.DefaultFormat},
		{"mcp_server_enabled", fmt.Sprintf("%v", cfg.MCPServerEnabled)},
		{"llm.base_url", cfg.LLMBaseURL},
		{"llm.api_key", apiKey},
		{"llm.model", cfg.LLMModel},
		{"llm.max_tokens", fmt.Sprintf("%d", cfg.LLMMaxTokens)},
		{"llm.temperature", fmt.Sprintf("%v", cfg.LLMTemperature)},
		{"agent.verify_mode", cfg.AgentVerifyMode},
		{"agent.max_turns", fmt.Sprintf("%d", cfg.AgentMaxTurns)},
		{"agent.headless", fmt.Sprintf("%v", cfg.AgentHeadless)},
		{"agent.yolo", fmt.Sprintf("%v", cfg.AgentYolo)},
		{"permissions.tools_allow", strings.Join(cfg.ToolsAllow, ",")},
		{"permissions.tools_deny", strings.Join(cfg.ToolsDeny, ",")},
		{"paths.mcp_config", cfg.PathsMCPConfig},
		{"paths.skills_dir", cfg.PathsSkillsDir},
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	return pairs
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func splitList(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ─── Show formats ──────────────────────────────────────────────────────────

func showJSON(cfg SinCodeConfig, mask bool) error {
	apiKey := cfg.LLMAPIKey
	if mask {
		apiKey = maskSecret(apiKey)
	}
	out := map[string]any{
		"theme":              cfg.Theme,
		"default_timeout":    cfg.DefaultTimeout,
		"default_format":     cfg.DefaultFormat,
		"mcp_server_enabled": cfg.MCPServerEnabled,
		"llm": map[string]any{
			"base_url":    cfg.LLMBaseURL,
			"api_key":     apiKey,
			"model":       cfg.LLMModel,
			"max_tokens":  cfg.LLMMaxTokens,
			"temperature": cfg.LLMTemperature,
		},
		"agent": map[string]any{
			"verify_mode": cfg.AgentVerifyMode,
			"max_turns":   cfg.AgentMaxTurns,
			"headless":    cfg.AgentHeadless,
			"yolo":        cfg.AgentYolo,
		},
		"permissions": map[string]any{
			"tools_allow": cfg.ToolsAllow,
			"tools_deny":  cfg.ToolsDeny,
		},
		"paths": map[string]any{
			"mcp_config": cfg.PathsMCPConfig,
			"skills_dir": cfg.PathsSkillsDir,
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func showTOML(cfg SinCodeConfig, mask bool) error {
	apiKey := cfg.LLMAPIKey
	if mask {
		apiKey = maskSecret(apiKey)
	}
	fmt.Println(renderConfigTOML(SinCodeConfig{
		Theme: cfg.Theme, DefaultTimeout: cfg.DefaultTimeout, DefaultFormat: cfg.DefaultFormat,
		MCPServerEnabled: cfg.MCPServerEnabled, LLMBaseURL: cfg.LLMBaseURL, LLMAPIKey: apiKey,
		LLMModel: cfg.LLMModel, LLMMaxTokens: cfg.LLMMaxTokens, LLMTemperature: cfg.LLMTemperature,
		AgentVerifyMode: cfg.AgentVerifyMode, AgentMaxTurns: cfg.AgentMaxTurns,
		AgentHeadless: cfg.AgentHeadless, AgentYolo: cfg.AgentYolo,
		ToolsAllow: cfg.ToolsAllow, ToolsDeny: cfg.ToolsDeny,
		PathsMCPConfig: cfg.PathsMCPConfig, PathsSkillsDir: cfg.PathsSkillsDir,
	}))
	return nil
}

// ─── Validation ────────────────────────────────────────────────────────────

func validateConfig(cfg SinCodeConfig) []string {
	var issues []string
	if cfg.Theme != "dark" && cfg.Theme != "light" {
		issues = append(issues, fmt.Sprintf("theme must be 'dark' or 'light', got %q", cfg.Theme))
	}
	if cfg.DefaultTimeout <= 0 {
		issues = append(issues, fmt.Sprintf("default_timeout must be > 0, got %d", cfg.DefaultTimeout))
	}
	if cfg.DefaultFormat != "text" && cfg.DefaultFormat != "json" {
		issues = append(issues, fmt.Sprintf("default_format must be 'text' or 'json', got %q", cfg.DefaultFormat))
	}
	if cfg.LLMMaxTokens <= 0 {
		issues = append(issues, fmt.Sprintf("llm.max_tokens must be > 0, got %d", cfg.LLMMaxTokens))
	}
	if cfg.LLMTemperature < 0 || cfg.LLMTemperature > 2 {
		issues = append(issues, fmt.Sprintf("llm.temperature must be in [0,2], got %v", cfg.LLMTemperature))
	}
	if cfg.AgentVerifyMode != "off" && cfg.AgentVerifyMode != "poc" && cfg.AgentVerifyMode != "oracle" {
		issues = append(issues, fmt.Sprintf("agent.verify_mode must be 'off', 'poc', or 'oracle', got %q", cfg.AgentVerifyMode))
	}
	if cfg.AgentMaxTurns <= 0 {
		issues = append(issues, fmt.Sprintf("agent.max_turns must be > 0, got %d", cfg.AgentMaxTurns))
	}
	return issues
}

func applyMap(cfg *SinCodeConfig, m map[string]string) {
	parseList := func(s string) []string {
		s = strings.Trim(s, "[]")
		return splitList(s)
	}
	for key, val := range m {
		switch key {
		case "theme":
			cfg.Theme = val
		case "default_timeout":
			_, _ = fmt.Sscanf(val, "%d", &cfg.DefaultTimeout)
		case "default_format":
			cfg.DefaultFormat = val
		case "mcp_server_enabled":
			cfg.MCPServerEnabled = val == "true"
		case "llm.base_url":
			cfg.LLMBaseURL = val
		case "llm.api_key":
			cfg.LLMAPIKey = val
		case "llm.model":
			cfg.LLMModel = val
		case "llm.max_tokens":
			_, _ = fmt.Sscanf(val, "%d", &cfg.LLMMaxTokens)
		case "llm.temperature":
			v, _ := strconv.ParseFloat(val, 64)
			cfg.LLMTemperature = v
		case "agent.verify_mode":
			cfg.AgentVerifyMode = val
		case "agent.max_turns":
			_, _ = fmt.Sscanf(val, "%d", &cfg.AgentMaxTurns)
		case "agent.headless":
			cfg.AgentHeadless = val == "true"
		case "agent.yolo":
			cfg.AgentYolo = val == "true"
		case "permissions.tools_allow":
			cfg.ToolsAllow = parseList(val)
		case "permissions.tools_deny":
			cfg.ToolsDeny = parseList(val)
		case "paths.mcp_config":
			cfg.PathsMCPConfig = val
		case "paths.skills_dir":
			cfg.PathsSkillsDir = val
		}
	}
}
