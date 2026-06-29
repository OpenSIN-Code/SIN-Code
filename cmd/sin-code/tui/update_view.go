// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"
)

func (m *Model) View() tea.View {
	if m.Quitting {
		v := tea.NewView("")
		v.AltScreen = true
		v.WindowTitle = "sin-code tui"
		return v
	}
	if m.Width < 20 || m.Height < 6 {
		v := tea.NewView(m.Styles.Muted.Render("Terminal too small — resize to at least 20x6."))
		v.AltScreen = true
		v.WindowTitle = "sin-code tui"
		return v
	}

	var content string
	var right string

	contentHeight := m.Height - 4
	if contentHeight < 3 {
		contentHeight = 3
	}

	switch m.ViewKind {
	case ViewTools:
		content = RenderToolsView(m.Sidebar, m.Styles, m.contentWidth(), contentHeight)
		if m.RightPanel {
			if t := m.Sidebar.SelectedTool(); t != nil {
				right = RenderRightPanel(t, m.ViewKind, m.Styles, m.rightWidth(), contentHeight)
			}
		}
	case ViewSessions:
		content = RenderSessionsView(m.Styles, m.Tabs, m.contentWidth(), contentHeight)
	case ViewEFM:
		content = RenderEFMView(m.EFMStks, m.Styles, m.contentWidth(), contentHeight, m.Spinner)
	case ViewConfig:
		content = RenderConfigView(m.Config, m.ConfigSel, m.Styles, m.contentWidth(), contentHeight)
	case ViewHistory:
		content = RenderHistoryView(m.History, len(m.History)-1, m.Styles, m.contentWidth(), contentHeight)
	case ViewTodos:
		content = m.RenderTodos(m.Styles, m.contentWidth(), contentHeight)
	case ViewChat:
		m.initChatInput()
		if m.SplitPane.Active() {
			cw := m.contentWidth()
			sideW := m.SplitPane.SideWidth(cw)
			chatW := cw - sideW - 1
			if chatW < 20 {
				chatW = 20
				sideW = cw - chatW - 1
			}
			chatContent := m.renderChat(m.Styles, chatW, contentHeight)
			sideContent := m.renderSidePanel(m.Styles, sideW, contentHeight)
			content = joinHorizontal(chatContent, sideContent, chatW, sideW, contentHeight)
		} else {
			content = m.renderChat(m.Styles, m.contentWidth(), contentHeight)
		}
	case ViewDAG:
		content = RenderDAGView(m.DAGState, m.Styles, m.contentWidth(), contentHeight)
	case ViewContextViz:
		content = RenderContextVizView(m.ContextState, m.Styles, m.contentWidth(), contentHeight)
	case ViewAgentDashboard:
		content = RenderAgentDashboardView(m.AgentDashboardState, m.Styles, m.contentWidth(), contentHeight)
	case ViewLSP:
		content = RenderLSPView(m.LSPState, m.Styles, m.contentWidth(), contentHeight)
	case ViewMemory:
		content = m.MemoryBrowser.Render(m.Styles, m.contentWidth(), contentHeight)
	case ViewKanban:
		content = m.KanbanView.Render(m.Styles, m.contentWidth(), contentHeight)
	}

	if m.NotificationBanner != nil {
		banner := m.RenderBanner(m.Styles, m.contentWidth())
		content = banner + content
	}

	if m.Mode == ModeArgInput && m.ArgInput.Open {
		prompt := fmt.Sprintf("args for %s: %s", m.ArgInput.Cmd, m.ArgInput.Input.View())
		content = m.Styles.ContentHdr.Render(prompt) + "\n" + content
	}

	if m.Sidebar.SelectedTool() != nil {
		m.Footer.Selection = m.Sidebar.SelectedTool().Name
	} else {
		m.Footer.Selection = ""
	}
	m.Footer.Loading = m.Loading

	layout := ComposeLayout(m.Tabs, m.Sidebar, m.ViewKind, content, right, m.Footer, m.Styles, m.Width, m.Height)

	if m.DebugLayout {
		layout = RenderLayoutDebug(m.Tabs, m.Sidebar, m.ViewKind, content, right, m.Footer, m.Styles, m.Width, m.Height)
	}

	if m.Footer.Transition != nil && m.Footer.Transition.Active() {
		layout = m.Footer.Transition.Render(layout, m.Styles, m.Width, m.Height)
	}

	if m.Mode == ModePalette {
		popup := RenderCommandPalette(m.Palette.Filter, m.Palette.Sel, m.Palette.Query, m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}
	if m.Mode == ModeSubagents {
		popup := RenderSubagentsPopup(m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}
	if m.Mode == ModeSessionSwitcher {
		popup := RenderSessionSwitcher(m.SessionSwitcher, m.Tabs, m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}
	if m.Mode == ModeModelSelector {
		popup := RenderModelSelector(m.ModelSelector, m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}
	if m.Mode == ModeModelSwitcher && m.ModelSwitcher != nil {
		popup := m.ModelSwitcher.Render(m.Styles, m.Width, m.Height)
		if popup != "" {
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
		}
	}
	if m.Mode == ModeModelCustom && m.ModelCustomInput != nil {
		popup := m.RenderModelCustomInput(m.Styles, m.Width, m.Height)
		if popup != "" {
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
		}
	}
	if m.Mode == ModeStatus && m.StatusPopup != nil {
		popup := m.RenderStatusPopup(m.Styles, m.Width, m.Height)
		if popup != "" {
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
		}
	}
	if m.Mode == ModeHelpOverlay && m.HelpOverlay != nil {
		layout = m.HelpOverlay.Render(m.Styles, m.Width, m.Height)
	}
	if m.Mode == ModeFilePicker && m.FilePicker != nil {
		popup := m.FilePicker.Render(m.Styles, m.Width, m.Height)
		if popup != "" {
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
		}
	}
	if m.Mode == ModePermissionDialog && m.Footer.PermissionPopup != nil {
		req := PermissionRequest{
			Tool: m.PermissionDialog.ToolName,
			Args: m.PermissionDialog.Detail,
			Risk: RiskFromTool(m.PermissionDialog.ToolName, m.PermissionDialog.Detail),
		}
		m.Footer.PermissionPopup.SetRequest(req)
		popup := m.Footer.PermissionPopup.Render(m.Styles, m.Width, m.Height)
		if popup != "" {
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
		}
	} else if m.Footer.PermissionPopup != nil && m.Footer.PermissionPopup.Active() {
		m.Footer.PermissionPopup.Dismiss()
	}

	if m.ViewKind == ViewChat && m.Mode == ModeSearch && m.ChatSearch != nil {
		popup := m.ChatSearch.Render(m.Styles, min(m.Width-8, 70), min(m.Height-6, 20))
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}

	if m.DiffPopupOpen {
		popup := RenderDiffPopupView(m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}

	if m.InlineDiffOpen && m.ViewKind == ViewChat {
		diffs := RecentDiffs()
		if len(diffs) > 0 {
			inlineDiff := RenderInlineDiff(diffs, m.Styles, m.contentWidth())
			layout = layout + "\n" + inlineDiff
		}
	}

	if m.Mode == ModeDiffApproval && m.DiffApproval != nil && m.DiffApproval.Open {
		m.DiffApproval.Styles = m.Styles
		popup := m.DiffApproval.Render()
		if popup != "" {
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
		}
	}

	if m.Mode == ModeCopy && m.CopyMode != nil && m.CopyMode.Active {
		overlay := m.CopyMode.Render(m.contentWidth(), contentHeight)
		if overlay != "" {
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, overlay)
		}
	}

	if m.FilePreview != "" {
		popup := RenderFilePreview(m, m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}

	// Verify panel overlay in chat mode
	if m.ViewKind == ViewChat && m.VerifyPanel.State != VerifyIdle {
		if m.VerifyPanelFull {
			vp := RenderVerifyPanel(m.VerifyPanel, m.Styles, m.contentWidth())
			if vp != "" {
				popupBox := lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(m.Styles.AccentText.GetForeground()).
					Padding(1, 2).
					Render(vp)
				layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popupBox)
			}
		} else {
			vp := RenderVerifyStatusBar(m.VerifyPanel, m.Styles)
			if vp != "" {
				layout = layout + "\n" + vp
			}
		}
	}

	// Tool call tree popup overlay (ctrl+t to toggle)
	if m.ToolTreeVisible && m.ToolTree != nil && len(m.ToolTree.Nodes) > 0 {
		tree := RenderToolCallTree(m.ToolTree, m.Styles, min(m.Width-4, 80))
		popup := m.Styles.ContentHdr.Render(tree)
		popupBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.Styles.AccentText.GetForeground()).
			Padding(1, 2).
			Render(popup)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popupBox)
	}

	// Session tree popup overlay (ctrl+y to toggle)
	if m.SessionTreeVisible && m.SessionTree != nil {
		tree := RenderSessionTree(m.SessionTree, m.Styles, min(m.Width-4, 80))
		popupBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.Styles.StatusOK.GetForeground()).
			Padding(1, 2).
			Render(tree)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popupBox)
	}

	if m.ViewKind == ViewChat {
		if m.SlashMenu != nil && m.SlashMenu.Open {
			m.SlashMenu.Styles = m.Styles
			m.SlashMenu.Width = min(m.Width-8, 70)
			popup := m.SlashMenu.Render()
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Left, lipgloss.Bottom, popup)
		} else if m.SlashAutocomplete != nil && m.SlashAutocomplete.Active() {
			popup := m.SlashAutocomplete.Render(m.Styles, min(m.Width-8, 70))
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Left, lipgloss.Bottom, popup)
		}
	}

	if m.Footer.Toast != nil && m.Footer.Toast.Active() {
		toastBox := m.Footer.Toast.Render(m.Styles, m.Width)
		if toastBox != "" {
			layout = lipgloss.Place(m.Width, m.Height, lipgloss.Right, lipgloss.Top, toastBox)
		}
	}

	v := tea.NewView(layout)
	v.AltScreen = true
	v.WindowTitle = "sin-code tui"
	v.ReportFocus = true
	v.MouseMode = tea.MouseModeCellMotion

	if m.ViewKind == ViewChat {
		pct := m.Footer.TokensPct
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}
		state := tea.ProgressBarDefault
		if pct >= 0.8 {
			state = tea.ProgressBarError
		} else if pct >= 0.5 {
			state = tea.ProgressBarWarning
		}
		v.ProgressBar = &tea.ProgressBar{
			State: state,
			Value: int(pct * 100),
		}

		if m.ChatInput != nil && len(m.ChatHistory) > 0 {
			textHeight := 3
			raw := m.ChatInput.RawValue()
			if lines := strings.Count(raw, "\n") + 1; lines > 3 {
				textHeight = min(lines+2, 10)
			}
			cursorY := m.Height - 3 - textHeight
			if cursorY < 1 {
				cursorY = 1
			}
			cursor := tea.NewCursor(0, cursorY)
			cursor.Shape = tea.CursorBar
			cursor.Blink = true
			v.Cursor = cursor
		}
	}

	return v
}

func (m *Model) contentWidth() int {
	left := 0
	if m.ViewKind != ViewChat && !m.Sidebar.Collapsed {
		left = m.Sidebar.Width
	}
	right := 0
	if m.RightPanel && m.ViewKind != ViewChat {
		if m.Width > 100 {
			right = 32
		} else if m.Width > 60 {
			right = 24
		}
	}
	cw := m.Width - left - right
	if cw < 20 {
		cw = 20
	}
	return cw
}

func (m *Model) rightWidth() int {
	if !m.RightPanel {
		return 0
	}
	if m.Width > 100 {
		return 32
	}
	if m.Width > 60 {
		return 24
	}
	return 20
}
