// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten
package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

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

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderError(err string, styles Styles) string {
	return styles.StatusErr.Render("❌ " + err)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderErrorExpanded(msg ChatMessage, styles Styles, width int) string {
	return renderErrorBubble(msg, styles, width)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderSpinner(text string, styles Styles) string {
	return styles.AccentText.Render("⏳ " + text)
}

func renderToolOutput(output string, styles Styles, width int) string {
	if output == "" {
		return ""
	}
	if width < 10 {
		width = 10
	}

	highlighter := NewSyntaxHighlighter(styles.Theme)

	if looksLikeGoCode(output) {
		return renderCodeBlock(output, "go", highlighter, styles, width, false) + "\n"
	}

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c(styles.Theme.Accent)).
		Padding(0, 1).
		Width(width - 2)

	return panelStyle.Render(styles.Content.Render(output))
}
