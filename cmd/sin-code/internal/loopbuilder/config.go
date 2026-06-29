// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loopbuilder is refactored
// Purpose: Config struct for the shared loop factory (issue #64, DRY refactor).
// Extracted from builder.go to keep each file ≤500 lines.
package loopbuilder

import (
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/fusion"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
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

	// ContainerRunner, when non-nil, executes the verify command inside a
	// container. ContainerImage is the image passed to the runner. Together they
	// implement containerized autonomous goals (issue #389).
	ContainerRunner autonomy.ContainerRunner
	ContainerImage  string

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
	FusionMode                fusion.Mode // issue #394
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

	// SelfReviewEnabled: when true (default), wires the SelfReviewReflector
	// into the agent loop. The reflector scans changed files for
	// TODO/FIXME/dummy/stub markers after the verify-gate passes but before
	// the stop-gate, forcing the agent to fix incomplete-work markers before
	// reporting completion. This is the "Ultra-CEO" doctrine baked into the
	// loop — every agent self-reviews automatically without the user having
	// to ask.
	// Also deactivated by config agentloop.self_review=false.
	SelfReviewEnabled bool

	// ContextCompactionMode selects the compaction algorithm (issue: compaction-modes).
	// off | deterministic | llm | hybrid. Empty = off (legacy behaviour).
	ContextCompactionMode string

	// CompactionTrigger decides when the compactor fires per turn.
	// turns | tokens | both. Default tokens.
	CompactionTrigger string

	// CompactionMaxTokens is the token budget for compacted messages. Default 8000.
	CompactionMaxTokens int

	// ContextWindow is the effective token cap for compaction. 0 = auto.
	ContextWindow int

	// CompactionPreserveEvidence enables evidence-preserving retain rules (M3). Default true.
	CompactionPreserveEvidence bool

	// CompactionRecentTurns is the number of recent human turns to retain. Default 4.
	CompactionRecentTurns int

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

	RepetitionThreshold int
	RepetitionWindow    int

	// ObserverWindow: rolling-history size for the LoopDetector
	// (issue #377). Defaults to 20 when zero.
	ObserverWindow int
	// ObserverMinPatternLength: minimum repeating pattern length.
	ObserverMinPatternLength int
	// ObserverMinRepeats: minimum repeat count to trip the detector.
	ObserverMinRepeats int

	// AutoCommit, when true, creates a git commit with a conventional
	// commit message after the verification gate passes (issue #487).
	// M3: the commit happens ONLY after verification — never before.
	// M4: --auto-commit explicitly grants permission for the session.
	AutoCommit bool

	// CommitPrefix overrides the auto-detected conventional commit
	// prefix. When empty, the prefix is inferred from the prompt content
	// (feat, fix, refactor, docs, etc.). When set, it is used verbatim.
	CommitPrefix string

	// AgentMode selects a specialized sub-agent mode (issue #485).
	// Empty or "default" preserves exact current behavior (non-breaking).
	// Other modes filter the tool set and prepend a mode-specific system
	// prompt. M4: mode filtering is additive — the permission engine still
	// gates every tool call regardless of mode.
	AgentMode string
}
