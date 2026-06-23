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
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vision"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
)

// isValidStyle reports whether s is one of the legal verbosity levels.
// Empty is rejected here so setConfigValueIn surfaces user error
// clearly; ParseMode in the style package treats empty as default.
func isValidStyle(s string) bool {
	switch s {
	case string(style.ModeDefault), string(style.ModeVerbose),
		string(style.ModeNormal), string(style.ModeTerse), string(style.ModeUltra):
		return true
	}
	return false
}

// SinCodeConfig is the unified configuration model. Fields are flat with
// namespaced keys (e.g. llm.base_url) for simple TOML-like parsing without
// adding a parser dependency.
type SinCodeConfig struct {
	Theme            string  `toml:"theme"`
	DefaultTimeout   int     `toml:"default_timeout"`
	DefaultFormat    string  `toml:"default_format"`
	MCPServerEnabled bool    `toml:"mcp_server_enabled"`
	LLMBaseURL       string  `toml:"llm.base_url"`
	LLMAPIKey        string  `toml:"llm.api_key"`
	LLMModel         string  `toml:"llm.model"`
	LLMMaxTokens     int     `toml:"llm.max_tokens"`
	LLMTemperature   float64 `toml:"llm.temperature"`
	// LLMThinkingEnabled flips the wire-side "thinking" block on per request
	// (Claude / Anthropic-style providers on NIM / OpenRouter gateways);
	// default false. Issue: Thinking Budget Enforcement (first PR).
	LLMThinkingEnabled bool `toml:"llm.thinking_enabled"`
	// LLMThinkingBudget is the per-request reasoning-token cap sent on the
	// wire as thinking.budget_tokens (when LLMThinkingEnabled is true).
	// 0 means "unbounded / provider default". Default 0.
	// Issue: Thinking Budget Enforcement (first PR).
	LLMThinkingBudget int `toml:"llm.thinking_budget"`
	// LLMStyle (issue #167) controls the verbosity mode injected into
	// the agent's system prompt: "default", "verbose", "normal",
	// "terse", "ultra". Empty == "default" == pass-through.
	LLMStyle                string   `toml:"llm.style"`
	AgentVerifyMode         string   `toml:"agent.verify_mode"`
	AgentMaxTurns           int      `toml:"agent.max_turns"`
	AgentHeadless           bool     `toml:"agent.headless"`
	AgentYolo               bool     `toml:"agent.yolo"`
	AgentLoopRequiredTools  []string `toml:"agentloop.required_tools"`
	AgentLoopForbiddenTools []string `toml:"agentloop.forbidden_tools"`
	// AgentLoopAutoLint enables post-edit auto-lint listener (issue #376).
	// Read-only: gofmt -l + go vet on every .go file edited by sin_write/sin_edit.
	// Default false; opt-in only.
	AgentLoopAutoLint bool `toml:"agentloop.auto_lint"`
	// AgentLoopAutoTest enables post-edit auto-test listener (issue #376).
	// go test -count=1 on every *_test.go file; may produce side-effects.
	// Default false; opt-in only.
	AgentLoopAutoTest bool `toml:"agentloop.auto_test"`
	// Per-command timeout cap (seconds). 0 -> 30 lint / 120 test.
	AgentLoopAutoLintTimeout int      `toml:"agentloop.auto_lint_timeout"`
	AgentLoopAutoTestTimeout int      `toml:"agentloop.auto_test_timeout"`
	ToolsAllow               []string `toml:"permissions.tools_allow"`
	ToolsDeny                []string `toml:"permissions.tools_deny"`
	PathsMCPConfig           string   `toml:"paths.mcp_config"`
	PathsSkillsDir           string   `toml:"paths.skills_dir"`
	// Test-First Verify-Loop thresholds (RFC-test-automation.md).
	TestCoverageThreshold float64 `toml:"test.coverage_threshold"`
	TestMutationThreshold float64 `toml:"test.mutation_threshold"`
	TestAutoGenerate      bool    `toml:"test.auto_generate"`
	TestTimeoutSeconds    int     `toml:"test.timeout_seconds"`
	// TestUseLLM, when true, lets sin_test_generate call the configured LLM
	// to fill realistic test cases (otherwise it emits a table-driven
	// scaffold with zero-value tasks). Off by default — privacy/cost.
	TestUseLLM bool `toml:"test.use_llm"`
	// AutoLintEnabled runs a language-appropriate formatter after edits.
	AutoLintEnabled bool `toml:"test.auto_lint"`
	// AutoTestEnabled runs tests after edits.
	AutoTestEnabled bool `toml:"test.auto_test"`
	// TestRepairRounds bounds the generate→compile→execute→repair loop
	// when test.use_llm is true. Default 3; 0 disables repair.
	TestRepairRounds int `toml:"test.repair_rounds"`
	// SIN Fusion v1: verify-tournament config (issue #290).
	FusionEnabled             bool     `toml:"fusion.enabled"`
	FusionProviders           []string `toml:"fusion.providers"`
	FusionMaxCostUSD          float64  `toml:"fusion.max_cost_usd"`
	FusionMinQuorum           int      `toml:"fusion.min_quorum"`
	FusionPerProviderTimeoutS int      `toml:"fusion.per_provider_timeout_s"`
	FusionDifficultyGate      bool     `toml:"fusion.difficulty_gate"`
	FusionOracleMode          bool     `toml:"fusion.oracle_mode"`
	// Memory: autoDream background consolidation + context priming.
	MemoryAutoDream         bool   `toml:"memory.autodream"`
	MemoryAutoDreamInterval string `toml:"memory.autodream_interval"`
	MemoryPrimeOnStart      bool   `toml:"memory.prime_on_start"`
	// Orchestrator: episodic replay of verified plans.
	OrchestratorEpisodicMemory bool `toml:"orchestrator.episodic_memory"`
	// Orchestrator: DeepPlanner produces parallel DAG plans (issue #282).
	OrchestratorDeepPlanner bool `toml:"orchestrator.deep_planner"`
	// Orchestrator: PatternDB learns task sequences from past sessions (issue #288).
	OrchestratorPatternLearning bool `toml:"orchestrator.pattern_learning"`
	// Orchestrator: PreWarmManager pre-warms agents before deps complete (issue #285).
	OrchestratorPreWarm bool `toml:"orchestrator.prewarm"`
	// ChatLazyTools enables lazy tool loading (issue #270). When true,
	// only a tool_search meta-tool is sent initially; the LLM discovers
	// real tools on demand, reducing tool-prompt tokens from ~134K to ~5K.
	// Default false; env SIN_LAZY_TOOLS=1 also enables.
	ChatLazyTools bool `toml:"chat.lazy_tools"`
	// ChatSemanticTools enables offline semantic retrieval for tool_search
	// (issue #364). When true, the LazyToolLoader uses deterministic TF-IDF
	// feature vectors instead of keyword matching. Default false; env
	// SIN_SEMANTIC_TOOLS=1 also enables.
	ChatSemanticTools bool `toml:"chat.semantic_tools"`
	// LLMPromptCache enables TTL-based prompt prefix caching (issue #277).
	// When true, a PromptCache is created and passed to the provider
	// adapter. The adapter only uses it for Anthropic/Claude models
	// (SupportsCaching). Default true.
	LLMPromptCache bool `toml:"llm.prompt_cache"`
	// AgentLoopCompactionStrategy controls context compaction (issue #278).
	// "off" (default) disables compaction; "summarize"|"truncate"|
	// "selective"|"sliding"|"sliding-window"|"hybrid" enables it.
	AgentLoopCompactionStrategy string `toml:"agentloop.compaction_strategy"`
	// AgentLoopCompactionThreshold is the fraction of maxTurns at which
	// compaction triggers (default 0.8 = 80%).
	AgentLoopCompactionThreshold float64 `toml:"agentloop.compaction_threshold"`
	// AgentLoopFrustrationDetection enables user frustration tracking
	// (issue #271). When true, the loop appends an adaptive system-prompt
	// suffix when frustration is detected. Default false.
	AgentLoopFrustrationDetection bool `toml:"agentloop.frustration_detection"`
	// AgentLoopInjectLessons gates lesson briefings in the session-
	// context block injected as the first user message of every run
	// (issue #379). Default false — privacy-first, opt-in only.
	AgentLoopInjectLessons bool `toml:"agentloop.inject_lessons"`
	// AgentLoopInjectMemory gates long-term memory entries in the
	// session-context block (issue #379). Default false.
	AgentLoopInjectMemory bool `toml:"agentloop.inject_memory"`
	// AgentLoopInjectGoals gates pending autonomous-goals rows in
	// the session-context block (issue #379). Default false.
	AgentLoopInjectGoals bool `toml:"agentloop.inject_goals"`
	// AgentLoopContextTopK bounds per-source entries pulled into the
	// session-context block (issue #379). Default 5; values <1 fall
	// back to 5 inside the injector.
	AgentLoopContextTopK int `toml:"agentloop.context_top_k"`
	// Permission: YOLO risk threshold (issue #272).
	PermissionYoloRiskThreshold string `toml:"permission.yolo_risk_threshold"`
	// AgentLoopObserverWindow is the rolling-history size used by
	// the LoopDetector (issue #377). 0 disables detection entirely;
	// any negative value is also treated as 0. Default 20.
	AgentLoopObserverWindow int `toml:"agentloop.observer_window"`
	// AgentLoopObserverMinRepeats is the minimum repeat count
	// required to trip the LoopDetector. Default 2.
	AgentLoopObserverMinRepeats int `toml:"agentloop.observer_min_repeats"`
	// AgentLoopObserverMinPatternLength is the minimum repeating
	// pattern length the LoopDetector will consider. Default 3.
	AgentLoopObserverMinPatternLength int `toml:"agentloop.observer_min_pattern_length"`
	// Worktree conflict prediction (issue #319).
	WorktreeConflictCheck string `toml:"worktree.conflict_check"`
	WorktreeTargetBranch  string `toml:"worktree.target_branch"`

	// ContextCompactionMode selects the compaction algorithm (issue: compaction-modes).
	// off | deterministic | llm | hybrid. Empty or off = legacy behaviour.
	AgentLoopContextCompaction string `toml:"agentloop.context_compaction"`

	// CompactionTrigger decides when the compactor fires per turn.
	// turns | tokens | both. Default tokens.
	AgentLoopCompactionTrigger string `toml:"agentloop.compaction_trigger"`

	// CompactionMaxTokens is the token budget for compacted messages. Default 8000.
	AgentLoopCompactionMaxTokens int `toml:"agentloop.compaction_max_tokens"`

	// ContextWindow is the effective token cap for compaction. 0 = auto.
	AgentLoopContextWindow int `toml:"agentloop.context_window"`

	// CompactionPreserveEvidence enables evidence-preserving retain rules (M3). Default true.
	AgentLoopCompactionPreserveEvidence bool `toml:"agentloop.compaction_preserve_evidence"`

	// CompactionRecentTurns is the number of recent human turns to retain. Default 4.
	AgentLoopCompactionRecentTurns int `toml:"agentloop.compaction_recent_turns"`

	// AutonomyContainerEnabled runs verify commands inside a container (issue #389).
	AutonomyContainerEnabled bool   `toml:"autonomy.container.enabled"`
	AutonomyContainerImage   string `toml:"autonomy.container.image"`

	// AgentLoopSessionContextEnabled injects a unified preamble at session
	// start (issue #379). Default true.
	AgentLoopSessionContextEnabled bool `toml:"agentloop.session_context.enabled"`

	// OutputProgress enables structured NDJSON progress output for headless
	// mode (sin-code chat -p, sin-code daemon). "off" (default) disables it;
	// "json" emits NDJSON events to OutputProgressDest.
	OutputProgress string `toml:"output.progress"`

	// OutputProgressDest selects where progress events go: stderr (default),
	// stdout, or file.
	OutputProgressDest string `toml:"output.progress_dest"`

	// OutputProgressFile is the path used when OutputProgressDest == "file".
	OutputProgressFile string `toml:"output.progress_file"`
}

