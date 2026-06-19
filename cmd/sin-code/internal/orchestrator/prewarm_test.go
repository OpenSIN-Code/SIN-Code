// SPDX-License-Identifier: MIT
// Purpose: tests for issue #285 — Anticipatory Agent Pre-warming. When a
// task starts running, the PreWarmManager pre-warms agents for dependent
// tasks with P > threshold. When a dependency fails, pre-warms are cancelled.
// All tests pass under -race (mandate M7).
package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"
)

func newPreWarmTestRegistry() *Registry {
	cfgs := DefaultAgents()
	agents := make([]Agent, len(cfgs))
	for i, cfg := range cfgs {
		agents[i] = NewMockAgent(cfg)
	}
	return NewRegistry(agents)
}

func TestPreWarmManager_PreWarmDependents(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.5, 4)

	archID := "arch-1"
	tasks := []*Task{
		{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning, Probability: 1.0},
		{ID: "coder-1", Type: TaskCode, AgentName: "coder", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.95},
		{ID: "sec-1", Type: TaskSecurity, AgentName: "security", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.70},
		{ID: "docs-1", Type: TaskDocs, AgentName: "docs", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.40}, // below threshold
	}

	pwm.PreWarmDependents(context.Background(), tasks, archID)

	// coder (P=0.95) and security (P=0.70) should be pre-warmed.
	// docs (P=0.40) is below threshold 0.5 → not pre-warmed.
	if !tasks[1].PreWarmed {
		t.Error("coder task should be pre-warmed (P=0.95 > 0.5)")
	}
	if !tasks[2].PreWarmed {
		t.Error("security task should be pre-warmed (P=0.70 > 0.5)")
	}
	if tasks[3].PreWarmed {
		t.Error("docs task should NOT be pre-warmed (P=0.40 < 0.5)")
	}

	// Wait for goroutines to finish.
	time.Sleep(20 * time.Millisecond)
	if pwm.PreWarmCount() < 2 {
		t.Errorf("expected >= 2 pre-warm calls, got %d", pwm.PreWarmCount())
	}
}

func TestPreWarmManager_CancelOnFailure(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.5, 4)

	archID := "arch-1"
	tasks := []*Task{
		{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning, Probability: 1.0},
		{ID: "coder-1", Type: TaskCode, AgentName: "coder", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.95},
	}

	// Pre-warm the coder agent.
	pwm.PreWarmDependents(context.Background(), tasks, archID)
	time.Sleep(10 * time.Millisecond)
	if !tasks[1].PreWarmed {
		t.Fatal("coder should be pre-warmed")
	}

	// Simulate architect failing — should cancel the pre-warm.
	pwm.CancelDependents(tasks, archID)
	if tasks[1].PreWarmed {
		t.Error("coder PreWarmed should be reset to false after dependency failure")
	}
	if pwm.CancelCount() < 1 {
		t.Errorf("expected >= 1 cancel, got %d", pwm.CancelCount())
	}
}

func TestPreWarmManager_ThresholdFiltering(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.8, 4) // high threshold

	archID := "arch-1"
	tasks := []*Task{
		{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning},
		{ID: "coder-1", Type: TaskCode, AgentName: "coder", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.95},
		{ID: "sec-1", Type: TaskSecurity, AgentName: "security", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.70},
	}

	pwm.PreWarmDependents(context.Background(), tasks, archID)
	time.Sleep(10 * time.Millisecond)

	// Only coder (P=0.95 > 0.8) should be pre-warmed.
	// security (P=0.70 < 0.8) should NOT.
	if !tasks[1].PreWarmed {
		t.Error("coder should be pre-warmed (P=0.95 > 0.8)")
	}
	if tasks[2].PreWarmed {
		t.Error("security should NOT be pre-warmed (P=0.70 < 0.8)")
	}
}

func TestPreWarmManager_MaxPreWarmedCap(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.5, 2) // max 2 pre-warmed

	archID := "arch-1"
	tasks := []*Task{
		{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning},
		{ID: "t1", Type: TaskCode, AgentName: "coder", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.9},
		{ID: "t2", Type: TaskSecurity, AgentName: "security", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.9},
		{ID: "t3", Type: TaskDocs, AgentName: "docs", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.9},
	}

	pwm.PreWarmDependents(context.Background(), tasks, archID)
	time.Sleep(20 * time.Millisecond)

	preWarmedCount := 0
	for _, task := range tasks[1:] {
		if task.PreWarmed {
			preWarmedCount++
		}
	}
	if preWarmedCount > 2 {
		t.Errorf("expected at most 2 pre-warmed (cap), got %d", preWarmedCount)
	}
}

