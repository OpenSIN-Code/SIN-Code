// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

var diffExpanded bool
var diffExpandedMu sync.Mutex

func ToggleDiffExpanded() {
	diffExpandedMu.Lock()
	defer diffExpandedMu.Unlock()
	diffExpanded = !diffExpanded
}

func DiffExpanded() bool {
	diffExpandedMu.Lock()
	defer diffExpandedMu.Unlock()
	return diffExpanded
}

type diffLine struct {
	prefix   string
	content  string
	lineType int
}

func computeUnifiedDiff(before, after string) []diffLine {
	if before == "" {
		afterLines := strings.Split(after, "\n")
		result := make([]diffLine, 0, len(afterLines))
		for _, line := range afterLines {
			result = append(result, diffLine{prefix: "+", content: line, lineType: 1})
		}
		return result
	}

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	minLen := len(beforeLines)
	if len(afterLines) < minLen {
		minLen = len(afterLines)
	}

	prefix := 0
	for prefix < minLen && beforeLines[prefix] == afterLines[prefix] {
		prefix++
	}

	suffix := 0
	for suffix < minLen-prefix &&
		beforeLines[len(beforeLines)-1-suffix] == afterLines[len(afterLines)-1-suffix] {
		suffix++
	}

	var result []diffLine

	for i := 0; i < prefix; i++ {
		result = append(result, diffLine{prefix: " ", content: beforeLines[i], lineType: 0})
	}

	for i := prefix; i < len(beforeLines)-suffix; i++ {
		result = append(result, diffLine{prefix: "-", content: beforeLines[i], lineType: 2})
	}

	for i := prefix; i < len(afterLines)-suffix; i++ {
		result = append(result, diffLine{prefix: "+", content: afterLines[i], lineType: 1})
	}

	for i := len(beforeLines) - suffix; i < len(beforeLines); i++ {
		result = append(result, diffLine{prefix: " ", content: beforeLines[i], lineType: 0})
	}

	return result
}

func RenderSingleDiff(entry DiffEntry, styles Styles, width int) string {
	var b strings.Builder

	b.WriteString(styles.AccentText.Render(entry.Path))
	b.WriteString("\n")

	badge := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Background)).
		Background(c(styles.Theme.Accent)).
		Bold(true).
		Padding(0, 1).
		Render(entry.Tool)

	ts := styles.Muted.Render(entry.Timestamp.Format("15:04:05"))
	b.WriteString(fmt.Sprintf("%s  %s", badge, ts))
	b.WriteString("\n")

	diffLines := computeUnifiedDiff(entry.Before, entry.After)

	added := 0
	removed := 0
	for _, dl := range diffLines {
		if dl.lineType == 1 {
			added++
		} else if dl.lineType == 2 {
			removed++
		}
	}

	maxWidth := width - 6
	if maxWidth < 20 {
		maxWidth = 20
	}

	if !DiffExpanded() && len(diffLines) > 10 {
		b.WriteString(styles.Muted.Render(
			fmt.Sprintf("  +%d -%d  (%d lines, press 'd' to expand)", added, removed, len(diffLines))))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(styles.Muted.Render(
		fmt.Sprintf("  +%d -%d", added, removed)))
	b.WriteString("\n")

	for _, dl := range diffLines {
		line := fmt.Sprintf("%s %s", dl.prefix, dl.content)
		if len(line) > maxWidth {
			line = line[:maxWidth-3] + "..."
		}
		switch dl.lineType {
		case 1:
			b.WriteString(styles.StatusOK.Render(line))
		case 2:
			b.WriteString(styles.StatusErr.Render(line))
		default:
			b.WriteString(styles.Muted.Render(line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func RenderDiffView(diffs []DiffEntry, styles Styles, width, height int) string {
	if len(diffs) == 0 {
		return styles.Muted.Render("  No file changes recorded yet.")
	}

	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" File Changes"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n")

	innerWidth := width - 6
	if innerWidth < 30 {
		innerWidth = 30
	}

	maxEntries := height - 5
	if maxEntries < 1 {
		maxEntries = 1
	}

	start := 0
	if len(diffs) > maxEntries {
		start = len(diffs) - maxEntries
	}

	for i := start; i < len(diffs); i++ {
		b.WriteString(RenderSingleDiff(diffs[i], styles, innerWidth))
		if i < len(diffs)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func RenderDiffPopup(diffs []DiffEntry, styles Styles, width, height int) string {
	popupWidth := width * 3 / 4
	if popupWidth < 50 {
		popupWidth = 50
	}
	if popupWidth > 100 {
		popupWidth = 100
	}

	popupHeight := height * 3 / 4
	if popupHeight < 10 {
		popupHeight = 10
	}

	content := RenderDiffView(diffs, styles, popupWidth, popupHeight)

	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c(styles.Theme.Accent)).
		Foreground(c(styles.Theme.Text)).
		Background(c(styles.Theme.Background)).
		Padding(1, 2).
		Width(popupWidth)

	rendered := popupStyle.Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, rendered)
}

func RenderDiffInline(diffs []DiffEntry, styles Styles, width int) string {
	if len(diffs) == 0 {
		return ""
	}

	var b strings.Builder

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(c(styles.Theme.Accent)).
		Padding(0, 1)

	innerWidth := width - 4
	if innerWidth < 30 {
		innerWidth = 30
	}

	b.WriteString(styles.AccentText.Render(" Recent File Changes"))
	b.WriteString("\n")

	for i, entry := range diffs {
		b.WriteString(RenderSingleDiff(entry, styles, innerWidth))
		if i < len(diffs)-1 {
			b.WriteString(styles.Muted.Render("  " + strings.Repeat("·", max(innerWidth-4, 10))))
			b.WriteString("\n")
		}
	}

	return panelStyle.Render(b.String())
}
