// SPDX-License-Identifier: MIT
// Purpose: tests for issue #355 — TUI memory browser.
package tui

import (
	"strings"
	"testing"
	"time"
)

func sampleRows() []MemoryRow {
	return []MemoryRow{
		{ID: "mem-1", Tags: []string{"auth", "security"}, Content: "Use JWT for authentication", Created: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), Importance: 0.9},
		{ID: "mem-2", Tags: []string{"testing"}, Content: "Always run tests with -race flag", Created: time.Date(2024, 2, 20, 12, 0, 0, 0, time.UTC), Importance: 0.7},
		{ID: "mem-3", Tags: []string{"auth", "config"}, Content: "OAuth2 callback URL must match", Created: time.Date(2024, 3, 10, 8, 30, 0, 0, time.UTC), Importance: 0.5},
	}
}

func TestViewMemoryEnum(t *testing.T) {
	if ViewMemory.String() != "Memory" {
		t.Errorf("ViewMemory.String() = %q, want %q", ViewMemory.String(), "Memory")
	}
	if !strings.Contains(ViewMemory.Short(), "Memory") {
		t.Errorf("ViewMemory.Short() = %q, want to contain 'Memory'", ViewMemory.Short())
	}
}

func TestViewMemoryInSidebar(t *testing.T) {
	items := DefaultSidebarItems()
	found := false
	for _, item := range items {
		if item.View == ViewMemory {
			found = true
			if item.Icon != "🧠" {
				t.Errorf("Memory icon = %q, want 🧠", item.Icon)
			}
			if item.Label != "Memory" {
				t.Errorf("Memory label = %q, want 'Memory'", item.Label)
			}
		}
	}
	if !found {
		t.Error("ViewMemory not found in DefaultSidebarItems")
	}
}

func TestNewMemoryBrowserEmpty(t *testing.T) {
	b := NewMemoryBrowser()
	if b == nil {
		t.Fatal("NewMemoryBrowser returned nil")
	}
	if b.Selected() != nil {
		t.Error("expected nil selection on empty browser")
	}
	styles := NewStyles(Themes[0])
	out := b.Render(styles, 80, 24)
	if !strings.Contains(out, "no memories") {
		t.Errorf("expected 'no memories' in empty render, got:\n%s", out)
	}
}

func TestSetMemoriesAndCount(t *testing.T) {
	b := NewMemoryBrowser()
	b.SetMemories(sampleRows())
	styles := NewStyles(Themes[0])
	out := b.Render(styles, 80, 24)
	if !strings.Contains(out, "3 memories") {
		t.Errorf("expected '3 memories' count, got:\n%s", out)
	}
	if !strings.Contains(out, "4 tags") {
		t.Errorf("expected '4 tags' count (auth, security, testing, config), got:\n%s", out)
	}
}

func TestMemoryBrowserMoveUpDown(t *testing.T) {
	b := NewMemoryBrowser()
	b.SetMemories(sampleRows())
	if b.sel != 0 {
		t.Errorf("expected sel 0, got %d", b.sel)
	}
	b.MoveDown()
	if b.sel != 1 {
		t.Errorf("expected sel 1 after MoveDown, got %d", b.sel)
	}
	b.MoveDown()
	b.MoveDown()
	if b.sel != 2 {
		t.Errorf("expected sel clamped at 2, got %d", b.sel)
	}
	b.MoveUp()
	if b.sel != 1 {
		t.Errorf("expected sel 1 after MoveUp, got %d", b.sel)
	}
	b.MoveUp()
	b.MoveUp()
	if b.sel != 0 {
		t.Errorf("expected sel clamped at 0, got %d", b.sel)
	}
}

func TestMemoryBrowserSelected(t *testing.T) {
	b := NewMemoryBrowser()
	b.SetMemories(sampleRows())
	b.MoveDown()
	sel := b.Selected()
	if sel == nil {
		t.Fatal("expected non-nil selection")
	}
	if sel.ID != "mem-2" {
		t.Errorf("expected mem-2, got %s", sel.ID)
	}
}

func TestMemoryBrowserSearchContent(t *testing.T) {
	b := NewMemoryBrowser()
	b.SetMemories(sampleRows())
	b.Search("JWT")
	sel := b.Selected()
	if sel == nil {
		t.Fatal("expected selection after content search")
	}
	if sel.ID != "mem-1" {
		t.Errorf("expected mem-1 for 'JWT' search, got %s", sel.ID)
	}
}

func TestMemoryBrowserSearchByTag(t *testing.T) {
	b := NewMemoryBrowser()
	b.SetMemories(sampleRows())
	b.Search("#auth")
	if len(b.filtered) != 2 {
		t.Errorf("expected 2 auth-tagged memories, got %d", len(b.filtered))
	}
	ids := map[string]bool{}
	for _, r := range b.filtered {
		ids[r.ID] = true
	}
	if !ids["mem-1"] || !ids["mem-3"] {
		t.Errorf("expected mem-1 and mem-3, got %v", ids)
	}
}

func TestMemoryBrowserRenderHighlight(t *testing.T) {
	b := NewMemoryBrowser()
	b.SetMemories(sampleRows())
	styles := NewStyles(Themes[0])
	out := b.Render(styles, 80, 24)
	if !strings.Contains(out, "Search:") {
		t.Error("expected search bar in render")
	}
	if !strings.Contains(out, "2024-02-20") {
		t.Error("expected date in render")
	}
}

func TestMemoryBrowserDetailView(t *testing.T) {
	b := NewMemoryBrowser()
	b.SetMemories(sampleRows())
	sel := b.Selected()
	styles := NewStyles(Themes[0])
	out := b.DetailView(sel, styles, 80)
	if !strings.Contains(out, "Memory Detail") {
		t.Error("expected 'Memory Detail' header")
	}
	if !strings.Contains(out, "mem-1") {
		t.Error("expected ID in detail view")
	}
	if !strings.Contains(out, "Use JWT for authentication") {
		t.Error("expected full content in detail view")
	}
	if !strings.Contains(out, "0.90") {
		t.Error("expected importance in detail view")
	}
}

func TestMemoryBrowserDetailViewNil(t *testing.T) {
	b := NewMemoryBrowser()
	styles := NewStyles(Themes[0])
	out := b.DetailView(nil, styles, 80)
	if !strings.Contains(out, "no memory selected") {
		t.Errorf("expected nil message, got:\n%s", out)
	}
}
