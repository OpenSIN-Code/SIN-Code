// SPDX-License-Identifier: MIT
// Purpose: tests for issue #286 — DAG Visualizer TUI view.
package tui

import (
	"strings"
	"testing"
)

func TestViewDAGExists(t *testing.T) {
	if ViewDAG.String() != "DAG" {
		t.Errorf("ViewDAG.String() = %q, want %q", ViewDAG.String(), "DAG")
	}
	if ViewDAG.Short() != "8·DAG" {
		t.Errorf("ViewDAG.Short() = %q, want %q", ViewDAG.Short(), "8·DAG")
	}
}

func TestDAGInSidebarItems(t *testing.T) {
	items := DefaultSidebarItems()
	found := false
	for _, item := range items {
		if item.View == ViewDAG {
			found = true
			if item.Shortcut != "8" {
				t.Errorf("DAG shortcut = %q, want 8", item.Shortcut)
			}
		}
	}
	if !found {
		t.Error("ViewDAG not found in DefaultSidebarItems")
	}
}

func TestDAGHintsExist(t *testing.T) {
	hints := DefaultHints(ViewDAG)
	if len(hints) == 0 {
		t.Fatal("expected hints for ViewDAG, got none")
	}
	hasNav := false
	for _, h := range hints {
		if h.Key == "↑/↓" {
			hasNav = true
		}
	}
	if !hasNav {
		t.Error("expected ↑/↓ navigate hint for DAG view")
	}
}

func TestRenderDAGViewEmpty(t *testing.T) {
	state := DAGState{Tasks: nil}
	styles := NewStyles(Themes[0])
	out := RenderDAGView(state, styles, 80, 24)
	if !strings.Contains(out, "No active plan") {
		t.Errorf("expected 'No active plan' message, got:\n%s", out)
	}
}

func TestRenderDAGViewWithTasks(t *testing.T) {
	state := DAGState{
		Prompt: "Build auth module",
		Tasks: []DAGTaskRow{
			{ID: "t1", Type: "architect", Description: "Design auth flow", Status: "completed", Probability: 1.0, AgentName: "architect"},
			{ID: "t2", Type: "coder", Description: "Implement JWT handler", Status: "running", Probability: 0.95, AgentName: "coder", DependsOn: []string{"t1"}},
			{ID: "t3", Type: "security", Description: "Security review", Status: "pending", Probability: 0.70, AgentName: "security", PreWarmed: true, DependsOn: []string{"t1"}},
			{ID: "t4", Type: "docs", Description: "Write API docs", Status: "pending", Probability: 0.50, AgentName: "docs", DependsOn: []string{"t1"}},
		},
		Selected: 1,
	}
	styles := NewStyles(Themes[0])
	out := RenderDAGView(state, styles, 80, 30)

	if !strings.Contains(out, "Orchestrator DAG") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "Build auth module") {
		t.Error("missing prompt")
	}
	if !strings.Contains(out, "architect") {
		t.Error("missing architect task")
	}
	if !strings.Contains(out, "coder") {
		t.Error("missing coder task")
	}
	if !strings.Contains(out, "security") {
		t.Error("missing security task")
	}
}

func TestRenderDAGViewPreWarmedIndicator(t *testing.T) {
	state := DAGState{
		Tasks: []DAGTaskRow{
			{ID: "t1", Type: "coder", Description: "Implement feature", Status: "pending", PreWarmed: true, AgentName: "coder"},
		},
	}
	styles := NewStyles(Themes[0])
	out := RenderDAGView(state, styles, 80, 24)
	if !strings.Contains(out, "🔥") {
		t.Error("missing pre-warmed fire icon")
	}
}

func TestRenderDAGViewSummaryLine(t *testing.T) {
	state := DAGState{
		Tasks: []DAGTaskRow{
			{ID: "t1", Type: "architect", Description: "Design", Status: "completed", AgentName: "architect"},
			{ID: "t2", Type: "coder", Description: "Code", Status: "running", AgentName: "coder"},
			{ID: "t3", Type: "docs", Description: "Docs", Status: "pending", PreWarmed: true, AgentName: "docs"},
			{ID: "t4", Type: "test", Description: "Test", Status: "failed", AgentName: "tester"},
		},
	}
	styles := NewStyles(Themes[0])
	out := RenderDAGView(state, styles, 80, 30)
	if !strings.Contains(out, "1 green") {
		t.Error("summary should show 1 green (completed)")
	}
	if !strings.Contains(out, "1 running") {
		t.Error("summary should show 1 running")
	}
	if !strings.Contains(out, "1 failed") {
		t.Error("summary should show 1 failed")
	}
	if !strings.Contains(out, "1 pre-warmed") {
		t.Error("summary should show 1 pre-warmed")
	}
}

func TestRenderDAGViewTaskDetail(t *testing.T) {
	state := DAGState{
		Tasks: []DAGTaskRow{
			{
				ID:             "t2",
				Type:           "coder",
				Description:    "Implement JWT handler",
				Status:         "running",
				Probability:    0.95,
				AgentName:      "coder",
				DependsOn:      []string{"t1"},
				ExpectedOutput: "jwt.go with Sign/Verify",
				TokensUsed:     1500,
				Cost:           0.003,
			},
		},
		Selected: 0,
	}
	styles := NewStyles(Themes[0])
	out := RenderDAGView(state, styles, 80, 30)
	if !strings.Contains(out, "Task Details") {
		t.Error("missing Task Details section")
	}
	if !strings.Contains(out, "coder") {
		t.Error("missing agent name in details")
	}
	if !strings.Contains(out, "95%") {
		t.Error("missing probability in details")
	}
	if !strings.Contains(out, "t1") {
		t.Error("missing dependency in details")
	}
}

