// SPDX-License-Identifier: MIT
// Purpose: SIN-Code core agent loop: PLAN -> ACT -> VERIFY -> DONE
// (mandates C1, C3, AGENTS.md §8). Hook engine (C7) and permission
// engine (M4) are wired at all documented event points (issues #46, #47).
package agentloop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

type Completion struct {
	Text      string
	ToolCalls []ToolCall
	Raw       session.Message
	// Usage carries token accounting returned by the model provider. All
	// fields optional; zero values mean "unknown" and never trigger the
	// budget guard (issue #151).
	Usage Usage
}

// Usage carries token accounting returned by the model provider. All fields
// optional; zero values mean "unknown" and never trigger the budget guard.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// ThinkingTokens is the count of tokens the model spent on its
	// internal reasoning phase (Claude / Anthropic-style providers,
	// OpenRouter gateways). Zero means "unknown / not surfaced" and
	// is never treated as a budget signal.
	ThinkingTokens int
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type LocalToolFunc func(ctx context.Context, name string, args map[string]any) (string, error)

type AskFunc func(tc ToolCall) bool

// StopSnapshot is the read-only view of a run handed to the stop-gate when
// the worker proposes completion (no more tool calls AND the verify-gate
// passed). It carries just enough signal for an independent evaluator to
// decide whether the goal is truly done.
type StopSnapshot struct {
	Prompt       string
	FinalOutput  string
	Turns        int
	ToolsUsed    []string
	VerifyPassed bool
	SessionID    string
}

// StopDecision is the verdict returned by a StopGate. Complete=false forces
// the loop to keep working, re-injecting OpenCriteria as the next instruction.
type StopDecision struct {
	Complete     bool
	OpenCriteria []string
	Report       string
}

// StopGate decouples completion authority from the worker. It is consulted
// only AFTER the verify-gate passes, and may reject the proposed completion
// (Complete=false) to force continued work — the core anti-babysitting hook.
// A nil StopGate preserves the legacy behavior exactly.
type StopGate func(ctx context.Context, snap StopSnapshot) StopDecision

// Reflection is the worker's self-critique of a proposed completion. Issues
// non-empty means the agent found problems in its own work and should fix
// them before the stop-gate is consulted.
type Reflection struct {
	Issues []string
	Notes  string
}

// Reflector performs a self-critique pass on a proposed completion. Returning
// a Reflection with non-empty Issues forces one more work turn. A nil
// Reflector disables the reflection step (legacy behavior).
// Issue #152.
type Reflector func(ctx context.Context, snap StopSnapshot) Reflection

type Loop struct {
	Gate      *verify.Gate
	LocalTool LocalToolFunc
	LocalSpec []ToolSpec
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

	// Observer, when set, refuses the dispatch of any tool call that
	// completes a repeated-sequence cycle (issue #377). When the
	// detector trips the loop fires hooks.LoopDetected with the
	// captured (pattern_length, repeats, tool, key) and surfaces a
	// "TOOL REFUSED" tool-result message so the model can break the
	// cycle on the next turn. A disabled detector (Window <= 0) is a
	// no-op so legacy callers are unaffected.
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

	// ResultPolicy, if set, scans the string returned by every executed
	// tool and surfaces warnings/escalations for secret leakage,
	// destructive confirmations, or network egress markers (issue #374).
	// Optional — nil preserves exact legacy behavior.
	ResultPolicy *permission.ResultPolicy

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

	// SessionContext, when set, is consulted at the start of a new session
	// (empty history) and its non-empty preamble is injected as a user
	// message before the Definition-of-Done preamble and the goal prompt.
	// Optional — nil preserves legacy behavior. Gated by the caller via
	// agentloop.session_context.enabled (issue #379).
	SessionContext *SessionContextBuilder
}

// TournamentRunner is the interface for fusion verify-tournaments (issue
// #290). The loop calls ShouldRun to check if a verify-fail warrants a
// tournament fan-out, and Run to execute it. The prompt is passed so the
// tournament can fan it out to each provider — without it, forked sessions
// would run with an empty task. On success, Run returns the winner's output
// and token count. On failure, the loop falls back to the legacy same-model
// retry. Defined here (not in internal/fusion) to avoid a circular import
// (fusion imports agentloop for Result).
type TournamentRunner interface {
	ShouldRun(vr verify.Result) bool
	Run(ctx context.Context, prompt string) (output string, tokens int, err error)
}

// saveHistoryHook is a test seam for injecting a mock around session
// SaveHistory calls. When nil, the real Session.SaveHistory is used.
// This exists so tests can force a save-history error without mocking
// the SQLite store.
var saveHistoryHook func(sess *session.Session, msgs []session.Message) error

func (l *Loop) saveHistory(ctx context.Context, sess *session.Session, msgs []session.Message) error {
	if saveHistoryHook != nil {
		return saveHistoryHook(sess, msgs)
	}
	return sess.SaveHistory(msgs)
}

// buildCompactorConfig materialises a CompactorConfig snapshot from the
// loop-level fields. Defaults are applied via CompactorConfig.Normalize
// inside Configure().
func (l *Loop) buildCompactorConfig() CompactorConfig {
	mode := l.ContextCompactionMode
	if mode == "" {
		mode = ContextCompactionOff
	}
	trigger := l.CompactionTrigger
	if trigger == "" {
		trigger = CompactionTriggerTokens
	}
	if l.CompactionMaxTokens <= 0 {
		l.CompactionMaxTokens = 8000
	}
	return CompactorConfig{
		Mode:             mode,
		Trigger:          trigger,
		Threshold:        l.CompactionThreshold,
		ContextWindow:    l.effectiveContextWindow(l.CompactionMaxTokens),
		MaxTokens:        l.CompactionMaxTokens,
		PreserveEvidence: l.CompactionPreserveEvidence,
		RecentTurns:      l.CompactionRecentTurns,
	}
}

