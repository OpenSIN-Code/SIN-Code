package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestDefaultAgentDashboardState(t *testing.T) {
	ds := DefaultAgentDashboardState()
	if len(ds.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(ds.Sessions))
	}
	if ds.Selected != 0 {
		t.Errorf("expected selected 0, got %d", ds.Selected)
	}
}

func TestSessionStatusColor(t *testing.T) {
	styles := NewStyles(Themes[0])
	cases := []string{"running", "blocked", "done", "error", "unknown"}
	for _, s := range cases {
		result := sessionStatusColor(s, styles)
		if result == "" {
			t.Errorf("expected non-empty color for status %q", s)
		}
	}
}

func TestFormatSessionDuration(t *testing.T) {
	if got := formatSessionDuration(0); got != "0s" {
		t.Errorf("formatSessionDuration(0) = %q, want 0s", got)
	}
	if got := formatSessionDuration(5 * time.Second); got != "5s" {
		t.Errorf("formatSessionDuration(5s) = %q, want 5s", got)
	}
	if got := formatSessionDuration(90 * time.Second); !strings.Contains(got, "1m") {
		t.Errorf("formatSessionDuration(90s) = %q, want contains 1m", got)
	}
}

func TestRenderAgentDashboardViewEmpty(t *testing.T) {
	styles := NewStyles(Themes[0])
	ds := DefaultAgentDashboardState()
	view := RenderAgentDashboardView(ds, styles, 80, 24)
	if !strings.Contains(view, "No active") {
		t.Errorf("expected empty message, got %q", view)
	}
}

func TestRenderAgentDashboardViewWithSessions(t *testing.T) {
	styles := NewStyles(Themes[0])
	ds := AgentDashboardState{
		Sessions: []AgentSessionRow{
			{ID: "1", AgentName: "Build", Task: "refactor auth", Status: "running", Duration: 5 * time.Minute, Tokens: 12000, Cost: 0.08},
			{ID: "2", AgentName: "Audit", Task: "security scan", Status: "blocked", Duration: 2 * time.Minute, Tokens: 5000, Cost: 0.03},
			{ID: "3", AgentName: "Stats", Task: "metrics analysis", Status: "done", Duration: 10 * time.Minute, Tokens: 8000, Cost: 0.05},
		},
		Selected: 0,
	}
	view := RenderAgentDashboardView(ds, styles, 80, 24)
	if !strings.Contains(view, "Agent Dashboard") {
		t.Errorf("expected 'Agent Dashboard' header")
	}
	if !strings.Contains(view, "Build") {
		t.Errorf("expected 'Build' agent name")
	}
	if !strings.Contains(view, "refactor auth") {
		t.Errorf("expected task 'refactor auth'")
	}
	if !strings.Contains(view, "running") {
		t.Errorf("expected 'running' status")
	}
	if !strings.Contains(view, "3 session") {
		t.Errorf("expected '3 session(s)' summary")
	}
	if !strings.Contains(view, "1 running") {
		t.Errorf("expected '1 running' in summary")
	}
	if !strings.Contains(view, "1 blocked") {
		t.Errorf("expected '1 blocked' in summary")
	}
	if !strings.Contains(view, "Total") {
		t.Errorf("expected 'Total' summary row")
	}
}

func TestRenderAgentDashboardViewTotals(t *testing.T) {
	styles := NewStyles(Themes[0])
	ds := AgentDashboardState{
		Sessions: []AgentSessionRow{
			{AgentName: "A", Status: "running", Tokens: 1000, Cost: 0.01},
			{AgentName: "B", Status: "running", Tokens: 2000, Cost: 0.02},
			{AgentName: "C", Status: "done", Tokens: 3000, Cost: 0.03},
		},
	}
	view := RenderAgentDashboardView(ds, styles, 80, 24)
	if !strings.Contains(view, "6.0K") || !strings.Contains(view, "6K") {
	}
	if !strings.Contains(view, "$0.0600") {
		t.Errorf("expected total cost $0.0600, got:\n%s", view)
	}
}

func TestRenderAgentDashboardViewSmallWidth(t *testing.T) {
	styles := NewStyles(Themes[0])
	ds := AgentDashboardState{
		Sessions: []AgentSessionRow{
			{AgentName: "A", Task: "task", Status: "running"},
		},
	}
	view := RenderAgentDashboardView(ds, styles, 20, 10)
	if view == "" {
		t.Error("expected non-empty view at small width")
	}
}

func TestRenderAgentDashboardViewInModel(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewAgentDashboard)
	view := m.View().Content
	if !strings.Contains(view, "Agent Dashboard") {
		t.Errorf("expected 'Agent Dashboard' in view, got:\n%s", view[:min(200, len(view))])
	}
}

func TestAgentDashboardNavigation(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.AgentDashboardState = AgentDashboardState{
		Sessions: []AgentSessionRow{
			{AgentName: "A", Status: "running"},
			{AgentName: "B", Status: "done"},
		},
		Selected: 0,
	}
	m.SwitchView(ViewAgentDashboard)
	start := m.AgentDashboardState.Selected
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.AgentDashboardState.Selected == start {
		t.Error("expected selection to move down")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.AgentDashboardState.Selected != start {
		t.Error("expected selection to return")
	}
}
