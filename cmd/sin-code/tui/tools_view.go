package tui

import (
	"fmt"
	"strings"
)

func RenderToolsView(sidebar Sidebar, styles Styles, width, height int) string {
	sel := sidebar.SelectedTool()
	if sel == nil {
		return styles.Muted.Render("No tool selected")
	}

	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("⚒ Tools (all " + fmt.Sprintf("%d", len(sidebar.ToolSubItems)) + " subcommands)"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	for i, t := range sidebar.ToolSubItems {
		icon := " "
		if t.Runnable {
			icon = "▶"
		}
		prefix := fmt.Sprintf("  %s %-14s", icon, t.Name)
		desc := t.Description
		line := prefix + "  " + desc
		if i == sidebar.ToolSel {
			b.WriteString(styles.SidebarSel.Render(padRight(line, width-4)))
		} else {
			b.WriteString(styles.Content.Render(padRight(line, width-4)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.AccentText.Render("▸ Selected: "))
	b.WriteString(styles.Bold.Render(sel.Name))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + sel.Description))
	b.WriteString("\n")
	if sel.Runnable {
		b.WriteString(styles.StatusOK.Render("  ✓ Runnable without args — press r to run"))
	} else {
		b.WriteString(styles.Muted.Render("  Press r to run with arguments"))
	}
	b.WriteString("\n")

	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
