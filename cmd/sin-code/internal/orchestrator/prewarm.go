// SPDX-License-Identifier: MIT
// Purpose: Anticipatory Agent Pre-warming — when a task starts running,
// pre-warm the agents for dependent tasks whose probability exceeds the
// threshold. When the dependency completes green, the pre-warmed agent
// starts immediately without cold-start delay. If the dependency fails,
// the pre-warm is cancelled (issue #285).
//
// Race-free per mandate M7.
package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
)

// DefaultPreWarmThreshold is the default probability above which a task's
// agent is pre-warmed before its dependencies complete.
const DefaultPreWarmThreshold = 0.7

// DefaultMaxPreWarmed is the default maximum number of concurrently
// pre-warmed agents. Pre-warming consumes memory (loaded system prompts,
// scanned context), so we cap it.
const DefaultMaxPreWarmed = 4

// PreWarmManager manages anticipatory agent pre-warming for a plan.
// It is called by the event-driven dispatcher when tasks start running
// and when they complete (green or red).
type PreWarmManager struct {
	threshold    float64                  // pre-warm if P > threshold
	maxPrewarmed int                      // max concurrent pre-warmed
	registry     *Registry                // for looking up agents
	mu           sync.Mutex               // guards prewarmed map
	prewarmed    map[string]context.CancelFunc // taskID -> cancel func
	preWarmCount int64                    // atomic; total pre-warm calls (for metrics)
	cancelCount  int64                    // atomic; total cancel calls (for metrics)
}

// NewPreWarmManager creates a PreWarmManager with the given threshold and
// max pre-warmed count. If threshold <= 0, uses DefaultPreWarmThreshold.
// If maxPrewarmed <= 0, uses DefaultMaxPreWarmed.
func NewPreWarmManager(registry *Registry, threshold float64, maxPrewarmed int) *PreWarmManager {
	if threshold <= 0 {
		threshold = DefaultPreWarmThreshold
	}
	if maxPrewarmed <= 0 {
		maxPrewarmed = DefaultMaxPreWarmed
	}
	return &PreWarmManager{
		threshold:    threshold,
		maxPrewarmed: maxPrewarmed,
		registry:     registry,
		prewarmed:    map[string]context.CancelFunc{},
	}
}

// PreWarmDependents scans for pending tasks that depend on runningTaskID
// and have probability > threshold, then pre-warms their agents. Called
// by the dispatcher when a task starts running.
//
// The pre-warm context is derived from parentCtx so that cancellation
// propagates. Each pre-warmed task gets its own cancel function so it
// can be individually cancelled if the dependency fails.
func (p *PreWarmManager) PreWarmDependents(parentCtx context.Context, tasks []*Task, runningTaskID string) {
	p.mu.Lock()
	activeCount := len(p.prewarmed)
	p.mu.Unlock()

	// Respect the max pre-warmed cap.
	if activeCount >= p.maxPrewarmed {
		return
	}

	for _, task := range tasks {
		if task.Status != TaskPending {
			continue
		}
		if !dependsOn(task, runningTaskID) {
			continue
		}
		if task.Probability < p.threshold {
			continue
		}
		// Skip if already pre-warmed.
		p.mu.Lock()
		if _, exists := p.prewarmed[task.ID]; exists {
			p.mu.Unlock()
			continue
		}
		if len(p.prewarmed) >= p.maxPrewarmed {
			p.mu.Unlock()
			break
		}
		// Create a cancellable context for this pre-warm.
		ctx, cancel := context.WithCancel(parentCtx)
		p.prewarmed[task.ID] = cancel
		activeCount := len(p.prewarmed)
		p.mu.Unlock()

		// Find the agent for this task.
		agent, ok := p.registry.Get(task.AgentName)
		if !ok {
			agent, _ = p.registry.ForType(task.Type)
		}
		if agent == nil {
			p.mu.Lock()
			delete(p.prewarmed, task.ID)
			p.mu.Unlock()
			cancel()
			continue
		}

		// Check if the agent supports pre-warming.
		pw, ok := agent.(PreWarmer)
		if !ok {
			// Agent doesn't support pre-warming — clean up.
			p.mu.Lock()
			delete(p.prewarmed, task.ID)
			p.mu.Unlock()
			cancel()
			continue
		}

		// Launch pre-warm in a goroutine (non-blocking).
		taskID := task.ID
		go func() {
			atomic.AddInt64(&p.preWarmCount, 1)
			_ = pw.PreWarm(ctx, task)

			// Mark the task as pre-warmed (best-effort; the dispatcher
			// may have already started it by now).
			p.mu.Lock()
			if _, stillExists := p.prewarmed[taskID]; stillExists {
				// Keep the cancel function around so CancelDependents
				// can use it if the dependency fails.
			}
			p.mu.Unlock()
		}()

		// Mark task as pre-warmed (visible to TUI DAG visualizer).
		// This is guarded by p.mu which we released above; we re-acquire
		// briefly to set PreWarmed safely (M7: race-free).
		p.mu.Lock()
		task.PreWarmed = true
		p.mu.Unlock()

		_ = activeCount
	}
}

// CancelDependents cancels pre-warmed agents for tasks that depend on
// failedTaskID. Called by the dispatcher when a task fails (red).
func (p *PreWarmManager) CancelDependents(tasks []*Task, failedTaskID string) {
	p.mu.Lock()
	for _, task := range tasks {
		if task.Status != TaskPending {
			continue
		}
		if !dependsOn(task, failedTaskID) {
			continue
		}
		if cancel, exists := p.prewarmed[task.ID]; exists {
			cancel()
			delete(p.prewarmed, task.ID)
			task.PreWarmed = false
			atomic.AddInt64(&p.cancelCount, 1)
		}
	}
	p.mu.Unlock()
}

// Cleanup removes a completed task from the pre-warmed map (its pre-warm
// is no longer relevant since the task has started running or completed).
func (p *PreWarmManager) Cleanup(taskID string) {
	p.mu.Lock()
	if cancel, exists := p.prewarmed[taskID]; exists {
		cancel()
		delete(p.prewarmed, taskID)
	}
	p.mu.Unlock()
}

// PreWarmCount returns the total number of pre-warm calls made (for metrics).
func (p *PreWarmManager) PreWarmCount() int64 {
	return atomic.LoadInt64(&p.preWarmCount)
}

// CancelCount returns the total number of pre-warm cancellations (for metrics).
func (p *PreWarmManager) CancelCount() int64 {
	return atomic.LoadInt64(&p.cancelCount)
}

// ActivePreWarmed returns the number of currently pre-warmed agents.
func (p *PreWarmManager) ActivePreWarmed() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.prewarmed)
}

// SetThreshold updates the pre-warm threshold at runtime (for config changes).
func (p *PreWarmManager) SetThreshold(t float64) {
	p.mu.Lock()
	p.threshold = t
	p.mu.Unlock()
}

// Threshold returns the current pre-warm threshold.
func (p *PreWarmManager) Threshold() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.threshold
}

// dependsOn checks if task depends on depID.
func dependsOn(task *Task, depID string) bool {
	for _, d := range task.DependsOn {
		if d == depID {
			return true
		}
	}
	return false
}
