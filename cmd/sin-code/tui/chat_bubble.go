// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

type chatBubbleStyles struct {
	user      lipgloss.Style
	assistant lipgloss.Style
	tool      lipgloss.Style
	system    lipgloss.Style
	error     lipgloss.Style
	success   lipgloss.Style
	warning   lipgloss.Style
}

func newChatBubbleStyles(styles Styles) chatBubbleStyles {
	return chatBubbleStyles{
		user: lipgloss.NewStyle().
			Foreground(lipgloss.Color(styles.Theme.Accent)).
			Bold(true).
			PaddingLeft(2).
			PaddingRight(2),

		assistant: lipgloss.NewStyle().
			Foreground(lipgloss.Color(styles.Theme.Text)).
			PaddingLeft(2).
			PaddingRight(2),

		tool: lipgloss.NewStyle().
			Foreground(lipgloss.Color(styles.Theme.AccentDim)).
			PaddingLeft(2).
			PaddingRight(2),

		system: lipgloss.NewStyle().
			Foreground(lipgloss.Color(styles.Theme.TextDim)).
			Italic(true).
			PaddingLeft(2).
			PaddingRight(2),

		error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(styles.Theme.Error)).
			Bold(true).
			PaddingLeft(2).
			PaddingRight(2),

		success: lipgloss.NewStyle().
			Foreground(lipgloss.Color(styles.Theme.Success)).
			Bold(true).
			PaddingLeft(2).
			PaddingRight(2),

		warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color(styles.Theme.Warn)).
			Bold(true).
			PaddingLeft(2).
			PaddingRight(2),
	}
}

func renderChatBubble(role, content string, styles chatBubbleStyles, width int) string {
	var b strings.Builder

	switch role {
	case "user":
		b.WriteString(styles.user.Render("❯ You"))
		b.WriteString("\n")
		wrapped := wrapText(content, width-4)
		b.WriteString(wrapped)
	case "assistant":
		b.WriteString(styles.assistant.Render("✓ Assistant"))
		b.WriteString("\n")
		wrapped := wrapText(content, width-4)
		b.WriteString(wrapped)
	case "tool":
		b.WriteString(styles.tool.Render("⚡ " + content))
	case "system":
		b.WriteString(styles.system.Render("⚠ " + content))
	case "error":
		b.WriteString(styles.error.Render("❌ " + content))
	case "success":
		b.WriteString(styles.success.Render("✅ " + content))
	case "warning":
		b.WriteString(styles.warning.Render("⚠ " + content))
	default:
		b.WriteString(content)
	}

	return b.String()
}

func wrapText(text string, width int) string {
	if width <= 0 {
		width = 80
	}

	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if len(paragraph) <= width {
			lines = append(lines, paragraph)
			continue
		}

		words := strings.Fields(paragraph)
		var currentLine strings.Builder
		for _, word := range words {
			if currentLine.Len()+len(word)+1 > width {
				if currentLine.Len() > 0 {
					lines = append(lines, currentLine.String())
					currentLine.Reset()
				}
			}
			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
		}
		if currentLine.Len() > 0 {
			lines = append(lines, currentLine.String())
		}
	}

	return strings.Join(lines, "\n")
}

func indentText(text string, indent int) string {
	prefix := strings.Repeat(" ", indent)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func renderChatDivider(styles Styles, width int) string {
	return styles.Muted.Render(strings.Repeat("─", width))
}

func renderChatHeader(title string, styles Styles) string {
	return styles.ContentHdr.Render("💬 " + title)
}

func renderChatStatus(status string, styles Styles) string {
	return styles.Muted.Render(status)
}

type chatMsgKind int

const (
	chatUser chatMsgKind = iota
	chatAssistant
	chatAgent
	chatTool
	chatVerify
	chatAsk
	chatDone
	chatError
	chatThinking
	chatSystem
)

type ChatMessage struct {
	ID         int64
	Kind       chatMsgKind
	Text       string
	Tool       string
	ToolInput  string
	ToolOutput string
	Detail     string
	Result     bool
	Timestamp  time.Time
	Tokens     int
	Error      error
	Expanded   bool
}
