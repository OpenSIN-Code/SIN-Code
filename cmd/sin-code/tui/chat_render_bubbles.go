// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten
package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

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

	if strings.TrimSpace(rendered) == "" {
		if streaming {
			rendered = styles.Muted.Render("Thinking…")
		} else {
			return headerLine + "\n"
		}
	}

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

	category := "Error"
	hint := "Press Esc to interrupt, then retry"
	if strings.Contains(errText, "context deadline exceeded") || strings.Contains(errText, "timeout") {
		category = "Timeout"
		hint = "The request took too long. Try again or use a faster model."
	} else if strings.Contains(errText, "connection refused") || strings.Contains(errText, "no such host") {
		category = "Network Error"
		hint = "Check your internet connection and API endpoint."
	} else if strings.Contains(errText, "unauthorized") || strings.Contains(errText, "401") || strings.Contains(errText, "api key") {
		category = "Auth Error"
		hint = "Check your API key with: sin-code config get llm.api_key"
	} else if strings.Contains(errText, "rate limit") || strings.Contains(errText, "429") {
		category = "Rate Limited"
		hint = "Too many requests. Wait a moment and try again."
	} else if strings.Contains(errText, "permission denied") {
		category = "Permission Denied"
		hint = "This action was blocked by the permission engine. Use --yolo to allow."
	}

	catStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Error)).
		Bold(true)

	bodyStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		BorderLeft(true).
		BorderLeftForeground(c(styles.Theme.Error)).
		Padding(0, 1).
		Width(bodyWidth)

	hintStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Faint(true)

	if !msg.Expanded {
		short := truncateString(errText, bodyWidth-4)
		return catStyle.Render("❌ "+category+": ") + styles.StatusErr.Render(short) + "\n"
	}

	var b strings.Builder
	b.WriteString(catStyle.Render("❌ " + category))
	b.WriteString("\n")
	b.WriteString(bodyStyle.Render(errText))
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("  → " + hint))
	b.WriteString("\n")
	return b.String()
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
			if lipgloss.Width(inputText) > bodyWidth-10 {
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
