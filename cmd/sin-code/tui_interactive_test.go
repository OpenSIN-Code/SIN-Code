package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
)

func TestTuiModelInit(t *testing.T) {
	m := newTUIModel()
	if cmd := m.Init(); cmd != nil {
		t.Errorf("expected Init() to return nil, got %T", cmd)
	}
	if m.themeIndex != 0 {
		t.Errorf("expected initial themeIndex 0, got %d", m.themeIndex)
	}
	if m.inputMode {
		t.Error("expected inputMode to be false initially")
	}
	if m.quitting {
		t.Error("expected quitting to be false initially")
	}
	if m.selectedCmd != "" {
		t.Errorf("expected selectedCmd empty, got %q", m.selectedCmd)
	}
}

func TestTuiModelDefaults(t *testing.T) {
	m := newTUIModel()
	if len(m.list.Items()) == 0 {
		t.Fatal("expected non-empty list of items")
	}
	firstItem, ok := m.list.SelectedItem().(tuiItem)
	if !ok {
		t.Fatal("expected first item to be tuiItem")
	}
	if firstItem.name != "discover" {
		t.Errorf("expected first item to be 'discover', got %q", firstItem.name)
	}
	if m.list.Title == "" {
		t.Error("expected non-empty list title")
	}
}

func TestTuiNavigateDown(t *testing.T) {
	m := newTUIModel()
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if mm.list.Index() != 1 {
		t.Errorf("expected cursor at 1 after KeyDown, got %d", mm.list.Index())
	}
}

func TestTuiNavigateUp(t *testing.T) {
	m := newTUIModel()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = nm.(tuiModel)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = nm.(tuiModel)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	mm, _ := nm.(tuiModel)
	if mm.list.Index() != 1 {
		t.Errorf("expected cursor at 1 after 2 down + 1 up, got %d", mm.list.Index())
	}
}

func TestTuiEnterShowsHelp(t *testing.T) {
	m := newTUIModel()
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if !mm.quitting {
		t.Error("expected quitting to be true after Enter")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after Enter, got nil")
	}
}

func TestTuiQuitQ(t *testing.T) {
	m := newTUIModel()
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if !mm.quitting {
		t.Error("expected quitting to be true after 'q'")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after 'q', got nil")
	}
}

func TestTuiQuitCtrlC(t *testing.T) {
	m := newTUIModel()
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if !mm.quitting {
		t.Error("expected quitting to be true after ctrl+c")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after ctrl+c, got nil")
	}
}

