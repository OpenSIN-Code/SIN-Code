package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func RenderLayoutDebug(tabs Tabs, sidebar Sidebar, view ViewKind, content string, right string, footer Footer, styles Styles, width, height int) string {
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}

	header := tabs.View(styles)
	footerView := footer.Render(styles)
	contentHeight := height - 4

	leftWidth := 0
	if view != ViewChat && !sidebar.Collapsed {
		leftWidth = sidebar.Width
	}
	rightWidth := 0
	if view != ViewChat && right != "" {
		rightWidth = max(28, width/4)
	}

	centerWidth := width - leftWidth - rightWidth
	if centerWidth < 20 {
		centerWidth = 20
		rightWidth = width - leftWidth - centerWidth
		if rightWidth < 0 {
			rightWidth = 0
		}
	}

	debugStyle := func(label string, w, h int, color string) lipgloss.Style {
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(color)).
			Foreground(lipgloss.Color(color)).
			Padding(0, 0).
			Width(w).
			Height(h)
	}

	var b strings.Builder

	hdrLines := strings.Split(header, "\n")
	hdrBox := debugStyle("header", width, len(hdrLines), styles.Theme.Accent).
		Render(header + fmt.Sprintf(" [header w=%d h=%d]", width, len(hdrLines)))
	b.WriteString(hdrBox)
	b.WriteString("\n")

	contentH := contentHeight
	if leftWidth > 0 {
		leftContent := splitLines(sidebar.View(styles), leftWidth, contentH)
		leftBox := debugStyle("sidebar", leftWidth, contentH, styles.Theme.Warn).
			Render(leftContent + fmt.Sprintf("\n[sidebar w=%d h=%d]", leftWidth, contentH))
		b.WriteString(leftBox)
		b.WriteString(" ")
	}

	centerContent := padContentExact(content, centerWidth, contentH)
	centerBox := debugStyle("content", centerWidth, contentH, styles.Theme.Success).
		Render(centerContent + fmt.Sprintf("\n[content w=%d h=%d]", centerWidth, contentH))
	b.WriteString(centerBox)
	b.WriteString("\n")

	if rightWidth > 0 {
		rightContent := padContentExact(right, rightWidth, contentH)
		rightBox := debugStyle("right", rightWidth, contentH, styles.Theme.AccentDim).
			Render(rightContent + fmt.Sprintf("\n[right w=%d h=%d]", rightWidth, contentH))
		b.WriteString(" ")
		b.WriteString(rightBox)
	}

	footerLines := strings.Split(footerView, "\n")
	ftrBox := debugStyle("footer", width, len(footerLines), styles.Theme.Error).
		Render(footerView + fmt.Sprintf(" [footer w=%d h=%d]", width, len(footerLines)))
	b.WriteString(ftrBox)
	b.WriteString("\n")

	return b.String()
}

func padContentExact(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for i := 0; i < height; i++ {
		var line string
		if i < len(lines) {
			line = lines[i]
		}
		b.WriteString(padRight(line, width))
		if i < height-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func RenderInlineDiff(diffs []DiffEntry, styles Styles, width int) string {
	if len(diffs) == 0 {
		return ""
	}
	return RenderDiffInline(diffs, styles, width)
}

func (m *Model) ToggleDebugLayout() {
	m.DebugLayout = !m.DebugLayout
}

func (m *Model) ToggleInlineDiff() {
	m.InlineDiffOpen = !m.InlineDiffOpen
}
