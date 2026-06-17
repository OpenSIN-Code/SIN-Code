package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDefaultContextState(t *testing.T) {
	cs := DefaultContextState()
	if cs.MaxTokens <= 0 {
		t.Error("expected positive max tokens")
	}
	if len(cs.Categories) == 0 {
		t.Error("expected categories")
	}
}

func TestContextStateRecompute(t *testing.T) {
	cs := DefaultContextState()
	cs.Recompute()
	if cs.UsedTokens <= 0 {
		t.Error("expected positive used tokens after recompute")
	}
	total := 0
	for _, c := range cs.Categories {
		total += c.Tokens
	}
	if cs.UsedTokens != total {
		t.Errorf("UsedTokens = %d, want %d", cs.UsedTokens, total)
	}
}

func TestContextBarWidth(t *testing.T) {
	if w := contextBarWidth(0, 200000, 40); w != 0 {
		t.Errorf("contextBarWidth(0,...) = %d, want 0", w)
	}
	if w := contextBarWidth(200000, 200000, 40); w != 40 {
		t.Errorf("contextBarWidth(full,...) = %d, want 40", w)
	}
	if w := contextBarWidth(100000, 200000, 40); w != 20 {
		t.Errorf("contextBarWidth(half,...) = %d, want 20", w)
	}
	if w := contextBarWidth(300000, 200000, 40); w != 40 {
		t.Errorf("contextBarWidth(over,...) = %d, want 40 (capped)", w)
	}
}

func TestRenderContextVizView(t *testing.T) {
	styles := NewStyles(Themes[0])
	cs := DefaultContextState()
	view := RenderContextVizView(cs, styles, 80, 24)
	if !strings.Contains(view, "Context Usage") {
		t.Errorf("expected 'Context Usage' in view")
	}
	if !strings.Contains(view, "Breakdown") {
		t.Errorf("expected 'Breakdown' in view")
	}
	if !strings.Contains(view, "System Prompt") {
		t.Errorf("expected 'System Prompt' category in view")
	}
	if !strings.Contains(view, "Free Space") {
		t.Errorf("expected 'Free Space' in view")
	}
}

func TestRenderContextVizViewEmpty(t *testing.T) {
	styles := NewStyles(Themes[0])
	cs := ContextState{MaxTokens: 200000, Categories: nil}
	view := RenderContextVizView(cs, styles, 80, 24)
	if !strings.Contains(view, "Context Usage") {
		t.Errorf("expected header even with empty categories")
	}
}

func TestRenderContextVizViewCompacted(t *testing.T) {
	styles := NewStyles(Themes[0])
	cs := DefaultContextState()
	cs.Compacted = true
	view := RenderContextVizView(cs, styles, 80, 24)
	if !strings.Contains(view, "Compaction") {
		t.Errorf("expected compaction indicator")
	}
}

func TestRenderContextVizViewNearThreshold(t *testing.T) {
	styles := NewStyles(Themes[0])
	cs := ContextState{
		MaxTokens: 200000,
		Categories: []ContextCategory{
			{Name: "Test", Tokens: 180000},
		},
	}
	view := RenderContextVizView(cs, styles, 80, 24)
	if !strings.Contains(view, "compaction") && !strings.Contains(view, "Compaction") {
		t.Errorf("expected near-compaction warning at 90%%")
	}
}

func TestRenderContextVizViewCacheHit(t *testing.T) {
	styles := NewStyles(Themes[0])
	cs := DefaultContextState()
	cs.CacheHit = 0.87
	view := RenderContextVizView(cs, styles, 80, 24)
	if !strings.Contains(view, "87%") {
		t.Errorf("expected 87%% cache hit in view")
	}
}

func TestRenderContextVizViewSmallWidth(t *testing.T) {
	styles := NewStyles(Themes[0])
	cs := DefaultContextState()
	view := RenderContextVizView(cs, styles, 20, 10)
	if view == "" {
		t.Error("expected non-empty view at small width")
	}
}

func TestRenderContextVizViewInModel(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.SwitchView(ViewContextViz)
	view := m.View().Content
	if !strings.Contains(view, "Context Usage") {
		t.Errorf("expected Context Usage in view, got:\n%s", view[:min(200, len(view))])
	}
}
