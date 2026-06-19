// SPDX-License-Identifier: MIT
// Purpose: runtime tool-coverage enforcer for the agent loop. Tracks which
// tools were successfully invoked in a session and rejects completion when
// required tools are missing or forbidden tools were used (issue #248).
// The enforcer is fail-closed: any missing required tool or any forbidden
// tool blocks completion.
//
// The enforcer is race-safe: tool use is recorded under a mutex so the
// completion check can be wired into the stop-gate path safely.
package agentloop

import (
	"fmt"
	"strings"
	"sync"
)

// ToolCoverageEnforcer tracks required and forbidden tool usage for a single
// run. It is created per-run in Loop.Run when any constraint is configured.
type ToolCoverageEnforcer struct {
	RequiredTools  []string
	ForbiddenTools []string

	mu   sync.Mutex
	used map[string]bool
}

// NewToolCoverageEnforcer creates an enforcer with the given constraints.
// Constraints are copied so callers cannot mutate them after construction.
func NewToolCoverageEnforcer(required, forbidden []string) *ToolCoverageEnforcer {
	e := &ToolCoverageEnforcer{
		RequiredTools:  append([]string(nil), required...),
		ForbiddenTools: append([]string(nil), forbidden...),
		used:           make(map[string]bool),
	}
	return e
}

// Record marks a tool as used. It is safe for concurrent use.
func (e *ToolCoverageEnforcer) Record(name string) {
	if e == nil || name == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.used == nil {
		e.used = make(map[string]bool)
	}
	e.used[name] = true
}

// Check returns whether the run satisfies the coverage constraints. When not
// ok, missing lists required tools that were never recorded and forbidden lists
// forbidden tools that were recorded.
func (e *ToolCoverageEnforcer) Check() (ok bool, missing, forbidden []string) {
	if e == nil {
		return true, nil, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, need := range e.RequiredTools {
		if !e.used[need] {
			missing = append(missing, need)
		}
	}
	for _, ban := range e.ForbiddenTools {
		if e.used[ban] {
			forbidden = append(forbidden, ban)
		}
	}
	return len(missing) == 0 && len(forbidden) == 0, missing, forbidden
}

// Used returns the distinct tool names recorded so far. The returned slice is
// a copy and is safe to read without holding the lock.
func (e *ToolCoverageEnforcer) Used() []string {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.used))
	for name := range e.used {
		out = append(out, name)
	}
	return out
}

// OpenCriteria turns coverage violations into the acceptance-criteria shape
// consumed by the loop's stop-gate rejection path.
func (e *ToolCoverageEnforcer) OpenCriteria(missing, forbidden []string) []string {
	var out []string
	for _, m := range missing {
		out = append(out, "required tool not used: "+m)
	}
	for _, f := range forbidden {
		out = append(out, "forbidden tool used: "+f)
	}
	return out
}

// Feedback renders a human-readable directive the model can act on.
func (e *ToolCoverageEnforcer) Feedback(missing, forbidden []string) string {
	if e == nil || (len(missing) == 0 && len(forbidden) == 0) {
		return ""
	}
	var parts []string
	if len(missing) == 1 {
		parts = append(parts, fmt.Sprintf("You still need to call `%s` before claiming completion.", missing[0]))
	} else if len(missing) > 1 {
		parts = append(parts, fmt.Sprintf("You still need to call: %s before claiming completion.", joinBackticks(missing)))
	}
	if len(forbidden) == 1 {
		parts = append(parts, fmt.Sprintf("You used forbidden tool `%s`; do not use it.", forbidden[0]))
	} else if len(forbidden) > 1 {
		parts = append(parts, fmt.Sprintf("You used forbidden tools: %s; do not use them.", joinBackticks(forbidden)))
	}
	return strings.Join(parts, " ")
}

// HasConstraints reports whether the enforcer is configured with any
// constraint that needs to be checked.
func (e *ToolCoverageEnforcer) HasConstraints() bool {
	if e == nil {
		return false
	}
	return len(e.RequiredTools) > 0 || len(e.ForbiddenTools) > 0
}

func joinBackticks(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "`" + item + "`"
	}
	return strings.Join(quoted, ", ")
}
