// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCompactModeDefaultOff(t *testing.T) {
	c := NewCompactMode()
	if c.Active() {
		t.Error("expected compact mode off by default")
	}
}

func TestCompactModeToggleOnOff(t *testing.T) {
	c := NewCompactMode()
	c.Toggle()
	if !c.Active() {
		t.Error("expected compact mode on after one toggle")
	}
	c.Toggle()
	if c.Active() {
		t.Error("expected compact mode off after second toggle")
	}
}

func TestCompactModeActiveState(t *testing.T) {
	c := NewCompactMode()
	c.Set(true)
	if !c.Active() {
		t.Error("expected active after Set(true)")
	}
	c.Set(false)
	if c.Active() {
		t.Error("expected inactive after Set(false)")
	}
}

func TestCompactModeDoubleToggleReturnsOff(t *testing.T) {
	c := NewCompactMode()
	c.Toggle()
	c.Toggle()
	if c.Active() {
		t.Error("double toggle should return to off")
	}
}

func TestCompactModeTogglePreservesState(t *testing.T) {
	c := NewCompactMode()
	c.Set(true)
	for i := 0; i < 5; i++ {
		_ = c.Active()
	}
	if !c.Active() {
		t.Error("Active() calls must not mutate state")
	}
	c.Toggle()
	if c.Active() {
		t.Error("Toggle after Set(true) should turn off")
	}
	c.Toggle()
	if !c.Active() {
		t.Error("Toggle after off should turn on")
	}
}

func TestCompactModeConcurrentToggle(t *testing.T) {
	c := NewCompactMode()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Toggle()
			_ = c.Active()
		}()
	}
	wg.Wait()
}

func TestRenderCompactMessageUserSingleLine(t *testing.T) {
	styles := NewStyles(Themes[0])
	msg := ChatMessage{Kind: chatUser, Text: "hello world"}
	out := cleanANSI(renderCompactMessage(msg, styles, 80, false, NewSpinner()))
	if !strings.Contains(out, "❯") {
		t.Errorf("expected ❯ prefix for compact user message: %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected message text: %q", out)
	}
}

func TestRenderCompactMessageAssistantShowMore(t *testing.T) {
	styles := NewStyles(Themes[0])
	long := "line1\nline2\nline3\nline4\nline5"
	msg := ChatMessage{Kind: chatAssistant, Text: long}
	out := cleanANSI(renderCompactMessage(msg, styles, 80, false, NewSpinner()))
	if !strings.Contains(out, "[show more]") {
		t.Errorf("expected [show more] indicator for >3 line assistant: %q", out)
	}
	if strings.Contains(out, "line5") {
		t.Errorf("expected 5th line collapsed: %q", out)
	}
}

func TestRenderCompactMessageToolSingleLine(t *testing.T) {
	styles := NewStyles(Themes[0])
	msg := ChatMessage{Kind: chatTool, Tool: "sin_edit", Detail: "auth/login.go:42", Result: true}
	out := cleanANSI(renderCompactMessage(msg, styles, 80, false, NewSpinner()))
	if !strings.Contains(out, "sin_edit") {
		t.Errorf("expected tool name: %q", out)
	}
	if !strings.Contains(out, "→") {
		t.Errorf("expected arrow for compact tool: %q", out)
	}
}

func TestCompactToggleViaChatKey(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	if m.CompactMode == nil || m.CompactMode.Active() {
		t.Fatal("expected compact mode off initially")
	}
	_ = m.updateChat(tea.KeyPressMsg{Text: "c"})
	if !m.CompactMode.Active() {
		t.Error("pressing 'c' in chat view should toggle compact mode on")
	}
	_ = m.updateChat(tea.KeyPressMsg{Text: "c"})
	if m.CompactMode.Active() {
		t.Error("pressing 'c' again should toggle compact mode off")
	}
}

func TestCompactToggleDoesNotConsumeOtherKeys(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	_ = m.updateChat(tea.KeyPressMsg{Text: "a"})
	if m.CompactMode.Active() {
		t.Error("pressing 'a' should not toggle compact mode")
	}
	if !strings.Contains(m.ChatInput.RawValue(), "a") {
		t.Error("pressing 'a' should still reach the chat input")
	}
}
