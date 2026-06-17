// SPDX-License-Identifier: MIT
package chat

import (
	"sync"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

func newVimTextarea() *textarea.Model {
	ta := textarea.New()
	ta.SetWidth(80)
	ta.SetHeight(5)
	ta.Focus()
	return &ta
}

func TestVimModeToggle(t *testing.T) {
	v := NewVimMode()
	if v.Active() {
		t.Error("expected inactive by default")
	}
	v.Toggle()
	if !v.Active() {
		t.Error("expected active after toggle")
	}
	v.Toggle()
	if v.Active() {
		t.Error("expected inactive after second toggle")
	}
}

func TestVimModeEnableDisable(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	if !v.Active() {
		t.Error("expected active after Enable")
	}
	v.Disable()
	if v.Active() {
		t.Error("expected inactive after Disable")
	}
}

func TestVimModeStateTransitions(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	if v.State() != VimNormal {
		t.Errorf("expected normal state, got %d", v.State())
	}
	v.SetState(VimInsert)
	if v.State() != VimInsert {
		t.Errorf("expected insert state, got %d", v.State())
	}
	v.SetState(VimNormal)
	if v.State() != VimNormal {
		t.Errorf("expected normal state, got %d", v.State())
	}
}

func TestVimModeIEntersInsert(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "i"}, ta)
	if !handled {
		t.Error("expected i key handled in normal mode")
	}
	if v.State() != VimInsert {
		t.Errorf("expected insert state after i, got %d", v.State())
	}
}

func TestVimModeEscReturnsToNormal(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	v.SetState(VimInsert)
	ta := newVimTextarea()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape}, ta)
	if !handled {
		t.Error("expected esc handled in insert mode")
	}
	if v.State() != VimNormal {
		t.Errorf("expected normal state after esc, got %d", v.State())
	}
}

func TestVimModeAEntersInsertAfterCursor(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello")
	ta.CursorStart()
	colBefore := ta.Column()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "a"}, ta)
	if !handled {
		t.Error("expected a key handled")
	}
	if v.State() != VimInsert {
		t.Errorf("expected insert after a, got %d", v.State())
	}
	if ta.Column() != colBefore+1 {
		t.Errorf("expected cursor moved right by 1, got col %d (was %d)", ta.Column(), colBefore)
	}
}

func TestVimModeOCreatesNewLine(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("line1")
	ta.CursorStart()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "o"}, ta)
	if !handled {
		t.Error("expected o key handled")
	}
	if v.State() != VimInsert {
		t.Errorf("expected insert after o, got %d", v.State())
	}
	if ta.LineCount() < 2 {
		t.Errorf("expected at least 2 lines after o, got %d", ta.LineCount())
	}
}

func TestVimModeHMovesLeft(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello")
	ta.CursorEnd()
	colBefore := ta.Column()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "h"}, ta)
	if !handled {
		t.Error("expected h handled")
	}
	if ta.Column() != colBefore-1 {
		t.Errorf("expected cursor left by 1, got col %d (was %d)", ta.Column(), colBefore)
	}
}

func TestVimModeLMovesRight(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello")
	ta.CursorStart()
	colBefore := ta.Column()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "l"}, ta)
	if !handled {
		t.Error("expected l handled")
	}
	if ta.Column() != colBefore+1 {
		t.Errorf("expected cursor right by 1, got col %d (was %d)", ta.Column(), colBefore)
	}
}

func TestVimModeJMovesDown(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("line1\nline2")
	ta.MoveToBegin()
	lineBefore := ta.Line()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "j"}, ta)
	if !handled {
		t.Error("expected j handled")
	}
	if ta.Line() != lineBefore+1 {
		t.Errorf("expected line +1, got %d (was %d)", ta.Line(), lineBefore)
	}
}

func TestVimModeKMovesUp(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("line1\nline2")
	ta.MoveToBegin()
	ta.CursorDown()
	lineBefore := ta.Line()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "k"}, ta)
	if !handled {
		t.Error("expected k handled")
	}
	if ta.Line() != lineBefore-1 {
		t.Errorf("expected line -1, got %d (was %d)", ta.Line(), lineBefore)
	}
}

func TestVimModeZeroMovesToLineStart(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello world")
	ta.CursorEnd()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "0"}, ta)
	if !handled {
		t.Error("expected 0 handled")
	}
	if ta.Column() != 0 {
		t.Errorf("expected column 0, got %d", ta.Column())
	}
}

