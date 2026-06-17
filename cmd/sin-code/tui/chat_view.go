// SPDX-License-Identifier: MIT
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

	inputHeight := 4
	chatHeight := height - inputHeight
	if chatHeight < 3 {
		chatHeight = 3
	}

	if len(m.ChatHistory) == 0 {
		welcome := "Send a message to get started.\n\nCtrl+S to send · /clear to reset · /attach for files"
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, styles.Muted.Render(welcome))
	}

	var content strings.Builder
	mdRenderer := newMarkdownRenderer(styles)

	for i, msg := range m.ChatHistory {
		rendered := renderChatMessageCompact(msg, mdRenderer, styles, width, i == m.ChatFocusIdx)
		content.WriteString(rendered)
	}

	m.ChatViewport.SetWidth(width)
	m.ChatViewport.SetHeight(chatHeight)
	m.ChatViewport.SetContent(content.String())
	if !m.ChatViewport.AtBottom() {
		m.ChatViewport.GotoBottom()
	}

	var b strings.Builder
	b.WriteString(m.ChatViewport.View())

	if m.Mode == ModeSearch {
		b.WriteString("\n")
		b.WriteString(styles.AccentText.Render("/" + m.SearchInput.View()))
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	if m.ChatInput != nil {
		m.ChatInput.SetSize(width, 3)
		b.WriteString(m.ChatInput.View())
	}

	return b.String()
}

func renderChatMessageCompact(msg ChatMessage, md *markdownRenderer, styles Styles, width int, focused bool) string {
	var b strings.Builder

	focusPrefix := ""
	if focused {
		focusPrefix = "▸ "
	}

	switch msg.Kind {
	case chatUser:
		b.WriteString(styles.UserMsg.Render(focusPrefix + "> "))
		b.WriteString(styles.Content.Render(msg.Text))
		b.WriteString("\n")

	case chatAssistant:
		rendered := md.render(msg.Text)
		b.WriteString(rendered)
		if !strings.HasSuffix(rendered, "\n") {
			b.WriteString("\n")
		}

	case chatTool:
		if msg.Expanded {
			b.WriteString(styles.AccentText.Render(focusPrefix + "⚡ " + msg.Tool))
			b.WriteString("\n")
			if msg.ToolInput != "" {
				b.WriteString(styles.Muted.Render("  input: "))
				b.WriteString(styles.Muted.Render(msg.ToolInput))
				b.WriteString("\n")
			}
			if msg.Detail != "" {
				b.WriteString(styles.Muted.Render("  output: "))
				b.WriteString(styles.Muted.Render(msg.Detail))
				b.WriteString("\n")
			}
		} else {
			if msg.Result {
				b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + msg.Tool))
				if msg.Detail != "" {
					detail := msg.Detail
					if len(detail) > 60 {
						detail = detail[:57] + "..."
					}
					b.WriteString(styles.Muted.Render(" → " + detail))
				}
			} else {
				b.WriteString(styles.AccentText.Render(focusPrefix + "⚡ " + msg.Tool))
				if msg.Detail != "" {
					detail := msg.Detail
					if len(detail) > 60 {
						detail = detail[:57] + "..."
					}
					b.WriteString(styles.Muted.Render(" " + detail))
				}
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
		b.WriteString(styles.StatusWarn.Render(focusPrefix + "🔒 " + msg.Detail))
		b.WriteString("\n")

	case chatDone:
		detail := msg.Detail
		if len(detail) > 80 {
			detail = detail[:77] + "..."
		}
		b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + detail))
		b.WriteString("\n")

	case chatError:
		errText := msg.Text
		if errText == "" && msg.Error != nil {
			errText = msg.Error.Error()
		}
		b.WriteString(renderError(errText, styles))
		b.WriteString("\n")

	case chatThinking:
		b.WriteString(renderSpinner("thinking...", styles))
		b.WriteString("\n")

	case chatSystem:
		b.WriteString(styles.StatusWarn.Render(focusPrefix + "⚠ " + msg.Text))
		b.WriteString("\n")

	case chatAgent:
		b.WriteString(styles.Muted.Render(focusPrefix + "⟳ " + msg.Text))
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

func (m *Model) setStreaming(streaming bool) {
	m.Footer.Streaming = streaming
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
