// SPDX-License-Identifier: MIT
// Purpose: Todos view — list todos with priority badges, live counts.
package tui

import (
	"fmt"
	"strings"
)

func (m *Model) RenderTodos(styles Styles, width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 5 {
		height = 5
	}
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("Todos"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n\n")

	open := m.Sidebar.TodoOpen
	ready := m.Sidebar.TodoReady
	blocked := m.Sidebar.TodoBlocked
	overdue := m.Sidebar.TodoOverdue

	countLine := fmt.Sprintf("  🔵 %d open  🟢 %d ready  🟡 %d blocked  🔴 %d overdue",
		open, ready, blocked, overdue)
	b.WriteString(styles.Content.Render(countLine))
	b.WriteString("\n\n")

	if len(m.TodoItems) == 0 {
		b.WriteString(styles.Muted.Render("  (no todos yet — press 'a' to add)"))
		b.WriteString("\n")
		return b.String()
	}

	header := fmt.Sprintf("  %-8s %-3s %-10s %-7s %s", "ID", "PRI", "STATUS", "TYPE", "TITLE")
	b.WriteString(styles.AccentText.Render(header))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", width-4)))
	b.WriteString("\n")

	limit := height - 8
	if limit < 1 {
		limit = 1
	}
	if limit > len(m.TodoItems) {
		limit = len(m.TodoItems)
	}
	for i := 0; i < limit; i++ {
		row := m.TodoItems[i]
		priStyle := styles.Muted
		switch row.Priority {
		case "P0":
			priStyle = styles.Bold
		case "P1":
			priStyle = styles.AccentText
		}
		line := fmt.Sprintf("  %-8s %-3s %-10s %-7s %s",
			row.ID, row.Priority, row.Status, row.Type, row.Title)
		if i == m.TodoSel {
			b.WriteString(styles.SidebarSel.Render(line))
		} else {
			b.WriteString(priStyle.Render(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}