func TestPreWarmManager_Cleanup(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.5, 4)

	archID := "arch-1"
	tasks := []*Task{
		{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning},
		{ID: "coder-1", Type: TaskCode, AgentName: "coder", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.95},
	}

	pwm.PreWarmDependents(context.Background(), tasks, archID)
	time.Sleep(10 * time.Millisecond)

	// Cleanup removes the task from the pre-warmed map.
	pwm.Cleanup("coder-1")
	if pwm.ActivePreWarmed() > 0 {
		t.Errorf("expected 0 active pre-warmed after cleanup, got %d", pwm.ActivePreWarmed())
	}
}

func TestPreWarmManager_NoDoublePreWarm(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.5, 4)

	archID := "arch-1"
	tasks := []*Task{
		{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning},
		{ID: "coder-1", Type: TaskCode, AgentName: "coder", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.95},
	}

	// Call PreWarmDependents twice — should only pre-warm once.
	pwm.PreWarmDependents(context.Background(), tasks, archID)
	pwm.PreWarmDependents(context.Background(), tasks, archID)
	time.Sleep(20 * time.Millisecond)

	if pwm.PreWarmCount() > 1 {
		t.Errorf("expected at most 1 pre-warm call (no double), got %d", pwm.PreWarmCount())
	}
}

func TestPreWarmManager_SkipRunningTasks(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.5, 4)

	archID := "arch-1"
	tasks := []*Task{
		{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning},
		{ID: "coder-1", Type: TaskCode, AgentName: "coder", Status: TaskRunning, DependsOn: []string{archID}, Probability: 0.95},
	}

	pwm.PreWarmDependents(context.Background(), tasks, archID)
	time.Sleep(10 * time.Millisecond)

	// coder-1 is already running, should not be pre-warmed.
	if pwm.PreWarmCount() > 0 {
		t.Errorf("expected 0 pre-warm calls for running task, got %d", pwm.PreWarmCount())
	}
}

func TestPreWarmManager_SkipAgentWithoutPreWarmer(t *testing.T) {
	// Registry with a plain agent that doesn't implement PreWarmer.
	cfg := AgentConfig{Name: "plain", Type: TaskCode, Model: "test"}
	r := NewRegistry([]Agent{&nonPreWarmerAgent{cfg: cfg}})
	pwm := NewPreWarmManager(r, 0.5, 4)

	archID := "arch-1"
	tasks := []*Task{
		{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning},
		{ID: "coder-1", Type: TaskCode, AgentName: "plain", Status: TaskPending, DependsOn: []string{archID}, Probability: 0.95},
	}

	pwm.PreWarmDependents(context.Background(), tasks, archID)
	time.Sleep(10 * time.Millisecond)

	if tasks[1].PreWarmed {
		t.Error("task should not be pre-warmed when agent doesn't implement PreWarmer")
	}
}

func TestPreWarmManager_SetThreshold(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.9, 4)

	if pwm.Threshold() != 0.9 {
		t.Errorf("threshold = %.2f, want 0.9", pwm.Threshold())
	}

	pwm.SetThreshold(0.5)
	if pwm.Threshold() != 0.5 {
		t.Errorf("threshold = %.2f, want 0.5", pwm.Threshold())
	}
}

func TestPreWarmManager_RaceFree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race stress test in short mode")
	}
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0.3, 10)

	archID := "arch-1"
	tasks := make([]*Task, 20)
	tasks[0] = &Task{ID: archID, Type: TaskArchitect, AgentName: "architect", Status: TaskRunning}
	for i := 1; i < 20; i++ {
		tasks[i] = &Task{
			ID:          GenerateID("tk"),
			Type:        TaskCode,
			AgentName:   "coder",
			Status:      TaskPending,
			DependsOn:   []string{archID},
			Probability: 0.9,
		}
	}

	// Concurrent pre-warm + cancel to stress the race detector.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			pwm.PreWarmDependents(context.Background(), tasks, archID)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			pwm.CancelDependents(tasks, archID)
		}
	}()
	wg.Wait()
}

func TestPreWarmManager_DefaultThreshold(t *testing.T) {
	r := newPreWarmTestRegistry()
	pwm := NewPreWarmManager(r, 0, 0) // zeros → defaults

	if pwm.Threshold() != DefaultPreWarmThreshold {
		t.Errorf("default threshold = %.2f, want %.2f", pwm.Threshold(), DefaultPreWarmThreshold)
	}
}

// nonPreWarmerAgent is an Agent that does NOT implement PreWarmer.
type nonPreWarmerAgent struct {
	cfg AgentConfig
}

func (n *nonPreWarmerAgent) Name() string        { return n.cfg.Name }
func (n *nonPreWarmerAgent) Config() AgentConfig { return n.cfg }
func (n *nonPreWarmerAgent) Run(ctx context.Context, task *Task, s *Scratchpad) (string, error) {
	return "ok", nil
}