func defaultConfig() SinCodeConfig {
	return SinCodeConfig{
		Theme:                               "dark",
		DefaultTimeout:                      60,
		DefaultFormat:                       "json",
		MCPServerEnabled:                    true,
		LLMBaseURL:                          "https://integrate.api.nvidia.com/v1",
		LLMAPIKey:                           "",
		LLMModel:                            "",
		LLMMaxTokens:                        8192,
		LLMTemperature:                      0.0,
		LLMThinkingEnabled:                  false,
		LLMThinkingBudget:                   0,
		LLMStyle:                            "default",
		AgentVerifyMode:                     "poc",
		AgentMaxTurns:                       80,
		AgentHeadless:                       false,
		AgentYolo:                           false,
		AgentLoopRequiredTools:              []string{},
		AgentLoopForbiddenTools:             []string{},
		AgentLoopAutoLint:                   false,
		AgentLoopAutoTest:                   false,
		AgentLoopAutoLintTimeout:            30,
		AgentLoopAutoTestTimeout:            120,
		ToolsAllow:                          []string{},
		ToolsDeny:                           []string{},
		PathsMCPConfig:                      filepath.Join("~", ".sin-code", "mcp.json"),
		PathsSkillsDir:                      "",
		TestCoverageThreshold:               0.0,
		TestMutationThreshold:               0.0,
		TestAutoGenerate:                    false,
		TestTimeoutSeconds:                  300,
		TestUseLLM:                          false,
		TestRepairRounds:                    3,
		ChatLazyTools:                       false,
		LLMPromptCache:                      true,
		AgentLoopCompactionStrategy:         "",
		AgentLoopCompactionThreshold:        0.8,
		AgentLoopContextCompaction:          "off",
		AgentLoopCompactionTrigger:          "tokens",
		AgentLoopCompactionMaxTokens:        8000,
		AgentLoopContextWindow:              0,
		AgentLoopCompactionPreserveEvidence: true,
		AgentLoopCompactionRecentTurns:      4,
		AgentLoopFrustrationDetection:       false,
		AgentLoopObserverWindow:             20,
		AgentLoopObserverMinRepeats:         2,
		AgentLoopObserverMinPatternLength:   3,
		FusionEnabled:                       false,
		FusionProviders:                     []string{"minimax-m3", "kimi-k2p7-code-fast", "kimi-k2p7-code", "deepseek-v4-pro", "qwen-3p7-plus", "glm-5p2"},
		FusionMaxCostUSD:                    5.0,
		FusionMinQuorum:                     2,
		FusionPerProviderTimeoutS:           120,
		FusionDifficultyGate:                true,
		FusionOracleMode:                    false,
		MemoryAutoDream:                     false,
		MemoryAutoDreamInterval:             "5m",
		MemoryPrimeOnStart:                  false,
		OrchestratorEpisodicMemory:          false,
		OrchestratorDeepPlanner:             false,
		OrchestratorPatternLearning:         false,
		OrchestratorPreWarm:                 false,
		PermissionYoloRiskThreshold:         "",
		WorktreeConflictCheck:               "off",
		WorktreeTargetBranch:                "",
		// Issue #379: every inject_* flag is opt-in. Default 0 / false.
		AgentLoopSessionContextEnabled: true,
		AgentLoopInjectLessons:         false,
		AgentLoopInjectMemory:          false,
		AgentLoopInjectGoals:           false,
		AgentLoopContextTopK:           5,

		// Progress output defaults: off unless explicitly enabled.
		OutputProgress:     "off",
		OutputProgressDest: "stderr",
		OutputProgressFile: "",
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

	// Atomic write: write to a temp file in the same directory, then rename.
	// This keeps readers from seeing a half-written file.
	tmp := path + ".tmp" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, []byte(content), filemode.Default()); err != nil {
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
	case "llm.thinking_enabled":
		return fmt.Sprintf("%v", cfg.LLMThinkingEnabled), nil
	case "llm.thinking_budget":
		return fmt.Sprintf("%d", cfg.LLMThinkingBudget), nil
	case "llm.style":
		return cfg.LLMStyle, nil
	case "agent.verify_mode":
		return cfg.AgentVerifyMode, nil
	case "agent.max_turns":
		return fmt.Sprintf("%d", cfg.AgentMaxTurns), nil
	case "agent.headless":
		return fmt.Sprintf("%v", cfg.AgentHeadless), nil
	case "agent.yolo":
		return fmt.Sprintf("%v", cfg.AgentYolo), nil
	case "agentloop.required_tools":
		return strings.Join(cfg.AgentLoopRequiredTools, ","), nil
	case "agentloop.forbidden_tools":
		return strings.Join(cfg.AgentLoopForbiddenTools, ","), nil
	case "agentloop.auto_lint":
		return fmt.Sprintf("%v", cfg.AgentLoopAutoLint), nil
	case "agentloop.auto_test":
		return fmt.Sprintf("%v", cfg.AgentLoopAutoTest), nil
	case "agentloop.auto_lint_timeout":
		return fmt.Sprintf("%d", cfg.AgentLoopAutoLintTimeout), nil
	case "agentloop.auto_test_timeout":
		return fmt.Sprintf("%d", cfg.AgentLoopAutoTestTimeout), nil
	case "permissions.tools_allow":
		return strings.Join(cfg.ToolsAllow, ","), nil
	case "permissions.tools_deny":
		return strings.Join(cfg.ToolsDeny, ","), nil
	case "paths.mcp_config":
		return cfg.PathsMCPConfig, nil
	case "paths.skills_dir":
		return cfg.PathsSkillsDir, nil
	case "test.coverage_threshold":
		return fmt.Sprintf("%v", cfg.TestCoverageThreshold), nil
	case "test.mutation_threshold":
		return fmt.Sprintf("%v", cfg.TestMutationThreshold), nil
	case "test.auto_generate":
		return fmt.Sprintf("%v", cfg.TestAutoGenerate), nil
	case "test.timeout_seconds":
		return fmt.Sprintf("%d", cfg.TestTimeoutSeconds), nil
	case "test.use_llm":
		return fmt.Sprintf("%v", cfg.TestUseLLM), nil
	case "test.repair_rounds":
		return fmt.Sprintf("%d", cfg.TestRepairRounds), nil
	case "chat.lazy_tools":
		return fmt.Sprintf("%v", cfg.ChatLazyTools), nil
	case "chat.semantic_tools":
		return fmt.Sprintf("%v", cfg.ChatSemanticTools), nil
	case "llm.prompt_cache":
		return fmt.Sprintf("%v", cfg.LLMPromptCache), nil
	case "fusion.enabled":
		return fmt.Sprintf("%v", cfg.FusionEnabled), nil
	case "fusion.providers":
		return strings.Join(cfg.FusionProviders, ","), nil
	case "fusion.max_cost_usd":
		return fmt.Sprintf("%v", cfg.FusionMaxCostUSD), nil
	case "fusion.min_quorum":
		return fmt.Sprintf("%d", cfg.FusionMinQuorum), nil
	case "fusion.per_provider_timeout_s":
		return fmt.Sprintf("%d", cfg.FusionPerProviderTimeoutS), nil
	case "fusion.difficulty_gate":
		return fmt.Sprintf("%v", cfg.FusionDifficultyGate), nil
	case "fusion.oracle_mode":
		return fmt.Sprintf("%v", cfg.FusionOracleMode), nil
	case "memory.autodream":
		return fmt.Sprintf("%v", cfg.MemoryAutoDream), nil
	case "memory.autodream_interval":
		return cfg.MemoryAutoDreamInterval, nil
	case "memory.prime_on_start":
		return fmt.Sprintf("%v", cfg.MemoryPrimeOnStart), nil
	case "orchestrator.episodic_memory":
		return fmt.Sprintf("%v", cfg.OrchestratorEpisodicMemory), nil
	case "orchestrator.deep_planner":
		return fmt.Sprintf("%v", cfg.OrchestratorDeepPlanner), nil
	case "orchestrator.pattern_learning":
		return fmt.Sprintf("%v", cfg.OrchestratorPatternLearning), nil
	case "orchestrator.prewarm":
		return fmt.Sprintf("%v", cfg.OrchestratorPreWarm), nil
	case "agentloop.compaction_strategy":
		return cfg.AgentLoopCompactionStrategy, nil
	case "agentloop.compaction_threshold":
		return fmt.Sprintf("%v", cfg.AgentLoopCompactionThreshold), nil
	case "agentloop.frustration_detection":
		return fmt.Sprintf("%v", cfg.AgentLoopFrustrationDetection), nil
	case "agentloop.inject_lessons":
		return fmt.Sprintf("%v", cfg.AgentLoopInjectLessons), nil
	case "agentloop.inject_memory":
		return fmt.Sprintf("%v", cfg.AgentLoopInjectMemory), nil
	case "agentloop.inject_goals":
		return fmt.Sprintf("%v", cfg.AgentLoopInjectGoals), nil
	case "agentloop.context_top_k":
		return fmt.Sprintf("%d", cfg.AgentLoopContextTopK), nil
	case "agentloop.session_context.enabled":
		return fmt.Sprintf("%v", cfg.AgentLoopSessionContextEnabled), nil
	case "permission.yolo_risk_threshold":
		return cfg.PermissionYoloRiskThreshold, nil
	case "agentloop.context_compaction":
		return cfg.AgentLoopContextCompaction, nil
	case "agentloop.compaction_trigger":
		return cfg.AgentLoopCompactionTrigger, nil
	case "agentloop.compaction_max_tokens":
		return fmt.Sprintf("%d", cfg.AgentLoopCompactionMaxTokens), nil
	case "agentloop.context_window":
		return fmt.Sprintf("%d", cfg.AgentLoopContextWindow), nil
	case "agentloop.compaction_preserve_evidence":
		return fmt.Sprintf("%v", cfg.AgentLoopCompactionPreserveEvidence), nil
	case "agentloop.compaction_recent_turns":
		return fmt.Sprintf("%d", cfg.AgentLoopCompactionRecentTurns), nil
	case "worktree.conflict_check":
		return cfg.WorktreeConflictCheck, nil
	case "worktree.target_branch":
		return cfg.WorktreeTargetBranch, nil
	case "autonomy.container.enabled":
		return fmt.Sprintf("%v", cfg.AutonomyContainerEnabled), nil
	case "autonomy.container.image":
		return cfg.AutonomyContainerImage, nil
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
	case "llm.thinking_enabled":
		cfg.LLMThinkingEnabled = value == "true" || value == "1"
	case "llm.thinking_budget":
		v, err := strconv.Atoi(value)
		if err != nil || v < 0 {
			return fmt.Errorf("llm.thinking_budget must be a non-negative integer, got %q", value)
		}
		cfg.LLMThinkingBudget = v
	case "llm.style":
		if !isValidStyle(value) {
			return fmt.Errorf("llm.style must be one of default|verbose|normal|terse|ultra, got %q", value)
		}
		cfg.LLMStyle = value
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
	case "agentloop.required_tools":
		cfg.AgentLoopRequiredTools = splitList(value)
	case "agentloop.forbidden_tools":
		cfg.AgentLoopForbiddenTools = splitList(value)
	case "permissions.tools_allow":
		cfg.ToolsAllow = splitList(value)
	case "permissions.tools_deny":
		cfg.ToolsDeny = splitList(value)
	case "paths.mcp_config":
		cfg.PathsMCPConfig = value
	case "paths.skills_dir":
		cfg.PathsSkillsDir = value
	case "test.coverage_threshold":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || v < 0 || v > 100 {
			return fmt.Errorf("test.coverage_threshold must be a percent between 0 and 100, got %q", value)
		}
		cfg.TestCoverageThreshold = v
	case "test.mutation_threshold":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || v < 0 || v > 100 {
			return fmt.Errorf("test.mutation_threshold must be a percent between 0 and 100, got %q", value)
		}
		cfg.TestMutationThreshold = v
	case "test.auto_generate":
		cfg.TestAutoGenerate = value == "true" || value == "1"
	case "test.timeout_seconds":
		v, err := strconv.Atoi(value)
		if err != nil || v <= 0 {
			return fmt.Errorf("test.timeout_seconds must be a positive integer, got %q", value)
		}
		cfg.TestTimeoutSeconds = v
	case "test.use_llm":
		cfg.TestUseLLM = value == "true" || value == "1"
	case "test.repair_rounds":
		v, err := strconv.Atoi(value)
		if err != nil || v < 0 {
			return fmt.Errorf("test.repair_rounds must be a non-negative integer, got %q", value)
		}
		cfg.TestRepairRounds = v
	case "chat.lazy_tools":
		cfg.ChatLazyTools = value == "true" || value == "1"
	case "chat.semantic_tools":
		cfg.ChatSemanticTools = value == "true" || value == "1"
	case "llm.prompt_cache":
		cfg.LLMPromptCache = value == "true" || value == "1"
	case "fusion.enabled":
		cfg.FusionEnabled = value == "true" || value == "1"
	case "fusion.providers":
		cfg.FusionProviders = splitList(value)
	case "fusion.max_cost_usd":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || v < 0 {
			return fmt.Errorf("fusion.max_cost_usd must be a non-negative float, got %q", value)
		}
		cfg.FusionMaxCostUSD = v
	case "fusion.min_quorum":
		v, err := strconv.Atoi(value)
		if err != nil || v < 0 {
			return fmt.Errorf("fusion.min_quorum must be a non-negative integer, got %q", value)
		}
		cfg.FusionMinQuorum = v
	case "fusion.per_provider_timeout_s":
		v, err := strconv.Atoi(value)
		if err != nil || v <= 0 {
			return fmt.Errorf("fusion.per_provider_timeout_s must be a positive integer, got %q", value)
		}
		cfg.FusionPerProviderTimeoutS = v
	case "fusion.difficulty_gate":
		cfg.FusionDifficultyGate = value == "true" || value == "1"
	case "fusion.oracle_mode":
		cfg.FusionOracleMode = value == "true" || value == "1"
	case "memory.autodream":
		cfg.MemoryAutoDream = value == "true" || value == "1"
	case "memory.autodream_interval":
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("memory.autodream_interval must be a duration (e.g. 5m), got %q", value)
		}
		cfg.MemoryAutoDreamInterval = value
	case "memory.prime_on_start":
		cfg.MemoryPrimeOnStart = value == "true" || value == "1"
	case "orchestrator.episodic_memory":
		cfg.OrchestratorEpisodicMemory = value == "true" || value == "1"
	case "orchestrator.deep_planner":
		cfg.OrchestratorDeepPlanner = value == "true" || value == "1"
	case "orchestrator.pattern_learning":
		cfg.OrchestratorPatternLearning = value == "true" || value == "1"
	case "orchestrator.prewarm":
		cfg.OrchestratorPreWarm = value == "true" || value == "1"
	case "agentloop.compaction_strategy":
		cfg.AgentLoopCompactionStrategy = value
	case "agentloop.compaction_threshold":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || v <= 0 || v > 1 {
			return fmt.Errorf("agentloop.compaction_threshold must be between 0 and 1, got %q", value)
		}
		cfg.AgentLoopCompactionThreshold = v
	case "agentloop.frustration_detection":
		cfg.AgentLoopFrustrationDetection = value == "true" || value == "1"
	case "agentloop.inject_lessons":
		cfg.AgentLoopInjectLessons = value == "true" || value == "1"
	case "agentloop.inject_memory":
		cfg.AgentLoopInjectMemory = value == "true" || value == "1"
	case "agentloop.inject_goals":
		cfg.AgentLoopInjectGoals = value == "true" || value == "1"
	case "agentloop.context_top_k":
		v, err := strconv.Atoi(value)
		if err != nil || v <= 0 {
			return fmt.Errorf("agentloop.context_top_k must be a positive integer, got %q", value)
		}
		cfg.AgentLoopContextTopK = v
	case "agentloop.session_context.enabled":
		cfg.AgentLoopSessionContextEnabled = value == "true" || value == "1"
	case "permission.yolo_risk_threshold":
		cfg.PermissionYoloRiskThreshold = value
	case "agentloop.context_compaction":
		if v := parseContextCompactionMode(value); v == nil {
			return fmt.Errorf("agentloop.context_compaction must be 'off', 'deterministic', 'llm', or 'hybrid', got %q", value)
		}
		cfg.AgentLoopContextCompaction = value
	case "agentloop.compaction_trigger":
		if v := parseCompactionTrigger(value); v == nil {
			return fmt.Errorf("agentloop.compaction_trigger must be 'turns', 'tokens', or 'both', got %q", value)
		}
		cfg.AgentLoopCompactionTrigger = value
	case "agentloop.compaction_max_tokens":
		v, err := strconv.Atoi(value)
		if err != nil || v <= 0 {
			return fmt.Errorf("agentloop.compaction_max_tokens must be a positive integer, got %q", value)
		}
		cfg.AgentLoopCompactionMaxTokens = v
	case "agentloop.context_window":
		v, err := strconv.Atoi(value)
		if err != nil || v < 0 {
			return fmt.Errorf("agentloop.context_window must be a non-negative integer, got %q", value)
		}
		cfg.AgentLoopContextWindow = v
	case "agentloop.compaction_preserve_evidence":
		cfg.AgentLoopCompactionPreserveEvidence = value == "true" || value == "1"
	case "agentloop.compaction_recent_turns":
		v, err := strconv.Atoi(value)
		if err != nil || v <= 0 {
			return fmt.Errorf("agentloop.compaction_recent_turns must be a positive integer, got %q", value)
		}
		cfg.AgentLoopCompactionRecentTurns = v
	case "worktree.conflict_check":
		if value != "off" && value != "warn" && value != "abort" {
			return fmt.Errorf("worktree.conflict_check must be 'off', 'warn', or 'abort', got %q", value)
		}
		cfg.WorktreeConflictCheck = value
	case "worktree.target_branch":
		cfg.WorktreeTargetBranch = value
	case "autonomy.container.enabled":
		cfg.AutonomyContainerEnabled = value == "true" || value == "1"
	case "autonomy.container.image":
		cfg.AutonomyContainerImage = value
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
		{"llm.thinking_enabled", fmt.Sprintf("%v", cfg.LLMThinkingEnabled)},
		{"llm.thinking_budget", fmt.Sprintf("%d", cfg.LLMThinkingBudget)},
		{"llm.style", cfg.LLMStyle},
		{"agent.verify_mode", cfg.AgentVerifyMode},
		{"agent.max_turns", fmt.Sprintf("%d", cfg.AgentMaxTurns)},
		{"agent.headless", fmt.Sprintf("%v", cfg.AgentHeadless)},
		{"agent.yolo", fmt.Sprintf("%v", cfg.AgentYolo)},
		{"agentloop.required_tools", strings.Join(cfg.AgentLoopRequiredTools, ",")},
		{"agentloop.forbidden_tools", strings.Join(cfg.AgentLoopForbiddenTools, ",")},
		{"permissions.tools_allow", strings.Join(cfg.ToolsAllow, ",")},
		{"permissions.tools_deny", strings.Join(cfg.ToolsDeny, ",")},
		{"paths.mcp_config", cfg.PathsMCPConfig},
		{"paths.skills_dir", cfg.PathsSkillsDir},
		{"test.coverage_threshold", fmt.Sprintf("%v", cfg.TestCoverageThreshold)},
		{"test.mutation_threshold", fmt.Sprintf("%v", cfg.TestMutationThreshold)},
		{"test.auto_generate", fmt.Sprintf("%v", cfg.TestAutoGenerate)},
		{"test.timeout_seconds", fmt.Sprintf("%d", cfg.TestTimeoutSeconds)},
		{"test.use_llm", fmt.Sprintf("%v", cfg.TestUseLLM)},
		{"test.repair_rounds", fmt.Sprintf("%d", cfg.TestRepairRounds)},
		{"chat.lazy_tools", fmt.Sprintf("%v", cfg.ChatLazyTools)},
		{"chat.semantic_tools", fmt.Sprintf("%v", cfg.ChatSemanticTools)},
		{"llm.prompt_cache", fmt.Sprintf("%v", cfg.LLMPromptCache)},
		{"fusion.enabled", fmt.Sprintf("%v", cfg.FusionEnabled)},
		{"fusion.providers", strings.Join(cfg.FusionProviders, ",")},
		{"fusion.max_cost_usd", fmt.Sprintf("%v", cfg.FusionMaxCostUSD)},
		{"fusion.min_quorum", fmt.Sprintf("%d", cfg.FusionMinQuorum)},
		{"fusion.per_provider_timeout_s", fmt.Sprintf("%d", cfg.FusionPerProviderTimeoutS)},
		{"fusion.difficulty_gate", fmt.Sprintf("%v", cfg.FusionDifficultyGate)},
		{"fusion.oracle_mode", fmt.Sprintf("%v", cfg.FusionOracleMode)},
		{"memory.autodream", fmt.Sprintf("%v", cfg.MemoryAutoDream)},
		{"memory.autodream_interval", cfg.MemoryAutoDreamInterval},
		{"memory.prime_on_start", fmt.Sprintf("%v", cfg.MemoryPrimeOnStart)},
		{"orchestrator.episodic_memory", fmt.Sprintf("%v", cfg.OrchestratorEpisodicMemory)},
		{"orchestrator.deep_planner", fmt.Sprintf("%v", cfg.OrchestratorDeepPlanner)},
		{"orchestrator.pattern_learning", fmt.Sprintf("%v", cfg.OrchestratorPatternLearning)},
		{"orchestrator.prewarm", fmt.Sprintf("%v", cfg.OrchestratorPreWarm)},
		{"agentloop.compaction_strategy", cfg.AgentLoopCompactionStrategy},
		{"agentloop.compaction_threshold", fmt.Sprintf("%v", cfg.AgentLoopCompactionThreshold)},
		{"agentloop.frustration_detection", fmt.Sprintf("%v", cfg.AgentLoopFrustrationDetection)},
		{"agentloop.inject_lessons", fmt.Sprintf("%v", cfg.AgentLoopInjectLessons)},
		{"agentloop.inject_memory", fmt.Sprintf("%v", cfg.AgentLoopInjectMemory)},
		{"agentloop.inject_goals", fmt.Sprintf("%v", cfg.AgentLoopInjectGoals)},
		{"agentloop.context_top_k", fmt.Sprintf("%d", cfg.AgentLoopContextTopK)},
		{"agentloop.context_compaction", cfg.AgentLoopContextCompaction},
		{"agentloop.compaction_trigger", cfg.AgentLoopCompactionTrigger},
		{"agentloop.compaction_max_tokens", fmt.Sprintf("%d", cfg.AgentLoopCompactionMaxTokens)},
		{"agentloop.context_window", fmt.Sprintf("%d", cfg.AgentLoopContextWindow)},
		{"agentloop.compaction_preserve_evidence", fmt.Sprintf("%v", cfg.AgentLoopCompactionPreserveEvidence)},
		{"agentloop.compaction_recent_turns", fmt.Sprintf("%d", cfg.AgentLoopCompactionRecentTurns)},
		{"agentloop.session_context.enabled", fmt.Sprintf("%v", cfg.AgentLoopSessionContextEnabled)},
		{"permission.yolo_risk_threshold", cfg.PermissionYoloRiskThreshold},
		{"worktree.conflict_check", cfg.WorktreeConflictCheck},
		{"worktree.target_branch", cfg.WorktreeTargetBranch},
		{"autonomy.container.enabled", fmt.Sprintf("%v", cfg.AutonomyContainerEnabled)},
		{"autonomy.container.image", cfg.AutonomyContainerImage},
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
			"style":       cfg.LLMStyle,
		},
		"agent": map[string]any{
			"verify_mode": cfg.AgentVerifyMode,
			"max_turns":   cfg.AgentMaxTurns,
			"headless":    cfg.AgentHeadless,
			"yolo":        cfg.AgentYolo,
		},
		"agentloop": map[string]any{
			"required_tools":               cfg.AgentLoopRequiredTools,
			"forbidden_tools":              cfg.AgentLoopForbiddenTools,
			"context_compaction":           cfg.AgentLoopContextCompaction,
			"compaction_trigger":           cfg.AgentLoopCompactionTrigger,
			"compaction_max_tokens":        cfg.AgentLoopCompactionMaxTokens,
			"context_window":               cfg.AgentLoopContextWindow,
			"compaction_preserve_evidence": cfg.AgentLoopCompactionPreserveEvidence,
			"compaction_recent_turns":      cfg.AgentLoopCompactionRecentTurns,
			"session_context": map[string]any{
				"enabled": cfg.AgentLoopSessionContextEnabled,
			},
		},
		"permissions": map[string]any{
			"tools_allow": cfg.ToolsAllow,
			"tools_deny":  cfg.ToolsDeny,
		},
		"paths": map[string]any{
			"mcp_config": cfg.PathsMCPConfig,
			"skills_dir": cfg.PathsSkillsDir,
		},
		"worktree": map[string]any{
			"conflict_check": cfg.WorktreeConflictCheck,
			"target_branch":  cfg.WorktreeTargetBranch,
		},
		"autonomy": map[string]any{
			"container": map[string]any{
				"enabled": cfg.AutonomyContainerEnabled,
				"image":   cfg.AutonomyContainerImage,
			},
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
		LLMStyle:        cfg.LLMStyle,
		AgentVerifyMode: cfg.AgentVerifyMode, AgentMaxTurns: cfg.AgentMaxTurns,
		AgentHeadless: cfg.AgentHeadless, AgentYolo: cfg.AgentYolo,
		AgentLoopRequiredTools: cfg.AgentLoopRequiredTools, AgentLoopForbiddenTools: cfg.AgentLoopForbiddenTools,
		ToolsAllow: cfg.ToolsAllow, ToolsDeny: cfg.ToolsDeny,
		PathsMCPConfig: cfg.PathsMCPConfig, PathsSkillsDir: cfg.PathsSkillsDir,
		TestCoverageThreshold: cfg.TestCoverageThreshold, TestMutationThreshold: cfg.TestMutationThreshold,
		TestAutoGenerate: cfg.TestAutoGenerate, TestTimeoutSeconds: cfg.TestTimeoutSeconds,
		TestUseLLM: cfg.TestUseLLM, TestRepairRounds: cfg.TestRepairRounds,
		WorktreeConflictCheck:               cfg.WorktreeConflictCheck,
		WorktreeTargetBranch:                cfg.WorktreeTargetBranch,
		AgentLoopContextCompaction:          cfg.AgentLoopContextCompaction,
		AgentLoopCompactionTrigger:          cfg.AgentLoopCompactionTrigger,
		AgentLoopCompactionMaxTokens:        cfg.AgentLoopCompactionMaxTokens,
		AgentLoopContextWindow:              cfg.AgentLoopContextWindow,
		AgentLoopCompactionPreserveEvidence: cfg.AgentLoopCompactionPreserveEvidence,
		AgentLoopCompactionRecentTurns:      cfg.AgentLoopCompactionRecentTurns,
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
	if !isValidStyle(cfg.LLMStyle) {
		issues = append(issues, fmt.Sprintf("llm.style must be one of default|verbose|normal|terse|ultra, got %q", cfg.LLMStyle))
	}
	if cfg.AgentVerifyMode != "off" && cfg.AgentVerifyMode != "poc" && cfg.AgentVerifyMode != "oracle" {
		issues = append(issues, fmt.Sprintf("agent.verify_mode must be 'off', 'poc', or 'oracle', got %q", cfg.AgentVerifyMode))
	}
	if cfg.AgentMaxTurns <= 0 {
		issues = append(issues, fmt.Sprintf("agent.max_turns must be > 0, got %d", cfg.AgentMaxTurns))
	}
	if cfg.TestCoverageThreshold < 0 || cfg.TestCoverageThreshold > 100 {
		issues = append(issues, fmt.Sprintf("test.coverage_threshold must be 0..100, got %v", cfg.TestCoverageThreshold))
	}
	if cfg.TestMutationThreshold < 0 || cfg.TestMutationThreshold > 100 {
		issues = append(issues, fmt.Sprintf("test.mutation_threshold must be 0..100, got %v", cfg.TestMutationThreshold))
	}
	if cfg.TestTimeoutSeconds <= 0 {
		issues = append(issues, fmt.Sprintf("test.timeout_seconds must be > 0, got %d", cfg.TestTimeoutSeconds))
	}
	if cfg.TestRepairRounds < 0 {
		issues = append(issues, fmt.Sprintf("test.repair_rounds must be >= 0, got %d", cfg.TestRepairRounds))
	}
	if cfg.AgentLoopCompactionStrategy != "off" && cfg.AgentLoopCompactionStrategy != "" {
		switch cfg.AgentLoopCompactionStrategy {
		case "summarize", "truncate", "selective", "sliding", "sliding-window", "hybrid":
		default:
			issues = append(issues, fmt.Sprintf("agentloop.compaction_strategy must be off|summarize|truncate|selective|sliding|hybrid, got %q", cfg.AgentLoopCompactionStrategy))
		}
	}
	if cfg.AgentLoopCompactionThreshold <= 0 || cfg.AgentLoopCompactionThreshold > 1 {
		issues = append(issues, fmt.Sprintf("agentloop.compaction_threshold must be in (0,1], got %v", cfg.AgentLoopCompactionThreshold))
	}
	if cfg.AgentLoopObserverWindow < 0 {
		issues = append(issues, fmt.Sprintf("agentloop.observer_window must be >= 0, got %d", cfg.AgentLoopObserverWindow))
	}
	if cfg.AgentLoopObserverMinRepeats < 1 {
		issues = append(issues, fmt.Sprintf("agentloop.observer_min_repeats must be >= 1, got %d", cfg.AgentLoopObserverMinRepeats))
	}
	if cfg.AgentLoopObserverMinPatternLength < 1 {
		issues = append(issues, fmt.Sprintf("agentloop.observer_min_pattern_length must be >= 1, got %d", cfg.AgentLoopObserverMinPatternLength))
	}
	if cfg.WorktreeConflictCheck != "" && cfg.WorktreeConflictCheck != "off" && cfg.WorktreeConflictCheck != "warn" && cfg.WorktreeConflictCheck != "abort" {
		issues = append(issues, fmt.Sprintf("worktree.conflict_check must be 'off', 'warn', or 'abort', got %q", cfg.WorktreeConflictCheck))
	}
	if cfg.AgentLoopContextCompaction != "" && cfg.AgentLoopContextCompaction != "off" {
		if parseContextCompactionMode(cfg.AgentLoopContextCompaction) == nil {
			issues = append(issues, fmt.Sprintf("agentloop.context_compaction must be 'off', 'deterministic', 'llm', or 'hybrid', got %q", cfg.AgentLoopContextCompaction))
		}
	}
	if cfg.AgentLoopCompactionTrigger != "" && cfg.AgentLoopCompactionTrigger != "tokens" {
		if parseCompactionTrigger(cfg.AgentLoopCompactionTrigger) == nil {
			issues = append(issues, fmt.Sprintf("agentloop.compaction_trigger must be 'turns', 'tokens', or 'both', got %q", cfg.AgentLoopCompactionTrigger))
		}
	}
	if cfg.AgentLoopCompactionMaxTokens < 0 {
		issues = append(issues, fmt.Sprintf("agentloop.compaction_max_tokens must be >= 0, got %d", cfg.AgentLoopCompactionMaxTokens))
	}
	if cfg.AgentLoopContextWindow < 0 {
		issues = append(issues, fmt.Sprintf("agentloop.context_window must be >= 0, got %d", cfg.AgentLoopContextWindow))
	}
	if cfg.AgentLoopCompactionRecentTurns <= 0 {
		issues = append(issues, fmt.Sprintf("agentloop.compaction_recent_turns must be > 0, got %d", cfg.AgentLoopCompactionRecentTurns))
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
		case "llm.thinking_enabled":
			cfg.LLMThinkingEnabled = val == "true" || val == "1"
		case "llm.thinking_budget":
			_, _ = fmt.Sscanf(val, "%d", &cfg.LLMThinkingBudget)
		case "llm.style":
			cfg.LLMStyle = val
		case "agent.verify_mode":
			cfg.AgentVerifyMode = val
		case "agent.max_turns":
			_, _ = fmt.Sscanf(val, "%d", &cfg.AgentMaxTurns)
		case "agent.headless":
			cfg.AgentHeadless = val == "true"
		case "agent.yolo":
			cfg.AgentYolo = val == "true"
		case "agentloop.required_tools":
			cfg.AgentLoopRequiredTools = parseList(val)
		case "agentloop.forbidden_tools":
			cfg.AgentLoopForbiddenTools = parseList(val)
		case "permissions.tools_allow":
			cfg.ToolsAllow = parseList(val)
		case "permissions.tools_deny":
			cfg.ToolsDeny = parseList(val)
		case "paths.mcp_config":
			cfg.PathsMCPConfig = val
		case "paths.skills_dir":
			cfg.PathsSkillsDir = val
		case "test.coverage_threshold":
			v, _ := strconv.ParseFloat(val, 64)
			cfg.TestCoverageThreshold = v
		case "test.mutation_threshold":
			v, _ := strconv.ParseFloat(val, 64)
			cfg.TestMutationThreshold = v
		case "test.auto_generate":
			cfg.TestAutoGenerate = val == "true"
		case "test.timeout_seconds":
			_, _ = fmt.Sscanf(val, "%d", &cfg.TestTimeoutSeconds)
		case "test.use_llm":
			cfg.TestUseLLM = val == "true" || val == "1"
		case "test.repair_rounds":
			_, _ = fmt.Sscanf(val, "%d", &cfg.TestRepairRounds)
		case "fusion.enabled":
			cfg.FusionEnabled = val == "true" || val == "1"
		case "fusion.providers":
			cfg.FusionProviders = parseList(val)
		case "fusion.max_cost_usd":
			v, _ := strconv.ParseFloat(val, 64)
			cfg.FusionMaxCostUSD = v
		case "fusion.min_quorum":
			_, _ = fmt.Sscanf(val, "%d", &cfg.FusionMinQuorum)
		case "fusion.per_provider_timeout_s":
			_, _ = fmt.Sscanf(val, "%d", &cfg.FusionPerProviderTimeoutS)
		case "fusion.difficulty_gate":
			cfg.FusionDifficultyGate = val == "true" || val == "1"
		case "fusion.oracle_mode":
			cfg.FusionOracleMode = val == "true" || val == "1"
		case "memory.autodream":
			cfg.MemoryAutoDream = val == "true" || val == "1"
		case "memory.autodream_interval":
			cfg.MemoryAutoDreamInterval = val
		case "memory.prime_on_start":
			cfg.MemoryPrimeOnStart = val == "true" || val == "1"
		case "orchestrator.episodic_memory":
			cfg.OrchestratorEpisodicMemory = val == "true" || val == "1"
		case "orchestrator.deep_planner":
			cfg.OrchestratorDeepPlanner = val == "true" || val == "1"
		case "orchestrator.pattern_learning":
			cfg.OrchestratorPatternLearning = val == "true" || val == "1"
		case "orchestrator.prewarm":
			cfg.OrchestratorPreWarm = val == "true" || val == "1"
		case "chat.lazy_tools":
			cfg.ChatLazyTools = val == "true" || val == "1"
		case "chat.semantic_tools":
			cfg.ChatSemanticTools = val == "true" || val == "1"
		case "llm.prompt_cache":
			cfg.LLMPromptCache = val == "true" || val == "1"
		case "agentloop.compaction_strategy":
			cfg.AgentLoopCompactionStrategy = val
		case "agentloop.compaction_threshold":
			v, _ := strconv.ParseFloat(val, 64)
			cfg.AgentLoopCompactionThreshold = v
		case "agentloop.frustration_detection":
			cfg.AgentLoopFrustrationDetection = val == "true" || val == "1"
		case "agentloop.inject_lessons":
			cfg.AgentLoopInjectLessons = val == "true" || val == "1"
		case "agentloop.inject_memory":
			cfg.AgentLoopInjectMemory = val == "true" || val == "1"
		case "agentloop.inject_goals":
			cfg.AgentLoopInjectGoals = val == "true" || val == "1"
		case "agentloop.context_top_k":
			v, _ := strconv.Atoi(val)
			if v > 0 {
				cfg.AgentLoopContextTopK = v
			}
		case "permission.yolo_risk_threshold":
			cfg.PermissionYoloRiskThreshold = val
		case "agentloop.context_compaction":
			cfg.AgentLoopContextCompaction = val
		case "agentloop.compaction_trigger":
			cfg.AgentLoopCompactionTrigger = val
		case "agentloop.compaction_max_tokens":
			_, _ = fmt.Sscanf(val, "%d", &cfg.AgentLoopCompactionMaxTokens)
		case "agentloop.context_window":
			_, _ = fmt.Sscanf(val, "%d", &cfg.AgentLoopContextWindow)
		case "agentloop.compaction_preserve_evidence":
			cfg.AgentLoopCompactionPreserveEvidence = val == "true" || val == "1"
		case "agentloop.compaction_recent_turns":
			_, _ = fmt.Sscanf(val, "%d", &cfg.AgentLoopCompactionRecentTurns)
		case "worktree.conflict_check":
			cfg.WorktreeConflictCheck = val
		case "worktree.target_branch":
			cfg.WorktreeTargetBranch = val
		}
	}
}
