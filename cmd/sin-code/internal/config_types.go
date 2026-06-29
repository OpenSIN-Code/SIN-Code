// SPDX-License-Identifier: MIT
// Purpose: SinCodeConfig type definition, style validation, and defaults.
package internal

// sin-debt: shrink, upgrade: when a second types-related function is needed, merge into a shared file

import (
	"path/filepath"

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

	// MCPConnectTimeoutS is the per-server connection timeout in seconds
	// for ConnectAll. Default 3. 0 falls back to 3 inside the manager.
	MCPConnectTimeoutS int `toml:"mcp.connect_timeout"`
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

		MCPConnectTimeoutS: 3,
	}
}
