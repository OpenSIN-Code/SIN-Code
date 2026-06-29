// SPDX-License-Identifier: MIT
// Purpose: config validation and map application (key→struct field).
package internal

// sin-debt: shrink, upgrade: when a second validate-related function is needed, merge into a shared file

import (
	"fmt"
	"strconv"
	"strings"
)

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
