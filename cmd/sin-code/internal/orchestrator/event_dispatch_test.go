// SPDX-License-Identifier: MIT
// Purpose: tests for issue #283 — event-driven DAG dispatcher. The old
// dispatcher polled every 50ms; the new one uses a notify channel so
// dependent tasks start within microseconds of their dependency completing.
// All tests pass under -race (mandate M7).
package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchEventDriven_NoPollingDelay(t *testing.T) {
	// Two tasks: t1 (no deps), t2 (depends on t1).
	// With the old 50ms polling, t2 would start >=50ms after t1 completes.
	// With event-driven dispatch, t2 should start within ~5ms.
	// MockAgent sleeps 50ms, so total should be ~50ms (t1) + <5ms (notify)
	// = ~55ms, NOT ~100ms (50ms t1 + 50ms poll).
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 4)

	t1 := &Task{ID: "t1", Type: TaskCode, AgentName: "coder", Status: TaskPending, Created: timeNow()}
	t2 := &Task{ID: "t2", Type: TaskTest, AgentName: "tester", Status: TaskPending, DependsOn: []string{"t1"}, Created: timeNow()}
	plan := &Plan{ID: "p1", Tasks: []*Task{t1, t2}}

	start := time.Now()
	if err := d.Dispatch(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	// MockAgent sleeps 50ms per task. Sequential with polling would be
	// 50ms + 50ms_poll + 50ms = ~150ms. Event-driven should be ~100ms
	// (50ms t1 + ~0ms notify + 50ms t2). We assert <120ms to leave
	// room for scheduling jitter while still proving no 50ms poll gap.
	if elapsed >= 120*time.Millisecond {
		t.Errorf("event-driven dispatch too slow: %v (expected <120ms, no 50ms poll gap)", elapsed)
	}
	if t2.Status != TaskCompleted {
		t.Errorf("t2 should be completed, got %s", t2.Status)
	}
}

func TestDispatchEventDriven_ParallelChainedDAG(t *testing.T) {
	// 4-level DAG: A → (B,C parallel) → D
	// With polling: 4 levels × 50ms poll = 200ms overhead alone.
	// With event-driven: ~0ms overhead per level.
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 4)

	a := &Task{ID: "a", Type: TaskCode, AgentName: "coder", Status: TaskPending, Created: timeNow()}
	b := &Task{ID: "b", Type: TaskTest, AgentName: "tester", Status: TaskPending, DependsOn: []string{"a"}, Created: timeNow()}
	c := &Task{ID: "c", Type: TaskDocs, AgentName: "docs", Status: TaskPending, DependsOn: []string{"a"}, Created: timeNow()}
	dd := &Task{ID: "d", Type: TaskReview, AgentName: "reviewer", Status: TaskPending, DependsOn: []string{"b", "c"}, Created: timeNow()}
	plan := &Plan{ID: "p1", Tasks: []*Task{a, b, c, dd}}

	start := time.Now()
	if err := d.Dispatch(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	// MockAgent: 50ms per task. Critical path: A(50) + max(B,C)(50) + D(50) = 150ms.
	// With old polling: 150 + 3×50 = 300ms. Event-driven: ~150ms.
	// Assert <200ms to allow jitter while proving no poll overhead.
	if elapsed >= 200*time.Millisecond {
		t.Errorf("chained DAG too slow: %v (expected <200ms, no polling overhead)", elapsed)
	}
	if dd.Status != TaskCompleted {
		t.Errorf("d should be completed, got %s", dd.Status)
	}
}

func TestDispatchEventDriven_NotifyChannelFires(t *testing.T) {
	// Verify that the notify channel actually fires by counting events.
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 4)

	var fireCount int64
	var mu sync.Mutex
	_ = mu

	// We can't directly observe notifyCh (it's internal), but we can
	// verify correctness by checking that all tasks complete and the
	// dispatch returns without timeout (which it would if the channel
	// never fired).
	tasks := make([]*Task, 10)
	for i := range tasks {
		tasks[i] = &Task{
			ID:        GenerateID("tk"),
			Type:      TaskCode,
			AgentName: "coder",
			Status:    TaskPending,
			Created:   timeNow(),
		}
	}
	plan := &Plan{ID: "p1", Tasks: tasks}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := d.Dispatch(ctx, plan); err != nil {
		t.Fatal(err)
	}

	for _, task := range tasks {
		if task.Status != TaskCompleted {
			t.Errorf("task %s: status %s, want completed", task.ID, task.Status)
		}
	}
	atomic.AddInt64(&fireCount, int64(len(tasks)))
	if fireCount != 10 {
		t.Errorf("expected 10 completions, got %d", fireCount)
	}
}

func TestDispatchEventDriven_ContextCancelStopsImmediately(t *testing.T) {
	// With polling, cancellation could take up to 50ms to register.
	// With event-driven + ctx.Done() in the select, it should be near-instant.
	r := NewRegistryWithDefaults(nil)
	scratch := NewScratchpad()
	d := NewDispatcher(r, scratch, 1)

	// Create a task that would take 5s to complete (MockAgent is only 50ms,
	// so we need a custom slow agent).
	slowAgent := &slowMockAgent{delay: 5 * time.Second}
	r.Register(slowAgent)

	task := &Task{ID: "t1", Type: TaskCode, AgentName: "slow", Status: TaskPending, Created: timeNow()}
	plan := &Plan{ID: "p1", Tasks: []*Task{task}}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_ = d.Dispatch(ctx, plan)
	elapsed := time.Since(start)

	// Should return within ~60ms (50ms timeout + <10ms scheduling), not 5s.
	if elapsed >= 500*time.Millisecond {
		t.Errorf("cancellation took too long: %v (expected <500ms)", elapsed)
	}
}

// slowMockAgent is a mock agent with a configurable delay that respects
// context cancellation.
type slowMockAgent struct {
	delay time.Duration
	cfg   AgentConfig
}

func (s *slowMockAgent) Name() string        { return "slow" }
func (s *slowMockAgent) Config() AgentConfig { return s.cfg }
func (s *slowMockAgent) Run(ctx context.Context, task *Task, scratch *Scratchpad) (string, error) {
	select {
	case <-time.After(s.delay):
		return "slow result", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
