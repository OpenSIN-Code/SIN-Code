// SPDX-License-Identifier: MIT
// Purpose: Sub-agent delegation — run a subtask in an isolated Loop with its
// own session and context, returning only the result summary to the parent.
// Keeps the parent context lean and enables later parallelization.
package agentloop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// SpawnSubagentTool is the reserved tool name the worker calls to delegate a
// self-contained subtask. The loop intercepts it before LocalTool so callers
// never wire it manually — registration is automatic whenever SubagentStore
// is set on the Loop.
const SpawnSubagentTool = "spawn_subagent"

// subagentSpec is the schema advertised to the model for spawn_subagent.
var subagentSpec = ToolSpec{
	Name: SpawnSubagentTool,
	Description: "Delegate a self-contained subtask to an isolated sub-agent. " +
		"The sub-agent runs in its own session/context and returns only a result " +
		"summary, keeping this conversation lean. Use for parallelizable or " +
		"context-heavy subtasks (e.g. 'investigate module X', 'write tests for Y').",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goal":       map[string]any{"type": "string", "description": "The subtask to accomplish, stated as a complete instruction."},
			"max_turns":  map[string]any{"type": "integer", "description": "Optional per-subagent turn cap; defaults to the parent's."},
			"max_tokens": map[string]any{"type": "integer", "description": "Optional per-subagent token cap; defaults to the parent's."},
		},
		"required": []any{"goal"},
	},
}

// subagentEnabled reports whether delegation is wired (a session store is set).
func (l *Loop) subagentEnabled() bool { return l.SubagentStore != nil }

// handleSpawnSubagent runs a delegated subtask and returns a JSON summary the
// worker can read. Errors are returned as strings (never as Go errors) so a
// failed sub-agent does not abort the parent run — the parent decides how to
// react to the reported failure.
func (l *Loop) handleSpawnSubagent(ctx context.Context, args map[string]any) string {
	goal, _ := args["goal"].(string)
	if goal == "" {
		return "SUBAGENT ERROR: 'goal' is required"
	}
	req := SubagentRequest{Goal: goal}
	if v, ok := asInt(args["max_turns"]); ok {
		req.MaxTurns = v
	}
	if v, ok := asInt(args["max_tokens"]); ok {
		req.MaxTokens = v
	}
	res, err := l.SpawnSubagent(ctx, l.SubagentStore, req)
	if err != nil {
		return "SUBAGENT ERROR: " + err.Error()
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "SUBAGENT ERROR: marshal result: " + err.Error()
	}
	return string(b)
}

// asInt coerces a JSON-decoded number (float64) or int into an int.
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

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
