package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRenderLayoutDebug(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewTools)
	out := RenderLayoutDebug(m.Tabs, m.Sidebar, m.ViewKind, "test content", "right panel", m.Footer, m.Styles, m.Width, m.Height)
	if !strings.Contains(out, "header") {
		t.Errorf("expected header label in debug layout")
	}
	if !strings.Contains(out, "sidebar") {
		t.Errorf("expected sidebar label in debug layout")
	}
	if !strings.Contains(out, "content") {
		t.Errorf("expected content label in debug layout")
	}
	if !strings.Contains(out, "footer") {
		t.Errorf("expected footer label in debug layout")
	}
}

func TestRenderLayoutDebugChat(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	out := RenderLayoutDebug(m.Tabs, m.Sidebar, ViewChat, "chat content", "", m.Footer, m.Styles, m.Width, m.Height)
	if !strings.Contains(out, "content") {
		t.Errorf("expected content label in chat debug layout")
	}
}

func TestToggleDebugLayout(t *testing.T) {
	m := NewModel()
	if m.DebugLayout {
		t.Error("expected debug layout off by default")
	}
	m.ToggleDebugLayout()
	if !m.DebugLayout {
		t.Error("expected debug layout on after toggle")
	}
	m.ToggleDebugLayout()
	if m.DebugLayout {
		t.Error("expected debug layout off after second toggle")
	}
}

func TestToggleInlineDiff(t *testing.T) {
	m := NewModel()
	if m.InlineDiffOpen {
		t.Error("expected inline diff off by default")
	}
	m.ToggleInlineDiff()
	if !m.InlineDiffOpen {
		t.Error("expected inline diff on after toggle")
	}
}

func TestPadContentExact(t *testing.T) {
	result := padContentExact("hello\nworld", 10, 5)
	lines := strings.Split(result, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
	for _, l := range lines {
		if len(l) != 10 {
			t.Errorf("expected each line padded to 10 chars, got %d: %q", len(l), l)
		}
	}
}

func TestPadContentExactEmpty(t *testing.T) {
	result := padContentExact("", 5, 3)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestRenderInlineDiffEmpty(t *testing.T) {
	styles := NewStyles(Themes[0])
	result := RenderInlineDiff(nil, styles, 80)
	if result != "" {
		t.Errorf("expected empty string for no diffs, got %q", result)
	}
}

func TestRenderInlineDiffWithDiffs(t *testing.T) {
	styles := NewStyles(Themes[0])
	ClearDiffs()
	RecordDiff("test.go", "old", "new", "sin_edit")
	diffs := RecentDiffs()
	result := RenderInlineDiff(diffs, styles, 80)
	if !strings.Contains(result, "test.go") {
		t.Errorf("expected file path in inline diff, got %q", result)
	}
	ClearDiffs()
}

func TestDebugLayoutInView(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewTools)
	m.DebugLayout = true
	out := m.View().Content
	if !strings.Contains(out, "header") && !strings.Contains(out, "content") {
		t.Errorf("expected debug labels in view when debug layout is on")
	}
}

func TestInlineDiffInView(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewChat)
	ClearDiffs()
	RecordDiff("example.go", "before", "after", "sin_edit")
	m.InlineDiffOpen = true
	out := m.View().Content
	if !strings.Contains(out, "example.go") {
		t.Errorf("expected inline diff content in chat view, got (first 200 chars): %s", out[:min(200, len(out))])
	}
	ClearDiffs()
}

func TestGoldenRulePureView(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.SwitchView(ViewTools)
	out1 := m.View().Content
	out2 := m.View().Content
	if out1 != out2 {
		t.Error("View() should be pure — same state must produce same output (Rule 1)")
	}
}

func TestGoldenRuleBoundsRespected(t *testing.T) {
	for _, size := range []struct{ w, h int }{
		{120, 40}, {80, 24}, {60, 15}, {45, 10}, {40, 10},
	} {
		padded := padContent("test", size.w, size.h)
		lines := strings.Split(padded, "\n")
		if len(lines) != size.h {
			t.Errorf("padContent at h=%d produced %d lines (Rule 3: must fill exact height)", size.h, len(lines))
		}
		for _, l := range lines {
			if len(l) != size.w {
				t.Errorf("padContent line width = %d, want %d (Rule 3: must fill exact width)", len(l), size.w)
			}
		}
	}
}

func TestGoldenRulePadContentExact(t *testing.T) {
	for _, h := range []int{5, 10, 20, 40} {
		result := padContentExact("hello", 30, h)
		lines := strings.Split(result, "\n")
		if len(lines) != h {
			t.Errorf("padContentExact h=%d produced %d lines", h, len(lines))
		}
	}
}

