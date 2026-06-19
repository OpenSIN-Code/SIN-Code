//go:build integration

// SPDX-License-Identifier: MIT
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

type stubLLM struct {
	mu        sync.Mutex
	responses map[string]string
	called    int
}

func newStubLLM() *stubLLM {
	return &stubLLM{
		responses: map[string]string{
			"fix the bug": "I'll fix the bug by correcting the nil pointer dereference on line 10. The issue is that `result` is not checked for nil before accessing `result.Value`.",
			"write tests": "I'll write comprehensive tests covering the main function and edge cases.",
			"default":     "I understand. Let me help you with that.",
		},
	}
}

func (s *stubLLM) run(ctx context.Context, prompt string, history []string) (string, int, error) {
	s.mu.Lock()
	s.called++
	s.mu.Unlock()

	lower := strings.ToLower(prompt)
	for key, resp := range s.responses {
		if key != "default" && strings.Contains(lower, key) {
			return resp, len(resp), nil
		}
	}
	return s.responses["default"], len(s.responses["default"]), nil
}

func (s *stubLLM) runStream(ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
	resp, tokens, err := s.run(ctx, prompt, history)
	if err != nil {
		return "", 0, err
	}
	words := strings.Fields(resp)
	for _, w := range words {
		onChunk(w + " ")
		time.Sleep(2 * time.Millisecond)
	}
	return resp, tokens, nil
}