// effectiveContextWindow resolves the loop's ContextWindow field. Zero
// means auto: derive a sensible cap from CompactionMaxTokens so the
// token trigger still emits a meaningful signal.
func (l *Loop) effectiveContextWindow(maxTkns int) int {
	if l.ContextWindow > 0 {
		return l.ContextWindow
	}
	if maxTkns <= 0 {
		maxTkns = 8000
	}
	return maxTkns * 4
}

// shouldFireCompaction combines turns- and tokens-based triggers. Any
// trigger that fires returns true; when both are off (the default) the
// compactor never runs.
//
// Backward-compat: legacy callers configure the threshold directly on the
// Compactor (compactor.Threshold); the loop-level CompactionThreshold is
// only used when the user opted in to the compaction-modes flow. When
// neither is set, we fall back to the original threshold default so a
// single-shot integration test like TestCompactionIntegration_TriggeredAtThreshold
// keeps firing.
func (l *Loop) shouldFireCompaction(maxTurns int, msgs []session.Message) bool {
	if l.Compactor == nil {
		return false
	}
	threshold := l.CompactionThreshold
	if threshold <= 0 && l.Compactor.Threshold > 0 {
		threshold = l.Compactor.Threshold
	}
	if threshold <= 0 && l.Compactor != nil {
		if cfg := l.Compactor.config(); cfg.Threshold > 0 {
			threshold = cfg.Threshold
		}
	}
	if threshold <= 0 {
		threshold = DefaultCompactionThreshold
	}
	trigger := l.CompactionTrigger
	if trigger == "" {
		// Legacy callers that set Compactor.Threshold but neither
		// CompactionStrategy turned-on nor CompactionTrigger want turns
		// semantics to keep the old single-knob behaviour.
		if l.Compactor.Threshold > 0 {
			trigger = CompactionTriggerTurns
		} else {
			trigger = CompactionTriggerTokens
		}
	}
	switch trigger {
	case CompactionTriggerTurns:
		return ShouldCompact(len(msgs), maxTurns, threshold)
	case CompactionTriggerTokens:
		ctxWin := l.effectiveContextWindow(l.CompactionMaxTokens)
		return ShouldCompactTokens(estimateTokens(msgs), ctxWin, threshold)
	case CompactionTriggerBoth:
		ctxWin := l.effectiveContextWindow(l.CompactionMaxTokens)
		return ShouldCompact(len(msgs), maxTurns, threshold) ||
			ShouldCompactTokens(estimateTokens(msgs), ctxWin, threshold)
	}
	return false
}

// compactionSnapshot bundles the inputs/outputs of Compact2 so the loop
// can route between request-only compaction (mode-based) and in-place
// (legacy strategy) with the same code path.
type compactionSnapshot struct {
	mode   ContextCompactionMode
	result CompactResult
}

// compactionSnapshot runs Compact2 with the loop's configured inputs. The
// returned result carries the kept/down/summary fields and may have
// SnapshotID populated when a sidecar was written.
func (l *Loop) compactionSnapshot(ctx context.Context, sess *session.Session, msgs []session.Message) compactionSnapshot {
	mode := l.ContextCompactionMode
	if mode == "" {
		mode = ContextCompactionOff
	}
	maxTkns := l.CompactionMaxTokens
	if maxTkns <= 0 {
		maxTkns = 8000
	}
	res, _ := l.Compactor.Compact2(ctx, CompactInput{
		Messages:        msgs,
		Mode:            mode,
		MaxTokens:       maxTkns,
		EvidenceIndices: identifyEvidence(msgs),
		SessionID:       sess.ID,
	})
	return compactionSnapshot{mode: mode, result: res}
}