func TestTuiRunnableCommandDirect(t *testing.T) {
	m := newTUIModel()
	var newM tea.Model
	for i := 0; i < 6; i++ {
		newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m, _ = newM.(tuiModel)
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if !mm.quitting {
		t.Error("expected quitting to be true when running orchestrate (runnable without args)")
	}
}

func TestTuiEnterInInputModeSubmits(t *testing.T) {
	m := newTUIModel()
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if !mm.inputMode {
		t.Fatal("expected inputMode to be true after 'r' on discover")
	}
	mm.input.SetValue("--help")
	newM, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm, _ = newM.(tuiModel)
	if mm.inputMode {
		t.Error("expected inputMode to be false after Enter in input mode")
	}
	if !mm.quitting {
		t.Error("expected quitting to be true after Enter in input mode")
	}
}

func TestTuiCtrlCInInputModeQuits(t *testing.T) {
	m := newTUIModel()
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mm, _ := newM.(tuiModel)
	if !mm.inputMode {
		t.Fatal("expected inputMode to be true")
	}
	newM, _ = mm.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mm, _ = newM.(tuiModel)
	if !mm.quitting {
		t.Error("expected quitting to be true after ctrl+c in input mode")
	}
}

func TestTuiViewQuitting(t *testing.T) {
	m := newTUIModel()
	m.quitting = true
	view := m.View()
	if view != "" {
		t.Errorf("expected empty view when quitting, got %q", view)
	}
}

func TestTuiViewInputMode(t *testing.T) {
	m := newTUIModel()
	m.inputMode = true
	m.selectedCmd = "discover"
	view := m.View()
	if !strings.Contains(view, "discover") {
		t.Errorf("expected view to mention 'discover' in input mode, got:\n%s", view)
	}
	if !strings.Contains(view, "Enter") {
		t.Errorf("expected view to show 'Enter' hint in input mode, got:\n%s", view)
	}
}

func TestTuiViewNormalMode(t *testing.T) {
	m := newTUIModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = nm.(tuiModel)
	view := m.View()
	if !strings.Contains(view, "sin-code") {
		t.Errorf("expected view to contain 'sin-code' title, got:\n%s", view)
	}
	if !strings.Contains(view, "Enter") {
		t.Errorf("expected view to show Enter hint, got:\n%s", view)
	}
}

func TestTuiViewRunnableHint(t *testing.T) {
	m := newTUIModel()
	for i := 0; i < len(m.list.Items()); i++ {
		m.list.Select(i)
		view := m.View()
		if strings.Contains(view, "run without args") {
			ti, ok := m.list.SelectedItem().(tuiItem)
			if !ok {
				t.Fatalf("item %d: expected tuiItem", i)
			}
			if !runnableWithoutArgs[ti.name] {
				t.Errorf("item %d (%q) should not show 'run without args' hint", i, ti.name)
			}
		}
	}
}

func TestTuiWindowResize(t *testing.T) {
	m := newTUIModel()
	originalWidth := m.list.Width()
	originalHeight := m.list.Height()
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if mm.list.Width() != 120 {
		t.Errorf("expected width 120, got %d", mm.list.Width())
	}
	if mm.list.Height() != 38 {
		t.Errorf("expected height 38 (40-2), got %d", mm.list.Height())
	}
	if mm.list.Width() == originalWidth && mm.list.Height() == originalHeight {
		t.Error("expected window size to have changed")
	}
}

func TestTuiFilterStateUnfiltered(t *testing.T) {
	m := newTUIModel()
	if m.list.FilterState() != list.Unfiltered {
		t.Errorf("expected Unfiltered state initially, got %v", m.list.FilterState())
	}
}

func TestTuiRunAll14SubcommandsHaveItems(t *testing.T) {
	m := newTUIModel()
	expected := []string{
		"discover", "execute", "map", "grasp", "scout", "harvest", "orchestrate",
		"ibd", "poc", "sckg", "adw", "oracle", "efm", "serve",
	}
	names := make(map[string]bool)
	for _, item := range m.list.Items() {
		if ti, ok := item.(tuiItem); ok {
			names[ti.name] = true
		}
	}
	for _, want := range expected {
		if !names[want] {
			t.Errorf("expected subcommand %q to be in TUI list", want)
		}
	}
}

func TestTuiThemeColorsLength(t *testing.T) {
	if len(themeColors) != 5 {
		t.Errorf("expected 5 themes, got %d", len(themeColors))
	}
	if len(themeNames) != 5 {
		t.Errorf("expected 5 theme names, got %d", len(themeNames))
	}
	if len(themeColors) != len(themeNames) {
		t.Error("themeColors and themeNames must have same length")
	}
}

func TestTuiThemeNamesExpected(t *testing.T) {
	expected := []string{"default", "Dracula", "Nord", "Solarized", "Monokai"}
	for i, want := range expected {
		if themeNames[i] != want {
			t.Errorf("expected themeNames[%d] = %q, got %q", i, want, themeNames[i])
		}
	}
}

func TestTuiRunnableMapLength(t *testing.T) {
	if len(runnableWithoutArgs) != 3 {
		t.Errorf("expected 3 runnable-without-args commands, got %d", len(runnableWithoutArgs))
	}
}

func TestTuiRunnableExactSet(t *testing.T) {
	expected := map[string]bool{"serve": true, "orchestrate": true, "tui": true}
	if len(runnableWithoutArgs) != len(expected) {
		t.Errorf("expected %d runnable entries, got %d", len(expected), len(runnableWithoutArgs))
	}
	for k := range expected {
		if !runnableWithoutArgs[k] {
			t.Errorf("expected %q to be runnable", k)
		}
	}
}

func TestTuiArgInputTyping(t *testing.T) {
	m := newTUIModel()
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mm, _ := newM.(tuiModel)
	if !mm.inputMode {
		t.Fatal("expected inputMode true")
	}
	newM, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-', '-'}})
	mm, _ = newM.(tuiModel)
	if !strings.Contains(mm.input.Value(), "--") {
		t.Errorf("expected input to contain '--', got %q", mm.input.Value())
	}
}
