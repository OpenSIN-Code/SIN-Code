// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
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
	timestamp lipgloss.Style
}

func newChatBubbleStyles(styles Styles) chatBubbleStyles {
	t := styles.Theme
	return chatBubbleStyles{
	user: lipgloss.NewStyle().
		Foreground(c(t.Background)).
		Background(c(t.Accent)).
		Bold(true).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()),

		assistant: lipgloss.NewStyle().
			Foreground(c(t.Text)).
			PaddingLeft(2).
			PaddingRight(2),

		tool: lipgloss.NewStyle().
			Foreground(c(t.AccentDim)).
			PaddingLeft(2).
			PaddingRight(2),

		system: lipgloss.NewStyle().
			Foreground(c(t.TextDim)).
			Italic(true).
			Padding(0, 2),

		error: lipgloss.NewStyle().
			Foreground(c(t.Error)).
			BorderLeft(true).
			BorderLeftForeground(c(t.Error)).
			Background(lipgloss.Color(t.Background)).
			Padding(0, 1).
			Bold(true),

		success: lipgloss.NewStyle().
			Foreground(c(t.Success)).
			Bold(true).
			Padding(0, 2),

		warning: lipgloss.NewStyle().
			Foreground(c(t.Warn)).
			Bold(true).
			Padding(0, 2),

		timestamp: lipgloss.NewStyle().
			Foreground(c(t.TextDim)).
			Faint(true),
	}
}

func renderChatBubble(role, content string, styles chatBubbleStyles, width int) string {
	var b strings.Builder

	switch role {
	case "user":
		label := styles.user.Render(" You ")
		ts := styles.timestamp.Render(time.Now().Format("15:04:05"))
		b.WriteString(label)
		b.WriteString("  ")
		b.WriteString(ts)
		b.WriteString("\n")
		wrapped := wrapText(content, width-6)
		b.WriteString(wrapped)
	case "assistant":
		label := styles.assistant.Render("Assistant")
		ts := styles.timestamp.Render(time.Now().Format("15:04:05"))
		b.WriteString(label)
		b.WriteString("  ")
		b.WriteString(ts)
		b.WriteString("\n")
		wrapped := wrapText(content, width-6)
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

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05")
}

func renderUserBubble(msg ChatMessage, styles Styles, width int) string {
	var b strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Background)).
		Background(c(styles.Theme.Accent)).
		Bold(true).
		Padding(0, 1)

	tsStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Faint(true)

	label := labelStyle.Render("You")
	ts := tsStyle.Render(formatTimestamp(msg.Timestamp))

	prefixWidth := lipgloss.Width(label + "  " + ts)
	_ = prefixWidth
	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}
	wrapped := wrapText(msg.Text, bodyWidth)

	bodyStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		Background(lipgloss.Color(styles.Theme.Background)).
		Padding(0, 1).
		Width(bodyWidth)

	body := bodyStyle.Render(wrapped)

	rightAligned := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Right)

	content := label + "  " + ts + "\n" + body
	b.WriteString(rightAligned.Render(content))
	b.WriteString("\n")
	return b.String()
}

func renderAssistantBubble(msg ChatMessage, highlighter *SyntaxHighlighter, styles Styles, width int, streaming bool, spinner Spinner) string {
	var b strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Accent)).
		Bold(true)

	tsStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Faint(true)

	label := labelStyle.Render("Assistant")
	ts := tsStyle.Render(formatTimestamp(msg.Timestamp))

	headerLine := label + "  " + ts

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	rendered := renderMarkdownWithCodeBlocks(msg.Text, highlighter, styles, bodyWidth)

	if streaming {
		cursor := renderStreamingCursor(spinner, styles)
		rendered = strings.TrimRight(rendered, "\n") + cursor
	}

	bodyStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		PaddingLeft(2).
		PaddingRight(2).
		Width(bodyWidth)

	body := bodyStyle.Render(rendered)

	b.WriteString(headerLine)
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}

func renderSystemBubble(msg ChatMessage, styles Styles, width int) string {
	text := msg.Text
	if text == "" {
		text = msg.Detail
	}

	style := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Italic(true).
		Align(lipgloss.Center).
		Width(width)

	return style.Render(text) + "\n"
}

func renderErrorBubble(msg ChatMessage, styles Styles, width int) string {
	errText := msg.Text
	if errText == "" && msg.Error != nil {
		errText = msg.Error.Error()
	}

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	style := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Error)).
		BorderLeft(true).
		BorderLeftForeground(c(styles.Theme.Error)).
		Background(lipgloss.Color(fmt.Sprintf("%s", styles.Theme.Background))).
		Padding(0, 1).
		Width(bodyWidth)

	if !msg.Expanded {
		short := truncateString(errText, bodyWidth-4)
		return style.Render("❌ " + short) + "\n"
	}

	body := style.Render(errText) + "\n" + styles.Muted.Render("  Press enter to collapse")
	return body + "\n"
}

func renderToolCard(msg ChatMessage, styles Styles, width int, focused bool) string {
	focusPrefix := ""
	if focused {
		focusPrefix = "▸ "
	}

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	if msg.Expanded {
		var b strings.Builder

		iconStyle := lipgloss.NewStyle().
			Foreground(c(styles.Theme.Accent)).
			Bold(true)

		hdr := iconStyle.Render(focusPrefix + "⚡ " + msg.Tool)
		b.WriteString(hdr)
		b.WriteString("\n")

		if msg.ToolInput != "" {
			inputText := msg.ToolInput
			if len(inputText) > bodyWidth-10 {
				inputText = truncateString(inputText, bodyWidth-13)
			}
			b.WriteString(styles.Muted.Render("  in: "))
			b.WriteString(styles.Muted.Render(inputText))
			b.WriteString("\n")
		}

		output := msg.ToolOutput
		if output == "" {
			output = msg.Detail
		}
		if output != "" {
			rendered := renderToolOutput(output, styles, bodyWidth)
			b.WriteString(rendered)
		}

		cardStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(c(styles.Theme.AccentDim)).
			Padding(0, 1).
			Width(bodyWidth)

		return cardStyle.Render(b.String()) + "\n"
	}

	var b strings.Builder
	if msg.Result {
		b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + msg.Tool))
	} else {
		b.WriteString(styles.AccentText.Render(focusPrefix + "⚡ " + msg.Tool))
	}
	if msg.Detail != "" {
		detail := msg.Detail
		if len(detail) > 60 {
			detail = detail[:57] + "..."
		}
		b.WriteString(styles.Muted.Render(" → " + detail))
	}
	b.WriteString("\n")
	return b.String()
}

func renderStreamingCursor(spinner Spinner, styles Styles) string {
	visible := spinner.pulse%2 == 0
	if visible {
		return styles.AccentText.Render("▋")
	}
	return " "
}

func renderTypingDots(spinner Spinner, styles Styles) string {
	phase := spinner.frame % 3
	switch phase {
	case 0:
		return styles.Muted.Render("·  ")
	case 1:
		return styles.Muted.Render("·· ")
	default:
		return styles.Muted.Render("···")
	}
}
