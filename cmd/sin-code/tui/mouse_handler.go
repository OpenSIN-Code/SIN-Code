// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second <type>-related handler is needed, merge into a shared file
package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) handleMouseAction(action MouseResolution) tea.Cmd {
	switch action.Kind {
	case "click":
		return m.handleMouseClick(action)
	case "scroll_up":
		return m.handleMouseScrollUp(action)
	case "scroll_down":
		return m.handleMouseScrollDown(action)
	}
	return nil
}

func (m *Model) handleMouseClick(action MouseResolution) tea.Cmd {
	switch action.Target {
	case "sidebar":
		return m.handleSidebarClick(action)
	case "chat":
		if m.ChatInput != nil {
			return m.ChatInput.Focus()
		}
		return nil
	case "tabs":
		return m.handleTabsClick(action)
	case "footer":
		return nil
	case "right_panel":
		return nil
	}
	return nil
}

func (m *Model) handleSidebarClick(action MouseResolution) tea.Cmd {
	const tabBarHeight = 3
	const sidebarHeaderHeight = 2 // header + separator

	relY := action.Y - tabBarHeight
	numMainItems := len(m.Sidebar.Items)

	// Check if we're in the tool sub-items area (only when ViewTools is active)
	if m.Sidebar.SelectedView() == ViewTools {
		subStart := sidebarHeaderHeight + numMainItems + 2 // separator + "Subcommands" header
		toolIdx := relY - subStart
		if toolIdx >= 0 && toolIdx < len(m.Sidebar.ToolSubItems) {
			m.Sidebar.ToolSel = toolIdx
			return nil
		}
	}

	// Main items area
	if relY < sidebarHeaderHeight {
		return nil // clicked on header/separator
	}

	itemIdx := relY - sidebarHeaderHeight
	if itemIdx >= 0 && itemIdx < numMainItems {
		m.Sidebar.Selected = itemIdx
		m.SwitchView(m.Sidebar.SelectedView())
	}

	return nil
}

func (m *Model) handleTabsClick(action MouseResolution) tea.Cmd {
	const tabStartX = 12 // "⚡ sin-code" header + space
	const tabWidth = 15
	if action.X < tabStartX {
		return nil
	}
	idx := (action.X - tabStartX) / tabWidth
	if idx >= 0 && idx < len(m.Tabs.Sessions) {
		m.Tabs.Select(idx)
	}
	return nil
}

func (m *Model) handleMouseScrollUp(action MouseResolution) tea.Cmd {
	if m.ViewKind == ViewChat {
		m.userScrolledUp = true
		m.ChatViewport.ScrollUp(3)
	}
	return nil
}

func (m *Model) handleMouseScrollDown(action MouseResolution) tea.Cmd {
	if m.ViewKind == ViewChat {
		m.ChatViewport.ScrollDown(3)
		if m.ChatViewport.AtBottom() {
			m.userScrolledUp = false
		}
	}
	return nil
}

func (m *Model) ToggleSplitPane() {
	m.SplitPane.Toggle()
	if m.SplitPane.Active() && m.SplitPane.SideKind() == PaneFileViewer {
		if m.FileBrowser.Root() == "" || !m.FileBrowser.Loaded() {
			root := m.Workspace
			if root == "" {
				root = "."
			}
			m.FileBrowser.SetRoot(root)
		}
	}
}

func (m *Model) renderSidePanel(styles Styles, width, height int) string {
	if m.FileViewer.CurrentPath() != "" {
		return m.FileViewer.Render(styles, width, height)
	}
	return m.FileBrowser.Render(styles, width, height)
}

func joinHorizontal(left, right string, leftWidth, rightWidth, height int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	maxLines := height
	if len(leftLines) > maxLines {
		maxLines = len(leftLines)
	}
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}

	var b strings.Builder
	for i := 0; i < maxLines; i++ {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		b.WriteString(padRight(l, leftWidth))
		b.WriteString("│")
		b.WriteString(padRight(r, rightWidth))
		if i < maxLines-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
