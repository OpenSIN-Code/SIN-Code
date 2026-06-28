// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// ChatBlock represents a collapsible message block in the chat view.
// Inspired by Codex CLI's block-based rendering: each message is a
// self-contained unit with a header bar and collapsible body.
type ChatBlock struct {
	Role         string    // "user", "assistant", "tool", "system", "error"
	Model        string    // model name for assistant messages
	Timestamp    time.Time
	Content      string // raw markdown content
	Collapsed    bool
	ToolCalls    int    // number of tool calls in this block
	VerifyResult string // "pass", "fail", "" if not a verify message
	Width        int
}

// ChatBlockList is a slice of ChatBlock with a batch renderer.
type ChatBlockList []ChatBlock

// roleIcons maps each role to its header icon.
var roleIcons = map[string]string{
	"user":      "\u25b6", // ▶
	"assistant": "\u2726", // ✦
	"tool":      "\u2699", // ⚙
	"system":    "\u2139", // ℹ
	"error":     "\u2717", // ✗
}

// roleBorderColors returns the theme color string for a role's border.
func roleBorderColors(theme Theme, role, verifyResult string) (border, left string) {
	switch {
	case verifyResult == "pass":
		return theme.Success, theme.Success
	case verifyResult == "fail":
		return theme.Error, theme.Error
	case role == "error":
		return theme.Error, theme.Error
	case role == "user":
		return theme.Accent, theme.Accent
	case role == "assistant":
		return theme.Success, theme.AccentDim
	case role == "tool":
		return theme.TextDim, theme.TextDim
	case role == "system":
		return theme.TextDim, theme.TextDim
	default:
		return theme.Border, theme.Border
	}
}

// renderHeader builds the 1-line header bar for a chat block.
func renderHeader(b ChatBlock, styles Styles, width int) string {
	icon := roleIcons[b.Role]
	if icon == "" {
		icon = "\u2022" // •
	}

	borderColor, leftColor := roleBorderColors(styles.Theme, b.Role, b.VerifyResult)
	_ = borderColor

	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(leftColor)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.Theme.TextDim))

	var parts []string
	parts = append(parts, iconStyle.Render(icon+" "+b.Role))

	if b.Model != "" {
		parts = append(parts, dimStyle.Render(b.Model))
	}

	if b.ToolCalls > 0 {
		toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.Theme.AccentDim))
		parts = append(parts, toolStyle.Render("\u2699 "+itoa(b.ToolCalls)+" tool calls"))
	}

	if b.VerifyResult == "pass" {
		parts = append(parts, styles.StatusOK.Render("\u2713"))
	} else if b.VerifyResult == "fail" {
		parts = append(parts, styles.StatusErr.Render("\u2717"))
	}

	ts := formatTimestamp(b.Timestamp)
	if ts != "" {
		parts = append(parts, dimStyle.Render(ts))
	}

	collapseIndicator := "\u25bc" // ▼
	if b.Collapsed {
		collapseIndicator = "\u25b6" // ▶
	}
	parts = append(parts, dimStyle.Render(collapseIndicator))

	header := strings.Join(parts, " \u00b7 ")

	// Right-align the collapse indicator by padding
	headerWidth := lipgloss.Width(header)
	if headerWidth < width {
		pad := width - headerWidth
		// Insert padding before the collapse indicator (last part)
		last := parts[len(parts)-1]
		rest := strings.Join(parts[:len(parts)-1], " \u00b7 ")
		header = rest + strings.Repeat(" ", pad) + " " + last
	}

	return header
}

// RenderBlockHeader renders only the header line of a chat block, without
// the body or border. Used when the body is rendered by a separate function.
func RenderBlockHeader(b ChatBlock, styles Styles, width int) string {
	if width < 10 {
		width = 10
	}
	if b.Width > 0 {
		width = b.Width
	}
	return renderHeader(b, styles, width)
}

// RenderBlock renders a single chat block with header + optional body.
func RenderBlock(b ChatBlock, styles Styles, width int) string {
	if width < 10 {
		width = 10
	}
	if b.Width > 0 {
		width = b.Width
	}

	borderColor, leftColor := roleBorderColors(styles.Theme, b.Role, b.VerifyResult)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		BorderLeftForeground(lipgloss.Color(leftColor)).
		Padding(0, 1).
		Width(width)

	header := renderHeader(b, styles, width)

	if b.Collapsed {
		return borderStyle.Render(header)
	}

	body := ""
	if b.Content != "" {
		bodyWidth := width - 4 // border + padding
		if bodyWidth < 10 {
			bodyWidth = 10
		}
		rendered := renderMarkdownWithWidth(b.Content, bodyWidth, styles)
		body = strings.TrimRight(rendered, "\n")
	}

	if body != "" {
		return borderStyle.Render(header + "\n" + body)
	}
	return borderStyle.Render(header)
}

// Render renders a ChatBlockList as a single string with blocks separated by newlines.
func (blocks ChatBlockList) Render(styles Styles, width int) string {
	if len(blocks) == 0 {
		return ""
	}
	var b strings.Builder
	for i, blk := range blocks {
		b.WriteString(RenderBlock(blk, styles, width))
		if i < len(blocks)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// ToggleBlockCollapse toggles the collapse state of the chat block at idx.
// It finds the last assistant message in ChatHistory and toggles its
// associated block. If no blocks are stored, it falls back to toggling
// the Expanded field on the ChatMessage at idx.
func (m *Model) ToggleBlockCollapse(idx int) {
	if idx < 0 || idx >= len(m.ChatHistory) {
		// Find the last assistant block
		for i := len(m.ChatHistory) - 1; i >= 0; i-- {
			if m.ChatHistory[i].Kind == chatAssistant {
				m.ChatHistory[i].Expanded = !m.ChatHistory[i].Expanded
				return
			}
		}
		return
	}
	m.ChatHistory[idx].Expanded = !m.ChatHistory[idx].Expanded
}
