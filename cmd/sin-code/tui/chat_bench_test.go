// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// benchChatHistory builds a slice of n ChatMessages alternating between
// user and assistant turns with realistic-length text.
func benchChatHistory(n int) []ChatMessage {
	hist := make([]ChatMessage, 0, n)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			hist = append(hist, ChatMessage{
				Kind: chatUser,
				Text: fmt.Sprintf("User message %d: please review the implementation of the foo function and ensure it handles edge cases properly.", i),
			})
		} else {
			hist = append(hist, ChatMessage{
				Kind: chatAssistant,
				Text: fmt.Sprintf("Assistant response %d: I have reviewed the code. The foo function looks correct. Here are my findings:\n\n1. Edge case A is handled\n2. Edge case B needs attention\n3. Performance is acceptable", i),
			})
		}
	}
	return hist
}

// benchToolCallTree builds a tree with n root nodes, each having 2 children.
func benchToolCallTree(n int) *ToolCallTree {
	tree := &ToolCallTree{}
	now := time.Now()
	for i := 0; i < n; i++ {
		root := &ToolCallNode{
			ID:        fmt.Sprintf("root-%d", i),
			Tool:      fmt.Sprintf("sin_tool_%d", i%5),
			Args:      fmt.Sprintf(`{"path": "/src/file%d.go", "mode": "read"}`, i),
			Output:    fmt.Sprintf("Output line one\nOutput line two for item %d", i),
			Status:    "success",
			StartTime: now,
			Duration:  time.Duration(i+1) * 10 * time.Millisecond,
			Expanded:  true,
		}
		for j := 0; j < 2; j++ {
			child := &ToolCallNode{
				ID:        fmt.Sprintf("child-%d-%d", i, j),
				Tool:      fmt.Sprintf("sin_sub_%d", j),
				Args:      fmt.Sprintf(`{"key": "value%d"}`, j),
				Output:    fmt.Sprintf("Child output %d-%d", i, j),
				Status:    "success",
				StartTime: now,
				Duration:  5 * time.Millisecond,
				Expanded:  false,
			}
			root.Children = append(root.Children, child)
		}
		tree.Nodes = append(tree.Nodes, root)
	}
	return tree
}

// benchSessionData builds n SessionNodeData entries with a tree structure:
// every 3rd session is a child of the previous one.
func benchSessionData(n int) []SessionNodeData {
	data := make([]SessionNodeData, 0, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		var parentID string
		if i > 0 && i%3 == 0 {
			parentID = fmt.Sprintf("sess-%d", i-1)
		}
		data = append(data, SessionNodeData{
			ID:           fmt.Sprintf("sess-%d", i),
			Name:         fmt.Sprintf("Session %d", i),
			ParentID:     parentID,
			CreatedAt:    now.Add(time.Duration(i) * time.Hour),
			LastActive:   now.Add(time.Duration(i) * time.Minute),
			MessageCount: i * 5,
			Preview:      fmt.Sprintf("Last assistant message preview for session %d", i),
		})
	}
	return data
}

// benchFuzzyItems builds n command-like strings for fuzzy filtering.
func benchFuzzyItems(n int) []string {
	items := make([]string, 0, n)
	verbs := []string{"discover", "execute", "map", "grasp", "scout", "harvest", "orchestrate", "security", "sbom", "config"}
	for i := 0; i < n; i++ {
		verb := verbs[i%len(verbs)]
		items = append(items, fmt.Sprintf("%s --option-%d --verbose --output result%d.txt", verb, i, i))
	}
	return items
}

// ── renderChat benchmarks ─────────────────────────────────────────────────────

func BenchmarkRenderChat10(b *testing.B) {
	m := NewModel()
	m.ChatHistory = benchChatHistory(10)
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.renderChat(styles, 80, 24)
	}
}

func BenchmarkRenderChat100(b *testing.B) {
	m := NewModel()
	m.ChatHistory = benchChatHistory(100)
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.renderChat(styles, 80, 24)
	}
}

func BenchmarkRenderChat500(b *testing.B) {
	m := NewModel()
	m.ChatHistory = benchChatHistory(500)
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.renderChat(styles, 80, 24)
	}
}

// ── Full View() benchmarks ────────────────────────────────────────────────────

func BenchmarkViewFull10(b *testing.B) {
	m := NewModel()
	m.ChatHistory = benchChatHistory(10)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := m.View()
		_ = v.Content
	}
}

func BenchmarkViewFull100(b *testing.B) {
	m := NewModel()
	m.ChatHistory = benchChatHistory(100)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := m.View()
		_ = v.Content
	}
}

func BenchmarkViewFull500(b *testing.B) {
	m := NewModel()
	m.ChatHistory = benchChatHistory(500)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := m.View()
		_ = v.Content
	}
}

// ── ToolCallTree rendering benchmarks ─────────────────────────────────────────

func BenchmarkRenderToolCallTree10(b *testing.B) {
	tree := benchToolCallTree(10)
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderToolCallTree(tree, styles, 80)
	}
}

func BenchmarkRenderToolCallTree50(b *testing.B) {
	tree := benchToolCallTree(50)
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderToolCallTree(tree, styles, 80)
	}
}

func BenchmarkRenderToolCallTree100(b *testing.B) {
	tree := benchToolCallTree(100)
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderToolCallTree(tree, styles, 80)
	}
}

// ── Fuzzy filter benchmarks ───────────────────────────────────────────────────

func BenchmarkFuzzyFilter100(b *testing.B) {
	items := benchFuzzyItems(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fuzzyFilter(items, "sec ver")
	}
}

func BenchmarkFuzzyFilter500(b *testing.B) {
	items := benchFuzzyItems(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fuzzyFilter(items, "sec ver")
	}
}

// ── SessionTree rendering benchmarks ──────────────────────────────────────────

func BenchmarkRenderSessionTree10(b *testing.B) {
	tree := BuildSessionTree(benchSessionData(10))
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderSessionTree(tree, styles, 80)
	}
}

func BenchmarkRenderSessionTree50(b *testing.B) {
	tree := BuildSessionTree(benchSessionData(50))
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderSessionTree(tree, styles, 80)
	}
}

func BenchmarkRenderSessionTree100(b *testing.B) {
	tree := BuildSessionTree(benchSessionData(100))
	styles := NewStyles(Themes[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderSessionTree(tree, styles, 80)
	}
}
