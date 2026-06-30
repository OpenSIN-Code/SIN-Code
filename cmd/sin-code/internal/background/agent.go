// SPDX-License-Identifier: MIT
// Purpose: Background agent execution — fire-and-forget async agent tasks.
// Tracks goroutine-backed agent jobs with status, progress, and result retrieval.
// Issue #479.
//
// Mandates:
//   M2 — pure Go, CGO_ENABLED=0.
//   M5 — module path github.com/OpenSIN-Code/SIN-Code.
//   M7 — all shared state guarded by sync.RWMutex; race-free.
package background

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Job represents a background agent task.
type Job struct {
	ID        string
	Prompt    string
	Status    JobStatus
	StartedAt time.Time
	EndedAt   time.Time
	Result    string
	Error     error
}

// JobStatus is the lifecycle state of a background job.
type JobStatus int

const (
	StatusPending JobStatus = iota
	StatusRunning
	StatusDone
	StatusFailed
)

func (s JobStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Manager tracks background agent jobs.
type Manager struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewManager creates a new background job manager.
func NewManager() *Manager {
	return &Manager{jobs: make(map[string]*Job)}
}

// Start launches a background job. The runFn is executed in a goroutine.
// Returns the job immediately (fire-and-forget).
func (m *Manager) Start(ctx context.Context, id, prompt string, runFn func(ctx context.Context) (string, error)) *Job {
	job := &Job{
		ID:        id,
		Prompt:    prompt,
		Status:    StatusPending,
		StartedAt: time.Now(),
	}
	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()

	go func() {
		m.mu.Lock()
		job.Status = StatusRunning
		m.mu.Unlock()

		result, err := runFn(ctx)

		m.mu.Lock()
		job.EndedAt = time.Now()
		job.Result = result
		job.Error = err
		if err != nil {
			job.Status = StatusFailed
		} else {
			job.Status = StatusDone
		}
		m.mu.Unlock()
	}()

	return job
}

// Get returns a job by ID.
func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	return j, ok
}

// List returns all jobs.
func (m *Manager) List() []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j)
	}
	return out
}

// Active returns running/pending jobs.
func (m *Manager) Active() []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Job
	for _, j := range m.jobs {
		if j.Status == StatusRunning || j.Status == StatusPending {
			out = append(out, j)
		}
	}
	return out
}

// FormatJob renders a job as a one-liner.
func FormatJob(j *Job) string {
	dur := ""
	if !j.EndedAt.IsZero() {
		dur = j.EndedAt.Sub(j.StartedAt).Round(time.Millisecond).String()
	} else {
		dur = time.Since(j.StartedAt).Round(time.Second).String()
	}
	return fmt.Sprintf("[%s] %s — %s (%s)", j.ID, j.Status, truncate(j.Prompt, 50), dur)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
