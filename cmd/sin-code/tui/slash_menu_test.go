// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestSlashMenu_DefaultCommands(t *testing.T) {
	sm := NewSlashMenu(NewStyles(Themes[0]))
	if len(sm.Commands) != len(DefaultSlashCommands) {
		t.Errorf("expected %d commands, got %d", len(DefaultSlashCommands), len(sm.Commands))
	}
}

func TestSlashMenu_Filter(t *testing.T) {
	sm := NewSlashMenu(NewStyles(Themes[0]))

	// Empty filter returns all.
	sm.Filter_("")
	filtered := sm.Filtered()
	if len(filtered) != len(DefaultSlashCommands) {
		t.Errorf("expected %d results for empty filter, got %d", len(DefaultSlashCommands), len(filtered))
	}

	// Filter by name: "commit" → /commit.
	sm.Filter_("/commit")
	filtered = sm.Filtered()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 result for '/commit', got %d", len(filtered))
	}
	if filtered[0].Name != "/commit" {
		t.Errorf("expected /commit, got %s", filtered[0].Name)
	}

	// Filter by partial name: "c" → /clear, /compact, /commit, /config.
	sm.Filter_("/c")
	filtered = sm.Filtered()
	names := make([]string, len(filtered))
	for i, r := range filtered {
		names[i] = r.Name
	}
	hasClear := false
	hasCompact := false
	hasCommit := false
	hasConfig := false
	for _, n := range names {
		switch n {
		case "/clear":
			hasClear = true
		case "/compact":
			hasCompact = true
		case "/commit":
			hasCommit = true
		case "/config":
			hasConfig = true
		}
	}
	if !hasClear || !hasCompact || !hasCommit || !hasConfig {
		t.Errorf("expected /clear, /compact, /commit, /config in results for '/c', got %v", names)
	}

	// Filter by description: "verification" → /verify.
	sm.Filter_("verification")
	filtered = sm.Filtered()
	found := false
	for _, r := range filtered {
		if r.Name == "/verify" {
			found = true
		}
	}
	if !found {
		t.Error("expected /verify in results for 'verification'")
	}

	// Filter by category: "git" → /commit, /diff, /pr.
	sm.Filter_("git")
	filtered = sm.Filtered()
	for _, r := range filtered {
		if r.Category != "git" {
			t.Errorf("expected only git category results, got %s for %s", r.Category, r.Name)
		}
	}
	if len(filtered) != 3 {
		t.Errorf("expected 3 git commands, got %d", len(filtered))
	}

	// No match.
	sm.Filter_("/zzzzz")
	filtered = sm.Filtered()
	if len(filtered) != 0 {
		t.Errorf("expected 0 results for no match, got %d", len(filtered))
	}

	// Empty selected after no-match filter.
	sel := sm.Selected()
	if sel.Name != "" {
		t.Errorf("expected zero-value selected on no match, got %s", sel.Name)
	}
}

func TestSlashMenu_Navigation(t *testing.T) {
	sm := NewSlashMenu(NewStyles(Themes[0]))
	sm.Filter_("")

	// Initial selection is first item.
	sel := sm.Selected()
	if sel.Name != "/help" {
		t.Errorf("expected first item /help, got %s", sel.Name)
	}

	// Move down.
	sm.Next()
	sel = sm.Selected()
	if sel.Name != "/clear" {
		t.Errorf("expected /clear after Next, got %s", sel.Name)
	}

	// Move down again.
	sm.Next()
	sel = sm.Selected()
	if sel.Name != "/compact" {
		t.Errorf("expected /compact after Next, got %s", sel.Name)
	}

	// Move up.
	sm.Prev()
	sel = sm.Selected()
	if sel.Name != "/clear" {
		t.Errorf("expected /clear after Prev, got %s", sel.Name)
	}

	// Wrap from first to last.
	sm.Filter_("")
	// Navigate to first by resetting.
	sm.Sel = 0
	sm.Prev()
	last := sm.Selected()
	if last.Name != "/export" {
		t.Errorf("expected wrap to last item /export, got %s", last.Name)
	}

	// Wrap from last to first.
	sm.Next()
	first := sm.Selected()
	if first.Name != "/help" {
		t.Errorf("expected wrap to first item /help, got %s", first.Name)
	}

	// Navigation on empty filter does nothing.
	sm.Filter_("/zzzzz")
	sm.Next()
	sm.Prev()
	if sm.Selected().Name != "" {
		t.Error("expected empty selected on no results navigation")
	}
}

func TestSlashMenu_Render(t *testing.T) {
	sm := NewSlashMenu(NewStyles(Themes[0]))
	sm.Filter_("")

	out := sm.Render()
	if !strings.Contains(out, "Slash Commands") {
		t.Error("expected 'Slash Commands' in render output")
	}
	if !strings.Contains(out, "/help") {
		t.Error("expected /help in render output")
	}
	if !strings.Contains(out, "/commit") {
		t.Error("expected /commit in render output")
	}
	if !strings.Contains(out, "navigate") {
		t.Error("expected navigation hint in render output")
	}

	// Render with no matches.
	sm.Filter_("/zzzzz")
	out = sm.Render()
	if !strings.Contains(out, "no matches") {
		t.Error("expected 'no matches' in render output")
	}

	// Render with narrow width.
	sm.Filter_("")
	sm.Width = 30
	out = sm.Render()
	if out == "" {
		t.Error("expected non-empty render for narrow width")
	}
}

func TestSlashMenu_OpenClose(t *testing.T) {
	sm := NewSlashMenu(NewStyles(Themes[0]))
	if sm.Open {
		t.Error("expected menu closed by default")
	}
	sm.OpenMenu()
	if !sm.Open {
		t.Error("expected menu open after OpenMenu")
	}
	if sm.Sel != 0 {
		t.Error("expected selection reset to 0 on open")
	}
	sm.Next()
	sm.Close()
	if sm.Open {
		t.Error("expected menu closed after Close")
	}
	if sm.Sel != 0 {
		t.Error("expected selection reset to 0 on close")
	}
	if sm.Filter != "" {
		t.Error("expected filter reset on close")
	}
}

func TestSlashMenu_ConcurrentAccess(t *testing.T) {
	sm := NewSlashMenu(NewStyles(Themes[0]))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.Filter_("/c")
			sm.Next()
			sm.Prev()
			_ = sm.Selected()
			_ = sm.Filtered()
			sm.OpenMenu()
			sm.Close()
		}()
	}
	wg.Wait()
}
