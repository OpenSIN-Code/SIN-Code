// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/key"
)

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.Mode == ModePalette {
		return m.handlePaletteKey(msg)
	}
	if m.Mode == ModeSubagents {
		if key.Matches(msg, keymap.Interrupt) || key.Matches(msg, keymap.Subagents) {
			m.CloseSubagents()
		}
		return m, nil
	}
	if m.Mode == ModeArgInput {
		return m.handleArgInputKey(msg)
	}
	if m.Mode == ModeSessionSwitcher {
		model, cmd := m.handleSessionSwitcherKey(msg)
		if msg.String() == "enter" {
			return model, tea.Batch(cmd, RefreshSessionTreeCmd())
		}
		return model, cmd
	}
	if m.Mode == ModeModelSelector {
		return m.handleModelSelectorKey(msg)
	}
	if m.Mode == ModeModelSwitcher {
		return m.handleModelSwitcherKey(msg)
	}
	if m.Mode == ModeModelCustom {
		return m.handleModelCustomKey(msg)
	}
	if m.Mode == ModeStatus {
		return m.handleStatusPopupKey(msg)
	}
	if m.Mode == ModeHelpOverlay {
		return m.handleHelpOverlayKey(msg)
	}
	if m.Mode == ModeFilePicker {
		return m.handleFilePickerKey(msg)
	}
	if m.Mode == ModePermissionDialog {
		k := msg.String()
		if k == "a" || k == "A" {
			m.answerPendingAsk(true)
			m.ClosePermissionDialog()
			if m.Footer.PermissionPopup != nil {
				m.Footer.PermissionPopup.Dismiss()
			}
			m.Footer.ShowToast(ToastSuccess, "Always allow: rule added")
			return m, nil
		}
		return m.handlePermissionDialogKey(msg)
	}
	if m.Mode == ModeSearch {
		return m.handleSearchKey(msg)
	}
	if m.Mode == ModeDiffApproval && m.DiffApproval != nil && m.DiffApproval.Open {
		return m.handleDiffApprovalKey(msg)
	}
	if m.Mode == ModeCopy && m.CopyMode != nil && m.CopyMode.Active {
		return m.handleCopyModeKey(msg)
	}

	if m.SplitPane.Active() && m.SplitPane.SideKind() == PaneFileViewer && m.ViewKind == ViewChat {
		switch {
		case key.Matches(msg, keySplitPane):
			m.ToggleSplitPane()
			return m, nil
		case key.Matches(msg, keyUp), key.Matches(msg, keymap.ToolUp):
			if m.FileViewer.CurrentPath() != "" {
				m.FileViewer.ScrollUp(1)
			} else {
				m.FileBrowser.MoveUp()
			}
			return m, nil
		case key.Matches(msg, keyDown), key.Matches(msg, keymap.ToolDown):
			if m.FileViewer.CurrentPath() != "" {
				m.FileViewer.ScrollDown(1)
			} else {
				m.FileBrowser.MoveDown()
			}
			return m, nil
		case key.Matches(msg, keyRight), key.Matches(msg, keymap.ShowHelp):
			if m.FileBrowser.SelectedIsDir() {
				m.FileBrowser.ToggleExpand()
			} else {
				p := m.FileBrowser.SelectedPath()
				if p != "" {
					_ = m.FileViewer.Load(p)
				}
			}
			return m, nil
		case key.Matches(msg, keyLeft):
			if m.FileViewer.CurrentPath() != "" {
				m.FileViewer.Clear()
			}
			return m, nil
		}
	}

	switch {
	case key.Matches(msg, keymap.Quit):
		m.saveChatHistory()
		m.SaveCrashState()
		m.Quitting = true
		return m, tea.Quit
	case key.Matches(msg, keymap.CopyMessage):
		if m.pendingAsk != nil {
			m.answerPendingAsk(true)
			m.ClosePermissionDialog()
			return m, nil
		}
		if m.ViewKind == ViewChat && len(m.ChatHistory) > 0 {
			idx := m.ChatFocusIdx
			if idx >= 0 && idx < len(m.ChatHistory) {
				text := m.ChatHistory[idx].Text
				if text == "" {
					text = m.ChatHistory[idx].Detail
				}
				copyToClipboard(text)
				m.SetBanner(&NotificationItem{
					ID:      fmt.Sprintf("copy-%d", time.Now().UnixNano()),
					Title:   "Copied!",
					Message: "Message text copied to clipboard",
					Type:    "info",
				})
			}
		}
		return m, nil
	case key.Matches(msg, keymap.Interrupt):
		if m.IsStreaming() {
			m.CancelPrompt()
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "Interrupted — cancelling in-flight work…"})
			m.AppendHistory(m.ViewKind.String(), "interrupt", "cancelled in-flight prompt", true)
		} else {
			m.AppendHistory(m.ViewKind.String(), "interrupt", "Esc pressed", true)
		}
		return m, nil
	case key.Matches(msg, keymap.NextView):
		m.NextView()
		return m, nil
	case key.Matches(msg, keymap.PrevView):
		m.PreviousView()
		return m, nil
	case key.Matches(msg, keymap.ToggleSidebar):
		m.Sidebar.Toggle()
		return m, nil
	case key.Matches(msg, keyBlockToggle):
		if m.ViewKind == ViewChat {
			m.ToggleBlockCollapse(-1)
		}
		return m, nil
	case key.Matches(msg, keymap.Palette):
		m.OpenPalette()
		return m, nil
	case key.Matches(msg, keymap.Search):
		m.OpenSearch()
		return m, nil
	case key.Matches(msg, keyDiffApproval) && m.InlineDiffOpen:
		m.OpenDiffApprovalFromInlineDiff()
		return m, nil
	case key.Matches(msg, keymap.Subagents):
		m.OpenSubagents()
		return m, nil
	case key.Matches(msg, keyDiffPopup):
		m.DiffPopupOpen = !m.DiffPopupOpen
		return m, nil
	case key.Matches(msg, keyToolTree):
		m.ToolTreeVisible = !m.ToolTreeVisible
		return m, nil
	case key.Matches(msg, keySessionTree):
		m.SessionTreeVisible = !m.SessionTreeVisible
		return m, RefreshSessionTreeCmd()
	case key.Matches(msg, keyVerifyFull):
		m.VerifyPanelFull = !m.VerifyPanelFull
		return m, nil
	case key.Matches(msg, keyDebugLayout):
		m.ToggleDebugLayout()
		return m, nil
	case key.Matches(msg, keyInlineDiff):
		m.ToggleInlineDiff()
		return m, nil
	case key.Matches(msg, keySplitPane):
		m.ToggleSplitPane()
		return m, nil
	case key.Matches(msg, keyCopyMode):
		if m.ViewKind == ViewChat {
			m.EnterCopyMode()
		}
		return m, nil
	case msg.String() == "ctrl+o":
		m.OpenFilePicker()
		return m, nil
	case key.Matches(msg, keyClosePreview) && m.FilePreview != "":
		m.ClearFilePreview()
		return m, nil
	case key.Matches(msg, keymap.SessionSwitch):
		m.OpenSessionSwitcher()
		return m, nil
	case key.Matches(msg, keymap.Help):
		if m.HelpOverlay != nil {
			m.HelpOverlay.Open()
			m.Mode = ModeHelpOverlay
		}
		return m, nil
	case key.Matches(msg, keymap.ModelSelect):
		m.OpenModelSelector()
		return m, nil
	case key.Matches(msg, keymap.ViewTools):
		m.SwitchView(ViewTools)
		return m, nil
	case key.Matches(msg, keymap.ViewSessions):
		m.SwitchView(ViewSessions)
		return m, nil
	case key.Matches(msg, keymap.ViewEFM):
		m.SwitchView(ViewEFM)
		return m, nil
	case key.Matches(msg, keymap.ViewConfig):
		m.SwitchView(ViewConfig)
		return m, nil
	case key.Matches(msg, keymap.ViewHistory):
		m.SwitchView(ViewHistory)
		return m, nil
	case key.Matches(msg, keymap.ViewTodos):
		m.SwitchView(ViewTodos)
		return m, nil
	case key.Matches(msg, keymap.ViewChat):
		m.SwitchView(ViewChat)
		return m, nil
	case key.Matches(msg, keymap.ViewDAG):
		m.SwitchView(ViewDAG)
		return m, nil
	case key.Matches(msg, keymap.ViewContext):
		m.SwitchView(ViewContextViz)
		return m, nil
	case key.Matches(msg, keymap.ViewDashboard):
		m.SwitchView(ViewAgentDashboard)
		return m, nil
	case key.Matches(msg, keymap.ViewKanban):
		m.SwitchView(ViewKanban)
		return m, nil
	case key.Matches(msg, keymap.CycleTheme):
		m.CycleTheme()
		m.AppendHistory(m.ViewKind.String(), "theme", Themes[m.ThemeIdx].Name, true)
		return m, nil
	case key.Matches(msg, keymap.CompactToggle):
		if m.ViewKind == ViewChat && m.CompactMode != nil {
			m.CompactMode.Toggle()
			active := m.CompactMode.Active()
			toggleMsg := "Compact mode off"
			if active {
				toggleMsg = "Compact mode on — messages rendered in condensed format"
			}
			m.appendChat(ChatMessage{Kind: chatSystem, Text: toggleMsg})
		}
		return m, nil
	case key.Matches(msg, keymap.CycleAgent):
		m.Footer.CycleAgent()
		m.AppendHistory(m.ViewKind.String(), "agent", m.Footer.AgentName(), true)
		return m, nil
	case key.Matches(msg, keymap.RunTool):
		if m.ViewKind == ViewTools {
			m.RunSelected()
		}
		return m, nil
	case key.Matches(msg, keymap.ShowHelp):
		if m.ViewKind == ViewChat && len(m.ChatHistory) > 0 {
			idx := m.ChatFocusIdx
			if idx >= 0 && idx < len(m.ChatHistory) {
				cm := &m.ChatHistory[idx]
				if cm.Kind == chatTool {
					cm.Expanded = !cm.Expanded
					return m, nil
				}
			}
		}
		if m.ViewKind == ViewTools {
			tool := m.Sidebar.SelectedTool()
			if tool != nil {
				m.AppendHistory(ViewTools.String(), "show-help", tool.Name, true)
			}
		}
		return m, nil
	case key.Matches(msg, keyPgUp):
		if m.ViewKind == ViewChat {
			m.userScrolledUp = true
			m.ChatViewport.PageUp()
			m.updateChatFocusFromViewport()
			return m, nil
		}
	case key.Matches(msg, keyPgDn):
		if m.ViewKind == ViewChat {
			m.ChatViewport.PageDown()
			if m.ChatViewport.AtBottom() {
				m.userScrolledUp = false
			}
			m.updateChatFocusFromViewport()
			return m, nil
		}
	case key.Matches(msg, keymap.ToolUp):
		if m.ViewKind == ViewChat {
			m.userScrolledUp = true
			m.ChatViewport.ScrollUp(1)
			m.updateChatFocusFromViewport()
			return m, nil
		}
		switch m.ViewKind {
		case ViewTools:
			m.Sidebar.ToolMoveUp()
		case ViewConfig:
			if m.ConfigSel > 0 {
				m.ConfigSel--
			}
		case ViewTodos:
			if m.TodoSel > 0 {
				m.TodoSel--
			}
		case ViewDAG:
			if m.DAGState.Selected > 0 {
				m.DAGState.Selected--
			}
		case ViewAgentDashboard:
			if m.AgentDashboardState.Selected > 0 {
				m.AgentDashboardState.Selected--
			}
		case ViewMemory:
			m.MemoryBrowser.MoveUp()
		case ViewKanban:
			m.KanbanView.MoveUp()
		}
		return m, nil
	case key.Matches(msg, keymap.ToolDown):
		if m.ViewKind == ViewChat {
			m.ChatViewport.ScrollDown(1)
			if m.ChatViewport.AtBottom() {
				m.userScrolledUp = false
			}
			m.updateChatFocusFromViewport()
			return m, nil
		}
		switch m.ViewKind {
		case ViewTools:
			m.Sidebar.ToolMoveDown()
		case ViewConfig:
			if m.ConfigSel < len(m.Config)-1 {
				m.ConfigSel++
			}
		case ViewTodos:
			if m.TodoSel < len(m.TodoItems)-1 {
				m.TodoSel++
			}
		case ViewDAG:
			if m.DAGState.Selected < len(m.DAGState.Tasks)-1 {
				m.DAGState.Selected++
			}
		case ViewAgentDashboard:
			if m.AgentDashboardState.Selected < len(m.AgentDashboardState.Sessions)-1 {
				m.AgentDashboardState.Selected++
			}
		case ViewMemory:
			m.MemoryBrowser.MoveDown()
		case ViewKanban:
			m.KanbanView.MoveDown()
		}
		return m, nil
	case key.Matches(msg, keyLeft):
		if m.ViewKind == ViewKanban {
			m.KanbanView.MoveLeft()
			return m, nil
		}
		return m, nil
	case key.Matches(msg, keyRight):
		if m.ViewKind == ViewKanban {
			m.KanbanView.MoveRight()
			return m, nil
		}
		return m, nil
	case key.Matches(msg, keyBannerOpen):
		if m.NotificationBanner != nil {
			m.AppendHistory(ViewTodos.String(), "banner-open", m.NotificationBanner.Title, true)
		}
		return m, nil
	case key.Matches(msg, keyBannerDismiss):
		if m.NotificationBanner != nil {
			m.DismissBanner()
			m.AppendHistory(ViewTodos.String(), "banner-dismiss", "", true)
		}
		return m, nil
	case key.Matches(msg, keyBannerNext):
		if m.pendingAsk != nil {
			m.answerPendingAsk(false)
			m.ClosePermissionDialog()
			return m, nil
		}
		if m.NotificationBanner != nil {
			m.BannerNext()
		}
		return m, nil
	case key.Matches(msg, keymap.NewSession):
		if m.ViewKind == ViewSessions {
			m.Tabs.Add("")
			m.AppendHistory(ViewSessions.String(), "session-add", "", true)
			return m, RefreshSessionTreeCmd()
		}
		return m, nil
	case key.Matches(msg, keymap.CloseSession):
		if m.ViewKind == ViewSessions {
			m.Tabs.Close(m.Tabs.ActiveIdx)
			m.AppendHistory(ViewSessions.String(), "session-close", "", true)
			return m, RefreshSessionTreeCmd()
		}
		return m, nil
	}

	return m, nil
}
