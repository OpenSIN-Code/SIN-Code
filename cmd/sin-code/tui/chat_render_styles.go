// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten
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

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatDivider(styles Styles, width int) string {
	return styles.Muted.Render(strings.Repeat("─", width))
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatHeader(title string, styles Styles) string {
	return styles.ContentHdr.Render("💬 " + title)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatStatus(status string, styles Styles) string {
	return styles.Muted.Render(status)
}
