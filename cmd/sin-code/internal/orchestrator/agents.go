// SPDX-License-Identifier: MIT
// Purpose: Agent abstraction + mock implementation. Real LLM-backed
// implementations would call a model API; for now, agents can be run in
// "mock" mode that produces deterministic placeholder output (useful for
// testing the dispatcher, planner, and aggregator in isolation).
package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Agent interface {
	Name() string
	Config() AgentConfig
	Run(ctx context.Context, task *Task, scratch *Scratchpad) (string, error)
}

type MockAgent struct {
	cfg AgentConfig
}

func NewMockAgent(cfg AgentConfig) *MockAgent {
	return &MockAgent{cfg: cfg}
}

func (m *MockAgent) Name() string     { return m.cfg.Name }
func (m *MockAgent) Config() AgentConfig { return m.cfg }

func (m *MockAgent) Run(ctx context.Context, task *Task, scratch *Scratchpad) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(50 * time.Millisecond):
	}

	scratch.Write(m.cfg.Name, "inputs", task.Description)
	var out strings.Builder
	fmt.Fprintf(&out, "[%s] processed task %s\n", m.cfg.Name, task.ID)
	fmt.Fprintf(&out, "Type: %s\n", task.Type)
	fmt.Fprintf(&out, "Description: %s\n", task.Description)
	fmt.Fprintf(&out, "Model: %s\n", m.cfg.Model)
	result := out.String()
	scratch.Write(m.cfg.Name, "outputs:"+task.ID, result)
	return result, nil
}

func DefaultAgents() []AgentConfig {
	return []AgentConfig{
		{
			Name:        "coder",
			Description: "Writes production-quality code following project conventions",
			Type:        TaskCode,
			Model:       "anthropic/claude-sonnet-4.7",
			MaxTokens:   16000,
			Temperature: 0.0,
			SystemFile:  "agents/coder/system.md",
			MaxContext:  100000,
			ToolsAllow:  []string{"sin_*", "bash", "edit", "read", "grep"},
			ToolsDeny:   []string{"web_browser"},
			MemoryNS:    "coder",
			RetentionDays: 30,
		},
		{
			Name:        "tester",
			Description: "Writes unit, integration, and end-to-end tests",
			Type:        TaskTest,
			Model:       "anthropic/claude-haiku-4.5",
			MaxTokens:   8000,
			Temperature: 0.0,
			SystemFile:  "agents/tester/system.md",
			MaxContext:  50000,
			ToolsAllow:  []string{"sin_*", "bash", "edit", "read", "grep"},
			MemoryNS:    "tester",
			RetentionDays: 30,
		},
		{
			Name:        "reviewer",
			Description: "Senior engineer reviewing code for correctness, style, and test coverage",
			Type:        TaskReview,
			Model:       "anthropic/claude-opus-5.1",
			MaxTokens:   8000,
			Temperature: 0.0,
			SystemFile:  "agents/reviewer/system.md",
			MaxContext:  100000,
			ToolsAllow:  []string{"sin_*", "read", "grep"},
			MemoryNS:    "reviewer",
			RetentionDays: 60,
		},
		{
			Name:        "docs",
			Description: "Technical writer producing clear, accurate documentation",
			Type:        TaskDocs,
			Model:       "anthropic/claude-haiku-4.5",
			MaxTokens:   8000,
			Temperature: 0.2,
			SystemFile:  "agents/docs/system.md",
			MaxContext:  50000,
			ToolsAllow:  []string{"sin_*", "read", "edit"},
			MemoryNS:    "docs",
			RetentionDays: 60,
		},
		{
			Name:        "security",
			Description: "Application security specialist scanning for vulnerabilities",
			Type:        TaskSecurity,
			Model:       "anthropic/claude-sonnet-4.7",
			MaxTokens:   8000,
			Temperature: 0.0,
			SystemFile:  "agents/security/system.md",
			MaxContext:  100000,
			ToolsAllow:  []string{"sin_*", "read", "grep"},
			MemoryNS:    "security",
			RetentionDays: 90,
		},
		{
			Name:        "architect",
			Description: "Principal architect designing high-level solutions",
			Type:        TaskArchitect,
			Model:       "anthropic/claude-opus-5.1",
			MaxTokens:   16000,
			Temperature: 0.1,
			SystemFile:  "agents/architect/system.md",
			MaxContext:  150000,
			ToolsAllow:  []string{"sin_*", "read", "map"},
			MemoryNS:    "architect",
			RetentionDays: 90,
		},
	}
}
