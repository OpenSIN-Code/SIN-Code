// SPDX-License-Identifier: MIT
// Purpose: Sub-agent delegation — run a subtask in an isolated Loop with its
// own session and context, returning only the result summary to the parent.
// Keeps the parent context lean and enables later parallelization.
package agentloop

import (
	"context"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// SubagentRequest describes a delegated subtask.
type SubagentRequest struct {
	Goal      string // the subtask prompt
	MaxTurns  int    // optional per-subagent turn cap
	MaxTokens int    // optional per-subagent token cap
}

// SubagentResult is what the parent loop sees — intentionally just the
// summary, never the sub-agent's full message history.
type SubagentResult struct {
	Summary      string   `json:"summary"`
	Verified     bool     `json:"verified"`
	Turns        int      `json:"turns"`
	OpenCriteria []string `json:"open_criteria,omitempty"`
}

// SpawnSubagent runs req in a fresh Loop that shares the parent's wiring
// (Gate, Completion, tools, hooks) but gets an isolated session so its
// context never pollutes the parent. Budgets default to the parent's.
//
// The session store is used to create a brand-new session via
// StartOrResume(""), giving the sub-agent its own independent history.
func (l *Loop) SpawnSubagent(ctx context.Context, store *session.Store, req SubagentRequest) (*SubagentResult, error) {
	sub, err := store.StartOrResume("")
	if err != nil {
		return nil, fmt.Errorf("subagent: new session: %w", err)
	}

	l.fire(ctx, hooks.AgentSpawn, "", map[string]any{"goal": req.Goal, "session": sub.ID})

	child := &Loop{
		Gate:            l.Gate,
		LocalTool:       l.LocalTool,
		LocalSpec:       l.LocalSpec,
		Workspace:       l.Workspace,
		MaxTurns:        firstNonZero(req.MaxTurns, l.MaxTurns),
		SessionID:       sub.ID,
		Completion:      l.Completion,
		Hooks:           l.Hooks,
		Perm:            l.Perm,
		Ask:             l.Ask,
		Lessons:         l.Lessons,
		StopGate:        l.StopGate,
		MaxStopRejects:  l.MaxStopRejects,
		StallThreshold:  l.StallThreshold,
		MaxTokens:       firstNonZero(req.MaxTokens, l.MaxTokens),
		BudgetWarnRatio: l.BudgetWarnRatio,
		Reflector:       l.Reflector,
		Ledger:          l.Ledger,
		// NOTE: deliberately NOT inheriting RunOverride.
	}
	res, err := child.Run(ctx, sub, req.Goal)
	if err != nil {
		return nil, err
	}

	l.fire(ctx, hooks.AgentComplete, "", map[string]any{
		"session": sub.ID, "verified": res.Verified, "turns": res.Turns,
	})

	return &SubagentResult{
		Summary:      res.Summary,
		Verified:     res.Verified,
		Turns:        res.Turns,
		OpenCriteria: res.OpenCriteria,
	}, nil
}

func firstNonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}
