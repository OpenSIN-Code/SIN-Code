// SPDX-License-Identifier: MIT
// Purpose: Modern chat view with markdown rendering, chat bubbles, and
// token/cost/time tracking. Built on existing ChatHistory structure.
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func (m *Model) renderChat(styles Styles, width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 6 {
		height = 6
	}

	var b strings.Builder

	// Reserve space for input at bottom
	inputHeight := 3
	chatHeight := height - inputHeight - 2
	if chatHeight < 3 {
		chatHeight = 3
	}

	if len(m.ChatHistory) == 0 {
		b.WriteString(styles.Muted.Render("  Send a message to get started."))
		b.WriteString("\n")
	} else {
		start := len(m.ChatHistory) - chatHeight
		if start < 0 {
			start = 0
		}

		mdRenderer := newMarkdownRenderer(styles)

		for _, entry := range m.ChatHistory[start:] {
			msg := parseChatEntry(entry)
			rendered := renderChatMessageCompact(msg, mdRenderer, styles, width)
			b.WriteString(rendered)
		}
	}

	// Add separator + input field at bottom
	separator := styles.Muted.Render(strings.Repeat("─", width))
	b.WriteString("\n")
	b.WriteString(separator)
	b.WriteString("\n")
	if m.ChatInput != nil {
		b.WriteString(m.ChatInput.View())
	}

	return b.String()
}

func renderChatMessageCompact(msg chatMsg, md *markdownRenderer, styles Styles, width int) string {
	var b strings.Builder

	switch msg.Kind {
	case chatUser:
		b.WriteString(styles.AccentText.Render("> "))
		b.WriteString(styles.Content.Render(msg.Text))
		b.WriteString("\n\n")

	case chatAssistant:
		rendered := md.render(msg.Text)
		b.WriteString(rendered)
		if !strings.HasSuffix(rendered, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")

	case chatTool:
		if msg.Result {
			b.WriteString(styles.StatusOK.Render("✓ " + msg.Tool))
			if msg.Detail != "" {
				b.WriteString(styles.Muted.Render(" → " + msg.Detail))
			}
		} else {
			b.WriteString(styles.AccentText.Render("⚡ " + msg.Tool))
			if msg.Detail != "" {
				b.WriteString(styles.Muted.Render(" " + msg.Detail))
			}
		}
		b.WriteString("\n")

	case chatVerify:
		status := "pending"
		if strings.Contains(msg.Detail, "PASS") {
			status = "pass"
		} else if strings.Contains(msg.Detail, "FAIL") {
			status = "fail"
		}
		b.WriteString(renderVerificationCompact(status, msg.Detail, styles))
		b.WriteString("\n")

	case chatAsk:
		b.WriteString(styles.StatusWarn.Render("🔒 " + msg.Detail))
		b.WriteString("\n")

	case chatDone:
		b.WriteString(styles.StatusOK.Render("✓ " + msg.Detail))
		b.WriteString("\n")

	case chatError:
		b.WriteString(renderError(msg.Text, styles))
		b.WriteString("\n")

	case chatThinking:
		b.WriteString(renderSpinner("thinking...", styles))
		b.WriteString("\n")

	case chatSystem:
		b.WriteString(styles.StatusWarn.Render("⚠ " + msg.Text))
		b.WriteString("\n")

	case chatAgent:
		// Show turn start as a subtle status line, not a full bubble.
		b.WriteString(styles.Muted.Render("  ⟳ agent " + msg.Text))
		b.WriteString("\n")
	}

	return b.String()
}

func renderVerificationCompact(status, message string, styles Styles) string {
	switch status {
	case "pass":
		return styles.StatusOK.Render("✓ " + message)
	case "fail":
		return styles.StatusErr.Render("✗ " + message)
	default:
		return styles.StatusWarn.Render("⏳ " + message)
	}
}

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
	
	// Auto-compact when approaching context window limit (80%)
	if m.Footer.TokensPct >= 0.8 && !m.Footer.Compacted {
		m.autoCompactContext()
	}
}

func (m *Model) autoCompactContext() {
	if len(m.ChatHistory) <= 50 {
		return
	}
	
	// Keep last 20 messages + summary of older ones
	summary := fmt.Sprintf("[context compacted: %d messages removed to free up space]", len(m.ChatHistory)-20)
	m.ChatHistory = append([]string{"system: " + summary}, m.ChatHistory[len(m.ChatHistory)-20:]...)
	m.Footer.Compacted = true
	
	m.SetBanner(&NotificationItem{
		ID:      "auto-compact",
		Title:   "Context Compacted",
		Message: fmt.Sprintf("Reduced from %d to 20 messages to stay within token limit", len(m.ChatHistory)),
		Type:    "info",
	})
	
	m.AppendHistory(ViewChat.String(), "auto-compact", summary, true)
}

func (m *Model) setStreaming(streaming bool) {
	m.Footer.Streaming = streaming
}

func (m *Model) updateSessionPreview() {
	if len(m.ChatHistory) == 0 {
		return
	}
	
	for i := len(m.ChatHistory) - 1; i >= 0; i-- {
		entry := m.ChatHistory[i]
		if strings.HasPrefix(entry, "assistant: ") {
			preview := strings.TrimPrefix(entry, "assistant: ")
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			m.Tabs.UpdatePreview(m.Tabs.ActiveIdx, preview)
			return
		}
	}
}

func renderToolCall(name, args string, styles Styles) string {
	var b strings.Builder
	b.WriteString(styles.AccentText.Render("⚡ "))
	b.WriteString(styles.Bold.Render(name))
	if args != "" {
		b.WriteString(styles.Muted.Render(" " + args))
	}
	return b.String()
}

func renderVerification(status, message string, styles Styles) string {
	var b strings.Builder
	switch status {
	case "pass":
		b.WriteString(styles.StatusOK.Render("✅ "))
		b.WriteString(styles.StatusOK.Render(message))
	case "fail":
		b.WriteString(styles.StatusErr.Render("❌ "))
		b.WriteString(styles.StatusErr.Render(message))
	default:
		b.WriteString(styles.StatusWarn.Render("⏳ "))
		b.WriteString(styles.StatusWarn.Render(message))
	}
	return b.String()
}

func renderError(err string, styles Styles) string {
	return styles.StatusErr.Render("❌ " + err)
}

func renderSpinner(text string, styles Styles) string {
	return styles.AccentText.Render("⏳ " + text)
}