// writeCompactionSidecar writes a JSON snapshot of the dropped messages
// to ~/.local/share/sin-code/context-snapshots/<session-hash>/<turn>.json
// so lossy compaction is reversible for debugging and audit (mandate M3:
// verification evidence is preserved AND traceable). Failure to write is
// non-fatal: the loop keeps running and logs the path for forensic use.
func (l *Loop) writeCompactionSidecar(sess *session.Session, turn int, result CompactResult) string {
	if sess == nil || sess.ID == "" {
		return ""
	}
	if len(result.Dropped) == 0 {
		return ""
	}
	if result.Mode == "" || result.Mode == ContextCompactionOff || !result.Mode.IsLossy() {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".local", "share", "sin-code", "context-snapshots", sessionIDHash(sess.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	id := fmt.Sprintf("turn-%05d", turn)
	path := filepath.Join(dir, id+".json")
	payload := map[string]any{
		"session_id":    sess.ID,
		"turn":          turn,
		"mode":          result.Mode.String(),
		"snapshot_id":   id,
		"tokens_before": result.TokensBefore,
		"tokens_after":  result.TokensAfter,
		"summary":       result.Summary,
		"dropped_count": len(result.Dropped),
		"kept_count":    len(result.Kept),
		"dropped":       result.Dropped,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return ""
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	return path
}

// identifyEvidence scans msgs for the canonical evidence markers and
// returns the matching message indices. Used to seed the retain filter
// for callers that do not pass an explicit map.
func identifyEvidence(msgs []session.Message) map[int]bool {
	out := make(map[int]bool)
	for i, m := range msgs {
		if containsEvidence(m.Content) {
			out[i] = true
		}
	}
	return out
}

// sessionIDHash returns the first 12 hex chars of sha256(sessionID),
// the workspace-isolation convention shared with internal/session and
// internal/lessons (issue #265).
func sessionIDHash(sessionID string) string {
	if sessionID == "" {
		return "unknown"
	}
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])[:12]
}

type Result struct {
	SessionID string `json:"session_id"`
	Summary   string `json:"summary"`
	Verified  bool   `json:"verified"`
	Turns     int    `json:"turns"`
	Tokens    int    `json:"tokens,omitempty"`
	// Continuation is true when the run hit maxTurns with AllowContinuation
	// enabled: the work is checkpointed (not failed) and should be resumed.
	Continuation bool `json:"continuation,omitempty"`
	// OpenCriteria carries the unmet acceptance criteria when the run ends
	// without verified completion (stop-gate reject or continuation).
	OpenCriteria []string `json:"open_criteria,omitempty"`
}

func (l *Loop) tools() []ToolSpec { return l.LocalSpec }

func (l *Loop) record(ctx context.Context, typ ledger.EntryType, data map[string]any, summary string) {
	if l.Ledger == nil || l.SessionID == "" {
		return
	}
	_, _ = l.Ledger.Record(ctx, ledger.Entry{
		SessionID: l.SessionID,
		Type:      typ,
		Data:      data,
		Summary:   summary,
	})
}

func (l *Loop) recordUsage(ctx context.Context, name string, outcome ledger.UsageOutcome) {
	if l.Ledger == nil || l.SessionID == "" {
		return
	}
	_ = l.Ledger.RecordUsage(ctx, ledger.UsageRecord{
		ToolName:  name,
		Outcome:   outcome,
		SessionID: l.SessionID,
		GoalID:    l.GoalID,
	})
}

func (l *Loop) fire(ctx context.Context, event, name string, data map[string]any) hooks.Result {
	if l.Hooks == nil {
		return hooks.Result{}
	}
	return l.Hooks.Fire(ctx, hooks.Payload{
		Event:     event,
		SessionID: l.SessionID,
		Workspace: l.Workspace,
		Name:      name,
		Data:      data,
	})
}

func (l *Loop) execute(ctx context.Context, tc ToolCall) (out string, injects []string) {
	pre := l.fire(ctx, hooks.ToolPre, tc.Name, map[string]any{"args": tc.Args})
	injects = append(injects, pre.PromptInjects...)
	if pre.Blocked {
		return "BLOCKED by hook: " + pre.BlockReason, injects
	}

	if l.Perm != nil {
		var pol permission.Policy
		if l.Perm.Risk != nil {
			pol = l.Perm.CheckWithArgs(tc.Name, tc.Args)
		} else {
			pol = l.Perm.Check(tc.Name)
		}
		switch pol {
		case permission.Deny:
			l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "deny"})
			l.recordUsage(ctx, tc.Name, ledger.OutcomeDenied)
			return "DENIED by permission policy", injects
		case permission.Ask:
			ask := l.fire(ctx, hooks.PermissionAsk, tc.Name, map[string]any{"args": tc.Args})
			injects = append(injects, ask.PromptInjects...)
			if ask.Blocked {
				l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "ask", "by": "hook"})
				l.recordUsage(ctx, tc.Name, ledger.OutcomeDenied)
				return "DENIED by hook: " + ask.BlockReason, injects
			}
			if l.Ask == nil || !l.Ask(tc) {
				l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "ask", "by": "user"})
				l.recordUsage(ctx, tc.Name, ledger.OutcomeDenied)
				return "DENIED by user", injects
			}
		case permission.Allow:
		}
	}

	if l.LocalTool == nil {
		return "TOOL ERROR: no LocalTool registered", injects
	}
	if l.BeforeMutate != nil {
		if p := mutatedPath(tc); p != "" {
			l.BeforeMutate(ctx, tc.Name, p)
		}
	}
	res, err := l.LocalTool(ctx, tc.Name, tc.Args)
	if err != nil {
		l.fire(ctx, hooks.ToolError, tc.Name, map[string]any{"error": err.Error()})
		l.record(ctx, ledger.TypeToolError, map[string]any{"tool": tc.Name}, "tool error: "+tc.Name)
		l.recordUsage(ctx, tc.Name, ledger.OutcomeError)
		if l.Lessons != nil {
			_ = l.Lessons.Record(ctx, lessons.Entry{
				Type:      lessons.TypeToolError,
				Workspace: l.Workspace,
				Context:   map[string]any{"tool": tc.Name},
				Lesson:    "Tool " + tc.Name + " failed: " + err.Error(),
			})
		}
		return "TOOL ERROR: " + err.Error(), injects
	}
	postData := map[string]any{"output_bytes": len(res)}
	if p := mutatedPath(tc); p != "" {
		postData["path"] = p
	}
	if l.ResultPolicy != nil {
		action, reason := l.ResultPolicy.ScanResult(tc.Name, res)
		if action != permission.ActionNoOp {
			postData["result_policy_action"] = action.String()
			postData["result_policy_reason"] = reason
			l.record(ctx, ledger.TypePermissionResult, map[string]any{
				"tool":   tc.Name,
				"action": action.String(),
				"reason": reason,
			}, "reactive permission: "+action.String()+" — "+reason)
			if action == permission.ActionEscalate {
				injects = append(injects, "PERMISSION ESCALATION: tool "+tc.Name+" output triggered '"+reason+"'. Stop and review before continuing.")
			} else {
				injects = append(injects, "PERMISSION WARNING: tool "+tc.Name+" output triggered '"+reason+"'.")
			}
		}
	}
	post := l.fire(ctx, hooks.ToolPost, tc.Name, postData)
	injects = append(injects, post.PromptInjects...)
	l.record(ctx, ledger.TypeToolCall, map[string]any{"tool": tc.Name}, "tool call: "+tc.Name)
	l.recordUsage(ctx, tc.Name, ledger.OutcomeOK)
	return res, injects
}

