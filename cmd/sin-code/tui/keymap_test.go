// SPDX-License-Identifier: MIT
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func TestDefaultKeymap(t *testing.T) {
	km := DefaultKeymap()
	bindings := []key.Binding{
		km.Quit, km.Help, km.Palette, km.ToggleSidebar, km.CycleTheme, km.CycleAgent, km.Interrupt,
		km.NextView, km.PrevView, km.ViewTools, km.ViewSessions, km.ViewEFM, km.ViewConfig, km.ViewHistory, km.ViewTodos, km.ViewChat, km.ViewDAG, km.ViewContext, km.ViewDashboard,
		km.RunTool, km.ShowHelp, km.ToolUp, km.ToolDown,
		km.Submit, km.Cancel, km.Search, km.CopyMessage, km.ScrollUp, km.ScrollDown,
		km.NewSession, km.CloseSession, km.SessionSwitch,
		km.ModelSelect, km.Subagents,
	}
	for i, b := range bindings {
		if !b.Enabled() {
			t.Errorf("binding %d is not enabled", i)
		}
		if len(b.Keys()) == 0 {
			t.Errorf("binding %d has no keys", i)
		}
		h := b.Help()
		if h.Key == "" {
			t.Errorf("binding %d has empty help key", i)
		}
		if h.Desc == "" {
			t.Errorf("binding %d has empty help desc", i)
		}
	}
}

func TestKeymapHelpView(t *testing.T) {
	km := DefaultKeymap()
	styles := NewStyles(Themes[0])
	help := km.HelpView(styles)
	if strings.TrimSpace(help) == "" {
		t.Fatal("help view is empty")
	}
	for _, cat := range []string{"Global", "Navigation", "Tools", "Chat", "Sessions", "Model", "Subagents"} {
		if !strings.Contains(help, cat) {
			t.Errorf("help view missing category %q", cat)
		}
	}
	if !strings.Contains(help, "quit") {
		t.Error("help view missing 'quit' description")
	}
}

func TestLoadKeyOverrides(t *testing.T) {
	content := `{"quit": ["ctrl+q"], "help": ["h"], "submit": ["ctrl+enter"]}`
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	ov, err := LoadKeyOverrides(path)
	if err != nil {
		t.Fatalf("LoadKeyOverrides: %v", err)
	}
	if len(ov.Quit) != 1 || ov.Quit[0] != "ctrl+q" {
		t.Errorf("expected quit=[ctrl+q], got %v", ov.Quit)
	}
	if len(ov.Help) != 1 || ov.Help[0] != "h" {
		t.Errorf("expected help=[h], got %v", ov.Help)
	}
	if len(ov.Submit) != 1 || ov.Submit[0] != "ctrl+enter" {
		t.Errorf("expected submit=[ctrl+enter], got %v", ov.Submit)
	}
}

func TestLoadKeyOverridesMissingFile(t *testing.T) {
	_, err := LoadKeyOverrides("/nonexistent/path/keys.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadKeyOverridesInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadKeyOverrides(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestApplyOverrides(t *testing.T) {
	km := DefaultKeymap()
	originalKeys := km.Quit.Keys()
	if len(originalKeys) == 0 {
		t.Fatal("default keymap has no quit keys")
	}
	ov := KeyOverrides{Quit: []string{"ctrl+q"}}
	km.ApplyOverrides(ov)
	keys := km.Quit.Keys()
	if len(keys) != 1 || keys[0] != "ctrl+q" {
		t.Errorf("expected [ctrl+q], got %v", keys)
	}
	help := km.Quit.Help()
	if help.Desc != "quit" {
		t.Errorf("override changed help desc: expected 'quit', got %q", help.Desc)
	}
}

func TestApplyOverridesEmptyNoChange(t *testing.T) {
	km := DefaultKeymap()
	originalKeys := km.Palette.Keys()
	ov := KeyOverrides{}
	km.ApplyOverrides(ov)
	if len(km.Palette.Keys()) != len(originalKeys) {
		t.Errorf("empty overrides changed keys: had %v, now %v", originalKeys, km.Palette.Keys())
	}
}

func TestResolveMouseSidebar(t *testing.T) {
	msg := tea.MouseClickMsg{X: 5, Y: 10}
	action := ResolveMouse(msg, 120, 40, 22, 32)
	if action.Target != "sidebar" {
		t.Errorf("expected sidebar, got %s", action.Target)
	}
	if action.Kind != "click" {
		t.Errorf("expected click, got %s", action.Kind)
	}
	if action.X != 5 || action.Y != 10 {
		t.Errorf("expected (5,10), got (%d,%d)", action.X, action.Y)
	}
}

func TestResolveMouseChat(t *testing.T) {
	msg := tea.MouseClickMsg{X: 50, Y: 20}
	action := ResolveMouse(msg, 120, 40, 22, 32)
	if action.Target != "chat" {
		t.Errorf("expected chat, got %s", action.Target)
	}
}

func TestResolveMouseTabs(t *testing.T) {
	msg := tea.MouseClickMsg{X: 10, Y: 1}
	action := ResolveMouse(msg, 120, 40, 22, 32)
	if action.Target != "tabs" {
		t.Errorf("expected tabs, got %s", action.Target)
	}
}

func TestResolveMouseFooter(t *testing.T) {
	msg := tea.MouseClickMsg{X: 50, Y: 38}
	action := ResolveMouse(msg, 120, 40, 22, 32)
	if action.Target != "footer" {
		t.Errorf("expected footer, got %s", action.Target)
	}
}

func TestResolveMouseRightPanel(t *testing.T) {
	msg := tea.MouseClickMsg{X: 110, Y: 20}
	action := ResolveMouse(msg, 120, 40, 22, 32)
	if action.Target != "right_panel" {
		t.Errorf("expected right_panel, got %s", action.Target)
	}
}

func TestResolveMouseScrollUp(t *testing.T) {
	msg := tea.MouseWheelMsg{X: 50, Y: 20, Button: tea.MouseWheelUp}
	action := ResolveMouse(msg, 120, 40, 22, 32)
	if action.Kind != "scroll_up" {
		t.Errorf("expected scroll_up, got %s", action.Kind)
	}
	if action.Target != "chat" {
		t.Errorf("expected chat, got %s", action.Target)
	}
}

func TestResolveMouseScrollDown(t *testing.T) {
	msg := tea.MouseWheelMsg{X: 5, Y: 10, Button: tea.MouseWheelDown}
	action := ResolveMouse(msg, 120, 40, 22, 32)
	if action.Kind != "scroll_down" {
		t.Errorf("expected scroll_down, got %s", action.Kind)
	}
}

func TestResolveMouseCollapsedSidebar(t *testing.T) {
	msg := tea.MouseClickMsg{X: 3, Y: 10}
	action := ResolveMouse(msg, 80, 40, 6, 24)
	if action.Target != "sidebar" {
		t.Errorf("expected sidebar, got %s", action.Target)
	}
}

func TestResolveMouseNoSidebar(t *testing.T) {
	msg := tea.MouseClickMsg{X: 3, Y: 10}
	action := ResolveMouse(msg, 80, 40, 0, 0)
	if action.Target != "chat" {
		t.Errorf("expected chat when no sidebar, got %s", action.Target)
	}
}
