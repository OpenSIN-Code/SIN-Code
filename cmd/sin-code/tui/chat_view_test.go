// SPDX-License-Identifier: MIT
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	agentrunner "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/tui"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
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
	m.Update(tea.KeyPressMsg{Text: "7"})
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
	if !strings.Contains(out, "Send a message") {
		t.Error("expected empty prompt")
	}
}

func TestRenderChatWithHistory(t *testing.T) {
	m := NewModel()
	m.ChatHistory = []ChatMessage{
		{Kind: chatUser, Text: "first message"},
		{Kind: chatAssistant, Text: "second message"},
	}
	out := m.renderChat(m.Styles, 80, 20)
	if !strings.Contains(out, "first message") {
		t.Errorf("expected first message in:\n%s", out[:min(300, len(out))])
	}
	// Markdown renderer may wrap text across lines, so check for
	// the key words separately.
	if !strings.Contains(out, "second") {
		t.Errorf("expected 'second' in:\n%s", out[:min(300, len(out))])
	}
}

func TestRenderChatViewIncludesChatView(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.Width = 100
	m.Height = 30
	m.Ready = true
	m.ViewKind = ViewChat
	out := m.View().Content
	if !strings.Contains(out, "Send a message") {
		t.Errorf("expected chat prompt in view, got:\n%s", out[:min(200, len(out))])
	}
}

func TestHandleChatSubmit(t *testing.T) {
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Unsetenv("SIN_NIM_API_KEY")
	// Stub AgentRunner to return nil so ChatRunner fallback is tested
	origAR := newAgentRunnerHook
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return nil, fmt.Errorf("no agent runner in test")
	}
	t.Cleanup(func() {
		if had {
			os.Setenv("SIN_NIM_API_KEY", prev)
		} else {
			os.Unsetenv("SIN_NIM_API_KEY")
		}
		newAgentRunnerHook = origAR
	})

	m := NewModel()
	m.initChatInput()
	handleChatSubmit(m, chat.SubmitMsg{Text: "hello"})
	if len(m.ChatHistory) != 2 {
		t.Errorf("expected 2 entries (user + assistant no-key), got %d: %+v",
			len(m.ChatHistory), m.ChatHistory)
	}
	if m.ChatHistory[1].Kind != chatSystem {
		t.Errorf("expected system entry second, got kind %d", m.ChatHistory[1].Kind)
	}
}

// stubNoAgentRunner makes submitAgentPrompt return nil so ChatRunner
// fallback is tested. Call cleanup() in t.Cleanup.
func stubNoAgentRunner() (cleanup func()) {
	orig := newAgentRunnerHook
	newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
		return nil, fmt.Errorf("no agent runner in test")
	}
	return func() { newAgentRunnerHook = orig }
}

func TestHandleChatSubmitWithAttachments(t *testing.T) {
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Unsetenv("SIN_NIM_API_KEY")
	cleanup := stubNoAgentRunner()
	t.Cleanup(func() {
		if had {
			os.Setenv("SIN_NIM_API_KEY", prev)
		}
		cleanup()
	})

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
	if !strings.Contains(m.ChatHistory[0].Text, "x.txt") {
		t.Error("expected attachment in user entry")
	}
}

func TestChatHistoryTrimmedAt500(t *testing.T) {
	prev, had := os.LookupEnv("SIN_NIM_API_KEY")
	os.Unsetenv("SIN_NIM_API_KEY")
	t.Cleanup(func() {
		if had {
			os.Setenv("SIN_NIM_API_KEY", prev)
		}
	})

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
	cleanup := stubNoAgentRunner()
	t.Cleanup(func() {
		if had {
			os.Setenv("SIN_NIM_API_KEY", prev)
		}
		cleanup()
	})

	m := NewModel()
	m.initChatInput()
	handleChatSubmit(m, chat.SubmitMsg{Text: "x"})
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if !strings.Contains(last.Text, "no API key") {
		t.Errorf("expected no-API-key assistant entry, got %q", last.Text)
	}
}

func TestUpdateChatRoutesKey(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ViewKind = ViewChat
	_, _ = m.Update(tea.KeyPressMsg{Text: "a"})
	if !strings.Contains(m.ChatInput.RawValue(), "a") {
		t.Error("expected 'a' routed to chat input")
	}
}

func TestHandleChatSubmitWithRunnerWritesThinkingPlaceholder(t *testing.T) {
	prevNIM, hadNIM := os.LookupEnv("SIN_NIM_API_KEY")
	prevLLM, hadLLM := os.LookupEnv("SIN_LLM_API_KEY")
	os.Setenv("SIN_NIM_API_KEY", "fake-key")
	os.Unsetenv("SIN_LLM_API_KEY")
	cleanup := stubNoAgentRunner()
	t.Cleanup(func() {
		if hadNIM {
			os.Setenv("SIN_NIM_API_KEY", prevNIM)
		} else {
			os.Unsetenv("SIN_NIM_API_KEY")
		}
		if hadLLM {
			os.Setenv("SIN_LLM_API_KEY", prevLLM)
		}
		cleanup()
	})

	m := NewModel()
	m.initChatInput()
	handleChatSubmit(m, chat.SubmitMsg{Text: "hello"})

	if len(m.ChatHistory) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(m.ChatHistory))
	}
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if last.Kind == chatThinking ||
		last.Kind == chatError ||
		last.Kind == chatAssistant {
		// ok
	} else {
		t.Errorf("unexpected last entry kind: %d", last.Kind)
	}
}

func TestHandleChatResponseReplacesPlaceholder(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []ChatMessage{
		{Kind: chatUser, Text: "hello"},
		{Kind: chatThinking},
	}
	m.handleChatResponse(chat.ChatResponseMsg{Text: "world"})
	if got := m.ChatHistory[len(m.ChatHistory)-1]; got.Text != "world" {
		t.Errorf("got %q", got.Text)
	}
}

func TestHandleChatResponseAppendsWhenNoPlaceholder(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []ChatMessage{{Kind: chatUser, Text: "hello"}}
	m.handleChatResponse(chat.ChatResponseMsg{Text: "world"})
	if got := m.ChatHistory[len(m.ChatHistory)-1]; got.Text != "world" {
		t.Errorf("got %q", got.Text)
	}
}

func TestHandleChatResponseError(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []ChatMessage{
		{Kind: chatUser, Text: "hello"},
		{Kind: chatThinking},
	}
	m.handleChatResponse(chat.ChatResponseMsg{Error: errFake{}})
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if last.Kind != chatError {
		t.Errorf("expected error entry, got kind %d", last.Kind)
	}
}

func TestHandleChatResponseEmpty(t *testing.T) {
	m := NewModel()
	m.initChatInput()
	m.ChatHistory = []ChatMessage{
		{Kind: chatUser, Text: "hello"},
		{Kind: chatThinking},
	}
	m.handleChatResponse(chat.ChatResponseMsg{Text: ""})
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if !strings.Contains(last.Text, "empty") {
		t.Errorf("expected empty marker, got %q", last.Text)
	}
}

type errFake struct{}

func (errFake) Error() string { return "fake error" }
