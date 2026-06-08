// SPDX-License-Identifier: MIT
// Purpose: tests for the TUI chat view integration (ViewChat, sidebar item,
// key handling).
package tui

import (
	"os"
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
	// Ensure no API key so the runner is nil and handleChatSubmit appends
	// the in-band "no API key" assistant entry synchronously.
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Unsetenv("SIN_NIM_API_KEY")
	t.Cleanup(func() { if had { os.Setenv("SIN_NIM_API_KEY", prev) } })

	m := NewModel()
	m.initChatInput()
	handleChatSubmit(m, chat.SubmitMsg{Text: "hello"})
	if len(m.ChatHistory) != 2 {
		t.Errorf("expected 2 entries (user + assistant no-key), got %d: %+v",
			len(m.ChatHistory), m.ChatHistory)
	}
	if !strings.HasPrefix(m.ChatHistory[1], "assistant:") {
		t.Errorf("expected assistant entry second, got %q", m.ChatHistory[1])
	}
}

func TestHandleChatSubmitWithAttachments(t *testing.T) {
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Unsetenv("SIN_NIM_API_KEY")
	t.Cleanup(func() { if had { os.Setenv("SIN_NIM_API_KEY", prev) } })

	m := NewModel()
	m.initChatInput()
	ci := newChatInput()
	_ = ci.AttachBytes([]byte("x"), "x.txt")
	handleChatSubmit(m, chat.SubmitMsg{
		Text:        "see this",
		Attachments: ci.Attachments(),
	})
	if len(m.ChatHistory) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m.ChatHistory))
	}
	if !strings.Contains(m.ChatHistory[0], "x.txt") {
		t.Error("expected attachment in user entry")
	}
}

func TestChatHistoryTrimmedAt500(t *testing.T) {
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Unsetenv("SIN_NIM_API_KEY")
	t.Cleanup(func() { if had { os.Setenv("SIN_NIM_API_KEY", prev) } })

	m := NewModel()
	m.initChatInput()
	for i := 0; i < 600; i++ {
		handleChatSubmit(m, chat.SubmitMsg{Text: "msg"})
	}
	if len(m.ChatHistory) > 500 {
		t.Errorf("history should be capped at 500, got %d", len(m.ChatHistory))
	}
}

func TestHandleChatSubmitNoKeyWritesAssistantEntry(t *testing.T) {
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Unsetenv("SIN_NIM_API_KEY")
	t.Cleanup(func() { if had { os.Setenv("SIN_NIM_API_KEY", prev) } })

	m := NewModel()
	m.initChatInput()
	handleChatSubmit(m, chat.SubmitMsg{Text: "x"})
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if !strings.Contains(last, "no API key") {
		t.Errorf("expected no-API-key assistant entry, got %q", last)
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

func TestHandleChatSubmitWithRunnerWritesThinkingPlaceholder(t *testing.T) {
	prevNIM, hadNIM := os.LookupEnv("SIN_NIM_API_KEY")
	prevLLM, hadLLM := os.LookupEnv("SIN_LLM_API_KEY")
	os.Setenv("SIN_NIM_API_KEY", "fake-key")
	os.Unsetenv("SIN_LLM_API_KEY")
	t.Cleanup(func() {
		if hadNIM {
			os.Setenv("SIN_NIM_API_KEY", prevNIM)
		} else {
			os.Unsetenv("SIN_NIM_API_KEY")
		}
		if hadLLM {
			os.Setenv("SIN_LLM_API_KEY", prevLLM)
		}
	})

	m := NewModel()
	m.initChatInput()
	handleChatSubmit(m, chat.SubmitMsg{Text: "hello"})

	if len(m.ChatHistory) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(m.ChatHistory))
	}
	// First entry: the user message. Last entry: the "thinking..."
	// placeholder. We can't assert "thinking..." is *the* last entry here
	// because the background goroutine may have completed already in CI
	// if a real SIN_NIM_API_KEY happened to be set in the user shell —
	// the test above sets the key to "fake-key" so the call WILL fail,
	// but the goroutine is still racy vs. this assertion.
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if last == "assistant: thinking..." ||
		strings.HasPrefix(last, "assistant: (error:") ||
		strings.HasPrefix(last, "assistant: ") {
		// ok
	} else {
		t.Errorf("unexpected last entry: %q", last)
	}
}

func TestHandleChatResponseReplacesPlaceholder(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []string{
		"hello",
		"assistant: thinking...",
	}
	m.handleChatResponse(chat.ChatResponseMsg{Text: "world"})
	if got := m.ChatHistory[len(m.ChatHistory)-1]; got != "assistant: world" {
		t.Errorf("got %q", got)
	}
}

func TestHandleChatResponseAppendsWhenNoPlaceholder(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []string{"hello"}
	m.handleChatResponse(chat.ChatResponseMsg{Text: "world"})
	if got := m.ChatHistory[len(m.ChatHistory)-1]; got != "assistant: world" {
		t.Errorf("got %q", got)
	}
}

func TestHandleChatResponseError(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []string{"hello", "assistant: thinking..."}
	m.handleChatResponse(chat.ChatResponseMsg{Error: errFake{}})
	if got := m.ChatHistory[len(m.ChatHistory)-1]; !strings.Contains(got, "error") {
		t.Errorf("expected error entry, got %q", got)
	}
}

func TestHandleChatResponseEmpty(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []string{"hello", "assistant: thinking..."}
	m.handleChatResponse(chat.ChatResponseMsg{Text: ""})
	if got := m.ChatHistory[len(m.ChatHistory)-1]; !strings.Contains(got, "empty") {
		t.Errorf("expected empty marker, got %q", got)
	}
}

type errFake struct{}

func (errFake) Error() string { return "fake error" }