func TestRenderDAGViewPromptTruncation(t *testing.T) {
	longPrompt := strings.Repeat("A", 200)
	state := DAGState{
		Prompt: longPrompt,
		Tasks:  []DAGTaskRow{{ID: "t1", Type: "coder", Description: "test", Status: "pending", AgentName: "coder"}},
	}
	styles := NewStyles(Themes[0])
	out := RenderDAGView(state, styles, 40, 20)
	if strings.Contains(out, longPrompt) {
		t.Error("long prompt should be truncated")
	}
	if !strings.Contains(out, "...") {
		t.Error("truncated prompt should end with ...")
	}
}

func TestDAGIconForStatus(t *testing.T) {
	tests := []struct {
		status     string
		preWarmed  bool
		wantNotIn  string
	}{
		{"completed", false, "✗"},
		{"running", false, "✓"},
		{"failed", false, "✓"},
		{"pending", true, "…"},
		{"pending", false, "○"},
	}
	for _, tt := range tests {
		icon := dagIconForStatus(tt.status, tt.preWarmed)
		if icon == tt.wantNotIn {
			t.Errorf("dagIconForStatus(%q, %v) = %q, should NOT be %q", tt.status, tt.preWarmed, icon, tt.wantNotIn)
		}
	}
}

func TestDAGViewNavigation(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewDAG
	m.DAGState = DAGState{
		Tasks: []DAGTaskRow{
			{ID: "t1", Type: "architect", Description: "d1", Status: "completed", AgentName: "a"},
			{ID: "t2", Type: "coder", Description: "d2", Status: "running", AgentName: "c"},
			{ID: "t3", Type: "docs", Description: "d3", Status: "pending", AgentName: "d"},
		},
		Selected: 0,
	}

	// Move down
	m.DAGState.Selected++
	if m.DAGState.Selected != 1 {
		t.Errorf("after down: selected = %d, want 1", m.DAGState.Selected)
	}

	// Move down again
	m.DAGState.Selected++
	if m.DAGState.Selected != 2 {
		t.Errorf("after 2nd down: selected = %d, want 2", m.DAGState.Selected)
	}

	// Move up
	m.DAGState.Selected--
	if m.DAGState.Selected != 1 {
		t.Errorf("after up: selected = %d, want 1", m.DAGState.Selected)
	}
}

func TestDAGViewSwitchView(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewTools
	m.SwitchView(ViewDAG)
	if m.ViewKind != ViewDAG {
		t.Errorf("SwitchView(ViewDAG) failed: view = %v", m.ViewKind)
	}
}

func TestTruncateDag(t *testing.T) {
	if got := truncateDag("hello world", 20); got != "hello world" {
		t.Errorf("truncateDag short string = %q, want %q", got, "hello world")
	}
	if got := truncateDag("hello world this is a long string", 10); got != "hello w..." {
		t.Errorf("truncateDag long string = %q, want %q", got, "hello w...")
	}
	if got := truncateDag("test", 0); got != "test" {
		t.Errorf("truncateDag maxLen=0 = %q, want %q", got, "test")
	}
}

func TestDAGViewRenderLayout(t *testing.T) {
	m := NewModel()
	m.ViewKind = ViewDAG
	m.DAGState = DAGState{
		Tasks: []DAGTaskRow{
			{ID: "t1", Type: "coder", Description: "test", Status: "running", AgentName: "coder", Probability: 0.9},
		},
	}
	styles := NewStyles(Themes[0])
	out := ComposeLayout(NewTabs(), NewSidebar(), ViewDAG, RenderDAGView(m.DAGState, styles, 80, 20), "", NewFooter(80), styles, 80, 24)
	if !strings.Contains(out, "Orchestrator DAG") {
		t.Error("layout should contain DAG view content")
	}
}

func TestDAGViewAllStatusIcons(t *testing.T) {
	state := DAGState{
		Tasks: []DAGTaskRow{
			{ID: "t1", Type: "a", Description: "d", Status: "completed", AgentName: "x"},
			{ID: "t2", Type: "b", Description: "d", Status: "running", AgentName: "x"},
			{ID: "t3", Type: "c", Description: "d", Status: "failed", AgentName: "x"},
			{ID: "t4", Type: "d", Description: "d", Status: "skipped", AgentName: "x"},
			{ID: "t5", Type: "e", Description: "d", Status: "pending", AgentName: "x"},
			{ID: "t6", Type: "f", Description: "d", Status: "pending", PreWarmed: true, AgentName: "x"},
		},
	}
	styles := NewStyles(Themes[0])
	out := RenderDAGView(state, styles, 80, 30)
	for _, expected := range []string{dagIconGreen, dagIconRunning, dagIconFailed, dagIconSkipped, dagIconPending, dagIconPreWarmed} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected icon %q in output", expected)
		}
	}
}
