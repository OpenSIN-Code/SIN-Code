// SPDX-License-Identifier: MIT
package tui

import (
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestResizeEnlarge(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.Width != 80 || m.Height != 24 {
		t.Fatalf("expected 80x24, got %dx%d", m.Width, m.Height)
	}

	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.Width != 120 || m.Height != 40 {
		t.Errorf("expected 120x40, got %dx%d", m.Width, m.Height)
	}
	v := m.View()
	if v.Content == "" {
		t.Error("View should produce content at 120x40")
	}
}

func TestResizeShrink(t *testing.T) {
	m := NewModel()
	m.ChatHistory = benchChatHistory(10)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.Width != 80 || m.Height != 24 {
		t.Errorf("expected 80x24, got %dx%d", m.Width, m.Height)
	}
	v := m.View()
	if v.Content == "" {
		t.Error("View should produce content at 80x24")
	}
}

func TestResizeVerySmall(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 5})

	if m.Width != 20 || m.Height != 5 {
		t.Errorf("expected 20x5, got %dx%d", m.Width, m.Height)
	}
	v := m.View()
	if v.Content == "" {
		t.Error("View should still produce content at 20x5")
	}
}

func TestResizeVeryLarge(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 400, Height: 100})

	if m.Width != 400 || m.Height != 100 {
		t.Errorf("expected 400x100, got %dx%d", m.Width, m.Height)
	}
	v := m.View()
	if v.Content == "" {
		t.Error("View should produce content at 400x100")
	}
	lines := 0
	for _, c := range v.Content {
		if c == '\n' {
			lines++
		}
	}
	if lines > 100 {
		t.Errorf("content should not exceed 100 lines, got %d", lines)
	}
}

func TestResizeRapidNoRace(t *testing.T) {
	m := NewModel()
	m.ChatHistory = benchChatHistory(20)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			w := 80 + i*20
			h := 24 + i*4
			m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			_ = m.View()
		}
	}()
	wg.Wait()
}

func TestResizeTmuxSplit(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	if m.Width != 60 {
		t.Errorf("expected width 60, got %d", m.Width)
	}
	v := m.View()
	if v.Content == "" {
		t.Error("View should produce content after tmux split")
	}
}

func TestResizeZeroHeight(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 0})
	v := m.View()
	if v.Content == "" {
		t.Error("View should produce content even with 0 height")
	}
}

func TestResizeZeroWidth(t *testing.T) {
	m := NewModel()
	m.Update(tea.WindowSizeMsg{Width: 0, Height: 24})
	v := m.View()
	if v.Content == "" {
		t.Error("View should produce content even with 0 width")
	}
}

func TestPadContentEdgeDimensions(t *testing.T) {
	result := padContent("hello", 1, 1)
	if result == "" {
		t.Error("padContent should not return empty for 1x1")
	}

	result = padContent("hello\nworld", 0, 0)
	if result != "" {
		t.Error("padContent with 0x0 should return empty")
	}

	result = padContent("", 5, 3)
	if result == "" {
		t.Error("padContent with empty string should still pad")
	}
}

func TestPadRightEdgeDimensions(t *testing.T) {
	result := padRight("hi", 0)
	if result != "hi" {
		t.Errorf("padRight with width 0 should return original, got %q", result)
	}

	result = padRight("hello", 3)
	if result != "hello" {
		t.Errorf("padRight with width < len should return original, got %q", result)
	}

	result = padRight("", 5)
	if len(result) != 5 {
		t.Errorf("padRight empty with width 5 should be 5 chars, got %d", len(result))
	}
}

func TestSplitLinesEdgeDimensions(t *testing.T) {
	result := splitLines("a\nb\nc", 5, 2)
	if result == "" {
		t.Error("splitLines should not return empty")
	}

	result = splitLines("a\nb\nc", 5, 0)
	if result != "" {
		t.Error("splitLines with height 0 should return empty")
	}

	result = splitLines("", 5, 3)
	if result == "" {
		t.Error("splitLines with empty string should still produce padded lines")
	}
}

func TestComposeLayoutOneByOne(t *testing.T) {
	m := NewModel()
	sidebar := NewSidebar()
	footer := NewFooter(1)
	tabs := NewTabs()
	tabs.Width = 1

	result := ComposeLayout(tabs, sidebar, ViewChat, "test", "", footer, m.Styles, 1, 1)
	if result == "" {
		t.Error("ComposeLayout should produce output for 1x1")
	}
}