func (l *Loop) Run(ctx context.Context, sess *session.Session, prompt string) (*Result, error) {
	if l.RunOverride != nil {
		return l.RunOverride(ctx, sess, prompt)
	}
	if l.Completion == nil {
		return nil, fmt.Errorf("agentloop: Completion func not wired")
	}
	if l.SessionID == "" {
		l.SessionID = sess.ID
	}
	// Ensure the coverage enforcer exists when constraints are configured.
	// It is recreated per-Run so REPL/dataset reuse of the same Loop gets
	// fresh state for each prompt/test case (issue #248).
	if len(l.CoverageRequiredTools) > 0 || len(l.CoverageForbiddenTools) > 0 {
		l.Coverage = NewToolCoverageEnforcer(l.CoverageRequiredTools, l.CoverageForbiddenTools)
	}
	l.record(ctx, ledger.TypeUserPrompt, map[string]any{"content": prompt}, "user prompt")
	msgs := sess.History()
	// Session-start context injection (issue #379): on a brand-new session
	// (empty history), build the unified preamble from todos, previous
	// session summary, and auto-memory. Errors are non-fatal; the loop
	// continues with the original prompt if the builder fails.
	if l.SessionContext != nil && len(msgs) == 0 {
		if preamble, err := l.SessionContext.Build(ctx); err == nil && strings.TrimSpace(preamble) != "" {
			msgs = append(msgs, session.Message{Role: "user", Content: preamble})
		}
	}
	// SinCode Loop System: state the Definition-of-Done before the goal so the
	// worker addresses tests/debug/docs/completeness proactively. Enforcement
	// still lives in the stop-gate; this only improves first-pass quality.
	if strings.TrimSpace(l.Preamble) != "" {
		msgs = append(msgs, session.Message{Role: "user", Content: l.Preamble})
	}
	msgs = append(msgs, session.Message{Role: "user", Content: prompt})

	if l.Frustration != nil {
		l.Frustration.Track(prompt, time.Now())
	}

	if l.MemoryPrime != nil {
		if primed, err := l.MemoryPrime(ctx, prompt); err == nil && strings.TrimSpace(primed) != "" {
			l.fire(ctx, hooks.MemoryPrime, "", map[string]any{"chars": len(primed)})
			msgs = append(msgs, session.Message{Role: "user", Content: primed})
		}
	}

	// Learning loop closed: inject accumulated workspace lessons before the
	// first turn so the agent never repeats a recorded mistake.
	if l.Lessons != nil {
		briefCtx := map[string]any{"prompt": prompt}
		if briefing, err := l.Lessons.BriefingForContext(ctx, l.Workspace, briefCtx, 10, 2048); err == nil && briefing != "" {
			msgs = append(msgs, session.Message{Role: "user", Content: briefing})
		}
	}

	maxTurns := l.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 80
	}
	tools := l.tools()

	var pendingInjects []string
	var lastText string
	var lastOpen []string
	stopRejects := 0 // tracks how many times the stop-gate rejected completion
	lastCritFingerprint := ""
	stallCount := 0
	totalTokens := 0      // issue #151: cumulative tokens across the run
	warnedBudget := false // fires hooks.BudgetWarn once per run
	// Issue: Thinking Budget Enforcement (first PR). Reset the per-run
	// thinking accumulator so a second Run() on the same Loop instance
	// starts at zero. The Loop itself is documented as one-Run-at-a-time
	// (mandate M7), so we do not need a mutex on this field.
	l.thinkingUsed = 0
	reflectedThisProposal := false
	toolsSeen := map[string]bool{}
	var toolsUsed []string

	for turn := 0; turn < maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(pendingInjects) > 0 {
			msgs = append(msgs, session.Message{
				Role:    "user",
				Content: "HOOK INJECT:\n" + strings.Join(pendingInjects, "\n"),
			})
			pendingInjects = nil
		}
		if l.Compactor != nil {
			if l.shouldFireCompaction(maxTurns, msgs) {
				// Mirror any config drift the compactor holds (e.g. CLI
				// override after NewCompactor) so the loop honours the
				// freshest settings on every turn.
				l.Compactor.Configure(l.buildCompactorConfig())
				maxTkns := l.CompactionMaxTokens
				if maxTkns <= 0 {
					maxTkns = 8000
				}
				ctxWin := l.effectiveContextWindow(maxTkns)
				threshold := l.CompactionThreshold
				if threshold <= 0 {
					threshold = DefaultCompactionThreshold
				}
				cpre := l.fire(ctx, hooks.CompactionPre, "", map[string]any{
					"messages_before": len(msgs),
					"strategy":        l.CompactionStrategy.String(),
					"mode":            l.ContextCompactionMode.String(),
					"trigger":         l.CompactionTrigger.String(),
					"max_tokens":      maxTkns,
					"context_window":  ctxWin,
					"threshold":       threshold,
				})
				// Issue: Context Compaction Modes (first PR). When a
				// non-off Mode is configured we keep the unbounded history
				// in `msgs` (so the persisted session DB stays complete,
				// mandate M3 verification audit) and produce a
				// deterministically compacted view for the model via
				// Compact2. The legacy Compact() path remains unchanged
				// when Mode is off.
				ctxSnapshot := l.compactionSnapshot(ctx, sess, msgs)
				if ctxSnapshot.mode != "" && ctxSnapshot.mode != ContextCompactionOff {
					if path := l.writeCompactionSidecar(sess, turn, ctxSnapshot.result); path != "" {
						cpre.PromptInjects = append(cpre.PromptInjects,
							"[CONTEXT-COMPACTION] prior turns were summarised — see "+path)
					}
				} else if len(ctxSnapshot.result.Kept) != len(msgs) {
					// Legacy in-place compaction: persist the compacted
					// history so future --resume reads the trimmed view.
					msgs = ctxSnapshot.result.Kept
				}
				for _, inj := range cpre.PromptInjects {
					msgs = append(msgs, session.Message{Role: "user", Content: inj})
				}
				if err := l.saveHistory(ctx, sess, msgs); err != nil {
					return nil, err
				}
			}
		}
		reqMsgs := msgs
		if l.CompressMessages != nil {
			if compressed, cerr := l.CompressMessages(ctx, msgs); cerr == nil && compressed != nil {
				reqMsgs = compressed
			}
		}
		// Mandate M6: the tool-preference block is prepended fresh each
		// turn so the model cannot compress it away.
		if l.SystemPrompt != "" {
			sysContent := l.SystemPrompt
			if l.Frustration != nil {
				sysContent += l.Frustration.SystemPromptSuffix()
			}
			reqMsgs = append([]session.Message{{Role: "system", Content: sysContent}}, reqMsgs...)
		}
		resp, err := l.Completion(ctx, reqMsgs, tools)
		if err != nil {
			return nil, fmt.Errorf("turn %d: %w", turn, err)
		}
		msgs = append(msgs, resp.Raw)
		// Token budget accounting (issue #151). Provider usage is optional;
		// if zero we simply skip the guard for that turn.
		if u := resp.Usage.TotalTokens; u > 0 {
			totalTokens += u
		} else {
			totalTokens += resp.Usage.PromptTokens + resp.Usage.CompletionTokens
		}
		if l.MaxTokens > 0 {
			if !warnedBudget && l.BudgetWarnRatio > 0 &&
				float64(totalTokens) >= l.BudgetWarnRatio*float64(l.MaxTokens) {
				warnedBudget = true
				l.fire(ctx, hooks.BudgetWarn, "", map[string]any{
					"total_tokens": totalTokens, "max_tokens": l.MaxTokens,
				})
			}
			if totalTokens >= l.MaxTokens {
				if serr := l.saveHistory(ctx, sess, msgs); serr != nil {
					return nil, serr
				}
				l.fire(ctx, hooks.BudgetExhausted, "", map[string]any{
					"dimension": "tokens", "total_tokens": totalTokens, "max_tokens": l.MaxTokens,
				})
				l.record(ctx, ledger.TypeTokenBudgetExhausted,
					map[string]any{"total_tokens": totalTokens, "max_tokens": l.MaxTokens},
					fmt.Sprintf("token budget exhausted: %d/%d", totalTokens, l.MaxTokens))
				if l.AllowContinuation {
					return &Result{
						SessionID: sess.ID, Summary: lastText, Verified: false,
						Turns: turn + 1, Continuation: true, OpenCriteria: lastOpen,
					}, nil
				}
				return nil, fmt.Errorf("token budget exhausted: %d/%d tokens used", totalTokens, l.MaxTokens)
			}
		}

		// Issue: Thinking Budget Enforcement (first PR). Accumulate the
		// provider's reported reasoning-token usage and stop the run
		// when the per-run cap is exceeded (ThinkingBudgetPerRequest > 0).
		// Zero values from providers that do not surface the field are
		// safe — they never trigger the guard.
		if resp.Usage.ThinkingTokens > 0 {
			l.thinkingUsed += resp.Usage.ThinkingTokens
		}
		if l.ThinkingBudgetPerRequest > 0 && l.thinkingUsed > l.ThinkingBudgetPerRequest {
			if serr := l.saveHistory(ctx, sess, msgs); serr != nil {
				return nil, serr
			}
			l.fire(ctx, hooks.BudgetExhausted, "", map[string]any{
				"dimension":           "thinking",
				"thinking_tokens":     l.thinkingUsed,
				"max_thinking_tokens": l.ThinkingBudgetPerRequest,
			})
			l.record(ctx, ledger.TypeTokenBudgetExhausted,
				map[string]any{
					"dimension":           "thinking",
					"thinking_tokens":     l.thinkingUsed,
					"max_thinking_tokens": l.ThinkingBudgetPerRequest,
				},
				fmt.Sprintf("thinking budget exhausted: %d > %d", l.thinkingUsed, l.ThinkingBudgetPerRequest))
			// Mandate M3: never skip verification when stopping early. Run
			// the gate on the current workspace first; if it passes the
			// work IS done and we hand back a Verified=true result
			// regardless of the budget. Only when verification FAILS do
			// we surface the budget outcome (Continuation or error).
			if l.Gate != nil {
				vr := l.Gate.Run(ctx, l.Workspace)
				if vr.Passed {
					l.fire(ctx, hooks.VerifyPass, "", map[string]any{
						"mode":                     string(vr.Mode),
						"report":                   vr.Report,
						"after_thinking_exhausted": true,
					})
					l.record(ctx, ledger.TypeVerifyPass,
						map[string]any{"mode": string(vr.Mode), "after_thinking_exhausted": true},
						"verification passed after thinking budget exhausted")
					result := &Result{
						SessionID: sess.ID, Summary: resp.Text,
						Verified: true, Turns: turn + 1,
						Tokens: totalTokens,
					}
					l.fire(ctx, hooks.TaskComplete, "", map[string]any{
						"summary":                         result.Summary,
						"turns":                           result.Turns,
						"verified":                        true,
						"thinking_exhausted_but_verified": true,
					})
					return result, nil
				}
				l.fire(ctx, hooks.VerifyFail, "", map[string]any{
					"mode":                     string(vr.Mode),
					"report":                   vr.Report,
					"after_thinking_exhausted": true,
				})
			}
			if l.AllowContinuation {
				return &Result{
					SessionID: sess.ID, Summary: lastText, Verified: false,
					Turns: turn + 1, Continuation: true, OpenCriteria: lastOpen,
				}, nil
			}
			return nil, fmt.Errorf("thinking budget exhausted (%d > %d)", l.thinkingUsed, l.ThinkingBudgetPerRequest)
		}

		if len(resp.ToolCalls) == 0 {
			vpre := l.fire(ctx, hooks.VerifyPre, "", nil)
			pendingInjects = append(pendingInjects, vpre.PromptInjects...)
			if vpre.Blocked {
				msgs = append(msgs, session.Message{
					Role:    "user",
					Content: "VERIFICATION BLOCKED by hook — fix before claiming completion:\n" + vpre.BlockReason,
				})
				if err := l.saveHistory(ctx, sess, msgs); err != nil {
					return nil, err
				}
				continue
			}

			res := l.Gate.Run(ctx, l.Workspace)
			if !res.Passed {
				vf := l.fire(ctx, hooks.VerifyFail, "", map[string]any{
					"mode": string(res.Mode), "report": res.Report,
				})
				l.record(ctx, ledger.TypeVerifyFail, map[string]any{"mode": string(res.Mode)}, "verification failed ("+string(res.Mode)+")")
				pendingInjects = append(pendingInjects, vf.PromptInjects...)
				if l.Lessons != nil {
					_ = l.Lessons.Record(ctx, lessons.Entry{
						Type:      lessons.TypeFailedVerification,
						Workspace: l.Workspace,
						Context:   map[string]any{"mode": string(res.Mode)},
						Lesson:    "Verification failed (" + string(res.Mode) + "): " + res.Report,
					})
				}

				// SIN Fusion v1: if a TournamentRunner is wired and the
				// failure is structural, fan out to N providers instead
				// of retrying with the same model. First PoC-pass wins.
				// Oracle mode is also supported when the tournament is
				// explicitly configured for oracle (issue #344); the
				// tournament judge selects the winner, not first-pass-wins.
				if l.TournamentRunner != nil &&
					(l.Gate.Mode() == verify.ModePoC || l.Gate.Mode() == verify.ModeOracle) &&
					l.TournamentRunner.ShouldRun(res) {
					output, tokens, terr := l.TournamentRunner.Run(ctx, prompt)
					if terr == nil && output != "" {
						l.fire(ctx, hooks.VerifyPass, "", map[string]any{
							"mode": "poc", "report": "fusion tournament: winner passed verify-gate",
						})
						l.record(ctx, ledger.TypeVerifyPass,
							map[string]any{"mode": "poc", "fusion": true},
							"fusion tournament winner passed verify-gate")
						totalTokens += tokens
						result := &Result{
							SessionID: sess.ID, Summary: output,
							Verified: true, Turns: turn + 1,
							Tokens: totalTokens,
						}
						l.fire(ctx, hooks.TaskComplete, "", map[string]any{
							"summary": result.Summary, "turns": result.Turns,
							"verified": true, "fusion": true,
						})
						l.record(ctx, ledger.TypeTaskComplete,
							map[string]any{"summary": result.Summary, "fusion": true},
							"fusion tournament task complete")
						return result, nil
					}
				}

				msgs = append(msgs, session.Message{
					Role:    "user",
					Content: "VERIFICATION FAILED (" + string(res.Mode) + ") — fix before claiming completion:\n" + res.Report,
				})
				if err := l.saveHistory(ctx, sess, msgs); err != nil {
					return nil, err
				}
				continue
			}
			l.fire(ctx, hooks.VerifyPass, "", map[string]any{
				"mode": string(res.Mode), "report": res.Report,
			})
			l.record(ctx, ledger.TypeVerifyPass, map[string]any{"mode": string(res.Mode)}, "verification passed ("+string(res.Mode)+")")

			// Self-reflection: one cheap self-critique pass before the
			// independent stop-gate. Reset the flag whenever the worker did
			// real work (tool calls) in between, so each fresh proposal gets
			// exactly one reflection. Issue #152.
			if l.Reflector != nil && !reflectedThisProposal {
				reflectedThisProposal = true
				reflectTools := toolsUsed
				if l.Coverage != nil {
					reflectTools = l.Coverage.Used()
				}
				ref := l.Reflector(ctx, StopSnapshot{
					Prompt: prompt, FinalOutput: resp.Text, Turns: turn + 1,
					ToolsUsed: reflectTools, VerifyPassed: res.Passed, SessionID: sess.ID,
				})
				if len(ref.Issues) > 0 {
					l.fire(ctx, hooks.ReflectIssues, "", map[string]any{"issues": ref.Issues})
					l.record(ctx, ledger.TypeReflection,
						map[string]any{"issues": ref.Issues},
						"self-reflection found issues; continuing")
					var b strings.Builder
					b.WriteString("SELF-REVIEW found issues to fix before completing:\n")
					for i, is := range ref.Issues {
						fmt.Fprintf(&b, "  %d. %s\n", i+1, is)
					}
					if strings.TrimSpace(ref.Notes) != "" {
						b.WriteString("Notes: " + ref.Notes + "\n")
					}
					msgs = append(msgs, session.Message{Role: "user", Content: b.String()})
					if err := l.saveHistory(ctx, sess, msgs); err != nil {
						return nil, err
					}
					continue
				}
			}

			// Stop-gate: completion authority is decoupled from the worker.
			// The verify-gate passing is necessary but not sufficient — an
			// independent evaluator confirms the goal contract is satisfied
			// before we accept DONE. A reject re-injects the open criteria
			// and keeps the loop working (the core anti-babysitting path).
			lastText = resp.Text
			effectiveStopGate := l.wrapStopGate()
			if effectiveStopGate != nil {
				snapTools := toolsUsed
				if l.Coverage != nil {
					snapTools = l.Coverage.Used()
				}
				dec := effectiveStopGate(ctx, StopSnapshot{
					Prompt:       prompt,
					FinalOutput:  resp.Text,
					Turns:        turn + 1,
					ToolsUsed:    snapTools,
					VerifyPassed: res.Passed,
					SessionID:    sess.ID,
				})
				l.fire(ctx, hooks.StopEval, "", map[string]any{
					"complete": dec.Complete, "open_criteria": dec.OpenCriteria,
				})
				if !dec.Complete {
					lastOpen = dec.OpenCriteria
					stopRejects++
					l.fire(ctx, hooks.StopContinue, "", map[string]any{
						"open_criteria": dec.OpenCriteria, "report": dec.Report,
					})
					l.record(ctx, ledger.TypeStopContinue,
						map[string]any{"open_criteria": dec.OpenCriteria},
						"stop-gate rejected completion; continuing")
					// Stagnation guard: identical open criteria across consecutive
					// rejects means the worker is stuck. Escalate early.
					fp := strings.Join(dec.OpenCriteria, "\x1f")
					if fp != "" && fp == lastCritFingerprint {
						stallCount++
					} else {
						stallCount = 1
						lastCritFingerprint = fp
					}
					if l.StallThreshold > 0 && stallCount >= l.StallThreshold {
						if serr := l.saveHistory(ctx, sess, msgs); serr != nil {
							return nil, serr
						}
						l.fire(ctx, hooks.StopStalled, "", map[string]any{
							"stall_count": stallCount, "open_criteria": lastOpen,
						})
						l.record(ctx, ledger.TypeStallDetected,
							map[string]any{"stall_count": stallCount, "open_criteria": lastOpen},
							fmt.Sprintf("no progress: identical open criteria %d turns in a row; escalating", stallCount))
						return nil, fmt.Errorf(
							"stop-gate stalled: identical open criteria %d turns in a row "+
								"(StallThreshold=%d); open criteria: %s",
							stallCount, l.StallThreshold, strings.Join(lastOpen, "; "),
						)
					}
					if l.Lessons != nil {
						_ = l.Lessons.Record(ctx, lessons.Entry{
							Type:      lessons.TypeFailedVerification,
							Workspace: l.Workspace,
							Context:   map[string]any{"open_criteria": dec.OpenCriteria},
							Lesson:    "Stop-gate rejected premature completion: " + strings.Join(dec.OpenCriteria, "; "),
						})
					}
					msgs = append(msgs, session.Message{
						Role:    "user",
						Content: formatStopContinue(dec),
					})
					if err := l.saveHistory(ctx, sess, msgs); err != nil {
						return nil, err
					}
					// Hard cap on stop-gate rejections. Independent of
					// StallThreshold (issue #150): MaxStopRejects is a
					// straight count, stall is a fingerprint match.
					maxRejects := l.MaxStopRejects
					if maxRejects <= 0 {
						maxRejects = 3
					}
					if stopRejects >= maxRejects {
						return nil, fmt.Errorf("stop-gate rejected completion %d times (max %d); open criteria: %s",
							stopRejects, maxRejects, strings.Join(lastOpen, "; "))
					}
					continue
				}
			}

			if err := l.saveHistory(ctx, sess, msgs); err != nil {
				return nil, err
			}
			result := &Result{
				SessionID: sess.ID, Summary: resp.Text,
				Verified: res.Passed, Turns: turn + 1,
				Tokens: totalTokens,
			}
			l.fire(ctx, hooks.TaskComplete, "", map[string]any{
				"summary": result.Summary, "turns": result.Turns, "verified": result.Verified,
			})
			l.record(ctx, ledger.TypeTaskComplete, map[string]any{"summary": result.Summary, "turns": result.Turns, "verified": result.Verified}, "task complete: "+result.Summary)
			return result, nil
		}

		for _, tc := range resp.ToolCalls {
			// Real work happened in this turn — reset the reflection
			// flag so a fresh proposal can be re-evaluated.
			reflectedThisProposal = false
			if l.Coverage != nil {
				l.Coverage.Record(tc.Name)
			}
			if l.LoopDetector != nil && l.LoopDetector.Record(tc.Name) {
				l.fire(ctx, hooks.LoopDetected, "", map[string]any{"tool": tc.Name})
				return nil, fmt.Errorf("observer loop detected: repeated tool calls (last tool %s)", tc.Name)
			}
			if !toolsSeen[tc.Name] {
				toolsSeen[tc.Name] = true
				toolsUsed = append(toolsUsed, tc.Name)
			}
			// Observer-loop detection (issue #377): any tool call
			// whose fingerprint closes a repeated-sequence cycle
			// returns ErrLoopDetected. Fail-closed: surface as a
			// TOOL REFUSED message and skip execute() so the model
			// gets feedback AND the dispatch site never reaches a
			// destructive mutator while the worker is thrashing.
			if l.Observer != nil && l.Observer.Enabled() {
				if oerr := l.Observer.Observe(tc, ""); oerr != nil {
					trip := l.Observer.LastTrip()
					data := map[string]any{"reason": "loop.detected"}
					if trip != nil {
						data["pattern_length"] = trip.Length
						data["repeats"] = trip.Repeats
						data["tool"] = trip.ToolName
						data["key"] = trip.Key
						data["history_len"] = trip.HistoryLen
					}
					l.fire(ctx, hooks.LoopDetected, tc.Name, data)
					msgs = append(msgs, session.Message{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content: "TOOL REFUSED: " + oerr.Error() +
							" — refusing dispatch of " + tc.Name +
							"; the model should break the cycle.",
					})
					continue
				}
			}
			out, injects := l.execute(ctx, tc)
			pendingInjects = append(pendingInjects, injects...)
			msgs = append(msgs, session.Message{
				Role: "tool", ToolCallID: tc.ID, Content: out,
			})
		}
		if err := l.saveHistory(ctx, sess, msgs); err != nil {
			return nil, err
		}
	}
	// maxTurns reached without verified completion.
	if l.AllowContinuation {
		// Checkpoint instead of abandoning: persist history and hand back a
		// resumable Result so the caller (daemon) can re-enqueue and continue
		// with the same session — a long task never needs a human restart.
		if err := l.saveHistory(ctx, sess, msgs); err != nil {
			return nil, err
		}
		summary := fmt.Sprintf("checkpoint after %d turns (max reached); resuming", maxTurns)
		l.record(ctx, ledger.TypeTaskCheckpoint, map[string]any{
			"turns": maxTurns, "open_criteria": lastOpen,
		}, summary)
		l.fire(ctx, hooks.TaskAbort, "", map[string]any{
			"reason": "max turns exceeded", "continuation": true,
		})
		if lastText == "" {
			lastText = summary
		}
		return &Result{
			SessionID:    sess.ID,
			Summary:      lastText,
			Verified:     false,
			Turns:        maxTurns,
			Tokens:       totalTokens,
			Continuation: true,
			OpenCriteria: lastOpen,
		}, nil
	}
	l.fire(ctx, hooks.TaskAbort, "", map[string]any{"reason": "max turns exceeded"})
	l.record(ctx, ledger.TypeTaskAbort, map[string]any{"reason": "max turns exceeded"}, "task aborted: max turns exceeded")
	return nil, fmt.Errorf("max turns (%d) exceeded without verified completion", maxTurns)
}

