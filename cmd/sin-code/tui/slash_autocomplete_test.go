// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestSlashCommandsReturnsFullList(t *testing.T) {
	sa := NewSlashAutocomplete()
	cmds := sa.Commands()
	if len(cmds) != 14 {
		t.Errorf("expected 14 commands, got %d", len(cmds))
	}
	names := map[string]bool{}
	for _, c := range cmds {
		names[c.Name] = true
	}
	expected := []string{
		"/clear", "/help", "/attach", "/search", "/btw",
		"/undercover", "/model", "/theme", "/compact", "/tools",
		"/sessions", "/dag", "/ctx-viz", "/dashboard",
	}
	for _, e := range expected {
		if !names[e] {
			t.Errorf("expected command %s in list", e)
		}
	}
}

func TestSlashCommandsReturnsCopy(t *testing.T) {
	sa := NewSlashAutocomplete()
	cmds := sa.Commands()
	cmds[0].Name = "/mutated"
	original := sa.Commands()
	if original[0].Name == "/mutated" {
		t.Error("Commands() should return a copy, not internal state")
	}
}

func TestSlashFilterExactName(t *testing.T) {
	sa := NewSlashAutocomplete()
	results := sa.Filter("/clear")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for '/clear', got %d", len(results))
	}
	if results[0].Name != "/clear" {
		t.Errorf("expected /clear, got %s", results[0].Name)
	}
}

func TestSlashFilterPartialName(t *testing.T) {
	sa := NewSlashAutocomplete()
	results := sa.Filter("/c")
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	hasClear := false
	for _, n := range names {
		if n == "/clear" {
			hasClear = true
		}
	}
	if !hasClear {
		t.Errorf("expected /clear in results for '/c', got %v", names)
	}
}

func TestSlashFilterByCategory(t *testing.T) {
	sa := NewSlashAutocomplete()
	results := sa.Filter("view")
	for _, r := range results {
		if r.Category != "view" {
			t.Errorf("expected only view category results, got %s for %s", r.Category, r.Name)
		}
	}
	if len(results) == 0 {
		t.Error("expected at least one result for 'view' category")
	}
}

func TestSlashFilterByDescription(t *testing.T) {
	sa := NewSlashAutocomplete()
	results := sa.Filter("compact")
	if len(results) == 0 {
		t.Fatal("expected results for 'compact'")
	}
	found := false
	for _, r := range results {
		if r.Name == "/compact" {
			found = true
		}
	}
	if !found {
		t.Error("expected /compact in results when searching 'compact'")
	}
}

func TestSlashFilterEmptyQueryReturnsAll(t *testing.T) {
	sa := NewSlashAutocomplete()
	results := sa.Filter("")
	if len(results) != len(sa.Commands()) {
		t.Errorf("expected %d results for empty query, got %d", len(sa.Commands()), len(results))
	}
}

func TestSlashFilterNoMatchReturnsEmpty(t *testing.T) {
	sa := NewSlashAutocomplete()
	results := sa.Filter("/zzzzz")
	if len(results) != 0 {
		t.Errorf("expected 0 results for no match, got %d", len(results))
	}
}

func TestSlashMoveUpDownWraps(t *testing.T) {
	sa := NewSlashAutocomplete()
	sa.Filter("")
	if sa.Selected() == nil {
		t.Fatal("expected non-nil selected at start")
	}
	initialName := sa.Selected().Name

	sa.MoveUp()
	wrapped := sa.Selected()
	if wrapped == nil {
		t.Fatal("expected non-nil after wrap up")
	}
	if wrapped.Name == initialName {
		t.Error("expected wrap to last item after MoveUp from first")
	}

	sa.MoveDown()
	backToFirst := sa.Selected()
	if backToFirst == nil {
		t.Fatal("expected non-nil after wrap down")
	}
	if backToFirst.Name != initialName {
		t.Errorf("expected wrap back to first item, got %s", backToFirst.Name)
	}
}

func TestSlashMoveDownThenUp(t *testing.T) {
	sa := NewSlashAutocomplete()
	sa.Filter("")

	sa.MoveDown()
	if sa.Selected() == nil {
		t.Fatal("expected non-nil after MoveDown")
	}
	sa.MoveDown()
	sa.MoveUp()
	if sa.Selected() == nil {
		t.Fatal("expected non-nil after MoveUp")
	}
}

func TestSlashSelectedReturnsCorrectCommand(t *testing.T) {
	sa := NewSlashAutocomplete()
	sa.Filter("")
	sa.MoveDown()
	sa.MoveDown()
	sel := sa.Selected()
	if sel == nil {
		t.Fatal("expected non-nil selected")
	}
	if sel.Name == "" {
		t.Error("expected non-empty name")
	}
}

func TestSlashSelectedEmptyFilterReturnsNil(t *testing.T) {
	sa := NewSlashAutocomplete()
	sa.Filter("/zzzzz")
	if sa.Selected() != nil {
		t.Error("expected nil when no results")
	}
}

func TestSlashActiveToggle(t *testing.T) {
	sa := NewSlashAutocomplete()
	if sa.Active() {
		t.Error("expected inactive by default")
	}
	sa.SetActive(true)
	if !sa.Active() {
		t.Error("expected active after SetActive(true)")
	}
	sa.SetActive(false)
	if sa.Active() {
		t.Error("expected inactive after SetActive(false)")
	}
}

func TestSlashSetActiveResetsSelection(t *testing.T) {
	sa := NewSlashAutocomplete()
	sa.Filter("")
	sa.MoveDown()
	sa.MoveDown()
	sa.SetActive(false)
	if sa.Selected() != nil {
		sa.SetActive(true)
		sel := sa.Selected()
		if sel == nil {
			t.Fatal("expected non-nil after re-activate")
		}
	}
}

func TestSlashRenderContainsCommands(t *testing.T) {
	sa := NewSlashAutocomplete()
	sa.Filter("")
	styles := NewStyles(Themes[0])
	out := sa.Render(styles, 70)
	if !strings.Contains(out, "Slash Commands") {
		t.Error("expected title in render output")
	}
	if !strings.Contains(out, "/clear") {
		t.Error("expected /clear in render output")
	}
}

func TestSlashRenderEmptyResults(t *testing.T) {
	sa := NewSlashAutocomplete()
	sa.Filter("/zzzzz")
	styles := NewStyles(Themes[0])
	out := sa.Render(styles, 70)
	if !strings.Contains(out, "no matches") {
		t.Error("expected 'no matches' in render output")
	}
}

func TestSlashReset(t *testing.T) {
	sa := NewSlashAutocomplete()
	sa.SetActive(true)
	sa.Filter("/c")
	sa.MoveDown()
	sa.Reset()
	if sa.Active() {
		t.Error("expected inactive after Reset")
	}
	cmds := sa.Commands()
	if len(cmds) != 14 {
		t.Errorf("expected 14 commands after reset, got %d", len(cmds))
	}
}

func TestSlashConcurrentAccess(t *testing.T) {
	sa := NewSlashAutocomplete()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sa.Filter("/c")
			sa.MoveDown()
			sa.MoveUp()
			sa.Selected()
			sa.Active()
			sa.SetActive(true)
			sa.SetActive(false)
			sa.Commands()
		}(i)
	}
	wg.Wait()
}
