// SPDX-License-Identifier: MIT
// Purpose: shared loop factory — eliminates duplication of provider /
// permission / hooks / gate / mcp / memory setup across chat / swarm /
// serve (issue #64, DRY refactor).
package loopbuilder

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/stopgate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

// Build constructs a fully wired agentloop.Loop with all mandates applied
// (C1-C8, M1-M4). Returns the loop and a cleanup function (defer it).
func Build(ctx context.Context, cfg Config, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
	// Apply config-file defaults for tool coverage when the caller did not
	// supply CLI values (issue #248).
	if len(cfg.CoverageRequiredTools) == 0 || len(cfg.CoverageForbiddenTools) == 0 {
		if sinCfg, err := internal.LoadMergedConfig(); err == nil {
			if len(cfg.CoverageRequiredTools) == 0 {
				cfg.CoverageRequiredTools = sinCfg.AgentLoopRequiredTools
			}
			if len(cfg.CoverageForbiddenTools) == 0 {
				cfg.CoverageForbiddenTools = sinCfg.AgentLoopForbiddenTools
			}
		}
	}

	// Merge required_tools from activated skills' SKILL.md frontmatter
	// into CoverageRequiredTools (additive, deduplicated, sorted).
	// Non-skill rule names in ActiveSkills are silently skipped.
	if len(cfg.ActiveSkills) > 0 {
		if skillFS, err := skills.ListFS(); err == nil {
			cfg.CoverageRequiredTools = skillmgr.MergeRequiredTools(
				cfg.CoverageRequiredTools, cfg.ActiveSkills, skillFS)
		}
	}

	// Apply config-file defaults for SIN Fusion v1 (issue #290).
	if !cfg.FusionEnabled {
		if sinCfg, err := internal.LoadMergedConfig(); err == nil {
			cfg.FusionEnabled = sinCfg.FusionEnabled
		}
	}

	// Apply config-file / env defaults for standalone orchestrator features.
	if sinCfg, err := internal.LoadMergedConfig(); err == nil {
		if !cfg.DeepPlannerEnabled {
			cfg.DeepPlannerEnabled = sinCfg.OrchestratorDeepPlanner
		}
		if !cfg.PatternLearningEnabled {
			cfg.PatternLearningEnabled = sinCfg.OrchestratorPatternLearning
		}
		if !cfg.PreWarmEnabled {
			cfg.PreWarmEnabled = sinCfg.OrchestratorPreWarm
		}
		if cfg.CompactionStrategy == "" {
			cfg.CompactionStrategy = sinCfg.AgentLoopCompactionStrategy
		}
		if !cfg.FrustrationDetectionEnabled {
			cfg.FrustrationDetectionEnabled = sinCfg.AgentLoopFrustrationDetection
		}
		if cfg.ObserverWindow == 0 {
			cfg.ObserverWindow = sinCfg.AgentLoopObserverWindow
		}
		if cfg.ObserverMinPatternLength == 0 {
			cfg.ObserverMinPatternLength = sinCfg.AgentLoopObserverMinPatternLength
		}
		if cfg.ObserverMinRepeats == 0 {
			cfg.ObserverMinRepeats = sinCfg.AgentLoopObserverMinRepeats
		}
		if cfg.YoloRiskThreshold == "" {
			cfg.YoloRiskThreshold = sinCfg.PermissionYoloRiskThreshold
		}
		if !cfg.MemoryPrimeEnabled {
			cfg.MemoryPrimeEnabled = sinCfg.MemoryPrimeOnStart
		}
		if !cfg.EpisodicMemoryEnabled {
			cfg.EpisodicMemoryEnabled = sinCfg.OrchestratorEpisodicMemory
		}
	}
	if os.Getenv("SIN_DEEP_PLANNER") == "1" {
		cfg.DeepPlannerEnabled = true
	}
	if cfg.FusionEnabled {
		if sinCfg, err := internal.LoadMergedConfig(); err == nil {
			if len(cfg.FusionProviders) == 0 {
				cfg.FusionProviders = sinCfg.FusionProviders
			}
			if cfg.FusionMaxCostUSD == 0 {
				cfg.FusionMaxCostUSD = sinCfg.FusionMaxCostUSD
			}
			if cfg.FusionMinQuorum == 0 {
				cfg.FusionMinQuorum = sinCfg.FusionMinQuorum
			}
			if cfg.FusionPerProviderTimeoutS == 0 {
				cfg.FusionPerProviderTimeoutS = sinCfg.FusionPerProviderTimeoutS
			}
			if !cfg.FusionDifficultyGate {
				cfg.FusionDifficultyGate = sinCfg.FusionDifficultyGate
			}
			if !cfg.FusionOracleMode {
				cfg.FusionOracleMode = sinCfg.FusionOracleMode
			}
		}
		if cfg.FusionMaxCostUSD == 0 {
			cfg.FusionMaxCostUSD = 5.0
		}
		if cfg.FusionMinQuorum == 0 {
			cfg.FusionMinQuorum = 2
		}
		if cfg.FusionPerProviderTimeoutS == 0 {
			cfg.FusionPerProviderTimeoutS = 120
		}
	}

	var agentCfg orchestrator.AgentConfig
	if cfg.AgentName != "" {
		loaded, _, err := internal.LoadEffectiveAgent(cfg.AgentName)
		if err != nil {
			return nil, nil, fmt.Errorf("load agent profile: %w", err)
		}
		agentCfg = loaded
	}

	sinCfg, _ := internal.LoadMergedConfig()
	baseURL := firstNonEmpty(cfg.BaseURL, agentCfg.BaseURL,
		sinCfg.LLMBaseURL, os.Getenv("SIN_LLM_BASE_URL"), "https://integrate.api.nvidia.com/v1")
	apiKey := firstNonEmpty(os.Getenv("SIN_LLM_API_KEY"),
		os.Getenv("NVIDIA_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	model := firstNonEmpty(cfg.Model, agentCfg.Model,
		sinCfg.LLMModel, os.Getenv("SIN_LLM_MODEL"))
	if model == "" {
		model = "nvidia/nemotron-3-nano-30b-a3b"
		fmt.Fprintln(os.Stderr, "WARN: no model configured, using default: "+model)
		fmt.Fprintln(os.Stderr, "Set it with: sin-code config set llm.model <model>")
	}
	client := llm.NewClient(baseURL, apiKey)
	thinkingCfg := &agentloop.ThinkingConfig{
		Enabled: cfg.ThinkingEnabled,
		Budget:  cfg.ThinkingBudgetPerRequest,
	}
	if !thinkingCfg.Enabled {
		if sinCfg, err := internal.LoadMergedConfig(); err == nil {
			if sinCfg.LLMThinkingEnabled {
				thinkingCfg.Enabled = true
				if thinkingCfg.Budget == 0 {
					thinkingCfg.Budget = sinCfg.LLMThinkingBudget
				}
			}
		}
	}
	completion := agentloop.NewProviderCompletionFull(client, model, agentCfg.MaxTokens, agentCfg.Temperature, nil, thinkingCfg)
	if sinCfg, err := internal.LoadMergedConfig(); err == nil {
		if sinCfg.LLMPromptCache {
			cache := llm.NewPromptCache(llm.DefaultCacheTTL)
			completion = agentloop.NewProviderCompletionFull(client, model, agentCfg.MaxTokens, agentCfg.Temperature, cache, thinkingCfg)
		}
	}

	perm := permission.New(internal.RulesForAgent(agentCfg))
	perm.Yolo = cfg.Yolo
	perm.Headless = cfg.Headless

	if cfg.Yolo && cfg.YoloRiskThreshold != "" {
		classifier := permission.NewRiskClassifier()
		if level, err := permission.ParseRiskLevel(cfg.YoloRiskThreshold); err == nil {
			classifier.SetThreshold(level)
		}
		perm.Risk = classifier
	}

	hookEngine := hooks.New(loadHooks(cfg.Workspace))

	mode := cfg.VerifyMode
	if mode == "" {
		if cfg.VerifyCmd != "" {
			mode = "poc"
		} else {
			mode = "off"
		}
	}
	runner := commandRunner(cfg.VerifyCmd, cfg.ContainerRunner, cfg.ContainerImage)
	gate := verify.NewGate(mode, runner, runner)

	mcpMgr := mcpclient.NewManager(mcpclient.LoadConfigs(cfg.Workspace))
	if sinCfg, err := internal.LoadMergedConfig(); err == nil && sinCfg.MCPConnectTimeoutS > 0 {
		mcpMgr.SetConnectTimeout(time.Duration(sinCfg.MCPConnectTimeoutS) * time.Second)
	}
	if !cfg.SkipMCP {
		if err := mcpMgr.ConnectAll(ctx); err != nil {
			return nil, nil, err
		}
	}

	// Tool wiring: explicit (LocalTool/LocalSpec) wins over factory.
	var localTool agentloop.LocalToolFunc = cfg.LocalTool
	var localSpec []agentloop.ToolSpec = cfg.LocalSpec
	if cfg.ToolFactory != nil && (localTool == nil || localSpec == nil) {
		localTool, localSpec = cfg.ToolFactory(mcpMgr)
	}

	ledgerStore, err := ledger.Open(ledger.DefaultPath())
	if err != nil {
		ledgerStore = nil // ledger is optional; do not fail the loop if it cannot open
	}

	loop := &agentloop.Loop{
		Gate:                     gate,
		LocalTool:                localTool,
		LocalSpec:                localSpec,
		Workspace:                cfg.Workspace,
		MaxTurns:                 cfg.MaxTurns,
		SessionID:                cfg.SessionID,
		GoalID:                   cfg.GoalID,
		SystemPrompt:             style.RenderSystemPrompt(cfg.Style),
		Completion:               completion,
		Hooks:                    hookEngine,
		Perm:                     perm,
		Ask:                      cfg.AskFunc,
		Lessons:                  memStore,
		Ledger:                   ledgerStore,
		CoverageRequiredTools:    cfg.CoverageRequiredTools,
		CoverageForbiddenTools:   cfg.CoverageForbiddenTools,
		ThinkingEnabled:          thinkingCfg.Enabled,
		ThinkingBudgetPerRequest: thinkingCfg.Budget,
		ResultPolicy:             permission.NewResultPolicy(),
	}

	if cfg.RepetitionThreshold > 0 {
		loop.LoopDetector = agentloop.NewSimpleLoopDetector(cfg.RepetitionThreshold, cfg.RepetitionWindow)
	}

	// Stop-gate (anti-babysitting): when a Definition-of-Done contract is
	// supplied, completion authority is taken away from the worker. The
	// hybrid gate runs deterministic checks first, then a strong/equal LLM
	// judge (SIN_EVALUATOR_MODEL, falling back to the worker model) for the
	// non-mechanical criteria. Without a contract the loop is unchanged.
	loop.AllowContinuation = cfg.AllowContinuation
	if cfg.Contract != nil && !cfg.Contract.IsEmpty() {
		var gateOpts []stopgate.Option
		evalModel := firstNonEmpty(os.Getenv("SIN_EVALUATOR_MODEL"), model)
		if len(cfg.Contract.SemanticCriteria) > 0 && evalModel != "" {
			evalClient := client
			if base := os.Getenv("SIN_EVALUATOR_BASE_URL"); base != "" {
				evalClient = llm.NewClient(base, firstNonEmpty(os.Getenv("SIN_EVALUATOR_API_KEY"), apiKey))
			}
			if judge, jerr := eval.NewJudge(eval.JudgeConfig{Model: evalModel, Strict: true}, evalClient); jerr == nil {
				gateOpts = append(gateOpts, stopgate.WithJudge(judge))
			} else {
				fmt.Fprintf(os.Stderr, "warn: stop-gate semantic judge disabled: %v\n", jerr)
			}
		}
		gate := stopgate.New(cfg.Workspace, gateOpts...)
		loop.StopGate = gate.LoopGate(*cfg.Contract)

		// Tell the worker the rubric up front (SinCode Loop System): the same
		// semantic criteria the stop-gate will enforce are injected as a
		// Definition-of-Done preamble so tests/debug/docs/completeness are
		// handled on the first pass instead of after a rejection.
		loop.Preamble = goalcontract.Preamble(*cfg.Contract)
	}

	// Headroom context compression (issue #118): opt-in via HEADROOM_ENABLED.
	// When disabled or unavailable the hook is a no-op and is not wired.
	headroomHook := agentloop.NewHeadroomHook(headroom.LoadConfigFromEnv())
	if headroomHook.Enabled() {
		loop.CompressMessages = headroomHook.CompressMessages
	}

	// Compactor (issue #278): opt-in via config agentloop.compaction_strategy.
	// Chained after CompressMessages — the compactor runs when
	// ShouldCompact(msgCount, maxTurns, threshold) is true.
	if cfg.CompactionStrategy != "" {
		strategy, serr := agentloop.ParseCompactionStrategy(cfg.CompactionStrategy)
		if serr != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid compaction strategy %q: %v (falling back to hybrid)\n", cfg.CompactionStrategy, serr)
			strategy = agentloop.DefaultCompactionStrategy()
		}
		loop.Compactor = agentloop.NewCompactor(nil)
		loop.CompactionStrategy = strategy
	}

	// Context compaction mode (issue: compaction-modes): config-file defaults for
	// the 6 mode-based compaction keys. When ContextCompactionMode is non-empty
	// and non-"off", the mode-based compactor replaces strategy-based compaction.
	if cfg.ContextCompactionMode != "off" || cfg.CompactionTrigger != "" ||
		cfg.CompactionMaxTokens != 0 || cfg.ContextWindow != 0 ||
		cfg.CompactionPreserveEvidence || cfg.CompactionRecentTurns != 0 {
		if sinCfg, err := internal.LoadMergedConfig(); err == nil {
			if cfg.ContextCompactionMode == "" || cfg.ContextCompactionMode == "off" {
				cfg.ContextCompactionMode = sinCfg.AgentLoopContextCompaction
			}
			if cfg.CompactionTrigger == "" {
				cfg.CompactionTrigger = sinCfg.AgentLoopCompactionTrigger
			}
			if cfg.CompactionMaxTokens == 0 {
				cfg.CompactionMaxTokens = sinCfg.AgentLoopCompactionMaxTokens
			}
			if cfg.ContextWindow == 0 {
				cfg.ContextWindow = sinCfg.AgentLoopContextWindow
			}
			if !cfg.CompactionPreserveEvidence {
				cfg.CompactionPreserveEvidence = sinCfg.AgentLoopCompactionPreserveEvidence
			}
			if cfg.CompactionRecentTurns == 0 {
				cfg.CompactionRecentTurns = sinCfg.AgentLoopCompactionRecentTurns
			}
		}
	}
	if cfg.ContextCompactionMode != "" && cfg.ContextCompactionMode != "off" {
		mode, _ := agentloop.ParseContextCompactionMode(cfg.ContextCompactionMode)
		trigger, _ := agentloop.ParseCompactionTrigger(cfg.CompactionTrigger)
		if loop.Compactor == nil {
			loop.Compactor = agentloop.NewCompactor(nil)
		}
		loop.Compactor.Configure(agentloop.CompactorConfig{
			Mode:             mode,
			Trigger:          trigger,
			PreserveEvidence: cfg.CompactionPreserveEvidence,
			RecentTurns:      cfg.CompactionRecentTurns,
			MaxTokens:        cfg.CompactionMaxTokens,
		})
		if cfg.ContextWindow > 0 {
			loop.ContextWindow = cfg.ContextWindow
		}
	}

	// FrustrationDetector (issue #271): opt-in via config
	// agentloop.frustration_detection. Appends a system-prompt suffix
	// when user frustration is detected.
	if cfg.FrustrationDetectionEnabled {
		loop.Frustration = agentloop.NewFrustrationDetector()
	}

	// LoopDetector / Observer (issue #377): opt-in via config
	// agentloop.observer_*. Wires a LoopDetector that refrains from
	// dispatching any tool call that would close a repeated-sequence
	// cycle. Window <= 0 disables detection entirely so legacy
	// callers see no behaviour change.
	if cfg.ObserverWindow > 0 {
		loop.LoopDetector = agentloop.NewLoopDetector(
			cfg.ObserverWindow,
			cfg.ObserverMinPatternLength,
			cfg.ObserverMinRepeats,
		)
	}

	if cfg.MemoryPrimeEnabled {
		memStore := cfg.MemoryStore
		if memStore == nil {
			if s, err := memory.Open(""); err == nil {
				memStore = s
			}
		}
		if memStore != nil {
			store := memStore
			loop.MemoryPrime = func(ctx context.Context, prompt string) (string, error) {
				return store.Prime(prompt, cfg.Workspace, 10)
			}
		}
	}

	// Session-start context injection (issue #379): when enabled, assemble a
	// unified preamble from todos, the previous session summary, and auto-memory
	// and prepend it to the first user message of a new session.
	var todoStore *todo.Store
	if sinCfg, err := internal.LoadMergedConfig(); err == nil && sinCfg.AgentLoopSessionContextEnabled {
		if ts, terr := todo.Open(""); terr == nil {
			todoStore = ts
		}
		loop.SessionContext = NewDefaultSessionContextBuilder(
			cfg.Workspace,
			todoStore,
			cfg.SessionID,
			ledgerStore,
			nil,
			nil,
			nil,
			"",
		)
	}

	// SIN Fusion v1 (issue #290): wire verify-tournament when enabled.
	WireFusion(loop, cfg, gate, client, memStore, ledgerStore, hookEngine)

	cleanup := func() error {
		mcpMgr.Close()
		if ledgerStore != nil {
			_ = ledgerStore.Close()
		}
		_ = headroomHook.Close()

		if todoStore != nil {
			_ = todoStore.Close()
		}
		return nil
	}
	return loop, cleanup, nil
}
