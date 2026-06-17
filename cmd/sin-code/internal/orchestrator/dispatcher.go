// SPDX-License-Identifier: MIT
// Purpose: dispatcher — runs plan tasks in parallel with dependency-aware
// scheduling. Tasks whose DependsOn are still running block until those
// complete. Results are merged into a shared scratchpad.
//
// Issue #283: replaced 50ms polling loop with event-driven channel-based
// dispatch. Task completion sends on notifyCh, which wakes the scheduler
// instantly to check newly-ready dependents. Zero polling latency.
package orchestrator

import (
	"context"
	"fmt"
	"sync"
)

type Dispatcher struct {
	registry *Registry
	scratch  *Scratchpad
	maxPar   int
	PreWarm  *PreWarmManager
}

func NewDispatcher(registry *Registry, scratch *Scratchpad, maxParallel int) *Dispatcher {
	if maxParallel <= 0 {
		maxParallel = 4
	}
	return &Dispatcher{
		registry: registry,
		scratch:  scratch,
		maxPar:   maxParallel,
	}
}

// TaskEvent is fired when a task changes state. The TUI DAG visualizer
// (issue #286) subscribes to these for live updates.
type TaskEvent struct {
	TaskID string
	Status TaskStatus
	Result string
	Error  string
}

func (d *Dispatcher) Dispatch(ctx context.Context, plan *Plan) error {
	plan.Started = timeNow()
	tasks := plan.Tasks
	if len(tasks) == 0 {
		plan.Completed = timeNow()
		return nil
	}

	completed := map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, d.maxPar)

	// Event-driven notify channel: task completion wakes the scheduler
	// instantly. Replaces the old 50ms time.After polling (issue #283).
	notifyCh := make(chan string, len(tasks))
	errCh := make(chan error, len(tasks))

	// Launch initial ready tasks, then wait on notifyCh for completions.
	d.launchReady(ctx, plan, tasks, &mu, completed, &wg, sem, notifyCh, errCh)

	for {
		mu.Lock()
		allDone := true
		for _, t := range tasks {
			if t.Status == TaskPending || t.Status == TaskRunning {
				allDone = false
				break
			}
		}
		mu.Unlock()
		if allDone {
			break
		}
		// Wait for a task to complete (event-driven, no polling).
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case <-notifyCh:
		}
		d.launchReady(ctx, plan, tasks, &mu, completed, &wg, sem, notifyCh, errCh)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			plan.Success = false
			plan.Completed = timeNow()
			return err
		}
	}
	allOK := true
	for _, t := range tasks {
		if t.Status != TaskCompleted {
			allOK = false
		}
		plan.TotalCost += t.Cost
		plan.TokensUsed += t.TokensUsed
	}
	plan.Success = allOK
	plan.Completed = timeNow()
	return nil
}

// launchReady finds all pending tasks whose dependencies are complete and
// launches them as goroutines. Must be called with mu NOT held.
func (d *Dispatcher) launchReady(ctx context.Context, plan *Plan, tasks []*Task, mu *sync.Mutex, completed map[string]bool, wg *sync.WaitGroup, sem chan struct{}, notifyCh chan<- string, errCh chan error) {
	mu.Lock()
	ready := []*Task{}
	for _, t := range tasks {
		if t.Status != TaskPending {
			continue
		}
		if allDepsDone(t, completed) {
			ready = append(ready, t)
		}
	}
	for _, task := range ready {
		task.Status = TaskRunning
		now := timeNow()
		task.Started = &now
		if d.PreWarm != nil {
			d.PreWarm.PreWarmDependents(ctx, tasks, task.ID)
		}
	}
	mu.Unlock()

	for _, task := range ready {
		wg.Add(1)
		sem <- struct{}{}
		go func(t *Task) {
			defer wg.Done()
			defer func() { <-sem }()
			d.runOne(ctx, plan, t, mu, completed, notifyCh, errCh)
		}(task)
	}
}

func (d *Dispatcher) runOne(ctx context.Context, plan *Plan, task *Task, mu *sync.Mutex, completed map[string]bool, notifyCh chan<- string, errCh chan error) {
	agent, ok := d.registry.Get(task.AgentName)
	if !ok {
		agent, _ = d.registry.ForType(task.Type)
	}
	if agent == nil {
		now := timeNow()
		mu.Lock()
		task.Status = TaskFailed
		task.Error = fmt.Sprintf("no agent for type %s", task.Type)
		task.Completed = &now
		completed[task.ID] = true
		mu.Unlock()
		notifyCh <- task.ID
		errCh <- fmt.Errorf("no agent for %s", task.Type)
		return
	}
	d.scratch.Write(task.AgentName, "plan:"+plan.ID, task.Description)
	out, err := agent.Run(ctx, task, d.scratch)
	now := timeNow()
	mu.Lock()
	task.Completed = &now
	if err != nil {
		task.Status = TaskFailed
		task.Error = err.Error()
		if d.PreWarm != nil {
			d.PreWarm.CancelDependents(plan.Tasks, task.ID)
		}
	} else {
		task.Status = TaskCompleted
		task.Result = out
		task.TokensUsed = estimateTokens(out)
		task.Cost = estimateCost(task.TokensUsed, agent.Config().Model)
		if d.PreWarm != nil {
			d.PreWarm.Cleanup(task.ID)
		}
	}
	completed[task.ID] = true
	mu.Unlock()
	// Notify the scheduler instantly — no polling delay (issue #283).
	notifyCh <- task.ID
}

func allDepsDone(t *Task, completed map[string]bool) bool {
	for _, dep := range t.DependsOn {
		if !completed[dep] {
			return false
		}
	}
	return true
}

func estimateTokens(s string) int {
	return len(s) / 4
}

func estimateCost(tokens int, model string) float64 {
	var perMillion float64
	switch {
	case containsAny(model, "opus"):
		perMillion = 15.0
	case containsAny(model, "sonnet"):
		perMillion = 3.0
	case containsAny(model, "haiku"):
		perMillion = 0.25
	default:
		perMillion = 1.0
	}
	return float64(tokens) / 1_000_000.0 * perMillion
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub == "" {
			continue
		}
		if len(s) >= len(sub) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
