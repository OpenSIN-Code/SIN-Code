// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func (m *Model) chatViewHelp() string {
	if m.ChatInput == nil {
		return "Ctrl+S submit · /attach <path> · /clear"
	}
	return fmt.Sprintf("Ctrl+S submit · /attach <path> · /clear · %d attachments", len(m.ChatInput.Attachments()))
}

func (m *Model) renderChatFooter(styles Styles, width int) string {
	if width < 20 {
		width = 20
	}

	var b strings.Builder

	tokens := m.Footer.Tokens
	tokensPct := m.Footer.TokensPct
	cost := m.Footer.Cost
	agent := m.Footer.AgentName()

	left := styles.FooterKey.Render(" " + agent + " ")
	mid := styles.Muted.Render(fmt.Sprintf("tokens %d (%.0f%%)", tokens, tokensPct*100))
	right := styles.FooterVal.Render(cost)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap > lipgloss.Width(mid) {
		mid += strings.Repeat(" ", gap-lipgloss.Width(mid))
	}

	b.WriteString(styles.Footer.Render(left + mid + right))
	return b.String()
}

func (m *Model) updateChatMetrics(tokens int, cost float64, duration time.Duration) {
	m.Footer.Tokens = tokens
	m.Footer.Cost = fmt.Sprintf("$%.2f", cost)
	m.Footer.Duration = duration
	m.Footer.TokensPct = float64(tokens) / 128000.0
	if m.Footer.TokensPct > 1.0 {
		m.Footer.TokensPct = 1.0
	}

	if m.Footer.TokensPct >= 0.8 && !m.Footer.Compacted {
		m.autoCompactContext()
	}
}

func (m *Model) autoCompactContext() {
	if len(m.ChatHistory) <= 50 {
		return
	}

	summary := fmt.Sprintf("[context compacted: %d messages removed to free up space]", len(m.ChatHistory)-20)
	m.ChatHistory = append([]ChatMessage{{Kind: chatSystem, Text: summary}}, m.ChatHistory[len(m.ChatHistory)-20:]...)
	m.Footer.Compacted = true

	m.SetBanner(&NotificationItem{
		ID:      "auto-compact",
		Title:   "Context Compacted",
		Message: fmt.Sprintf("Reduced from %d to 20 messages to stay within token limit", len(m.ChatHistory)),
		Type:    "info",
	})

	m.AppendHistory(ViewChat.String(), "auto-compact", summary, true)
}

func (m *Model) manualCompactContext() {
	if len(m.ChatHistory) <= 10 {
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "Not enough messages to compact (need >10)"})
		return
	}
	before := len(m.ChatHistory)
	keep := 8
	summary := fmt.Sprintf("[manual compaction: %d messages removed to free up space]", before-keep)
	m.ChatHistory = append([]ChatMessage{{Kind: chatSystem, Text: summary}}, m.ChatHistory[before-keep:]...)
	m.Footer.Compacted = true

	m.SetBanner(&NotificationItem{
		ID:      "manual-compact",
		Title:   "Context Compacted",
		Message: fmt.Sprintf("Reduced from %d to %d messages", before, keep+1),
		Type:    "info",
	})

	m.AppendHistory(ViewChat.String(), "manual-compact", summary, true)
}

func (m *Model) setStreaming(streaming bool) {
	m.Footer.Streaming = streaming
	if streaming && m.TypewriterBuf != nil {
		m.TypewriterBuf.Reset()
	}
	if m.ChatInput != nil {
		m.ChatInput.SetDisabled(streaming)
	}
}

// IsStreaming reports whether the model is currently receiving a streaming
// response or waiting for the agent loop to produce output.
func (m *Model) IsStreaming() bool {
	return m.Footer.Streaming
}

func (m *Model) updateSessionPreview() {
	if len(m.ChatHistory) == 0 {
		return
	}

	for i := len(m.ChatHistory) - 1; i >= 0; i-- {
		msg := m.ChatHistory[i]
		if msg.Kind == chatAssistant {
			preview := msg.Text
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			m.Tabs.UpdatePreview(m.Tabs.ActiveIdx, preview)
			return
		}
	}
}
