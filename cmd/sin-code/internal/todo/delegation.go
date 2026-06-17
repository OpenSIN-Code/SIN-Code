// SPDX-License-Identifier: MIT
// Purpose: Delegation assigns todos to sub-agent sessions (issue #334).
// It is an in-memory tracker that records which todo is owned by which
// agent session, lets a caller recall (cancel) a delegation, query its
// status, list delegations by agent, and mark a delegation complete with
// a result string. All shared state is guarded by sync.RWMutex (M7) —
// no SQLite is needed.
//
// The Delegation type is the bridge between the todo layer and the
// agent-team session layer: a future `sin-code todo delegate` command
// will call Delegate, hand the SessionID to the session/worktree
// manager, and later call Complete when the sub-agent session reports
// success (issue #334 acceptance criteria).
package todo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// DelegationStatus is the lifecycle state of a delegated todo.
type DelegationStatus string

const (
	// DelegationPending means the delegation was recorded but the
	// sub-agent session has not yet acknowledged it.
	DelegationPending DelegationStatus = "pending"
	// DelegationRunning means the sub-agent session is actively
	// working on the delegated todo.
	DelegationRunning DelegationStatus = "running"
	// DelegationComplete means the sub-agent session finished the
	// todo and reported a result via Complete.
	DelegationComplete DelegationStatus = "complete"
	// DelegationRecalled means the delegating caller cancelled the
	// delegation before completion (Recall).
	DelegationRecalled DelegationStatus = "recalled"
)

// DelegatedTodo is the record of a single todo delegated to a sub-agent
// session. It is the value type returned by Status / List methods.
type DelegatedTodo struct {
	TodoID      string           `json:"todo_id"`
	SessionID   string           `json:"session_id"`
	AgentName   string           `json:"agent_name"`
	DelegatedAt time.Time        `json:"delegated_at"`
	Status      DelegationStatus `json:"status"`
	Result      string           `json:"result,omitempty"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
}

// Delegation assigns todos to sub-agent sessions and tracks their
// lifecycle in memory. It is safe for concurrent use (M7).
type Delegation struct {
	mu          sync.RWMutex
	delegations map[string]*DelegatedTodo
	sessionSeq  uint64

	// sessionIDFn generates a new session ID for a delegation. It is
	// hookable so tests can produce deterministic IDs. The default
	// produces "sess-<hex(sha256(nano|seq))>" values.
	sessionIDFn func() string
}

// NewDelegation creates an empty in-memory delegation tracker.
func NewDelegation() *Delegation {
	d := &Delegation{
		delegations: make(map[string]*DelegatedTodo),
	}
	d.sessionIDFn = d.defaultSessionID
	return d
}

// defaultSessionID mints a session ID from a monotonic sequence and the
// current wall clock. The hash keeps the ID opaque and collision-resistant
// without pulling in crypto/rand at call time.
func (d *Delegation) defaultSessionID() string {
	seq := atomic.AddUint64(&d.sessionSeq, 1)
	h := sha256.Sum256([]byte(fmt.Sprintf("delegation-%d-%d", time.Now().UnixNano(), seq)))
	return "sess-" + hex.EncodeToString(h[:8])
}

// Delegate records a delegation of todoID to the named agent, mints a
// new session ID, and returns the resulting DelegatedTodo. The initial
// status is DelegationPending. A todo that is already actively delegated
// (pending or running) cannot be delegated again — recall or complete it
// first. Empty todoID or agentName is rejected.
func (d *Delegation) Delegate(todoID string, agentName string) (*DelegatedTodo, error) {
	if todoID == "" {
		return nil, fmt.Errorf("todo: delegation: empty todoID")
	}
	if agentName == "" {
		return nil, fmt.Errorf("todo: delegation: empty agentName")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.delegations[todoID]; ok {
		if existing.Status == DelegationPending || existing.Status == DelegationRunning {
			return nil, fmt.Errorf("todo: delegation: todo %s already delegated (status=%s)", todoID, existing.Status)
		}
	}

	rec := &DelegatedTodo{
		TodoID:      todoID,
		SessionID:   d.sessionIDFn(),
		AgentName:   agentName,
		DelegatedAt: time.Now().UTC(),
		Status:      DelegationPending,
	}
	d.delegations[todoID] = rec
	return rec.clone(), nil
}

// Recall cancels an active delegation, setting its status to
// DelegationRecalled. A delegation that is already complete or recalled
// is left untouched (idempotent). Returns an error when no delegation
// exists for the todo.
func (d *Delegation) Recall(todoID string) error {
	if todoID == "" {
		return fmt.Errorf("todo: delegation: empty todoID")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	rec, ok := d.delegations[todoID]
	if !ok {
		return fmt.Errorf("todo: delegation: no delegation for todo %s", todoID)
	}
	if rec.Status == DelegationComplete || rec.Status == DelegationRecalled {
		return nil
	}
	rec.Status = DelegationRecalled
	return nil
}

// Status returns a copy of the DelegatedTodo for the given todo, or an
// error when no delegation exists.
func (d *Delegation) Status(todoID string) (*DelegatedTodo, error) {
	if todoID == "" {
		return nil, fmt.Errorf("todo: delegation: empty todoID")
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	rec, ok := d.delegations[todoID]
	if !ok {
		return nil, fmt.Errorf("todo: delegation: no delegation for todo %s", todoID)
	}
	return rec.clone(), nil
}

// ListByAgent returns all delegations assigned to the named agent, in
// insertion-order-independent map order. Returns an empty slice (not nil)
// when there are none.
func (d *Delegation) ListByAgent(agentName string) ([]DelegatedTodo, error) {
	if agentName == "" {
		return nil, fmt.Errorf("todo: delegation: empty agentName")
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]DelegatedTodo, 0)
	for _, rec := range d.delegations {
		if rec.AgentName == agentName {
			out = append(out, *rec.clone())
		}
	}
	return out, nil
}

// ListAll returns every recorded delegation. Returns an empty slice
// (not nil) when there are none.
func (d *Delegation) ListAll() ([]DelegatedTodo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]DelegatedTodo, 0, len(d.delegations))
	for _, rec := range d.delegations {
		out = append(out, *rec.clone())
	}
	return out, nil
}

// Complete marks the delegation for todoID as DelegationComplete and
// records the result string. A delegation that was recalled cannot be
// completed. Returns an error when no delegation exists.
func (d *Delegation) Complete(todoID string, result string) error {
	if todoID == "" {
		return fmt.Errorf("todo: delegation: empty todoID")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	rec, ok := d.delegations[todoID]
	if !ok {
		return fmt.Errorf("todo: delegation: no delegation for todo %s", todoID)
	}
	if rec.Status == DelegationRecalled {
		return fmt.Errorf("todo: delegation: cannot complete recalled todo %s", todoID)
	}
	now := time.Now().UTC()
	rec.Status = DelegationComplete
	rec.Result = result
	rec.CompletedAt = &now
	return nil
}

// clone returns a shallow copy of the DelegatedTodo so callers cannot
// mutate the in-memory record through the returned pointer. CompletedAt
// is a pointer and is copied by value (the pointed-to time is immutable).
func (r *DelegatedTodo) clone() *DelegatedTodo {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}
