// SPDX-License-Identifier: MIT
// Purpose: shared loop factory — eliminates duplication of provider /
// permission / hooks / gate / mcp / memory setup across chat / swarm /
// serve (issue #64, DRY refactor).
package loopbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/fusion"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/stopgate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

type Config struct {
	Workspace   string
	SessionID   string
	AgentName   string
	Model       string
	BaseURL     string
	MaxTurns    int
	VerifyMode  string
	VerifyCmd   string
	Yolo        bool
	Headless    bool
	Style       string
	AskFunc     agentloop.AskFunc
	LocalTool   agentloop.LocalToolFunc
	LocalSpec   []agentloop.ToolSpec
	ToolFactory func(*mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec)
	SkipMCP     bool

	// Contract, when non-nil and non-empty, activates the stop-gate: the
	// worker's "done" is confirmed against this Definition-of-Done by an
	// independent hybrid evaluator before the loop returns DONE.
	Contract *goalcontract.GoalContract
	// AllowContinuation switches the maxTurns outcome from a hard error to a
	// resumable checkpoint (used by the daemon).
	AllowContinuation bool

	// GoalID is an optional identifier for the autonomous goal that owns this
	// run. It is forwarded into ledger tool-usage records.
	GoalID string

	SessionStore *session.Store

	// CoverageRequiredTools and CoverageForbiddenTools are passed through
	// to the agent loop's tool-coverage enforcer (issue #248).
	CoverageRequiredTools  []string
	CoverageForbiddenTools []string

	// ActiveSkills lists skill names whose `required_tools` frontmatter
	// field should be merged into CoverageRequiredTools (additive,
	// deduplicated). The skills are looked up in the embedded
	// skills.ListFS(). Non-skill rule names are silently skipped.
	// See skillmgr.MergeRequiredTools (issue #248 skill activation path).
	ActiveSkills []string

	// SIN Fusion v1 (issue #290): when FusionEnabled is true and ≥2
	// providers are available, a verify-tournament is wired into the
	// loop. On verify.fail, the task is fanned out to N providers in
	// parallel; the first to pass the PoC gate wins.
	FusionEnabled             bool
	FusionProviders           []string
	FusionMaxCostUSD          float64
	FusionMinQuorum           int
	FusionPerProviderTimeoutS int
	FusionDifficultyGate      bool
	FusionOracleMode          bool
	FusionMode                fusion.Mode // issue #394: explicit mode override ("poc" | "oracle" | "plan-merge")
	FusionProfilesDir         string

	// DeepPlanner: when true, the orchestrator uses the parallel DAG
	// DeepPlanner instead of the legacy linear Planner (issue #282).
	// Also activated by SIN_DEEP_PLANNER=1 or config
	// orchestrator.deep_planner=true.
	DeepPlannerEnabled bool

	// PatternLearning: when true, completed plans are recorded into a
	// PatternDB and matched patterns feed into the DeepPlanner (issue #288).
	// Also activated by config orchestrator.pattern_learning=true.
	PatternLearningEnabled bool

	// PreWarmEnabled: when true, the dispatcher pre-warms dependent agents
	// before their dependencies complete (issue #285).
	// Also activated by config orchestrator.prewarm=true.
	PreWarmEnabled bool

	// CompactionStrategy: when non-empty, wires a Compactor into the agent
	// loop with the named strategy (issue #278).
	// Also activated by config agentloop.compaction_strategy=<strategy>.
	CompactionStrategy string

	// FrustrationDetection: when true, wires a FrustrationDetector into the
	// agent loop (issue #271).
	// Also activated by config agentloop.frustration_detection=true.
	FrustrationDetectionEnabled bool

	// YoloRiskThreshold: when non-empty and Yolo is true, wires a
	// RiskClassifier into the permission engine so YOLO auto-approves
	// only low/medium/high risk tools (issue #272).
	// Also activated by config permission.yolo_risk_threshold=<level>.
	YoloRiskThreshold string

	// ThinkingEnabled flips the wire-side "thinking" block on per request
	// (Claude / Anthropic-style providers on NIM / OpenRouter gateways).
	// Also activated by config llm.thinking_enabled=true.
	// Issue: Thinking Budget Enforcement (first PR).
	ThinkingEnabled bool

	// ThinkingBudgetPerRequest is the per-request reasoning-token cap
	// sent on the wire as thinking.budget_tokens (when ThinkingEnabled
	// is true). 0 = unbounded / provider default.
	// Also activated by config llm.thinking_budget=<n>.
	// Issue: Thinking Budget Enforcement (first PR).
	ThinkingBudgetPerRequest int

	// MemoryPrimeEnabled: when true, wires a MemoryPrime function that
	// queries the long-term memory store and injects relevant memories
	// into the conversation before the first turn.
	// Also activated by config memory.prime_on_start=true.
	MemoryPrimeEnabled bool

	// MemoryStore is the long-term memory store used for MemoryPrime.
	// When nil and MemoryPrimeEnabled is true, Build opens a default store.
	MemoryStore *memory.Store

	// EpisodicMemoryEnabled: when true, the orchestrator records verified
	// plans as episodes and injects similar past episodes as a planning
	// prior on new plan creation.
	// Also activated by config orchestrator.episodic_memory=true.
	EpisodicMemoryEnabled bool
}

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

	baseURL := firstNonEmpty(cfg.BaseURL, agentCfg.BaseURL,
		os.Getenv("SIN_LLM_BASE_URL"), "https://integrate.api.nvidia.com/v1")
	apiKey := firstNonEmpty(os.Getenv("SIN_LLM_API_KEY"),
		os.Getenv("NVIDIA_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	model := firstNonEmpty(cfg.Model, agentCfg.Model, os.Getenv("SIN_LLM_MODEL"))
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
	runner := commandRunner(cfg.VerifyCmd)
	gate := verify.NewGate(mode, runner, runner)

	mcpMgr := mcpclient.NewManager(mcpclient.LoadConfigs(cfg.Workspace))
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
		Gate:                    gate,
		LocalTool:               localTool,
		LocalSpec:               localSpec,
		Workspace:               cfg.Workspace,
		MaxTurns:                cfg.MaxTurns,
		SessionID:               cfg.SessionID,
		GoalID:                  cfg.GoalID,
		SystemPrompt:            style.RenderSystemPrompt(cfg.Style),
		Completion:              completion,
		Hooks:                   hookEngine,
		Perm:                    perm,
		Ask:                     cfg.AskFunc,
		Lessons:                 memStore,
		Ledger:                  ledgerStore,
		CoverageRequiredTools:   cfg.CoverageRequiredTools,
		CoverageForbiddenTools:  cfg.CoverageForbiddenTools,
		ThinkingEnabled:         thinkingCfg.Enabled,
		ThinkingBudgetPerRequest: thinkingCfg.Budget,
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

	// FrustrationDetector (issue #271): opt-in via config
	// agentloop.frustration_detection. Appends a system-prompt suffix
	// when user frustration is detected.
	if cfg.FrustrationDetectionEnabled {
		loop.Frustration = agentloop.NewFrustrationDetector()
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

	// SIN Fusion v1 (issue #290): wire verify-tournament when enabled.
	WireFusion(loop, cfg, gate, client, memStore, ledgerStore, hookEngine)

	cleanup := func() error {
		mcpMgr.Close()
		if ledgerStore != nil {
			_ = ledgerStore.Close()
		}
		_ = headroomHook.Close()
		return nil
	}
	return loop, cleanup, nil
}

// OrchestratorDeps holds the standalone orchestrator components wired by
// WireOrchestrator. Callers use these to build a Dispatcher with
// pre-warming, feed patterns to the DeepPlanner, and record completed
// plans into the PatternDB.
type OrchestratorDeps struct {
	DeepPlanner *orchestrator.DeepPlanner
	PreWarm     *orchestrator.PreWarmManager
	PatternDB   *orchestrator.PatternDB
}

// WireOrchestrator builds and wires the standalone orchestrator components
// (DeepPlanner, PreWarmManager, PatternDB) based on the Config flags. Returns
// an OrchestratorDeps struct; fields are nil when the corresponding feature
// is not enabled (backward compat: everything is opt-in).
//
// When DeepPlannerEnabled is true, a DeepPlanner replaces the legacy linear
// Planner. When PatternLearningEnabled is true, a PatternDB is created and
// injected into the DeepPlanner via SetPatternDB so learned patterns refine
// probability scores. When PreWarmEnabled is true, a PreWarmManager is
// created for use with a Dispatcher.
func WireOrchestrator(cfg Config, registry *orchestrator.Registry) *OrchestratorDeps {
	deps := &OrchestratorDeps{}
	if !cfg.DeepPlannerEnabled {
		return deps
	}
	agents := orchestrator.DefaultAgents()
	if registry != nil {
		agents = registry.List()
	}
	deps.DeepPlanner = orchestrator.NewDeepPlanner(agents)

	if cfg.PatternLearningEnabled {
		deps.PatternDB, _ = orchestrator.NewPatternDB(nil)
		deps.DeepPlanner.SetPatternDB(deps.PatternDB)
	}

	if cfg.PreWarmEnabled && registry != nil {
		deps.PreWarm = orchestrator.NewPreWarmManager(registry, 0, 0)
	}

	return deps
}

// RecordPlanCompletion records a completed plan into the PatternDB if
// pattern learning is enabled. This should be called after a plan's
// dispatch completes (success or failure). No-op when deps.PatternDB
// is nil (pattern learning disabled).
func (deps *OrchestratorDeps) RecordPlanCompletion(ctx context.Context, plan *orchestrator.Plan) {
	if deps == nil || deps.PatternDB == nil || plan == nil {
		return
	}
	_ = deps.PatternDB.RecordSequence(ctx, plan)
}

// WireFusion wires a SIN Fusion v1 verify-tournament (issue #290) into
// an existing agentloop.Loop. It is extracted from Build so callers that
// construct their loop manually (e.g. chat_cmd.go) can opt into fusion
// without duplicating the wiring logic.
//
// Only active when cfg.FusionEnabled is true and the gate is in PoC or Oracle
// mode (issue #344). Oracle mode requires explicit FusionOracleMode=true and
// wires a judge that evaluates all candidates together, not first-pass-wins.
// Requires >=2 providers from the Fireworks pool; otherwise the call is a
// no-op and the loop keeps legacy behavior.
func WireFusion(loop *agentloop.Loop, cfg Config, gate *verify.Gate, client *llm.Client,
	memStore *lessons.Store, ledgerStore *ledger.Store, hookEngine *hooks.Engine) {
	if !cfg.FusionEnabled || (gate.Mode() != verify.ModePoC && gate.Mode() != verify.ModeOracle) {
		return
	}
	providers := fusion.LoadFireworksPool(nil, cfg.FusionProviders)
	if len(providers) < 2 {
		return
	}
	forkFunc := fusion.ForkFunc(nil)
	runFunc := fusion.RunFunc(nil)
	if cfg.SessionStore != nil {
		forkFunc = func(srcSessionID string, turn int) (*session.Session, error) {
			return cfg.SessionStore.Fork(srcSessionID, turn)
		}
		runFunc = func(ctx context.Context, prov fusion.ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
			provClient := llm.NewClient(prov.BaseURL, prov.APIKey)
			provCompletion := agentloop.NewProviderCompletion(provClient, prov.Model, prov.MaxTokens, 0)
			provLoop := &agentloop.Loop{
				Gate:         gate,
				LocalTool:    loop.LocalTool,
				LocalSpec:    loop.LocalSpec,
				Workspace:    loop.Workspace,
				MaxTurns:     loop.MaxTurns,
				SessionID:    sess.ID,
				SystemPrompt: loop.SystemPrompt,
				Completion:   provCompletion,
				Hooks:        hookEngine,
				Perm:         loop.Perm,
				Lessons:      memStore,
				Ledger:       ledgerStore,
			}
			return provLoop.Run(ctx, sess, prompt)
		}
	}
	mode := fusion.ModeOracle // issue #394: Oracle is the default (quality over cost)
	if cfg.FusionMode != "" {
		mode = cfg.FusionMode // explicit override
	}
	maxCost := cfg.FusionMaxCostUSD
	if cfg.FusionOracleMode && maxCost > 2.0 {
		// Oracle mode defaults to a tighter cap unless explicitly higher.
		maxCost = 2.0
	}
	tournament := &fusion.Tournament{
		Providers:          providers,
		MaxCostUSD:         maxCost,
		MinQuorum:          cfg.FusionMinQuorum,
		PerProviderTimeout: time.Duration(cfg.FusionPerProviderTimeoutS) * time.Second,
		Workspace:          cfg.Workspace,
		SourceSessionID:    cfg.SessionID,
		Lessons:            memStore,
		Ledger:             ledgerStore,
		Hooks:              hookEngine,
		HookSessionID:      cfg.SessionID,
		VerifyFn:           func(ctx context.Context, ws string) verify.Result { return gate.Run(ctx, ws) },
		ForkFunc:           forkFunc,
		RunFunc:            runFunc,
		Mode:               mode,
	}
	if mode == fusion.ModeOracle {
		judgeModel := firstNonEmpty(os.Getenv("SIN_EVALUATOR_MODEL"), cfg.Model)
		judgeClient := client
		if evalBase := os.Getenv("SIN_EVALUATOR_BASE_URL"); evalBase != "" {
			judgeClient = llm.NewClient(evalBase, os.Getenv("SIN_EVALUATOR_API_KEY"))
		}
		judge := fusion.NewLLMOracleJudge(judgeClient, judgeModel)
		tournament.OracleJudge = judge.Judge
	}
	if mode == fusion.ModePlanMerge {
		judgeModel := firstNonEmpty(os.Getenv("SIN_EVALUATOR_MODEL"), cfg.Model)
		judgeClient := client
		if evalBase := os.Getenv("SIN_EVALUATOR_BASE_URL"); evalBase != "" {
			judgeClient = llm.NewClient(evalBase, os.Getenv("SIN_EVALUATOR_API_KEY"))
		}
		mergeJudge := fusion.NewLLMPlanMergeJudge(judgeClient, judgeModel)
		tournament.PlanMergeJudge = mergeJudge.Merge
	}
	loop.TournamentRunner = &fusionAdapter{t: tournament, gate: gate, cfg: cfg, client: client, memStore: memStore}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func loadHooks(workspace string) []hooks.Hook {
	var all []hooks.Hook
	paths := []string{}
	if cfg, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.json"))
	}
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.json"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var hs []hooks.Hook
		if err := json.Unmarshal(data, &hs); err != nil {
			fmt.Fprintf(os.Stderr, "warn: skipping invalid hooks file %s: %v\n", p, err)
			continue
		}
		all = append(all, hs...)
	}
	return all
}

func commandRunner(command string) verify.Runner {
	if command == "" {
		return nil
	}
	return func(ctx context.Context, workspace string) (bool, string, error) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(cctx, "sh", "-c", command)
		cmd.Dir = workspace
		out, err := cmd.CombinedOutput()
		report := strings.TrimSpace(string(out))
		if err != nil {
			return false, report, nil
		}
		return true, report, nil
	}
}

