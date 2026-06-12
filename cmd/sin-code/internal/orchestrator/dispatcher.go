// SPDX-License-Identifier: MIT
// Purpose: dispatcher — runs plan tasks in parallel with dependency-aware
// scheduling. Tasks whose DependsOn are still running block until those
// complete. Results are merged into a shared scratchpad.
package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Dispatcher struct {
	registry *Registry
	scratch  *Scratchpad
	maxPar   int
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
	errCh := make(chan error, len(tasks))

	for {
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
		mu.Unlock()
		if len(ready) == 0 {
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
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}
		for _, task := range ready {
			mu.Lock()
			task.Status = TaskRunning
			now := timeNow()
			task.Started = &now
			mu.Unlock()
			wg.Add(1)
			sem <- struct{}{}
			go func(t *Task) {
				defer wg.Done()
				defer func() { <-sem }()
				d.runOne(ctx, plan, t, &mu, completed, errCh)
			}(task)
		}
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

func (d *Dispatcher) runOne(ctx context.Context, plan *Plan, task *Task, mu *sync.Mutex, completed map[string]bool, errCh chan error) {
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
	} else {
		task.Status = TaskCompleted
		task.Result = out
		task.TokensUsed = estimateTokens(out)
		task.Cost = estimateCost(task.TokensUsed, agent.Config().Model)
	}
	completed[task.ID] = true
	mu.Unlock()
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
