package tui

import (
	"fmt"
	"strings"
)

func RenderRightPanel(sel *ToolSubItem, view ViewKind, styles Styles, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("Details"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	if sel == nil {
		b.WriteString(styles.Muted.Render("  (no selection)"))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(styles.AccentText.Render("Name"))
	b.WriteString("\n")
	b.WriteString(styles.Bold.Render("  " + sel.Name))
	b.WriteString("\n\n")
	b.WriteString(styles.AccentText.Render("Description"))
	b.WriteString("\n")
	b.WriteString(styles.Content.Render("  " + sel.Description))
	b.WriteString("\n\n")
	b.WriteString(styles.AccentText.Render("Usage"))
	b.WriteString("\n")
	b.WriteString(styles.Content.Render("  sin-code " + sel.Name + " <args>"))
	b.WriteString("\n\n")
	if sel.Runnable {
		b.WriteString(styles.StatusOK.Render("  ✓ Runnable without args"))
	} else {
		b.WriteString(styles.Muted.Render("  Requires arguments"))
	}
	b.WriteString("\n\n")

	switch view {
	case ViewTools:
		b.WriteString(styles.AccentText.Render("Shortcuts"))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  r   run this tool"))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  Tab next view"))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  /   filter"))
		b.WriteString("\n")
	}

	return b.String()
}

func RenderSessionsView(styles Styles, tabs Tabs, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("❒ Sessions"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	if len(tabs.Sessions) == 0 {
		b.WriteString(styles.Muted.Render("  No active sessions."))
		b.WriteString("\n")
		return b.String()
	}

	for i, sess := range tabs.Sessions {
		marker := " "
		if sess.Dirty {
			marker = "●"
		}
		label := fmt.Sprintf("  %s  %s", marker, sess.Name)
		if i == tabs.ActiveIdx {
			b.WriteString(styles.SidebarSel.Render(padRight(label+"   ◀ active", width-4)))
		} else {
			b.WriteString(styles.Content.Render(padRight(label, width-4)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  %d session(s)", len(tabs.Sessions))))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  Press + to add a new session, - to close."))
	b.WriteString("\n")

	return b.String()
}

func RenderCommandPalette(items []string, sel int, query string, styles Styles, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Command Palette"))
	b.WriteString("\n")
	if query != "" {
		b.WriteString(styles.Muted.Render("  > " + query))
	} else {
		b.WriteString(styles.Muted.Render("  > (type to filter)"))
	}
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n")

	maxShow := 8
	if len(items) < maxShow {
		maxShow = len(items)
	}
	for i := 0; i < maxShow; i++ {
		if i == sel {
			b.WriteString(styles.PopupSel.Render(padRight("  "+items[i], width-6)))
		} else {
			b.WriteString(styles.PopupItem.Render(padRight("  "+items[i], width-6)))
		}
		b.WriteString("\n")
	}
	if len(items) == 0 {
		b.WriteString(styles.Muted.Render("  (no matches)"))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  ↑/↓ select · Enter run · Esc close"))
	b.WriteString("\n")
	return styles.Popup.Render(b.String())
}

func RenderSubagentsPopup(styles Styles, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Subagents"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  (placeholder) — feature coming soon"))
	b.WriteString("\n\n")
	b.WriteString(styles.Muted.Render("  Use ctrl+x again to close."))
	b.WriteString("\n")
	return styles.Popup.Render(b.String())
}

func ComposeLayout(tabs Tabs, sidebar Sidebar, view ViewKind, content string, right string, footer Footer, styles Styles, width, height int) string {
	if width < 40 {
		width = 40
	}
	if height < 10 {
		height = 10
	}

	header := tabs.View(styles)
	footerView := footer.Render(styles)

	contentHeight := height - 4
	if contentHeight < 3 {
		contentHeight = 3
	}

	leftWidth := 0
	if !sidebar.Collapsed {
		leftWidth = sidebar.Width
	}
	rightWidth := 0
	if right != "" {
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

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	if leftWidth > 0 {
		leftLines := splitLines(sidebar.View(styles), leftWidth, contentHeight)
		b.WriteString(leftLines)
		b.WriteString(strings.Repeat(" ", 1))
	}

	center := padContent(content, centerWidth, contentHeight)
	b.WriteString(center)
	b.WriteString("\n")

	if rightWidth > 0 {
		rightPadded := padContent(right, rightWidth, contentHeight)
		b.WriteString(" ")
		b.WriteString(rightPadded)
	}

	b.WriteString(footerView)
	b.WriteString("\n")
	return b.String()
}

func splitLines(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	var b strings.Builder
	for i := 0; i < height && i < len(lines); i++ {
		b.WriteString(padRight(lines[i], width))
		if i < height-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func padContent(s string, width, height int) string {
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
