// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loop is refactored
// Purpose: Loop struct definition extracted from loop.go for readability.
// Pure file split, same package, no behavioural change.
package agentloop

import (
	"context"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

type Loop struct {
	Gate      *verify.Gate
	LocalTool LocalToolFunc
	LocalSpec []ToolSpec

	// ToolStart/ToolEnd are optional live callbacks fired around every
	// local tool invocation. They are the foundation for live TUI tool
	// tree updates and headless structured progress output.
	ToolStart func(ctx context.Context, tc ToolCall)
	ToolEnd   func(ctx context.Context, tc ToolCall, duration time.Duration, output string, err error)

	// ProgressWriter emits structured NDJSON progress events for headless
	// consumers. Optional — nil disables progress output.
	ProgressWriter *ProgressWriter

	Workspace string
	MaxTurns  int
	// BeforeMutate, if set, is called before a mutating tool
	// (sin_write / sin_edit) executes, with the workspace-relative
	// path it will change. The loopbuilder wires this to
	// checkpoint.Store.Capture so every edit is auto-snapshotted and
	// rewind-able. Optional — nil disables auto-checkpoint.
	// (issue #194)
	BeforeMutate func(ctx context.Context, tool, path string)
	// MaxStopRejects caps how many times the stop-gate can reject
	// completion before the run errors. Zero falls back to the
	// default of 3. Independent of StallThreshold (issue #150):
	// MaxStopRejects is a hard count, StallThreshold is an
	// identical-criteria fingerprint count.
	MaxStopRejects int
	SessionID      string
	// LoopDetector, if set, observes tool calls during the run and flags
	// observer loops (repeated identical tool calls or repeated sequences).
	// When a loop is detected the loop returns a non-nil error. Issue #377.
	LoopDetector *LoopDetector
	// SystemPrompt is prepended to every model request as a system
	// message. It is immutable for the lifetime of the loop (mandate M7).
	SystemPrompt string
	Completion   func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error)

	Hooks   *hooks.Engine
	Perm    *permission.Engine
	Ask     AskFunc
	Lessons *lessons.Store

	// StopGate, if set, is consulted when the worker proposes completion and
	// the verify-gate has already passed. If it returns Complete=false the
	// loop re-injects the open criteria and keeps working instead of
	// returning DONE. Optional — nil keeps the legacy single-gate behavior.
	StopGate StopGate

	// StallThreshold escalates early when the stop-gate returns the SAME set
	// of open criteria this many times consecutively (no progress). Zero
	// disables stall detection. Recommended: 3. Independent of MaxStopRejects.
	StallThreshold int

	// MaxTokens is a hard cap on cumulative tokens (prompt+completion) across
	// the whole run. Zero means unlimited. When exceeded the run checkpoints
	// (if AllowContinuation) or errors, rather than continuing to spend.
	// Issue #151.
	MaxTokens int

	// BudgetWarnRatio, if set, fires hooks.BudgetWarn once when token usage
	// crosses this fraction of MaxTokens (e.g. 0.8). Useful for alerting.
	BudgetWarnRatio float64

	// ThinkingEnabled flips the wire-side "thinking" block on per request
	// (Claude / Anthropic-style providers on NIM / OpenRouter gateways).
	// When true, the provider adapter sends thinking{type:"enabled"}.
	// Pure wire-side flag — does NOT affect the gate, only the request shape.
	// Issue: Thinking Budget Enforcement (first PR).
	ThinkingEnabled bool

	// ThinkingBudgetPerRequest is the per-request reasoning-token cap sent
	// on the wire as thinking.budget_tokens (when ThinkingEnabled is also
	// true). 0 means "unlimited / provider default". Zero does NOT disable
	// the wire field, only the cap.
	// Issue: Thinking Budget Enforcement (first PR).
	ThinkingBudgetPerRequest int
	// thinkingUsed is the running per-run accumulator of the
	// Completion.Usage.ThinkingTokens returned by the model. It lives
	// only on the Loop instance and is reset by re-entering Run, so two
	// concurrent Run invocations on the same Loop would race — the loop
	// is documented to be one-Run-at-a-time (mandate M7).
	thinkingUsed int // unexported per-run accumulator

	// PerTurnBudget caps total tokens for a SINGLE model turn (issue #375). 0=unlimited.
	PerTurnBudget int
	// PerTurnThinkingBudget caps reasoning tokens for a SINGLE model turn (issue #375). 0=unlimited.
	PerTurnThinkingBudget int
	// perTurnBudget: lazy-constructed on first Run with at least one non-zero cap. Race-clean (M7).
	perTurnBudget *PerTurnBudget

	// Reflector, if set, runs a self-critique pass right BEFORE the stop-gate.
	// If it returns issues, the loop injects them and continues working — a
	// cheap quality lift that reduces stop-gate rejections. Runs at most once
	// per proposed completion to avoid infinite self-doubt loops.
	// Issue #152.
	Reflector Reflector

	// AllowContinuation switches the maxTurns outcome from a hard error to a
	// checkpointed, resumable Result (Continuation=true). Daemons set this so
	// a long task is re-enqueued and resumed rather than abandoned; one-shot
	// CLI callers leave it false to preserve the legacy error.
	AllowContinuation bool

	// Preamble, if set, is injected as a user message before the goal prompt.
	// The SinCode Loop System uses it to state the Definition-of-Done up front
	// (write tests, no debug leftovers, finish the job, keep docs in sync) so
	// the worker does that work proactively instead of waiting to be told. It
	// is advisory; the stop-gate independently enforces the same contract.
	Preamble string

	// CompressMessages, if set, is invoked on the message history before
	// every model request to reduce token usage (e.g. via Headroom). It
	// returns a possibly-rewritten history; on error or nil result the
	// original history is used so compression never breaks a run.
	CompressMessages func(ctx context.Context, msgs []session.Message) ([]session.Message, error)

	// Ledger records every prompt, tool call, and verification result for
	// auditability and auto-summaries (issue #43). Optional — loop works
	// without it for backward compatibility.
	Ledger *ledger.Store

	// GoalID is the autonomous goal identifier this run belongs to. Empty for
	// interactive chat runs. It is forwarded into ledger tool-usage records.
	GoalID string

	// Coverage, if set, tracks required/forbidden tool usage and rejects
	// completion when constraints are violated (issue #248). The loop
	// creates it automatically when CoverageRequiredTools or
	// CoverageForbiddenTools are non-empty; callers may also set it
	// directly for dataset runners.
	Coverage *ToolCoverageEnforcer

	// CoverageRequiredTools lists tools the model must invoke before the
	// run can complete. Comma-separated via CLI; set directly for tests.
	CoverageRequiredTools []string

	// CoverageForbiddenTools lists tools that block completion if invoked.
	CoverageForbiddenTools []string

	// ResultPolicy, if set, scans every tool result for secret leakage,
	// destructive operations, and network egress (issue #374).
	ResultPolicy *permission.ResultPolicy

	// Observer, when set, observes tool calls via Observe/LastTrip
	// for full fingerprint-based loop detection (issue #377).
	Observer *LoopDetector

	// RunOverride, if set, replaces the default Run. Used by the
	// WebUI v2 chat API (issue #52) so tests can swap in a
	// deterministic result without wiring a real LLM.
	RunOverride func(ctx context.Context, sess *session.Session, prompt string) (*Result, error)

	// TournamentRunner, if set, is invoked on verify.fail instead of
	// the legacy same-model retry. It fans the task out to N providers
	// in parallel; the first to pass the verify-gate wins. Optional —
	// nil preserves exact legacy behavior. Only active when verify_mode
	// == "poc" (issue #290).
	TournamentRunner TournamentRunner

	// Frustration, when set, tracks user message patterns for frustration
	// signals and appends a system-prompt suffix when detected (issue #271).
	// Optional — nil preserves legacy behavior.
	Frustration *FrustrationDetector

	// Compactor, when set, is consulted before each turn to compact the
	// message history when it exceeds the compaction threshold (issue #278).
	// Optional — nil preserves legacy behavior. Chained after
	// CompressMessages if both are set.
	Compactor *Compactor
	// CompactionStrategy controls which compaction algorithm the Compactor
	// uses. Default is CompactionHybrid. Only used when Compactor is set
	// AND ContextCompactionMode == off (legacy callers).
	CompactionStrategy CompactionStrategy
	// CompactionMaxTokens is the token budget for compacted messages.
	// Zero defaults to 8000.
	CompactionMaxTokens int

	// ContextCompactionMode selects the compaction algorithm. Closed set:
	// off | deterministic | llm | hybrid. Empty == off (legacy behaviour).
	// When non-off, mode-based compaction replaces strategy-based and
	// evidence preservation is applied (issue: compaction-modes, M3).
	ContextCompactionMode ContextCompactionMode
	// CompactionTrigger decides when the compactor fires per turn.
	// turns > msg count / maxTurns / threshold;
	// tokens > estimated tokens / ctxWindow / threshold;
	// both > OR of the above.
	CompactionTrigger CompactionTrigger
	// CompactionThreshold is the fraction of the cap at which compaction
	// fires. Default 0.8 = 80%.
	CompactionThreshold float64
	// ContextWindow is the effective token cap for compaction. Zero means
	// "auto": use max(LLMMaxTokens, CompactionMaxTokens * 4) so an unset
	// window still produces a sensible signal.
	ContextWindow int
	// CompactionPreserveEvidence enables evidence-preserving retain rules
	// in deterministic and hybrid modes. Default true (mandate M3: keep
	// verification evidence intact across compaction).
	CompactionPreserveEvidence bool
	// CompactionRecentTurns is the number of recent human turns (≈2x
	// messages) the retain rule keeps. Default 4.
	CompactionRecentTurns int

	// MemoryPrime, when set, is called before the first turn to inject
	// relevant memories from the long-term store into the conversation.
	// The returned string is appended as a user message before the prompt.
	// Optional — nil preserves legacy behavior.
	MemoryPrime func(ctx context.Context, prompt string) (string, error)

	// SessionContextBuilder, when set, is called once at session start
	// to assemble a markdown block aggregating top-K entries from
	// lessons, memory, and pending goals. The block is appended as a
	// user message immediately BEFORE the goal prompt so the worker
	// reads it on the first turn. Privacy-first — off unless the
	// caller wires an explicit ContextInjector (issue #379). Optional —
	// nil preserves legacy behavior.
	SessionContextBuilder func(ctx context.Context, prompt string) (string, error)

	// SessionContext, when set, is consulted at the start of a new session
	// (empty history) to build a unified preamble from todos, previous
	// session summary, and auto-memory (issue #379). Nil preserves legacy
	// behavior.
	SessionContext *SessionContextBuilder
}
