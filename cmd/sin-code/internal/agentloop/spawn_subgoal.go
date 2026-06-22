// SPDX-License-Identifier: MIT
// Purpose: synchronous sub-goal delegation (issue #385). The caller
// enqueues a child goal through the autonomy queue and BLOCKS until the
// child terminates or the configured timeout elapses. Each sub-goal
// still passes the verify gate (mandate M3) — the parent never gains a
// "success shortcut" by delegating.
//
// Architecture notes:
//
//   - Pure sub-goal worker: a separate process / daemon worker drains
//     the queue and runs each child end-to-end. The caller does NOT
//     execute the child inline; it just enqueues and waits. This
//     keeps isolation (M3 evidence preservation: each child gets its
//     own session, its own ledger, its own loop), lets multiple
//     workers drain the queue in parallel, and ensures the depth
//     ceiling is enforced ONE place (the queue itself).
//
//   - Depth check happens BEFORE enqueue so a runaway recursion is
//     caught immediately rather than after a long-running child. The
//     check is `parentDepth >= maxDepth` (matches the daemon's
//     wrapWithSpawn semantics at daemon_cmd.go:421).
//
//   - Polling uses context.WithTimeout against the caller's timeout
//     AND a ticker for the bound; when ctx is cancelled the latest
//     observed status is returned so the caller surfaces something
//     useful instead of an opaque timeout.
//
//   - Race-safe per mandate M7: the only mutable state is the queue
//     (SQLite, single-writer), the local ticker (per-call), and the
//     result-from-loop variable (local). All shared access goes
//     through the queue.
package agentloop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
)

// ErrSubgoalTimeout is returned when the synchronous wait for a
// sub-goal exceeds the configured Timeout. The latest observed status
// is included in the wrapped error so the caller can route the
// outcome instead of treating it as a hard failure.
var ErrSubgoalTimeout = errors.New("spawn_subgoal: timed out waiting for sub-goal completion")

// ErrSubgoalDepthExceeded is returned when the depth check rejects a
// spawn request — i.e. parentDepth >= maxDepth. Inspection with
// errors.Is is supported (M4 contract).
var ErrSubgoalDepthExceeded = errors.New("spawn_subgoal: sub-goal depth limit exceeded")

// ErrSubgoalQueue is returned when the queue is nil / the queue open
// failed. A nil queue means no persistence → no isolation → silent
// scheduler poisoning, so we refuse instead of degrading.
var ErrSubgoalQueue = errors.New("spawn_subgoal: queue required")

const (
	// SpawnSubgoalDefaultTimeout is the synchronous wait deadline
	// used when SpawnSubgoalRequest.Timeout is zero.
	SpawnSubgoalDefaultTimeout = 5 * time.Minute
	// SpawnSubgoalDefaultPoll is the status-poll interval used when
	// SpawnSubgoalRequest.Poll is zero.
	SpawnSubgoalDefaultPoll = 500 * time.Millisecond
	// SpawnSubgoalDefaultMaxDepth is the depth ceiling used when
	// neither the request nor the config supplies one. Matches the
	// default in AGENTS.md / config spec.
	SpawnSubgoalDefaultMaxDepth = 2
	// SpawnSubgoalDefaultTotalTimeoutS is the autonomous
	// single-goal timeout (seconds) used when the chat dispatcher
	// cannot read config — falls back to a safe ceiling.
	SpawnSubgoalDefaultTotalTimeoutS = 300
)

// SpawnSubgoalRequest describes a sub-goal to be spawned and waited
// on synchronously. All fields are optional except Description;
// zero values fall back to the package defaults or to the supplied
// defaultMaxDepth parameter.
type SpawnSubgoalRequest struct {
	Title       string        // short identifier (informational)
	Description string        // the prompt passed to the sub-goal worker
	Workspace   string        // optional sub-goal workspace (empty -> inherit from parent)
	MaxTurns    int           // optional per-sub-goal turn cap (0 -> queue default)
	MaxDepth    int           // optional depth-limit override (0 -> use defaultMaxDepth)
	Timeout     time.Duration // optional synchronous-wait timeout (0 -> SpawnSubgoalDefaultTimeout)
	Poll        time.Duration // optional status-poll interval (0 -> SpawnSubgoalDefaultPoll)
}

// SpawnSubgoalResult is what the calling loop sees — the verified
// status, summary, error string (if any), and the assigned goal ID.
type SpawnSubgoalResult struct {
	ID           int64               `json:"id"`
	Status       autonomy.GoalStatus `json:"status"`
	Verified     bool                `json:"verified"`
	Summary      string              `json:"summary,omitempty"`
	ErrorMessage string              `json:"error,omitempty"`
	Attempts     int                 `json:"attempts,omitempty"`
	SessionID    string              `json:"session_id,omitempty"`
	TerminalAt   time.Time           `json:"terminal_at,omitempty"`
}

