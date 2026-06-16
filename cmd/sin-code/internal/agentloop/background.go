// SPDX-License-Identifier: MIT
// Purpose: Background tasks — fire-and-keep-working agent runs
// (issue #195). Each task owns a goroutine, a context.CancelFunc,
// and a Result. A process-local registry tracks them by short id.
//
// v0 scope: in-process tasks only (no daemon, no IPC). v1 will
// persist the registry to SQLite and surface tasks across
// `sin-code` invocations.
package agentloop

import (
	"context"
	"errors"
	"sync"
	"time"
)

// BackgroundTask is one in-process long-running agent run.
type BackgroundTask struct {
	ID         string    `json:"id"`
	Goal       string    `json:"goal"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Status     string    `json:"status"` // "running" | "verified" | "failed" | "cancelled"
	Result     *Result   `json:"result,omitempty"`
	Err        string    `json:"error,omitempty"`
}

// ErrTaskNotFound is returned by Get/Cancel for unknown task ids.
var ErrTaskNotFound = errors.New("background task not found")

// TaskRegistry is a process-local in-memory map of BackgroundTasks.
// All methods are safe for concurrent use. The zero value is
// not ready — use NewTaskRegistry.
type TaskRegistry struct {
	mu      sync.RWMutex
	tasks   map[string]*BackgroundTask
	cancels map[string]context.CancelFunc
	seq     int
}

// NewTaskRegistry returns an empty registry. The process should
// have at most one registry; it is the singleton entry point
// for the `sin-code background` subcommand.
func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{
		tasks:   map[string]*BackgroundTask{},
		cancels: map[string]context.CancelFunc{},
	}
}

// Add registers a new task with a pre-allocated id. The caller
// is expected to start the goroutine immediately after Add returns.
// Returns the task (with id).
func (r *TaskRegistry) Add(goal string) *BackgroundTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	id := taskIDFromSeq(r.seq)
	t := &BackgroundTask{
		ID:        id,
		Goal:      goal,
		StartedAt: time.Now().UTC(),
		Status:    "running",
	}
	r.tasks[id] = t
	return t
}

// SetCancel records the cancel function for id. Called by the
// goroutine immediately after Add.
func (r *TaskRegistry) SetCancel(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[id] = cancel
}

// Finish marks the task as done. status is "verified", "failed",
// or "cancelled". result is the agent run's Result; err is the
// error string if status is "failed" or "cancelled".
func (r *TaskRegistry) Finish(id, status string, result *Result, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[id]
	if !ok {
		return
	}
	t.Status = status
	t.FinishedAt = time.Now().UTC()
	t.Result = result
	if err != nil {
		t.Err = err.Error()
	}
	delete(r.cancels, id)
}

// Get returns a copy of the task with the given id. Returns
// ErrTaskNotFound if no such task is registered.
func (r *TaskRegistry) Get(id string) (*BackgroundTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, ErrTaskNotFound
	}
	cp := *t
	return &cp, nil
}

// List returns a copy of every registered task, sorted by
// StartedAt descending (newest first).
func (r *TaskRegistry) List() []*BackgroundTask {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*BackgroundTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		cp := *t
		out = append(out, &cp)
	}
	sortByStartedDesc(out)
	return out
}

// Cancel cancels the goroutine for id. The task is marked
// "cancelled" by the goroutine when it observes the context
// cancellation. Returns ErrTaskNotFound if no such task.
func (r *TaskRegistry) Cancel(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, ok := r.cancels[id]
	if !ok {
		return ErrTaskNotFound
	}
	cancel()
	delete(r.cancels, id)
	return nil
}

// taskIDFromSeq formats seq as "bg-001", "bg-002", ...
func taskIDFromSeq(seq int) string {
	return "bg-" + itoa3(seq)
}

// itoa3 is a tiny zero-padded integer formatter for ids.
func itoa3(n int) string {
	if n < 0 {
		n = -n
	}
	var buf [3]byte
	for i := 2; i >= 0; i-- {
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[:])
}

// sortByStartedDesc sorts tasks newest-first.
func sortByStartedDesc(tasks []*BackgroundTask) {
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0 && tasks[j].StartedAt.After(tasks[j-1].StartedAt); j-- {
			tasks[j], tasks[j-1] = tasks[j-1], tasks[j]
		}
	}
}
