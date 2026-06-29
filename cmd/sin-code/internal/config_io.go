// SPDX-License-Identifier: MIT
// Purpose: config file loading, saving, merging, path resolution, and
// parse helpers for compaction modes and triggers.
package internal

// sin-debt: shrink, upgrade: when a second io-related function is needed, merge into a shared file

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vision"
)

// parseContextCompactionMode validates a context compaction mode string.
// Accepts exact values and common aliases. Returns nil on invalid.
func parseContextCompactionMode(s string) *string {
	valid := map[string]string{
		"off":           "off",
		"none":          "off",
		"default":       "off",
		"deterministic": "deterministic",
		"det":           "deterministic",
		"llm":           "llm",
		"hybrid":        "hybrid",
	}
	if v, ok := valid[strings.ToLower(strings.TrimSpace(s))]; ok {
		return &v
	}
	return nil
}

// parseCompactionTrigger validates a compaction trigger string.
// Accepts exact values and common aliases. Returns nil on invalid.
func parseCompactionTrigger(s string) *string {
	valid := map[string]string{
		"turns":    "turns",
		"messages": "turns",
		"tokens":   "tokens",
		"both":     "both",
		"any":      "both",
	}
	if v, ok := valid[strings.ToLower(strings.TrimSpace(s))]; ok {
		return &v
	}
	return nil
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
// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

// LoadMergedConfig returns the merged user + project configuration. It is
// exported for use by the command layer (issue #248 and others).
func LoadMergedConfig() (SinCodeConfig, error) {
	return loadMergedConfig()
}

// VisionConfigFromEnv returns a vision.Config wired from the merged sin-code
// config plus optional SIN_ANALYSE_IMAGE_* environment overrides. This helper
// lives in the internal package to avoid an import cycle between vision and
// internal (issue #423).
func VisionConfigFromEnv() (vision.Config, error) {
	cfg, err := loadMergedConfig()
	if err != nil {
		return vision.Config{}, fmt.Errorf("load merged config: %w", err)
	}
	return vision.Config{
		BaseURL: firstNonEmpty(os.Getenv("SIN_ANALYSE_IMAGE_BASE_URL"), cfg.LLMBaseURL),
		APIKey:  firstNonEmpty(os.Getenv("SIN_ANALYSE_IMAGE_API_KEY"), cfg.LLMAPIKey),
		Model:   firstNonEmpty(os.Getenv("SIN_ANALYSE_IMAGE_MODEL"), cfg.LLMModel, vision.DefaultVisionModel),
		Prompt:  vision.DefaultPrompt,
		HTTP:    http.DefaultClient,
	}, nil
}

// firstNonEmpty returns the first non-whitespace string in values, or "" if
// all are empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

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

// LLMStyle returns the merged llm.style value, or "default" if the config
// cannot be loaded. Exported so chat / daemon commands can inject the
// same style level into the system prompt without exposing the full
// merged config.
func LLMStyle() string {
	cfg, err := loadMergedConfig()
	if err != nil {
		return "default"
	}
	return cfg.LLMStyle
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

	tmp := path + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, []byte(content), filemode.Default()); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
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

# Agent output verbosity (issue #167):
#   "default"|"verbose" = no ruleset injected (legacy behavior)
#   "normal"            = drop pleasantries + tool narration
#   "terse"             = caveman-`+"`full`"+` analog
#   "ultra"             = caveman-`+"`ultra`"+` analog (tightest valid compression)
llm.style = %q

agent.verify_mode = %q
agent.max_turns = %d
agent.headless = %v
agent.yolo = %v

# Tool-coverage constraints (issue #248): comma-separated tool names.
# Required tools must be invoked before completion; forbidden tools block it.
agentloop.required_tools = %q
agentloop.forbidden_tools = %q

agentloop.auto_lint = %v
agentloop.auto_test = %v
agentloop.auto_lint_timeout = %d
agentloop.auto_test_timeout = %d

permissions.tools_allow = %q
permissions.tools_deny = %q

paths.mcp_config = %q
paths.skills_dir = %q

# Test-First Verify-Loop defaults (RFC-test-automation.md).
# coverage_threshold: minimum coverage percent for sin_quality_gate (0 = disabled)
# mutation_threshold: minimum mutation score for sin_mutation (0 = disabled)
# auto_generate: run sin_test_generate after every sin_write/sin_edit to .go files
# use_llm: let sin_test_generate call the configured LLM to fill realistic test cases
# repair_rounds: max generate→compile→execute→repair iterations when use_llm is true (0 disables)
test.coverage_threshold = %v
test.mutation_threshold = %v
test.auto_generate = %v
test.timeout_seconds = %d
test.use_llm = %v
test.repair_rounds = %d

# Worktree conflict prediction (issue #319).
# conflict_check: off|warn|abort — action when git merge-tree predicts conflicts
# target_branch: integration branch to compare against when creating a worktree
worktree.conflict_check = %q
worktree.target_branch = %q

# Context compaction modes (issue: compaction-modes):
#   "off" (default) = legacy compaction only
#   "deterministic" = deterministic dedupe + byte-budget
#   "llm"           = LLM summarization with byte-preservation
#   "hybrid"        = deterministic dedupe first, then LLM
agentloop.context_compaction = %q
agentloop.compaction_trigger = %q
agentloop.compaction_max_tokens = %d
agentloop.context_window = %d
agentloop.compaction_preserve_evidence = %v
agentloop.compaction_recent_turns = %d

# Session-start context injection (issue #379): injects a markdown preamble
# from open todos, session summaries, and auto-memory at the start of a new
# session. Privacy-first — off by default; opt-in per source via inject_*.
agentloop.session_context.enabled = %v
`, cfg.Theme, cfg.DefaultTimeout, cfg.DefaultFormat, cfg.MCPServerEnabled,
		cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMMaxTokens, cfg.LLMTemperature,
		cfg.LLMStyle,
		cfg.AgentVerifyMode, cfg.AgentMaxTurns, cfg.AgentHeadless, cfg.AgentYolo,
		strings.Join(cfg.AgentLoopRequiredTools, ","), strings.Join(cfg.AgentLoopForbiddenTools, ","),
		cfg.AgentLoopAutoLint, cfg.AgentLoopAutoTest,
		cfg.AgentLoopAutoLintTimeout, cfg.AgentLoopAutoTestTimeout,
		strings.Join(cfg.ToolsAllow, ","), strings.Join(cfg.ToolsDeny, ","),
		cfg.PathsMCPConfig, cfg.PathsSkillsDir,
		cfg.TestCoverageThreshold, cfg.TestMutationThreshold, cfg.TestAutoGenerate, cfg.TestTimeoutSeconds, cfg.TestUseLLM, cfg.TestRepairRounds,
		cfg.WorktreeConflictCheck, cfg.WorktreeTargetBranch,
		cfg.AgentLoopContextCompaction, cfg.AgentLoopCompactionTrigger, cfg.AgentLoopCompactionMaxTokens, cfg.AgentLoopContextWindow,
		cfg.AgentLoopCompactionPreserveEvidence, cfg.AgentLoopCompactionRecentTurns,
		cfg.AgentLoopSessionContextEnabled)
}

func initConfig() error {
	cfg := defaultConfig()
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("✅ Created default configuration at %s\n", userConfigPath())
	fmt.Printf("   Theme: %s\n", cfg.Theme)
	fmt.Printf("   Default timeout: %d seconds\n", cfg.DefaultTimeout)
	fmt.Printf("   Default format: %s\n", cfg.DefaultFormat)
	fmt.Printf("   MCP server enabled: %v\n", cfg.MCPServerEnabled)
	fmt.Printf("   LLM base URL: %s\n", cfg.LLMBaseURL)
	fmt.Printf("   Agent verify mode: %s\n", cfg.AgentVerifyMode)
	fmt.Println()
	fmt.Println("Tip: Use 'sin-code config set theme light' to switch themes.")
	fmt.Println("Tip: Run 'sin-code chat --setup' for the interactive LLM setup wizard.")
	return nil
}
