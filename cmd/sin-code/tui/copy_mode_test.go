// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"testing"
)

func TestCopyMode_EnterExit(t *testing.T) {
	cm := NewCopyMode(testStyles())
	if cm.Active {
		t.Error("expected inactive on init")
	}
	lines := []string{"line1", "line2", "line3"}
	cm.Enter(lines)
	if !cm.Active {
		t.Error("expected active after Enter")
	}
	if len(cm.Lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(cm.Lines))
	}
	if cm.CursorLine != 0 {
		t.Errorf("expected cursor at 0, got %d", cm.CursorLine)
	}
	if cm.SelStart != -1 {
		t.Errorf("expected SelStart -1, got %d", cm.SelStart)
	}
	cm.Exit()
	if cm.Active {
		t.Error("expected inactive after Exit")
	}
	if len(cm.Lines) != 0 {
		t.Errorf("expected 0 lines after Exit, got %d", len(cm.Lines))
	}
}

func TestCopyMode_Navigation(t *testing.T) {
	cm := NewCopyMode(testStyles())
	cm.Enter([]string{"l0", "l1", "l2", "l3", "l4"})

	cm.Down()
	if cm.CursorLine != 1 {
		t.Errorf("expected cursor 1 after Down, got %d", cm.CursorLine)
	}
	cm.Down()
	cm.Down()
	if cm.CursorLine != 3 {
		t.Errorf("expected cursor 3 after 3x Down, got %d", cm.CursorLine)
	}
	cm.Up()
	if cm.CursorLine != 2 {
		t.Errorf("expected cursor 2 after Up, got %d", cm.CursorLine)
	}

	// PageDown (half of 5 = 2)
	cm.PageDown()
	if cm.CursorLine != 4 {
		t.Errorf("expected cursor 4 after PageDown, got %d", cm.CursorLine)
	}

	// PageUp from bottom
	cm.PageUp()
	if cm.CursorLine != 2 {
		t.Errorf("expected cursor 2 after PageUp, got %d", cm.CursorLine)
	}

	// Top and Bottom
	cm.Top()
	if cm.CursorLine != 0 {
		t.Errorf("expected cursor 0 after Top, got %d", cm.CursorLine)
	}
	cm.Bottom()
	if cm.CursorLine != 4 {
		t.Errorf("expected cursor 4 after Bottom, got %d", cm.CursorLine)
	}

	// Clamping
	cm.Top()
	cm.Up()
	if cm.CursorLine != 0 {
		t.Errorf("expected cursor 0 clamped, got %d", cm.CursorLine)
	}
	cm.Bottom()
	cm.Down()
	if cm.CursorLine != 4 {
		t.Errorf("expected cursor 4 clamped, got %d", cm.CursorLine)
	}
}

func TestCopyMode_VisualSelect(t *testing.T) {
	cm := NewCopyMode(testStyles())
	cm.Enter([]string{"l0", "l1", "l2", "l3", "l4"})

	// Toggle visual at line 0
	cm.ToggleVisual()
	if !cm.VisualMode {
		t.Error("expected visual mode on")
	}
	if cm.SelStart != 0 || cm.SelEnd != 0 {
		t.Errorf("expected sel 0-0, got %d-%d", cm.SelStart, cm.SelEnd)
	}

	// Move down to extend selection
	cm.Down()
	cm.Down()
	if cm.SelStart > cm.SelEnd {
		t.Errorf("expected SelStart <= SelEnd, got %d > %d", cm.SelStart, cm.SelEnd)
	}
	minSel, maxSel := cm.SelStart, cm.SelEnd
	if minSel > maxSel {
		minSel, maxSel = maxSel, minSel
	}
	if minSel != 0 || maxSel != 2 {
		t.Errorf("expected selection 0-2, got %d-%d", minSel, maxSel)
	}

	// Toggle off
	cm.ToggleVisual()
	if cm.VisualMode {
		t.Error("expected visual mode off")
	}
	if cm.SelStart != -1 {
		t.Errorf("expected SelStart -1, got %d", cm.SelStart)
	}
}

func TestCopyMode_Yank(t *testing.T) {
	cm := NewCopyMode(testStyles())
	cm.Enter([]string{"l0", "l1", "l2", "l3", "l4"})

	// Yank single line (no visual mode)
	cm.Down() // cursor at 1
	text := cm.Yank()
	if text != "l1" {
		t.Errorf("expected 'l1', got %q", text)
	}

	// Yank with visual selection
	cm.ToggleVisual() // start at line 1
	cm.Down()         // extend to line 2
	cm.Down()         // extend to line 3
	text = cm.Yank()
	lines := strings.Split(text, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines in yank, got %d: %q", len(lines), text)
	}
	if lines[0] != "l1" || lines[2] != "l3" {
		t.Errorf("expected l1..l3, got %q", text)
	}
	// After yank, visual mode should be cleared
	if cm.VisualMode {
		t.Error("expected visual mode off after Yank")
	}
}

func TestCopyMode_YankAll(t *testing.T) {
	cm := NewCopyMode(testStyles())
	cm.Enter([]string{"l0", "l1", "l2"})
	text := cm.YankAll()
	expected := "l0\nl1\nl2"
	if text != expected {
		t.Errorf("expected %q, got %q", expected, text)
	}
}

func TestCopyMode_Render(t *testing.T) {
	cm := NewCopyMode(testStyles())
	cm.Enter([]string{"line one", "line two", "line three"})

	rendered := cm.Render(80, 24)
	if !strings.Contains(rendered, "[COPY") {
		t.Errorf("expected [COPY] header in render, got %q", rendered)
	}
	if !strings.Contains(rendered, "line one") {
		t.Errorf("expected 'line one' in render, got %q", rendered)
	}
	if !strings.Contains(rendered, "j/k move") {
		t.Errorf("expected help footer in render, got %q", rendered)
	}

	// Test with visual selection
	cm.ToggleVisual()
	cm.Down()
	rendered = cm.Render(80, 24)
	if !strings.Contains(rendered, "line one") {
		t.Errorf("expected selected lines in render, got %q", rendered)
	}

	// Test with empty lines
	cm2 := NewCopyMode(testStyles())
	cm2.Enter([]string{"(empty)"})
	rendered = cm2.Render(20, 5)
	if !strings.Contains(rendered, "[COPY") {
		t.Errorf("expected header even with few lines, got %q", rendered)
	}
}

func TestCopyMode_EmptyLines(t *testing.T) {
	cm := NewCopyMode(testStyles())
	cm.Enter([]string{})
	if cm.Active {
		// Enter should still set active even with empty lines
	}
	cm.Down()
	// Should not panic with empty lines
	cm.Up()
	cm.Top()
	cm.Bottom()
	_ = cm.Yank()
	_ = cm.YankAll()
	_ = cm.Render(80, 24)
}
