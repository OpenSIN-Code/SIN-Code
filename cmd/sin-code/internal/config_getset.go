// SPDX-License-Identifier: MIT
// Purpose: config get/set key-value operations.
package internal

// sin-debt: shrink, upgrade: when a second getset-related function is needed, merge into a shared file

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
	case "agentloop.self_review":
		return fmt.Sprintf("%v", cfg.AgentLoopSelfReview), nil
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
	case "mcp.connect_timeout":
		return fmt.Sprintf("%d", cfg.MCPConnectTimeoutS), nil
	case "agentloop.auto_commit":
		return fmt.Sprintf("%v", cfg.AgentLoopAutoCommit), nil
	case "agentloop.commit_prefix":
		return cfg.AgentLoopCommitPrefix, nil
	case "agentloop.mode":
		return cfg.AgentLoopMode, nil
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
	case "agentloop.self_review":
		cfg.AgentLoopSelfReview = value == "true" || value == "1"
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
	case "mcp.connect_timeout":
		v, err := strconv.Atoi(value)
		if err != nil || v < 0 {
			return fmt.Errorf("mcp.connect_timeout must be a non-negative integer (seconds), got %q", value)
		}
		cfg.MCPConnectTimeoutS = v
	case "agentloop.auto_commit":
		cfg.AgentLoopAutoCommit = value == "true" || value == "1"
	case "agentloop.commit_prefix":
		cfg.AgentLoopCommitPrefix = value
	case "agentloop.mode":
		switch value {
		case "default", "architect", "debug", "code", "review":
			cfg.AgentLoopMode = value
		default:
			return fmt.Errorf("agentloop.mode must be one of default|architect|debug|code|review, got %q", value)
		}
	default:
		return fmt.Errorf("unknown config key: %q", key)
	}
	fmt.Printf("✅ Set %s = %q\n", key, value)
	return nil
}
