package tui

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
)

type markdownRenderer struct {
	styles Styles
}

func newMarkdownRenderer(styles Styles) *markdownRenderer {
	return &markdownRenderer{
		styles: styles,
	}
}

func (m *markdownRenderer) render(text string) string {
	if text == "" {
		return ""
	}

	width := 80
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return strings.TrimRight(text, "\n") + "\n"
	}

	rendered, err := r.Render(text)
	if err != nil {
		return strings.TrimRight(text, "\n") + "\n"
	}

	// Glamour adds leading/trailing newlines — trim them for compact chat display
	rendered = strings.TrimSpace(rendered)
	return rendered + "\n"
}

func (m *markdownRenderer) renderToolCall(toolName, args string) string {
	var b strings.Builder
	b.WriteString(m.styles.AccentText.Render("⚡ "))
	b.WriteString(m.styles.Bold.Render(toolName))
	if args != "" {
		b.WriteString(m.styles.Muted.Render(" " + args))
	}
	return b.String()
}

func (m *markdownRenderer) renderVerification(status, message string) string {
	var b strings.Builder

	switch status {
	case "pass":
		b.WriteString(m.styles.StatusOK.Render("✅ "))
		b.WriteString(m.styles.StatusOK.Render(message))
	case "fail":
		b.WriteString(m.styles.StatusErr.Render("❌ "))
		b.WriteString(m.styles.StatusErr.Render(message))
	default:
		b.WriteString(m.styles.StatusWarn.Render("⏳ "))
		b.WriteString(m.styles.StatusWarn.Render(message))
	}

	return b.String()
}

func (m *markdownRenderer) renderError(err string) string {
	return m.styles.StatusErr.Render("❌ " + err)
}

func (m *markdownRenderer) renderSpinner(text string) string {
	return m.styles.AccentText.Render("⏳ " + text)
}

func renderMarkdown(text string, styles Styles) string {
	r := newMarkdownRenderer(styles)
	return r.render(text)
}

func renderMarkdownWithWidth(text string, width int, styles Styles) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return strings.TrimRight(text, "\n") + "\n"
	}

	rendered, err := r.Render(text)
	if err != nil {
		return strings.TrimRight(text, "\n") + "\n"
	}

	return strings.TrimSpace(rendered) + "\n"
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func formatTokens(tokens int) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1000000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(tokens)/1000000)
}

func formatCost(cost float64) string {
	if cost < 0.01 {
		return "$0.00"
	}
	if cost < 1.0 {
		return fmt.Sprintf("$%.2f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}