// wrapStopGate returns a StopGate that first evaluates tool-coverage
// constraints (issue #248), then delegates to the configured StopGate. If no
// coverage constraints and no StopGate are configured, it returns nil so the
// loop preserves exact legacy behavior.
func (l *Loop) wrapStopGate() StopGate {
	hasCoverage := l.Coverage != nil && l.Coverage.HasConstraints()
	if !hasCoverage && l.StopGate == nil {
		return nil
	}
	return func(ctx context.Context, snap StopSnapshot) StopDecision {
		if l.Coverage != nil {
			if ok, missing, forbidden := l.Coverage.Check(); !ok {
				return StopDecision{
					Complete:     false,
					OpenCriteria: l.Coverage.OpenCriteria(missing, forbidden),
					Report:       l.Coverage.Feedback(missing, forbidden),
				}
			}
		}
		if l.StopGate != nil {
			return l.StopGate(ctx, snap)
		}
		return StopDecision{Complete: true}
	}
}

// formatStopContinue renders the stop-gate rejection into a directive the
// model can act on: explicit, numbered, and unambiguous about NOT being done.
func formatStopContinue(dec StopDecision) string {
	var b strings.Builder
	b.WriteString("NOT DONE — the work is not complete yet. ")
	b.WriteString("An independent evaluator rejected the proposed completion.\n")
	if len(dec.OpenCriteria) > 0 {
		b.WriteString("Open acceptance criteria that MUST be satisfied:\n")
		for i, c := range dec.OpenCriteria {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, c)
		}
	}
	if strings.TrimSpace(dec.Report) != "" {
		b.WriteString("Evaluator notes:\n")
		b.WriteString(dec.Report)
		b.WriteString("\n")
	}
	b.WriteString("Continue working until every criterion is met, then stop.")
	return b.String()
}

// mutatedPath extracts the target path for tools that mutate the workspace
// so the auto-checkpoint snapshots exactly the file about to change (cheap,
// O(1)). Returns "" for tools that don't mutate the workspace or have no
// "path" argument. (issue #194)
func mutatedPath(tc ToolCall) string {
	switch tc.Name {
	case "sin_write", "sin_edit":
		if p, ok := tc.Args["path"].(string); ok {
			return p
		}
	}
	return ""
}
