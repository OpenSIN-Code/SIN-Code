// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loop is refactored
// Purpose: max-turns / continuation handling extracted from Run().
// Pure file split, same package, no behavioural change.
package agentloop

import (
	"context"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// handleMaxTurns handles the end-of-run case when maxTurns is reached
// without verified completion. When AllowContinuation is set, it
// checkpoints the session and returns a resumable Result; otherwise it
// returns an error.
func (l *Loop) handleMaxTurns(
	ctx context.Context,
	sess *session.Session,
	msgs []session.Message,
	maxTurns int,
	totalTokens int,
	lastText string,
	lastOpen []string,
) (*Result, error) {
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
		l.emitProgress(ProgressEvent{
			Event: "task.abort",
			Data:  map[string]any{"reason": "max turns exceeded", "continuation": true},
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
	l.emitProgress(ProgressEvent{
		Event: "task.abort",
		Data:  map[string]any{"reason": "max turns exceeded"},
	})
	l.record(ctx, ledger.TypeTaskAbort, map[string]any{"reason": "max turns exceeded"}, "task aborted: max turns exceeded")
	return nil, fmt.Errorf("max turns (%d) exceeded without verified completion", maxTurns)
}