func createTempProject(t *testing.T) string {
	dir := t.TempDir()
	mainGo := `package main

import "fmt"

func process(data []string) string {
	result := compute(data)
	return result.Value
}

func compute(data []string) *Result {
	if len(data) == 0 {
		return nil
	}
	return &Result{Value: data[0]}
}

type Result struct {
	Value string
}

func main() {
	fmt.Println(process(nil))
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module testproject\n\ngo 1.23\n")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func setupDogfoodModel(t *testing.T, dir string) *Model {
	stub := newStubLLM()
	origRunHook := chatRunnerRunHook
	origStreamHook := chatRunnerStreamHook
	chatRunnerRunHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string) (string, int, error) {
		return stub.run(ctx, prompt, history)
	}
	chatRunnerStreamHook = func(r *chat.Runner, ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
		return stub.runStream(ctx, prompt, history, onChunk)
	}
	t.Cleanup(func() {
		chatRunnerRunHook = origRunHook
		chatRunnerStreamHook = origStreamHook
	})

	m := NewModel()
	m.Workspace = dir
	m.initChatInput()
	m.ViewKind = ViewChat
	m.SwitchView(ViewChat)
	m.Footer.SetView(ViewChat)

	return m
}

func TestDogfoodFullSessionFlow(t *testing.T) {
	dir := createTempProject(t)
	m := setupDogfoodModel(t, dir)

	m.appendChat(ChatMessage{Kind: chatUser, Text: "Fix the bug in main.go"})
	m.AppendHistory(ViewChat.String(), "chat-submit", "Fix the bug in main.go", true)

	if len(m.ChatHistory) != 1 {
		t.Fatalf("expected 1 chat message, got %d", len(m.ChatHistory))
	}
	if m.ChatHistory[0].Kind != chatUser {
		t.Errorf("expected chatUser kind, got %v", m.ChatHistory[0].Kind)
	}
	if !strings.Contains(m.ChatHistory[0].Text, "Fix the bug") {
		t.Errorf("expected 'Fix the bug' in message, got %q", m.ChatHistory[0].Text)
	}

	m.appendChat(ChatMessage{
		Kind:   chatTool,
		Tool:   "sin_edit",
		Result: true,
		Detail: "main.go:7",
	})
	m.appendChat(ChatMessage{
		Kind: chatAssistant,
		Text: "I fixed the bug by adding a nil check before accessing result.Value.",
	})

	if len(m.ChatHistory) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(m.ChatHistory))
	}

	m.VerifyPanel.State = VerifyPassed
	m.VerifyPanel.Mode = "poc"
	m.VerifyPanel.Evidence = "go build ./... succeeded"
	m.VerifyPanel.Target = "main.go"
	m.appendChat(ChatMessage{Kind: chatVerify, Detail: "PASS: go build succeeded", Result: true})

	if m.VerifyPanel.State != VerifyPassed {
		t.Error("expected verify panel to show passed")
	}

	verifyRendered := RenderVerifyPanel(m.VerifyPanel, m.Styles, 80)
	if !strings.Contains(verifyRendered, "PASSED") {
		t.Errorf("expected 'PASSED' in verify panel, got %q", verifyRendered)
	}

	m.appendChat(ChatMessage{Kind: chatUser, Text: "Now write tests"})
	m.AppendHistory(ViewChat.String(), "chat-submit", "Now write tests", true)
	m.appendChat(ChatMessage{
		Kind:   chatTool,
		Tool:   "sin_test_generate",
		Result: true,
		Detail: "main_test.go",
	})
	m.appendChat(ChatMessage{
		Kind: chatAssistant,
		Text: "I've created comprehensive tests for the process and compute functions.",
	})

	m.VerifyPanel.State = VerifyPassed
	m.VerifyPanel.Evidence = "go test ./... passed"
	m.appendChat(ChatMessage{Kind: chatVerify, Detail: "PASS: go test succeeded", Result: true})

	userCount := 0
	toolCount := 0
	verifyCount := 0
	assistantCount := 0
	for _, msg := range m.ChatHistory {
		switch msg.Kind {
		case chatUser:
			userCount++
		case chatTool:
			toolCount++
		case chatVerify:
			verifyCount++
		case chatAssistant:
			assistantCount++
		}
	}
	if userCount != 2 {
		t.Errorf("expected 2 user messages, got %d", userCount)
	}
	if toolCount != 2 {
		t.Errorf("expected 2 tool calls, got %d", toolCount)
	}
	if verifyCount != 2 {
		t.Errorf("expected 2 verify messages, got %d", verifyCount)
	}
	if assistantCount != 2 {
		t.Errorf("expected 2 assistant messages, got %d", assistantCount)
	}

	historyCount := len(m.HistoryState.History)
	if historyCount < 2 {
		t.Errorf("expected at least 2 history entries, got %d", historyCount)
	}
}

func TestDogfoodToolCallRendering(t *testing.T) {
	dir := createTempProject(t)
	m := setupDogfoodModel(t, dir)

	m.appendChat(ChatMessage{Kind: chatUser, Text: "Fix the bug"})
	m.appendChat(ChatMessage{
		Kind:       chatTool,
		Tool:       "sin_edit",
		ToolInput:  `{"path":"main.go","old":"return result.Value","new":"if result != nil { return result.Value } return """}`,
		ToolOutput: "File edited successfully",
		Result:     true,
		Detail:     "main.go:7",
		Expanded:   true,
	})

	styles := m.Styles
	rendered := renderToolCard(m.ChatHistory[1], styles, 80, true)
	if !strings.Contains(rendered, "sin_edit") {
		t.Errorf("expected 'sin_edit' in tool card, got %q", rendered)
	}

	m.ChatHistory[1].Expanded = false
	renderedCollapsed := renderToolCard(m.ChatHistory[1], styles, 80, false)
	if !strings.Contains(renderedCollapsed, "sin_edit") {
		t.Errorf("expected 'sin_edit' in collapsed tool card, got %q", renderedCollapsed)
	}

	compactRendered := renderCompactMessage(m.ChatHistory[1], styles, 80, false, m.Spinner)
	if !strings.Contains(compactRendered, "sin_edit") {
		t.Errorf("expected 'sin_edit' in compact message, got %q", compactRendered)
	}

	m.appendChat(ChatMessage{
		Kind:   chatTool,
		Tool:   "sin_test_generate",
		Result: false,
		Detail: "generating tests...",
	})
	rendered2 := renderToolCard(m.ChatHistory[2], styles, 80, false)
	if !strings.Contains(rendered2, "sin_test_generate") {
		t.Errorf("expected 'sin_test_generate' in tool card, got %q", rendered2)
	}
}

func TestDogfoodCompactCommand(t *testing.T) {
	dir := createTempProject(t)
	m := setupDogfoodModel(t, dir)

	for i := 0; i < 20; i++ {
		m.appendChat(ChatMessage{
			Kind: chatAssistant,
			Text: fmt.Sprintf("Response %d with some content that fills the context window", i),
		})
	}

	if len(m.ChatHistory) != 20 {
		t.Fatalf("expected 20 messages, got %d", len(m.ChatHistory))
	}

	if m.CompactMode == nil {
		t.Fatal("expected compact mode to be initialized")
	}

	m.CompactMode.Toggle()
	if !m.CompactMode.Active() {
		t.Error("expected compact mode to be active after toggle")
	}

	styles := m.Styles
	compactRendered := renderCompactMessage(m.ChatHistory[0], styles, 80, false, m.Spinner)
	if compactRendered == "" {
		t.Error("expected non-empty compact render")
	}

	m.Footer.Compacted = true
	footerRendered := m.Footer.Render(styles)
	if !strings.Contains(footerRendered, "idle") {
		t.Errorf("expected 'idle' in footer, got %q", footerRendered)
	}

	m.CompactMode.Toggle()
	if m.CompactMode.Active() {
		t.Error("expected compact mode to be inactive after second toggle")
	}

	m.ChatHistory = nil
	m.appendChat(ChatMessage{Kind: chatUser, Text: "/clear"})
	clearHandled := handleChatSubmit(m, chat.SubmitMsg{Text: "/clear"})
	if clearHandled != nil {
		t.Error("expected nil cmd from /clear")
	}
	if len(m.ChatHistory) != 0 {
		t.Errorf("expected 0 messages after /clear, got %d", len(m.ChatHistory))
	}
}

func TestDogfoodViewSwitching(t *testing.T) {
	dir := createTempProject(t)
	m := setupDogfoodModel(t, dir)

	if m.ViewKind != ViewChat {
		t.Fatalf("expected initial view to be Chat, got %v", m.ViewKind)
	}

	views := []ViewKind{
		ViewTools, ViewSessions, ViewEFM, ViewConfig,
		ViewHistory, ViewTodos, ViewChat, ViewDAG,
		ViewContextViz, ViewAgentDashboard, ViewLSP,
		ViewMemory, ViewKanban,
	}

	for _, v := range views {
		m.SwitchView(v)
		if m.ViewKind != v {
			t.Errorf("expected view %v, got %v", v, m.ViewKind)
		}
		if m.Footer.View() != v {
			t.Errorf("expected footer view %v, got %v", v, m.Footer.View())
		}
	}

	m.SwitchView(ViewChat)
	for i := 0; i < viewCount; i++ {
		m.NextView()
	}
	if m.ViewKind != ViewChat {
		t.Errorf("expected wrap-around to Chat after %d nexts, got %v", viewCount, m.ViewKind)
	}

	for i := 0; i < viewCount; i++ {
		m.PrevView()
	}
	if m.ViewKind != ViewChat {
		t.Errorf("expected wrap-around to Chat after %d prevs, got %v", viewCount, m.ViewKind)
	}

	m.Sidebar.Toggle()
	if !m.Sidebar.Collapsed {
		t.Error("expected sidebar to be collapsed after toggle")
	}
	m.Sidebar.Toggle()
	if m.Sidebar.Collapsed {
		t.Error("expected sidebar to be visible after second toggle")
	}

	styles := m.Styles
	sidebarView := m.Sidebar.View(styles)
	if sidebarView == "" {
		t.Error("expected non-empty sidebar view")
	}
}

func TestDogfoodKeybindingStress(t *testing.T) {
	dir := createTempProject(t)
	m := setupDogfoodModel(t, dir)

	keys := []string{
		"tab", "shift+tab", "1", "2", "3", "4", "5", "6", "7",
		"8", "9", "0", "ctrl+b", "ctrl+p", "esc",
		"up", "down", "pgup", "pgdown",
		"?", "q",
	}

	for _, k := range keys {
		_, _ = m.Update(tea.KeyPressMsg{Text: k})
	}

	for i := 0; i < 50; i++ {
		m.NextView()
		m.PrevView()
		m.SwitchView(ViewChat)
	}

	m.SwitchView(ViewChat)
	m.initChatInput()
	if m.ChatInput != nil {
		m.ChatInput.SetValue("test message")
		if m.ChatInput.RawValue() != "test message" {
			t.Error("expected chat input to contain 'test message'")
		}
		m.ChatInput.Clear()
		if m.ChatInput.RawValue() != "" {
			t.Error("expected chat input to be cleared")
		}
	}

	m.SwitchView(ViewTools)
	_ = m.View()

	themeCount := len(Themes)
	for i := 0; i < themeCount+1; i++ {
		m.CycleTheme()
	}
	if m.ThemeIdx < 0 || m.ThemeIdx >= themeCount {
		t.Errorf("expected theme index in range [0,%d), got %d", themeCount, m.ThemeIdx)
	}

	mainPath := filepath.Join(dir, "main.go")
	m.ShowFilePreview(mainPath)
	if m.FilePreview == "" {
		t.Error("expected non-empty file preview after ShowFilePreview")
	}
	if !strings.Contains(m.FilePreview, "package main") {
		t.Errorf("expected 'package main' in preview, got %q", m.FilePreview)
	}
	m.ClearFilePreview()
	if m.FilePreview != "" {
		t.Error("expected empty file preview after ClearFilePreview")
	}

	_, err := os.Stat(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Errorf("main.go should still exist: %v", err)
	}
}
