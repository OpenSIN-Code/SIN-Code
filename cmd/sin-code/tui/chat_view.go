// SPDX-License-Identifier: MIT
// Purpose: render the chat view in the TUI center panel. Shows recent
// history at the top, chat input at the bottom.
package tui

import (
	"fmt"
	"strings"
)

func (m *Model) renderChat(styles Styles, width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 6 {
		height = 6
	}
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("Chat (orchestrator-driven)"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n\n")

	historyLines := height - 8
	if historyLines < 1 {
		historyLines = 1
	}
	if len(m.ChatHistory) == 0 {
		b.WriteString(styles.Muted.Render("  (no messages yet — type a prompt and press Ctrl+S)"))
		b.WriteString("\n")
	} else {
		start := len(m.ChatHistory) - historyLines
		if start < 0 {
			start = 0
		}
		for _, entry := range m.ChatHistory[start:] {
			lines := strings.Split(entry, "\n")
			for _, line := range lines {
				if len(line) > width-4 {
					line = line[:width-4] + "…"
				}
				b.WriteString(styles.Content.Render("  " + line))
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}
	if m.ChatInput == nil {
		return b.String()
	}
	status := m.ChatInput.RenderStatus()
	b.WriteString(styles.Muted.Render(status))
	b.WriteString("\n")
	b.WriteString(m.ChatInput.View())
	return b.String()
}

func (m *Model) chatViewHelp() string {
	return fmt.Sprintf("Ctrl+S submit · /attach <path> · /clear · %d attachments", len(m.ChatInput.Attachments()))
}
