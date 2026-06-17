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
	b.WriteString(styles.ContentHdr.Render("💬 Chat"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n\n")

	historyLines := height - 10
	if historyLines < 1 {
		historyLines = 1
	}

	if len(m.ChatHistory) == 0 {
		b.WriteString(styles.Muted.Render("  (no messages yet — type a prompt and press Ctrl+S)"))
		b.WriteString("\n\n")
	} else {
		start := len(m.ChatHistory) - historyLines
		if start < 0 {
			start = 0
		}

		mdRenderer := newMarkdownRenderer(styles)

		for _, entry := range m.ChatHistory[start:] {
			msg := parseChatEntry(entry)
			text := msg.Text
			if len(text) > width-4 {
				text = text[:width-4] + "…"
			}
			msg.Text = text
			rendered := renderChatMessage(msg, mdRenderer, styles, width-4)
			b.WriteString(rendered)
			b.WriteString("\n")
		}
	}

	b.WriteString(styles.Muted.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n")

	if m.ChatInput != nil {
		status := m.ChatInput.RenderStatus()
		b.WriteString(styles.Muted.Render(status))
		b.WriteString("\n")
		b.WriteString(m.ChatInput.View())
	}

	return b.String()
}

func renderChatMessage(msg chatMsg, md *markdownRenderer, styles Styles, width int) string {
	var b strings.Builder

	switch msg.Kind {
	case chatUser:
		b.WriteString(styles.AccentText.Render("❯ You"))
		b.WriteString("\n")
		b.WriteString(styles.Content.Render("  " + msg.Text))
		b.WriteString("\n\n")

	case chatAssistant:
		b.WriteString(styles.StatusOK.Render("✓ Assistant"))
		b.WriteString("\n")
		rendered := md.render(msg.Text)
		for _, line := range strings.Split(rendered, "\n") {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")

	case chatTool:
		b.WriteString(renderToolCall(msg.Tool, msg.Detail, styles))
		b.WriteString("\n\n")

	case chatVerify:
		status := "pending"
		if strings.Contains(msg.Detail, "pass") {
			status = "pass"
		} else if strings.Contains(msg.Detail, "fail") {
			status = "fail"
		}
		b.WriteString(renderVerification(status, msg.Detail, styles))
		b.WriteString("\n\n")

	case chatAsk:
		b.WriteString(styles.StatusWarn.Render("🔒 " + msg.Detail))
		b.WriteString("\n\n")

	case chatDone:
		b.WriteString(styles.StatusOK.Render("✅ " + msg.Detail))
		b.WriteString("\n\n")

	case chatError:
		b.WriteString(renderError(msg.Text, styles))
		b.WriteString("\n\n")

	case chatThinking:
		b.WriteString(renderSpinner("thinking...", styles))
		b.WriteString("\n\n")

	case chatSystem:
		b.WriteString(styles.StatusWarn.Render("⚠ " + msg.Text))
		b.WriteString("\n\n")

	case chatAgent:
		b.WriteString(styles.AccentText.Render("⟳ " + msg.Detail))
		b.WriteString("\n\n")
	}

	return b.String()
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
