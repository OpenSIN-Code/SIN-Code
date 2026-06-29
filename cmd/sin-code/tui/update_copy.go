// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// EnterCopyMode activates copy mode with the current chat history flattened
// into lines.
func (m *Model) EnterCopyMode() {
	lines := m.flattenChatLines()
	if m.CopyMode == nil {
		m.CopyMode = NewCopyMode(m.Styles)
	}
	m.CopyMode.Styles = m.Styles
	m.CopyMode.Enter(lines)
	m.Mode = ModeCopy
}

// ExitCopyMode deactivates copy mode and returns to normal mode.
func (m *Model) ExitCopyMode() {
	if m.CopyMode != nil {
		m.CopyMode.Exit()
	}
	m.Mode = ModeNormal
}

// flattenChatLines converts the chat history into a flat list of text lines
// suitable for the copy mode overlay.
func (m *Model) flattenChatLines() []string {
	var lines []string
	for _, msg := range m.ChatHistory {
		var label string
		switch msg.Kind {
		case chatUser:
			label = "USER"
		case chatAssistant:
			label = "ASSISTANT"
		case chatTool:
			label = "TOOL: " + msg.Tool
		case chatError:
			label = "ERROR"
		case chatSystem:
			label = "SYSTEM"
		default:
			label = "MSG"
		}
		lines = append(lines, "["+label+"]")
		text := msg.Text
		if text == "" {
			text = msg.Detail
		}
		if text == "" && msg.Tool != "" {
			text = msg.ToolInput
			if msg.ToolOutput != "" {
				text += "\n" + msg.ToolOutput
			}
		}
		if text != "" {
			lines = append(lines, strings.Split(text, "\n")...)
		}
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		lines = []string{"(no chat history)"}
	}
	return lines
}

func (m *Model) handleCopyModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.CopyMode == nil || !m.CopyMode.Active {
		m.Mode = ModeNormal
		return m, nil
	}
	k := msg.String()
	switch k {
	case "q", "esc":
		m.ExitCopyMode()
		return m, nil
	case "j", "down":
		m.CopyMode.Down()
		return m, nil
	case "k", "up":
		m.CopyMode.Up()
		return m, nil
	case "pgdown", "ctrl+d":
		m.CopyMode.PageDown()
		return m, nil
	case "pgup", "ctrl+u":
		m.CopyMode.PageUp()
		return m, nil
	case "g":
		m.CopyMode.Top()
		return m, nil
	case "G":
		m.CopyMode.Bottom()
		return m, nil
	case "v":
		m.CopyMode.ToggleVisual()
		return m, nil
	case "y":
		text := m.CopyMode.Yank()
		m.ExitCopyMode()
		if text != "" {
			m.SetBanner(&NotificationItem{
				ID:      fmt.Sprintf("yank-%d", time.Now().UnixNano()),
				Title:   "Yanked!",
				Message: "Selected text copied to clipboard",
				Type:    "info",
			})
		}
		return m, nil
	case "Y":
		text := m.CopyMode.YankAll()
		m.ExitCopyMode()
		if text != "" {
			m.SetBanner(&NotificationItem{
				ID:      fmt.Sprintf("yank-all-%d", time.Now().UnixNano()),
				Title:   "Yanked!",
				Message: "All text copied to clipboard",
				Type:    "info",
			})
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) updateChatFocusFromViewport() {
	yOffset := m.ChatViewport.YOffset()
	if yOffset < 0 {
		yOffset = 0
	}
	if yOffset < len(m.ChatHistory) {
		m.ChatFocusIdx = yOffset
	}
}

func copyToClipboard(text string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
}
