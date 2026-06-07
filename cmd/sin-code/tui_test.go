package main

import (
	"bytes"
	"strings"
	"testing"
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
