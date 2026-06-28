// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderBlock_User(t *testing.T) {
	styles := NewStyles(Themes[0])
	block := ChatBlock{
		Role:      "user",
		Content:   "Hello, world!",
		Timestamp: time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
		Width:     60,
	}
	out := RenderBlock(block, styles, 60)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(out, "user") {
		t.Error("expected 'user' in output")
	}
	if !strings.Contains(out, "Hello, world!") {
		t.Error("expected content in output")
	}
	if !strings.Contains(out, "\u25b6") {
		t.Error("expected user icon \u25b6 in header")
	}
}

func TestRenderBlock_Assistant(t *testing.T) {
	styles := NewStyles(Themes[0])
	block := ChatBlock{
		Role:      "assistant",
		Model:     "glm-5p2",
		Content:   "I can help with that.",
		Timestamp: time.Date(2026, 6, 28, 12, 1, 0, 0, time.UTC),
		Width:     60,
	}
	out := RenderBlock(block, styles, 60)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(out, "assistant") {
		t.Error("expected 'assistant' in output")
	}
	if !strings.Contains(out, "glm-5p2") {
		t.Error("expected model name in output")
	}
	if !strings.Contains(out, "I can help with that.") {
		t.Error("expected content in output")
	}
	if !strings.Contains(out, "\u2726") {
		t.Error("expected assistant icon \u2726 in header")
	}
}

func TestRenderBlock_Collapsed(t *testing.T) {
	styles := NewStyles(Themes[0])
	block := ChatBlock{
		Role:      "assistant",
		Model:     "glm-5p2",
		Content:   "This body should be hidden when collapsed.",
		Collapsed: true,
		Timestamp: time.Date(2026, 6, 28, 12, 2, 0, 0, time.UTC),
		Width:     60,
	}
	out := RenderBlock(block, styles, 60)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if strings.Contains(out, "This body should be hidden when collapsed.") {
		t.Error("expected body to be hidden when collapsed")
	}
	if !strings.Contains(out, "\u25b6") {
		t.Error("expected collapse indicator \u25b6 in header")
	}
	// Header should still be present
	if !strings.Contains(out, "assistant") {
		t.Error("expected role in collapsed header")
	}
}

func TestRenderBlock_VerifyPass(t *testing.T) {
	styles := NewStyles(Themes[0])
	block := ChatBlock{
		Role:         "assistant",
		Content:      "Task done.",
		VerifyResult: "pass",
		Timestamp:    time.Date(2026, 6, 28, 12, 3, 0, 0, time.UTC),
		Width:        60,
	}
	out := RenderBlock(block, styles, 60)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(out, "\u2713") {
		t.Error("expected verify-pass checkmark \u2713 in header")
	}
}

func TestRenderBlock_VerifyFail(t *testing.T) {
	styles := NewStyles(Themes[0])
	block := ChatBlock{
		Role:         "assistant",
		Content:      "Task failed.",
		VerifyResult: "fail",
		Timestamp:    time.Date(2026, 6, 28, 12, 4, 0, 0, time.UTC),
		Width:        60,
	}
	out := RenderBlock(block, styles, 60)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(out, "\u2717") {
		t.Error("expected verify-fail X mark \u2717 in header")
	}
}

func TestRenderBlock_ToolCalls(t *testing.T) {
	styles := NewStyles(Themes[0])
	block := ChatBlock{
		Role:      "assistant",
		Model:     "glm-5p2",
		Content:   "I used 3 tools.",
		ToolCalls: 3,
		Timestamp: time.Date(2026, 6, 28, 12, 5, 0, 0, time.UTC),
		Width:     60,
	}
	out := RenderBlock(block, styles, 60)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(out, "3 tool calls") {
		t.Error("expected '3 tool calls' in header")
	}
	if !strings.Contains(out, "\u2699") {
		t.Error("expected tool icon \u2699 in header")
	}
}

func TestToggleBlockCollapse(t *testing.T) {
	m := NewModel()
	m.ChatHistory = []ChatMessage{
		{Kind: chatUser, Text: "hello"},
		{Kind: chatAssistant, Text: "hi there"},
		{Kind: chatTool, Tool: "sin_edit"},
		{Kind: chatAssistant, Text: "done"},
	}

	// Toggle with idx=-1 should collapse the last assistant block
	m.ToggleBlockCollapse(-1)
	last := m.ChatHistory[len(m.ChatHistory)-1]
	if !last.Expanded {
		t.Error("expected last assistant block to be expanded (toggled from default false)")
	}

	// Toggle again should collapse it back
	m.ToggleBlockCollapse(-1)
	last = m.ChatHistory[len(m.ChatHistory)-1]
	if last.Expanded {
		t.Error("expected last assistant block to be collapsed after second toggle")
	}

	// Toggle by explicit index
	m.ToggleBlockCollapse(1)
	if !m.ChatHistory[1].Expanded {
		t.Error("expected block at idx 1 to be expanded")
	}
	m.ToggleBlockCollapse(1)
	if m.ChatHistory[1].Expanded {
		t.Error("expected block at idx 1 to be collapsed after second toggle")
	}

	// Out-of-bounds should not panic
	m.ToggleBlockCollapse(999)
}

func TestChatBlockList_Render(t *testing.T) {
	styles := NewStyles(Themes[0])
	blocks := ChatBlockList{
		{Role: "user", Content: "Hello", Timestamp: time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC), Width: 60},
		{Role: "assistant", Model: "glm-5p2", Content: "Hi!", Timestamp: time.Date(2026, 6, 28, 12, 1, 0, 0, time.UTC), Width: 60},
	}
	out := blocks.Render(styles, 60)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(out, "Hello") {
		t.Error("expected first block content")
	}
	if !strings.Contains(out, "Hi!") {
		t.Error("expected second block content")
	}
}
