// SPDX-License-Identifier: MIT
// Purpose: coverage tests for agents.go (MockAgent PreWarm/PreWarmCount) and
// related functions that run without the "coverage" build tag.
package orchestrator

import (
	"context"
	"testing"
)

func TestMockAgentPreWarm(t *testing.T) {
	a := NewMockAgent(AgentConfig{Name: "test", Type: TaskCode, Model: "m"})
	if a.PreWarmCount() != 0 {
		t.Fatalf("initial PreWarmCount = %d, want 0", a.PreWarmCount())
	}
	if err := a.PreWarm(context.Background(), &Task{ID: "t1"}); err != nil {
		t.Fatalf("PreWarm: %v", err)
	}
	if a.PreWarmCount() != 1 {
		t.Errorf("PreWarmCount = %d, want 1", a.PreWarmCount())
	}
	// Multiple calls should increment.
	_ = a.PreWarm(context.Background(), &Task{ID: "t2"})
	_ = a.PreWarm(context.Background(), &Task{ID: "t3"})
	if a.PreWarmCount() != 3 {
		t.Errorf("PreWarmCount = %d, want 3", a.PreWarmCount())
	}
}

func TestMockAgentPreWarmCancelledContext(t *testing.T) {
	a := NewMockAgent(AgentConfig{Name: "test", Type: TaskCode, Model: "m"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.PreWarm(ctx, &Task{ID: "t1"}); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// PreWarmCount should NOT increment when context is cancelled.
	if a.PreWarmCount() != 0 {
		t.Errorf("PreWarmCount = %d, want 0 (cancelled context)", a.PreWarmCount())
	}
}

func TestMockAgentRunPreWarmedFlag(t *testing.T) {
	a := NewMockAgent(AgentConfig{Name: "test", Type: TaskCode, Model: "m"})
	task := &Task{
		ID:          "t1",
		Type:        TaskCode,
		Description: "do something",
		PreWarmed:   true,
	}
	out, err := a.Run(context.Background(), task, NewScratchpad())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestMockAgentRunCancelledContext(t *testing.T) {
	a := NewMockAgent(AgentConfig{Name: "test", Type: TaskCode, Model: "m"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Run(ctx, &Task{ID: "t1"}, NewScratchpad())
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestMockAgentNameAndConfig(t *testing.T) {
	cfg := AgentConfig{Name: "myagent", Type: TaskReview, Model: "m"}
	a := NewMockAgent(cfg)
	if a.Name() != "myagent" {
		t.Errorf("Name() = %q, want %q", a.Name(), "myagent")
	}
	if a.Config().Name != "myagent" {
		t.Errorf("Config().Name = %q, want %q", a.Config().Name, "myagent")
	}
	if a.Config().Type != TaskReview {
		t.Errorf("Config().Type = %q, want %q", a.Config().Type, TaskReview)
	}
}

func TestDefaultAgentsComplete(t *testing.T) {
	agents := DefaultAgents()
	if len(agents) != 6 {
		t.Fatalf("expected 6 default agents, got %d", len(agents))
	}
	names := map[string]bool{}
	for _, a := range agents {
		if a.Name == "" {
			t.Error("agent with empty name")
		}
		if a.Model == "" {
			t.Errorf("agent %q has empty model", a.Name)
		}
		if a.Type == "" {
			t.Errorf("agent %q has empty type", a.Name)
		}
		names[a.Name] = true
	}
	expected := []string{"coder", "tester", "reviewer", "docs", "security", "architect"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing agent: %s", name)
		}
	}
}

func TestDefaultAgentsToolsAllow(t *testing.T) {
	agents := DefaultAgents()
	for _, a := range agents {
		if len(a.ToolsAllow) == 0 {
			t.Errorf("agent %q has empty ToolsAllow", a.Name)
		}
	}
}
