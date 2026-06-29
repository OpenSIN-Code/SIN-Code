// SPDX-License-Identifier: MIT
// Purpose: cobra command definitions, config display/output helpers, and
// utility functions for secret masking and list splitting.
package internal

// sin-debt: shrink, upgrade: when a second cli-related function is needed, merge into a shared file

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// sin-debt: delete, upgrade: remove when test no longer needs this override
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