func TestVimModeDollarMovesToEnd(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello")
	ta.CursorStart()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "$"}, ta)
	if !handled {
		t.Error("expected $ handled")
	}
	if ta.Column() != 5 {
		t.Errorf("expected column 5 (end of 'hello'), got %d", ta.Column())
	}
}

func TestVimModeWMovesWordForward(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello world foo")
	ta.CursorStart()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "w"}, ta)
	if !handled {
		t.Error("expected w handled")
	}
	if ta.Column() != 6 {
		t.Errorf("expected column 6 (start of 'world'), got %d", ta.Column())
	}
}

func TestVimModeBMovesWordBackward(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello world foo")
	ta.CursorEnd()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "b"}, ta)
	if !handled {
		t.Error("expected b handled")
	}
	if ta.Column() != 12 {
		t.Errorf("expected column 12 (start of 'foo'), got %d", ta.Column())
	}
}

func TestVimModeDDDeletesLine(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("line1\nline2\nline3")
	ta.CursorStart()
	ta.CursorDown()
	lineBefore := ta.Line()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "d"}, ta)
	if !handled {
		t.Error("expected first d handled")
	}
	if v.Pending() != "d" {
		t.Errorf("expected pending 'd', got %q", v.Pending())
	}
	handled, _ = v.HandleKey(tea.KeyPressMsg{Text: "d"}, ta)
	if !handled {
		t.Error("expected second d handled")
	}
	if v.Pending() != "" {
		t.Errorf("expected pending cleared after dd, got %q", v.Pending())
	}
	val := ta.Value()
	if lineBefore == 1 {
		if val != "line1\nline3" {
			t.Errorf("expected line2 deleted, got %q", val)
		}
	}
}

func TestVimModeDWDeletesWord(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello world foo")
	ta.CursorStart()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "d"}, ta)
	if !handled {
		t.Error("expected first d handled")
	}
	handled, _ = v.HandleKey(tea.KeyPressMsg{Text: "w"}, ta)
	if !handled {
		t.Error("expected w handled after pending d")
	}
	val := ta.Value()
	if val != " world foo" {
		t.Errorf("expected 'hello' deleted, got %q", val)
	}
	if ta.Column() != 0 {
		t.Errorf("expected cursor at 0 after dw, got %d", ta.Column())
	}
}

func TestVimModeModeIndicator(t *testing.T) {
	v := NewVimMode()
	if v.ModeIndicator() != vimNormalStr {
		t.Errorf("expected %s, got %s", vimNormalStr, v.ModeIndicator())
	}
	v.SetState(VimInsert)
	if v.ModeIndicator() != vimInsertStr {
		t.Errorf("expected %s, got %s", vimInsertStr, v.ModeIndicator())
	}
	v.SetState(VimVisual)
	if v.ModeIndicator() != vimVisualStr {
		t.Errorf("expected %s, got %s", vimVisualStr, v.ModeIndicator())
	}
	v.SetState(VimNormal)
	if v.ModeIndicator() != vimNormalStr {
		t.Errorf("expected %s, got %s", vimNormalStr, v.ModeIndicator())
	}
}

func TestVimModeInactiveDoesNotHandle(t *testing.T) {
	v := NewVimMode()
	ta := newVimTextarea()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "h"}, ta)
	if handled {
		t.Error("expected not handled when vim inactive")
	}
}

func TestVimModeInsertPassesThrough(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	v.SetState(VimInsert)
	ta := newVimTextarea()
	handled, _ := v.HandleKey(tea.KeyPressMsg{Text: "x"}, ta)
	if handled {
		t.Error("expected insert mode to pass through non-esc keys")
	}
}

func TestVimModeReset(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	v.SetState(VimInsert)
	v.Reset()
	if v.State() != VimNormal {
		t.Errorf("expected normal after reset, got %d", v.State())
	}
}

func TestVimModeDPendingCancelledByOtherKey(t *testing.T) {
	v := NewVimMode()
	v.Enable()
	ta := newVimTextarea()
	ta.SetValue("hello")
	v.HandleKey(tea.KeyPressMsg{Text: "d"}, ta)
	if v.Pending() != "d" {
		t.Fatalf("expected pending d, got %q", v.Pending())
	}
	v.HandleKey(tea.KeyPressMsg{Text: "x"}, ta)
	if v.Pending() != "" {
		t.Errorf("expected pending cleared by non-d/w key, got %q", v.Pending())
	}
}

func TestVimModeConcurrentToggle(t *testing.T) {
	v := NewVimMode()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v.Toggle()
			_ = v.Active()
			_ = v.ModeIndicator()
		}()
	}
	wg.Wait()
}
