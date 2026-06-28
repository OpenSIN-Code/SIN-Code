// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// CopyMode lets the user navigate and select chat text for yanking
// to the system clipboard. Inspired by vim's visual mode + tmux copy mode.
type CopyMode struct {
	Active     bool
	Lines      []string // flattened chat lines available for selection
	CursorLine int      // current cursor position
	SelStart   int      // selection start line (-1 = no selection)
	SelEnd     int      // selection end line
	VisualMode bool     // true when selecting a range
	Styles     Styles
}

// NewCopyMode creates a CopyMode with the given styles.
func NewCopyMode(styles Styles) *CopyMode {
	return &CopyMode{
		SelStart: -1,
		Styles:   styles,
	}
}

// Enter activates copy mode with the given chat lines.
func (c *CopyMode) Enter(lines []string) {
	c.Active = true
	c.Lines = lines
	c.CursorLine = 0
	c.SelStart = -1
	c.SelEnd = 0
	c.VisualMode = false
}

// Exit deactivates copy mode and clears state.
func (c *CopyMode) Exit() {
	c.Active = false
	c.Lines = nil
	c.CursorLine = 0
	c.SelStart = -1
	c.SelEnd = 0
	c.VisualMode = false
}

func (c *CopyMode) clampCursor() {
	max := len(c.Lines) - 1
	if max < 0 {
		max = 0
	}
	if c.CursorLine < 0 {
		c.CursorLine = 0
	}
	if c.CursorLine > max {
		c.CursorLine = max
	}
}

// Up moves the cursor up one line.
func (c *CopyMode) Up() {
	c.CursorLine--
	c.clampCursor()
	if c.VisualMode {
		c.SelEnd = c.CursorLine
		c.normalizeSelection()
	}
}

// Down moves the cursor down one line.
func (c *CopyMode) Down() {
	c.CursorLine++
	c.clampCursor()
	if c.VisualMode {
		c.SelEnd = c.CursorLine
		c.normalizeSelection()
	}
}

// PageUp moves the cursor up by half the visible height.
func (c *CopyMode) PageUp() {
	pageSize := len(c.Lines) / 2
	if pageSize < 1 {
		pageSize = 1
	}
	c.CursorLine -= pageSize
	c.clampCursor()
	if c.VisualMode {
		c.SelEnd = c.CursorLine
		c.normalizeSelection()
	}
}

// PageDown moves the cursor down by half the visible height.
func (c *CopyMode) PageDown() {
	pageSize := len(c.Lines) / 2
	if pageSize < 1 {
		pageSize = 1
	}
	c.CursorLine += pageSize
	c.clampCursor()
	if c.VisualMode {
		c.SelEnd = c.CursorLine
		c.normalizeSelection()
	}
}

// Top moves the cursor to the first line.
func (c *CopyMode) Top() {
	c.CursorLine = 0
	if c.VisualMode {
		c.SelEnd = c.CursorLine
		c.normalizeSelection()
	}
}

// Bottom moves the cursor to the last line.
func (c *CopyMode) Bottom() {
	c.CursorLine = len(c.Lines) - 1
	if c.CursorLine < 0 {
		c.CursorLine = 0
	}
	if c.VisualMode {
		c.SelEnd = c.CursorLine
		c.normalizeSelection()
	}
}

// ToggleVisual starts or stops visual selection. When starting, the current
// cursor line becomes both the start and end of the selection.
func (c *CopyMode) ToggleVisual() {
	if c.VisualMode {
		c.VisualMode = false
		c.SelStart = -1
		return
	}
	c.VisualMode = true
	c.SelStart = c.CursorLine
	c.SelEnd = c.CursorLine
}

// normalizeSelection ensures SelStart <= SelEnd.
func (c *CopyMode) normalizeSelection() {
	if c.SelStart > c.SelEnd {
		c.SelStart, c.SelEnd = c.SelEnd, c.SelStart
	}
}

// selectedText returns the lines within the current selection (if any).
func (c *CopyMode) selectedText() string {
	if !c.VisualMode || c.SelStart < 0 {
		if c.CursorLine >= 0 && c.CursorLine < len(c.Lines) {
			return c.Lines[c.CursorLine]
		}
		return ""
	}
	start := c.SelStart
	end := c.SelEnd
	c.normalizeSelection()
	c.SelStart = start
	c.SelEnd = end
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end >= len(c.Lines) {
		end = len(c.Lines) - 1
	}
	if start > end || start >= len(c.Lines) {
		return ""
	}
	return strings.Join(c.Lines[start:end+1], "\n")
}

// Yank returns the selected text and copies it to the system clipboard.
// After yanking, visual mode is cleared but copy mode stays active.
func (c *CopyMode) Yank() string {
	text := c.selectedText()
	if text != "" {
		_ = CopyToClipboard(text)
	}
	c.VisualMode = false
	c.SelStart = -1
	return text
}

// YankAll returns the entire chat text and copies it to the clipboard.
func (c *CopyMode) YankAll() string {
	text := strings.Join(c.Lines, "\n")
	if text != "" {
		_ = CopyToClipboard(text)
	}
	return text
}

// Render renders the copy mode overlay with numbered lines, cursor highlight,
// visual selection highlight, and a footer help bar.
func (cm *CopyMode) Render(width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	t := cm.Styles.Theme
	lineNumStyle := lipgloss.NewStyle().Foreground(c(t.TextDim))
	cursorStyle := lipgloss.NewStyle().
		Foreground(c(t.Background)).
		Background(c(t.Accent)).
		Bold(true)
	selStyle := lipgloss.NewStyle().
		Foreground(c(t.Background)).
		Background(c(t.Success))
	footerStyle := lipgloss.NewStyle().
		Foreground(c(t.Accent)).
		Bold(true)

	cm.normalizeSelection()

	maxLines := height - 2 // reserve 2 lines for header + footer
	startLine := 0
	if len(cm.Lines) > maxLines {
		if cm.CursorLine > maxLines-1 {
			startLine = cm.CursorLine - maxLines + 1
		}
		if startLine+maxLines > len(cm.Lines) {
			startLine = len(cm.Lines) - maxLines
		}
	}
	if startLine < 0 {
		startLine = 0
	}

	var b strings.Builder
	b.WriteString(footerStyle.Render("[COPY MODE]"))
	b.WriteString("\n")

	endLine := startLine + maxLines
	if endLine > len(cm.Lines) {
		endLine = len(cm.Lines)
	}

	for i := startLine; i < endLine; i++ {
		numStr := fmt.Sprintf("%4d ", i+1)
		line := cm.Lines[i]

		displayNum := lineNumStyle.Render(numStr)
		displayLine := line

		if cm.VisualMode && i >= cm.SelStart && i <= cm.SelEnd {
			displayLine = selStyle.Render(line)
		} else if i == cm.CursorLine {
			displayLine = cursorStyle.Render(line)
		}

		// Truncate long lines to fit width
		maxLineWidth := width - len(numStr) - 1
		if maxLineWidth > 0 && len([]rune(line)) > maxLineWidth {
			runes := []rune(line)
			line = string(runes[:maxLineWidth])
			if cm.VisualMode && i >= cm.SelStart && i <= cm.SelEnd {
				displayLine = selStyle.Render(line)
			} else if i == cm.CursorLine {
				displayLine = cursorStyle.Render(line)
			}
		}

		b.WriteString(displayNum)
		b.WriteString(displayLine)
		b.WriteString("\n")
	}

	b.WriteString(footerStyle.Render("[COPY] j/k move · v visual · y yank · q quit"))
	return b.String()
}
