// SPDX-License-Identifier: MIT
// Purpose: Parallel sub-agent delegation — spawn N sub-agents simultaneously,
// each in an isolated session with its own context and verify gate. Results
// are returned in input order. Partial failures do not cancel siblings.
//
// Issue #284. Race-free per mandate M7.
package agentloop

import (
	"context"
	"fmt"
	"sync"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// SubagentParallelResult pairs a SubagentResult with its error so callers
// can detect partial failures without losing the results of successful
// siblings.
type SubagentParallelResult struct {
	Index  int             `json:"index"`
	Goal   string          `json:"goal"`
	Result *SubagentResult `json:"result,omitempty"`
	Err    error           `json:"error,omitempty"`
}

// SpawnSubagentsParallel runs N sub-agents concurrently, each in its own
// isolated session. The parent's wiring (Gate, Completion, hooks, permission
// engine, lessons, stop-gate, coverage enforcer) is shared by value — each
// child gets a copy of the parent's Loop struct, but with its own SessionID
// and budget overrides from the corresponding SubagentRequest.
//
// Key properties:
//   - Results are returned in the same order as the input requests.
//   - A failure in one sub-agent does NOT cancel the others (partial failure
//     tolerant). The failing entry's Err field is set; Result is nil.
//   - Parent context cancellation propagates to all children immediately.
//   - Each child gets a fresh session via sessions.StartOrResume(""). Sessions
//     are allocated up-front (before goroutines launch) so session-IDs are
//     deterministic and never race.
//   - Race-free (M7): no shared mutable state between children. The only
//     shared reference is the session store, which is concurrency-safe
//     (SQLite with mutex-protected writes).
//
// Issue #284.
func (l *Loop) SpawnSubagentsParallel(ctx context.Context, sessions *session.Store, reqs []SubagentRequest) ([]*SubagentParallelResult, error) {
	if l == nil {
		return nil, fmt.Errorf("subagents: nil parent loop")
	}
	if sessions == nil {
		return nil, fmt.Errorf("subagents: nil session store")
	}
	if len(reqs) == 0 {
		return nil, fmt.Errorf("subagents: at least one request required")
	}
	for i, req := range reqs {
		if req.Goal == "" {
			return nil, fmt.Errorf("subagents: request %d has empty goal", i)
		}
	}

	// Pre-allocate sessions up-front (before goroutines) so there is no
	// race on session-ID allocation and the IDs are deterministic.
	type childSetup struct {
		sess *session.Session
		req  SubagentRequest
	}
	setups := make([]childSetup, len(reqs))
	for i, req := range reqs {
		sub, err := sessions.StartOrResume("")
		if err != nil {
			return nil, fmt.Errorf("subagents: start session %d: %w", i, err)
		}
		setups[i] = childSetup{sess: sub, req: req}
	}

	results := make([]*SubagentParallelResult, len(reqs))
	var wg sync.WaitGroup

	for i, setup := range setups {
		i, setup := i, setup
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = l.runOneSubagent(ctx, setup.sess, setup.req, i)
		}()
	}
	wg.Wait()
	return results, nil
}

// runOneSubagent executes a single sub-agent in an isolated Loop. It is
// called concurrently from SpawnSubagentsParallel. Each call gets its own
// Loop struct (by value from the parent) with its own SessionID and budget.
func (l *Loop) runOneSubagent(ctx context.Context, sub *session.Session, req SubagentRequest, index int) *SubagentParallelResult {
	out := &SubagentParallelResult{
		Index: index,
		Goal:  req.Goal,
	}
	child := &Loop{
		Gate:                   l.Gate,
		LocalTool:              l.LocalTool,
		LocalSpec:              l.LocalSpec,
		Workspace:              l.Workspace,
		MaxTurns:               firstNonZero(req.MaxTurns, l.MaxTurns),
		MaxTokens:              firstNonZero(req.MaxTokens, l.MaxTokens),
		SessionID:              sub.ID,
		Completion:             l.Completion,
		Hooks:                  l.Hooks,
		Perm:                   l.Perm,
		Ask:                    l.Ask,
		Lessons:                l.Lessons,
		StopGate:               l.StopGate,
		MaxStopRejects:         l.MaxStopRejects,
		StallThreshold:         l.StallThreshold,
		Reflector:              l.Reflector,
		Ledger:                 l.Ledger,
		Coverage:               l.Coverage,
		CoverageRequiredTools:  l.CoverageRequiredTools,
		CoverageForbiddenTools: l.CoverageForbiddenTools,
		// NOTE: deliberately NOT inheriting RunOverride — same as
		// SpawnSubagent (issue #153). The sub-agent's Run is always
		// the default.
	}
	res, err := child.Run(ctx, sub, req.Goal)
	if err != nil {
		out.Err = fmt.Errorf("subagent %d (%s): %w", index, req.Goal, err)
		return out
	}
	out.Result = &SubagentResult{
		Summary:      res.Summary,
		Verified:     res.Verified,
		Turns:        res.Turns,
		OpenCriteria: res.OpenCriteria,
	}
	return out
}

// SpawnSubagentsParallelCallback is like SpawnSubagentsParallel but calls
// onProgress as each sub-agent completes. The callback is called from the
// goroutine that finished — callers must not mutate shared state without
// synchronization. The index matches the position in reqs.
//
// This is useful for TUI live updates: the DAG visualizer subscribes to
// progress events to update task status boxes in real time.
func (l *Loop) SpawnSubagentsParallelCallback(ctx context.Context, sessions *session.Store, reqs []SubagentRequest, onProgress func(index int, result *SubagentParallelResult)) ([]*SubagentParallelResult, error) {
	if l == nil {
		return nil, fmt.Errorf("subagents: nil parent loop")
	}
	if sessions == nil {
		return nil, fmt.Errorf("subagents: nil session store")
	}
	if len(reqs) == 0 {
		return nil, fmt.Errorf("subagents: at least one request required")
	}
	for i, req := range reqs {
		if req.Goal == "" {
			return nil, fmt.Errorf("subagents: request %d has empty goal", i)
		}
	}

	type childSetup struct {
		sess *session.Session
		req  SubagentRequest
	}
	setups := make([]childSetup, len(reqs))
	for i, req := range reqs {
		sub, err := sessions.StartOrResume("")
		if err != nil {
			return nil, fmt.Errorf("subagents: start session %d: %w", i, err)
		}
		setups[i] = childSetup{sess: sub, req: req}
	}

	results := make([]*SubagentParallelResult, len(reqs))
	var wg sync.WaitGroup
	var mu sync.Mutex // guards onProgress calls (M7)

	for i, setup := range setups {
		i, setup := i, setup
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := l.runOneSubagent(ctx, setup.sess, setup.req, i)
			results[i] = res
			if onProgress != nil {
				mu.Lock()
				onProgress(i, res)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return results, nil
}
