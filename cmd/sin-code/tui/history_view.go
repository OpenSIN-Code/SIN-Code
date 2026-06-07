package tui

import (
	"fmt"
	"strings"
)

func RenderHistoryView(entries []HistoryEntry, selected int, styles Styles, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("⏱ History — last actions"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	if len(entries) == 0 {
		b.WriteString(styles.Muted.Render("  No actions yet."))
		b.WriteString("\n")
		return b.String()
	}

	start := 0
	maxRows := max(height-10, 5)
	if len(entries) > maxRows {
		start = len(entries) - maxRows
	}

	for i := start; i < len(entries); i++ {
		e := entries[i]
		icon := "✓"
		style := styles.StatusOK
		if !e.Success {
			icon = "✗"
			style = styles.StatusErr
		}
		line := fmt.Sprintf("  %s  %s  %-9s  %-14s  %s", e.Time.Format("15:04:05"), icon, e.View, truncate(e.Action, 14), truncate(e.Detail, 40))
		if i == selected {
			b.WriteString(styles.SidebarSel.Render(padRight(line, width-4)))
		} else {
			b.WriteString(style.Render(padRight(line, width-4)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  %d entries", len(entries))))
	b.WriteString(" ")
	b.WriteString(styles.Muted.Render("· press c to clear"))
	b.WriteString("\n")

	return b.String()
}