// fusionAdapter wraps a fusion.Tournament to satisfy the
// agentloop.TournamentRunner interface. It bridges the loop's
// ShouldRun/Run calls to the tournament's internal logic, injecting
// the fork function and run function that require loopbuilder-scoped
// dependencies (session store, llm client, etc.) (issue #290).
type fusionAdapter struct {
	t        *fusion.Tournament
	gate     *verify.Gate
	cfg      Config
	client   *llm.Client
	memStore *lessons.Store
}

func (a *fusionAdapter) ShouldRun(vr verify.Result) bool {
	if !a.cfg.FusionDifficultyGate {
		return !vr.Passed
	}
	return fusion.ShouldTournament(vr)
}

func (a *fusionAdapter) Run(ctx context.Context, prompt string) (string, int, error) {
	if a.t.ForkFunc == nil || a.t.RunFunc == nil {
		return "", 0, fmt.Errorf("fusion: tournament not fully wired (phase 1 — fork/run funcs nil)")
	}
	a.t.Prompt = prompt
	if a.t.SourceSessionID == "" {
		a.t.SourceSessionID = a.cfg.SessionID
	}
	result, err := a.t.Run(ctx)
	if err != nil {
		return "", 0, err
	}
	if result.Winner == nil {
		return "", 0, fmt.Errorf("fusion: no winner")
	}
	return result.Winner.Output, result.Winner.TokensUsed, nil
}
