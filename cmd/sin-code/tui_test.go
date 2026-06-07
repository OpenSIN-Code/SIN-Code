package main

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTuiFallbackOutput(t *testing.T) {
	// Capture the fallback plain-text output by simulating a non-TTY environment.
	// We do this by calling tuiCmd.RunE directly with a custom stdout buffer.
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// Execute tui directly — in a non-TTY environment it should print the catalog.
	tuiCmd.SetOut(buf)
	if err := tuiCmd.RunE(tuiCmd, []string{}); err != nil {
		t.Fatalf("tuiCmd.RunE failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sin-code subcommands") {
		t.Errorf("expected fallback header, got:\n%s", out)
	}
	if !strings.Contains(out, "discover") {
		t.Errorf("expected 'discover' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "serve") {
		t.Errorf("expected 'serve' in output, got:\n%s", out)
	}
}

func TestTuiRunnableCommands(t *testing.T) {
	// Verify that serve and orchestrate are marked as runnable without args.
	if !runnableWithoutArgs["serve"] {
		t.Error("expected 'serve' to be runnable without args")
	}
	if !runnableWithoutArgs["orchestrate"] {
		t.Error("expected 'orchestrate' to be runnable without args")
	}
	if runnableWithoutArgs["discover"] {
		t.Error("did not expect 'discover' to be runnable without args")
	}
}

func TestTuiCmdStructure(t *testing.T) {
	if tuiCmd.Name() != "tui" {
		t.Errorf("expected command name 'tui', got %q", tuiCmd.Name())
	}
	if tuiCmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
}

func TestGetSubcommand(t *testing.T) {
	// Ensure getSubcommand can find all registered subcommands.
	for _, c := range rootCmd.Commands() {
		if c.Name() == "tui" || c.Name() == "help" {
			continue
		}
		found := getSubcommand(c.Name())
		if found == nil {
			t.Errorf("getSubcommand(%q) returned nil", c.Name())
		}
		if found.Name() != c.Name() {
			t.Errorf("getSubcommand(%q) returned %q", c.Name(), found.Name())
		}
	}

	// Unknown subcommand should return nil.
	if getSubcommand("nonexistent") != nil {
		t.Error("expected nil for unknown subcommand")
	}
}

func TestTuiItemInterface(t *testing.T) {
	item := tuiItem{name: "discover", description: "test desc"}
	if item.Title() != "discover" {
		t.Errorf("Title() = %q, want 'discover'", item.Title())
	}
	if item.Description() != "test desc" {
		t.Errorf("Description() = %q, want 'test desc'", item.Description())
	}
	if item.FilterValue() != "discover test desc" {
		t.Errorf("FilterValue() = %q, want 'discover test desc'", item.FilterValue())
	}
}

func TestTuiThemeCycling(t *testing.T) {
	m := newTUIModel()
	if m.themeIndex != 0 {
		t.Fatalf("expected initial themeIndex 0, got %d", m.themeIndex)
	}
	// Press 't' four times
	for i := 1; i <= 4; i++ {
		newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		mm, ok := newM.(tuiModel)
		if !ok {
			t.Fatal("expected tuiModel")
		}
		m = mm
		if m.themeIndex != i {
			t.Fatalf("expected themeIndex %d after %d 't' presses, got %d", i, i, m.themeIndex)
		}
	}
	// One more press wraps around to 0
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if mm.themeIndex != 0 {
		t.Fatalf("expected themeIndex 0 after wrapping, got %d", mm.themeIndex)
	}
}

func TestTuiThemeApplied(t *testing.T) {
	m := newTUIModel()
	initialTitle := m.list.Styles.Title.GetForeground()
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	mm := newM.(tuiModel)
	newTitle := mm.list.Styles.Title.GetForeground()
	if initialTitle == newTitle {
		t.Fatal("expected title color to change after cycling theme")
	}
}

func TestTuiArgInputMode(t *testing.T) {
	m := newTUIModel()
	// Press 'r' on discover (needs args)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mm, ok := newM.(tuiModel)
	if !ok {
		t.Fatal("expected tuiModel")
	}
	if !mm.inputMode {
		t.Fatal("expected inputMode to be true for discover")
	}
	if mm.selectedCmd != "discover" {
		t.Errorf("expected selectedCmd 'discover', got %q", mm.selectedCmd)
	}
}

func TestTuiArgInputCancel(t *testing.T) {
	m := newTUIModel()
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mm := newM.(tuiModel)
	if !mm.inputMode {
		t.Fatal("expected inputMode to be true")
	}
	newM, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEscape})
	mm = newM.(tuiModel)
	if mm.inputMode {
		t.Error("expected inputMode to be false after Esc")
	}
	if mm.quitting {
		t.Error("expected quitting to be false after cancel")
	}
}

func TestTuiArgInputSubmit(t *testing.T) {
	m := newTUIModel()
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mm := newM.(tuiModel)
	if !mm.inputMode {
		t.Fatal("expected inputMode to be true")
	}
	mm.input.SetValue(".")
	newM, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = newM.(tuiModel)
	if mm.inputMode {
		t.Error("expected inputMode to be false after Enter")
	}
	if !mm.quitting {
		t.Error("expected quitting to be true after running command")
	}
}
