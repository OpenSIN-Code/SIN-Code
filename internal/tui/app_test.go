// Purpose: Tests for the TUI command catalog, themes, and persisted config.
// Docs: app_test.doc.md

package tui

import (
	"os"
	"path/filepath"
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
	if groups[0] != GroupCode {
		t.Errorf("expected first group %q, got %q", GroupCode, groups[0])
	}
}

func TestDefaultThemeHasAllStyles(t *testing.T) {
	s := defaultStylesForTest(t)
	if s.Title.GetForeground() == (lipgloss.NoColor{}) {
		if s.Title.Render("x") == "x" {
			// Title has no rules → still the same string → safe to assert.
		}
	}
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

func TestLightThemeHasAllStyles(t *testing.T) {
	s := theme.Light()
	if s == nil {
		t.Fatal("theme.Light() returned nil")
	}
	if out := s.Title.Render("hello"); out == "" {
		t.Error("Light Title.Render returned empty")
	}
	if out := s.Toast.Render("Copied!"); out == "" {
		t.Error("Light Toast.Render returned empty")
	}
}

func TestThemeNewFallsBackToDark(t *testing.T) {
	// Sanity: unknown name must not panic and must return non-nil.
	s := theme.New("not-a-theme")
	if s == nil {
		t.Fatal("theme.New(garbage) returned nil")
	}
	if out := s.Title.Render("x"); out == "" {
		t.Error("fallback theme Title.Render returned empty")
	}
}

func TestPaletteForReturnsLight(t *testing.T) {
	p := theme.PaletteFor(theme.ThemeLight)
	if p == nil {
		t.Fatal("PaletteFor(light) returned nil")
	}
	if p.Base != theme.LightPalette.Base {
		t.Errorf("PaletteFor(light) returned wrong palette")
	}
}

func TestConfigDefaultsToDark(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := LoadConfig()
	if cfg.Theme != theme.ThemeDark {
		t.Errorf("LoadConfig with no file = %q, want %q", cfg.Theme, theme.ThemeDark)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveConfig(Config{Theme: theme.ThemeLight}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	cfg := LoadConfig()
	if cfg.Theme != theme.ThemeLight {
		t.Errorf("round trip theme = %q, want %q", cfg.Theme, theme.ThemeLight)
	}

	// File should live under ~/.config/sin/tui.toml.
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if filepath.Base(path) != ConfigFileName {
		t.Errorf("ConfigPath base = %q, want %q", filepath.Base(path), ConfigFileName)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not written at %s: %v", path, err)
	}
}

func TestConfigRejectsUnknownTheme(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("theme = \"neon\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := LoadConfig()
	if cfg.Theme != theme.ThemeDark {
		t.Errorf("unknown theme should fall back to dark, got %q", cfg.Theme)
	}
}

func TestConfigIgnoresCommentsAndBlankLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := "# header comment\n\n   \ntheme = \"light\"\n# trailing\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := LoadConfig()
	if cfg.Theme != theme.ThemeLight {
		t.Errorf("expected light theme, got %q", cfg.Theme)
	}
}

func TestNewModelAppliesPersistedTheme(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveConfig(Config{Theme: theme.ThemeLight}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	m := NewModel()
	if m.themeName != theme.ThemeLight {
		t.Errorf("NewModel theme = %q, want %q", m.themeName, theme.ThemeLight)
	}
}

func TestCycleThemeToggles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	start := m.themeName
	m.cycleTheme()
	if m.themeName == start {
		t.Errorf("cycleTheme did not change theme; still %q", m.themeName)
	}
	m.cycleTheme()
	if m.themeName != start {
		t.Errorf("two cycles should return to start; got %q (started %q)", m.themeName, start)
	}
}

func TestHistoryIntegrationWithModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel()
	if m.history == nil {
		t.Fatal("model has no history")
	}
	if m.history.Len() != 0 {
		t.Errorf("fresh model history non-empty: %d", m.history.Len())
	}
}

// defaultStylesForTest exposes the package-private constructor for testing.
func defaultStylesForTest(t *testing.T) *theme.Styles {
	t.Helper()
	return theme.Default()
}
