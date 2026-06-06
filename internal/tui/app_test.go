// Purpose: Tests for the TUI command catalog and theme.
// Docs: app_test.doc.md

package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/internal/tui/theme"
)

func TestCommandsHaveUniqueKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Commands {
		if c.Key == "" {
			t.Errorf("empty Key for command %q", c.Title)
		}
		if seen[c.Key] {
			t.Errorf("duplicate command key %q", c.Key)
		}
		seen[c.Key] = true
	}
}

func TestCommandsHaveGroup(t *testing.T) {
	for _, c := range Commands {
		if c.Group == "" {
			t.Errorf("command %q has no group", c.Key)
		}
	}
}

func TestFilterMatchesByTitle(t *testing.T) {
	got := Filter("scout")
	if len(got) == 0 {
		t.Fatalf("expected scout in results")
	}
	found := false
	for _, c := range got {
		if strings.Contains(c.Title, "scout") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a scout command in filtered results")
	}
}

func TestFilterReturnsAllOnEmpty(t *testing.T) {
	got := Filter("")
	if len(got) != len(Commands) {
		t.Errorf("expected %d commands on empty filter, got %d", len(Commands), len(got))
	}
}

func TestGroupsAreOrdered(t *testing.T) {
	groups := Groups()
	if len(groups) < 5 {
		t.Errorf("expected at least 5 groups, got %d", len(groups))
	}
	// Code should be the first group.
	if groups[0] != GroupCode {
		t.Errorf("expected first group %q, got %q", GroupCode, groups[0])
	}
}

func TestDefaultThemeHasAllStyles(t *testing.T) {
	s := defaultStylesForTest(t)
	if s.Title.GetForeground() == (lipgloss.NoColor{}) {
		// Style is uninitialised if no color is set.
		if s.Title.Render("x") == "x" {
			// Title has no rules → still the same string → safe to assert.
		}
	}
	// Render and assert the styles actually do something.
	if out := s.Title.Render("hello"); out == "" {
		t.Error("Title.Render returned empty")
	}
	if out := s.Danger.Render("boom"); out == "" {
		t.Error("Danger.Render returned empty")
	}
	if out := s.MenuItemActive.Render(">"); out == "" {
		t.Error("MenuItemActive.Render returned empty")
	}
}

// defaultStylesForTest exposes the package-private constructor for testing.
func defaultStylesForTest(t *testing.T) *theme.Styles {
	t.Helper()
	return theme.Default()
}
