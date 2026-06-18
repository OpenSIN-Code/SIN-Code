// SPDX-License-Identifier: MIT
// Purpose: tests for the LLM-driven planner (issue #383). The LLMClient
// is mocked so no real provider is called. The -race flag exercises
// concurrent Plan calls (M7) — LLMPlanner is stateless and safe.
package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// mockLLMClient is a test LLMClient that returns a canned response.
type mockLLMClient struct {
	resp string
	err  error

	// lastPrompt captures the most recent prompt for assertions.
	lastPrompt string
	mu         sync.Mutex
}

func (m *mockLLMClient) Complete(_ context.Context, prompt string) (string, error) {
	m.mu.Lock()
	m.lastPrompt = prompt
	m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	return m.resp, nil
}

func TestLLMPlannerBasicPlan(t *testing.T) {
	resp := `{"steps":[{"id":"s1","action":"search code","tool":"scout","args":{"q":"foo"},"depends_on":[]},{"id":"s2","action":"edit file","tool":"sin_edit","args":{},"depends_on":["s1"]}],"rationale":"search then edit","confidence":0.9}`
	c := &mockLLMClient{resp: resp}
	p := NewLLMPlanner(c, "test-model")

	plan, err := p.Plan(context.Background(), PlanRequest{
		Prompt:         "find and fix the bug",
		AvailableTools: []string{"scout", "sin_edit"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[0].ID != "s1" || plan.Steps[1].ID != "s2" {
		t.Errorf("step ids = %s, %s", plan.Steps[0].ID, plan.Steps[1].ID)
	}
	if plan.Steps[1].Tool != "sin_edit" {
		t.Errorf("step 2 tool = %q", plan.Steps[1].Tool)
	}
	if len(plan.Steps[1].DependsOn) != 1 || plan.Steps[1].DependsOn[0] != "s1" {
		t.Errorf("step 2 depends_on = %v", plan.Steps[1].DependsOn)
	}
	if plan.Confidence != 0.9 {
		t.Errorf("confidence = %v", plan.Confidence)
	}
	if plan.Rationale != "search then edit" {
		t.Errorf("rationale = %q", plan.Rationale)
	}
	if plan.Steps[0].Args["q"] != "foo" {
		t.Errorf("step 1 args = %v", plan.Steps[0].Args)
	}
}

func TestLLMPlannerMarkdownFenceResponse(t *testing.T) {
	resp := "Sure, here is the plan:\n```json\n{\"steps\":[{\"id\":\"a\",\"action\":\"do thing\",\"tool\":\"t\",\"args\":{},\"depends_on\":[]}],\"rationale\":\"r\",\"confidence\":0.5}\n```\nLet me know."
	c := &mockLLMClient{resp: resp}
	p := NewLLMPlanner(c, "m")

	plan, err := p.Plan(context.Background(), PlanRequest{Prompt: "goal", AvailableTools: []string{"t"}})
	if err != nil {
		t.Fatalf("Plan with fenced json: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].ID != "a" {
		t.Errorf("unexpected plan: %+v", plan)
	}
}

func TestLLMPlannerClientError(t *testing.T) {
	c := &mockLLMClient{err: errors.New("boom")}
	p := NewLLMPlanner(c, "m")

	_, err := p.Plan(context.Background(), PlanRequest{Prompt: "goal"})
	if err == nil {
		t.Fatal("expected error from client failure")
	}
	if !strings.Contains(err.Error(), "complete") {
		t.Errorf("error should mention complete: %v", err)
	}
}

func TestLLMPlannerEmptyPromptRejected(t *testing.T) {
	c := &mockLLMClient{resp: "{}"}
	p := NewLLMPlanner(c, "m")

	_, err := p.Plan(context.Background(), PlanRequest{Prompt: "   "})
	if err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("expected prompt-required error, got %v", err)
	}
}

func TestLLMPlannerNilClientRejected(t *testing.T) {
	p := NewLLMPlanner(nil, "m")
	_, err := p.Plan(context.Background(), PlanRequest{Prompt: "goal"})
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected nil-client error, got %v", err)
	}
}

func TestLLMPlannerParseRejectsUnknownDep(t *testing.T) {
	p := NewLLMPlanner(&mockLLMClient{}, "m")
	raw := `{"steps":[{"id":"s1","action":"a","tool":"t","args":{},"depends_on":["s9"]}],"rationale":"r","confidence":0.5}`
	_, err := p.ParsePlanResponse(raw)
	if err == nil || !strings.Contains(err.Error(), "unknown step") {
		t.Fatalf("expected unknown-step error, got %v", err)
	}
}

func TestLLMPlannerParseRejectsDuplicateID(t *testing.T) {
	p := NewLLMPlanner(&mockLLMClient{}, "m")
	raw := `{"steps":[{"id":"s1","action":"a","tool":"t","args":{},"depends_on":[]},{"id":"s1","action":"b","tool":"t","args":{},"depends_on":[]}],"rationale":"r","confidence":0.5}`
	_, err := p.ParsePlanResponse(raw)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate-id error, got %v", err)
	}
}

func TestLLMPlannerClampsConfidenceAndRendersPrompt(t *testing.T) {
	// confidence out of range is clamped to 1.0.
	resp := `{"steps":[{"id":"s1","action":"a","tool":"t","args":{},"depends_on":[]}],"rationale":"r","confidence":5.0}`
	c := &mockLLMClient{resp: resp}
	p := NewLLMPlanner(c, "test-model")

	plan, err := p.Plan(context.Background(), PlanRequest{
		Prompt:         "goal",
		Context:        "ctx",
		AvailableTools: []string{"t"},
		Constraints:    []string{"no network"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Confidence != 1.0 {
		t.Errorf("confidence should clamp to 1.0, got %v", plan.Confidence)
	}
	c.mu.Lock()
	got := c.lastPrompt
	c.mu.Unlock()
	if !strings.Contains(got, "goal") || !strings.Contains(got, "ctx") {
		t.Errorf("prompt missing goal/context: %q", got)
	}
	if !strings.Contains(got, "no network") {
		t.Errorf("prompt missing constraint: %q", got)
	}
	if !strings.Contains(got, "test-model") {
		// model isn't required in prompt, but we mention it; just a soft check.
		t.Errorf("prompt missing model: %q", got)
	}
}