// SpawnSubgoalSpec is the public ToolSpec for the chat tool surface.
// The chat dispatcher advertises it via extraSpecs() and routes
// invocations to the synchronous-wait implementation.
func SpawnSubgoalSpec() ToolSpec {
	return ToolSpec{
		Name: "spawn_subgoal",
		Description: "Enqueue a child sub-goal through the verified autonomy queue and BLOCK until the sub-goal " +
			"terminates, fails, or the synchronous timeout elapses. Each sub-goal still passes the verify gate " +
			"(M3): the parent never gains a success shortcut by delegating. Default timeout " +
			SpawnSubgoalDefaultTimeout.String() + ", default depth ceiling " +
			fmt.Sprintf("%d", SpawnSubgoalDefaultMaxDepth) +
			" (overridable via autonomy.max_subgoal_depth / autonomy.subgoal_timeout_s).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":       map[string]any{"type": "string", "description": "short identifier for the sub-goal (informational)"},
				"description": map[string]any{"type": "string", "description": "the prompt passed to the sub-goal worker — REQUIRED"},
				"workspace":   map[string]any{"type": "string", "description": "optional sub-goal workspace"},
				"max_turns":   map[string]any{"type": "integer", "description": "optional per-sub-goal turn cap"},
				"max_depth":   map[string]any{"type": "integer", "description": "optional depth-limit override; 0 = use autonomy.max_subgoal_depth from config"},
				"timeout":     map[string]any{"type": "string", "description": "max synchronous wait duration (e.g. 2m, 30s); empty = 5m"},
				"poll":        map[string]any{"type": "string", "description": "status poll interval (e.g. 500ms); empty = 500ms"},
			},
			"required": []any{"description"},
		},
	}
}

// isTerminalSubgoalStatus reports whether the status is one of the
// four outcomes that should end the synchronous wait.
func isTerminalSubgoalStatus(s autonomy.GoalStatus) bool {
	switch s {
	case autonomy.StatusVerified,
		autonomy.StatusFailed,
		autonomy.StatusExhausted,
		autonomy.StatusBlocked:
		return true
	}
	return false
}

// SpawnSubgoal enqueues a sub-goal under parentID in queue and blocks
// until the sub-goal reaches a terminal status, the caller's ctx is
// cancelled, or the configured Timeout elapses (→ ErrSubgoalTimeout).
//
// parentDepth is the depth of the calling goal — 0 for top-level
// chat sessions, the queue-stored Depth for agents running inside
// the daemon. defaultMaxDepth is the ceiling used when req.MaxDepth
// is zero; a request where parentDepth >= effectiveMaxDepth is
// rejected with ErrSubgoalDepthExceeded BEFORE enqueueing, so a
// runaway recursion never writes to the queue.
//
// A nil queue returns ErrSubgoalQueue — no silent degradation.
func SpawnSubgoal(
	ctx context.Context,
	queue *autonomy.Queue,
	parentID int64,
	parentDepth int,
	defaultMaxDepth int,
	req SpawnSubgoalRequest,
) (*SpawnSubgoalResult, error) {
	if queue == nil {
		return nil, ErrSubgoalQueue
	}
	if req.Description == "" {
		return nil, fmt.Errorf("spawn_subgoal: description required")
	}
	maxDepth := effectiveMaxDepth(req.MaxDepth, defaultMaxDepth)
	if parentDepth >= maxDepth {
		return nil, fmt.Errorf("%w (parent_depth=%d, max=%d)", ErrSubgoalDepthExceeded, parentDepth, maxDepth)
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = SpawnSubgoalDefaultTimeout
	}
	poll := req.Poll
	if poll <= 0 {
		poll = SpawnSubgoalDefaultPoll
	}

	id, err := queue.AddSub(ctx, parentID, req.Description, 0, req.MaxTurns, "")
	if err != nil {
		return nil, fmt.Errorf("spawn_subgoal: enqueue: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	var lastStatus autonomy.GoalStatus
	for {
		status, serr := queue.GetStatus(waitCtx, id)
		if serr == nil && status != "" {
			lastStatus = status
			if isTerminalSubgoalStatus(status) {
				return buildSpawnSubgoalResult(ctx, queue, id, status)
			}
		}
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w (status=%s)", ErrSubgoalTimeout, lastStatus)
			}
			return buildSpawnSubgoalResult(ctx, queue, id, lastStatus)
		case <-ticker.C:
		}
	}
}

// effectiveMaxDepth returns req.MaxDepth when positive, else the
// supplied default. When BOTH are zero, the package default is used.
// Extracted so tests can assert the override semantics deterministically.
func effectiveMaxDepth(reqMax, defaultMax int) int {
	if reqMax > 0 {
		return reqMax
	}
	if defaultMax > 0 {
		return defaultMax
	}
	return SpawnSubgoalDefaultMaxDepth
}

// buildSpawnSubgoalResult re-queries the goal so the returned fields
// (Attempts, SessionID, LastError, Verified) reflect the final state.
// Race-safe via the queue's atomic Get.
func buildSpawnSubgoalResult(ctx context.Context, queue *autonomy.Queue, id int64, status autonomy.GoalStatus) (*SpawnSubgoalResult, error) {
	g, err := queue.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("spawn_subgoal: read result: %w", err)
	}
	if g == nil {
		return &SpawnSubgoalResult{
			ID:     id,
			Status: status,
		}, nil
	}
	return &SpawnSubgoalResult{
		ID:           g.ID,
		Status:       g.Status,
		Verified:     g.Status == autonomy.StatusVerified,
		Summary:      titleOrFirstLine(g.Prompt),
		ErrorMessage: g.LastError,
		Attempts:     g.Attempts,
		SessionID:    g.SessionID,
		TerminalAt:   g.UpdatedAt,
	}, nil
}

// titleOrFirstLine strips multi-line content to a short summary.
func titleOrFirstLine(prompt string) string {
	if i := indexNewline(prompt); i >= 0 {
		return prompt[:i]
	}
	return prompt
}

func indexNewline(s string) int {
	for i, c := range s {
		if c == '\n' {
			return i
		}
	}
	return -1
}
