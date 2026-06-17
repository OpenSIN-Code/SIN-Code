// SPDX-License-Identifier: MIT
// Purpose: Sub-agent delegation — run a subtask in an isolated Loop with
// its own session and context, returning only the result summary to the
// parent (issue #153).
//
// The parent's context stays small because the sub-agent's full message
// history is in its own session, never sent back. Budgets default to the
// parent's; the caller can override per-subagent.
//
// v0: sequential delegation. Parallelism (multiple sub-agents at once)
// is a follow-up — the SpawnSubagent signature is already designed for
// it (one call per sub-agent, no shared state).
package agentloop

import (
	"context"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// SubagentRequest describes a delegated subtask. Empty fields fall
// back to the parent's wiring (the calling Loop).
type SubagentRequest struct {
	Goal      string // the subtask prompt
	MaxTurns  int    // optional per-subagent turn cap; 0 -> parent
	MaxTokens int    // optional per-subagent token cap; 0 -> parent
}

// SubagentResult is what the parent loop sees — intentionally just the
// summary plus a few control fields, never the sub-agent's full
// message history.
type SubagentResult struct {
	Summary      string   `json:"summary"`
	Verified     bool     `json:"verified"`
	Turns        int      `json:"turns"`
	OpenCriteria []string `json:"open_criteria,omitempty"`
}

// SpawnSubagent runs req in a fresh Loop that shares the parent's
// wiring (Gate, Completion, hooks) but gets an isolated session so
// its context never pollutes the parent. Budgets default to the
// parent's. A nil sessions store is a hard error (no session
// store means no isolation).
//
// Issue #153.
func (l *Loop) SpawnSubagent(ctx context.Context, sessions *session.Store, req SubagentRequest) (*SubagentResult, error) {
	if l == nil {
		return nil, fmt.Errorf("subagent: nil parent loop")
	}
	if sessions == nil {
		return nil, fmt.Errorf("subagent: nil session store")
	}
	if req.Goal == "" {
		return nil, fmt.Errorf("subagent: goal is required")
	}
	// Start a fresh session. The empty id triggers a new
	// session-id allocation (see session.StartOrResume). The
	// sub-agent's session is unrelated to the parent's session —
	// issue body says "shares wiring" not "shares session".
	sub, err := sessions.StartOrResume("")
	if err != nil {
		return nil, fmt.Errorf("subagent: start session: %w", err)
	}
	child := &Loop{
		Gate:           l.Gate,
		LocalTool:      l.LocalTool,
		LocalSpec:      l.LocalSpec,
		Workspace:      l.Workspace,
		MaxTurns:       firstNonZero(req.MaxTurns, l.MaxTurns),
		MaxTokens:      firstNonZero(req.MaxTokens, l.MaxTokens),
		SessionID:      sub.ID,
		Completion:     l.Completion,
		Hooks:          l.Hooks,
		Perm:           l.Perm,
		Ask:            l.Ask,
		Lessons:        l.Lessons,
		StopGate:               l.StopGate,
		MaxStopRejects:         l.MaxStopRejects,
		StallThreshold:         l.StallThreshold,
		Reflector:              l.Reflector,
		Ledger:                 l.Ledger,
		Coverage:               l.Coverage,
		CoverageRequiredTools:  l.CoverageRequiredTools,
		CoverageForbiddenTools: l.CoverageForbiddenTools,
		// NOTE: deliberately NOT inheriting RunOverride. The
		// sub-agent's Run is always the default; the parent's
		// RunOverride is its own concern.
	}
	res, err := child.Run(ctx, sub, req.Goal)
	if err != nil {
		return nil, err
	}
	return &SubagentResult{
		Summary:      res.Summary,
		Verified:     res.Verified,
		Turns:        res.Turns,
		OpenCriteria: res.OpenCriteria,
	}, nil
}

// firstNonZero returns a if non-zero, else b. Used to fall back to
// the parent's budget when the sub-agent request does not override.
func firstNonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}
