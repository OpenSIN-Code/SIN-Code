// SPDX-License-Identifier: MIT
// Purpose: tests for the TUI chat view integration (ViewChat, sidebar item,
// key handling).
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/tui/chat"
)

func TestViewChatAdded(t *testing.T) {
	if ViewChat.String() != "Chat" {
		t.Errorf("expected 'Chat', got %q", ViewChat.String())
	}
	if ViewChat.Short() != "7·Chat" {
		t.Errorf("expected '7·Chat', got %q", ViewChat.Short())
	}
}

func TestSidebarHasChat(t *testing.T) {
	items := DefaultSidebarItems()
	hasChat := false
	for _, it := range items {
		if it.View == ViewChat {
			hasChat = true
			if it.Shortcut != "7" {
				t.Errorf("expected shortcut 7, got %q", it.Shortcut)
			}
		}
	}
	if !hasChat {
		t.Error("expected Chat in default sidebar items")
	}
}

func TestNextViewCyclesAll7(t *testing.T) {
	m := NewModel()
	seen := map[ViewKind]bool{}
	for i := 0; i < 7; i++ {
		seen[m.ViewKind] = true
		m.NextView()
	}
	if len(seen) != 7 {
		t.Errorf("expected 7 unique views, got %d", len(seen))
	}
}

func TestSwitchToChatVia7(t *testing.T) {
	m := NewModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	if m.ViewKind != ViewChat {
		t.Errorf("expected ViewChat, got %v", m.ViewKind)
	}
}

func TestChatInputInitializedOnView(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	if m.ChatInput == nil {
		t.Error("expected chat input")
	}
	m.initChatInput()
	if m.ChatInput == nil {
		t.Error("expected idempotent init")
	}
}

func TestRenderChatEmpty(t *testing.T) {
	m := NewModel()
	out := m.renderChat(m.Styles, 80, 20)
	if !strings.Contains(out, "Chat") {
		t.Error("expected title")
	}
	if !strings.Contains(out, "no messages") {
		t.Error("expected empty message")
	}
}

func TestRenderChatWithHistory(t *testing.T) {
	m := NewModel()
	m.ChatHistory = []string{"first message", "second message"}
	out := m.renderChat(m.Styles, 80, 20)
	if !strings.Contains(out, "first message") {
		t.Error("expected first message")
	}
	if !strings.Contains(out, "second message") {
		t.Error("expected second message")
	}
}

func TestRenderChatViewIncludesChatView(t *testing.T) {
	m := NewModel()
	m.Width = 100
	m.Height = 30
	m.Ready = true
	m.ViewKind = ViewChat
	out := m.View()
	if !strings.Contains(out, "Chat") {
		t.Error("expected Chat in view")
	}
}

func TestHandleChatSubmit(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	handleChatSubmit(m, chat.SubmitMsg{Text: "hello"})
	if len(m.ChatHistory) != 1 {
		t.Errorf("expected 1 entry, got %d", len(m.ChatHistory))
	}
}

func TestHandleChatSubmitWithAttachments(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	ci := newChatInput()
	_ = ci.AttachBytes([]byte("x"), "x.txt")
	handleChatSubmit(m, chat.SubmitMsg{
		Text:        "see this",
		Attachments: ci.Attachments(),
	})
	if len(m.ChatHistory) != 1 {
		t.Errorf("expected 1 entry, got %d", len(m.ChatHistory))
	}
	if !strings.Contains(m.ChatHistory[0], "x.txt") {
		t.Error("expected attachment in history")
	}
}

func TestChatHistoryTrimmedAt200(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	for i := 0; i < 250; i++ {
		handleChatSubmit(m, chat.SubmitMsg{Text: "msg"})
	}
	if len(m.ChatHistory) > 200 {
		t.Errorf("history should be capped at 200, got %d", len(m.ChatHistory))
	}
}

func TestUpdateChatRoutesKey(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ViewKind = ViewChat
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !strings.Contains(m.ChatInput.RawValue(), "a") {
		t.Error("expected 'a' routed to chat input")
	}
}
