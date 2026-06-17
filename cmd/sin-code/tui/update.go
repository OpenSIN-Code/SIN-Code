package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

var keymap = DefaultKeymap()

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.Spinner.Init(),
		ListenForNotifications(),
		RefreshTodosCmd(),
		InitGitRefresh(),
	}
	return tea.Batch(cmds...)
}

func (m *Model) ApplyTheme() {
	if m.ThemeIdx < 0 {
		m.ThemeIdx = 0
	}
	if m.ThemeIdx >= len(Themes) {
		m.ThemeIdx = 0
	}
	m.Styles = NewStyles(Themes[m.ThemeIdx])

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(Themes[m.ThemeIdx].Background)).
		Background(lipgloss.Color(Themes[m.ThemeIdx].Accent))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color(Themes[m.ThemeIdx].Background)).
		Background(lipgloss.Color(Themes[m.ThemeIdx].Accent))
	m.ToolList.SetDelegate(delegate)
	m.Sidebar.SetSelectedView(m.ViewKind)
}

func (m *Model) CycleTheme() {
	m.ThemeIdx = (m.ThemeIdx + 1) % len(Themes)
	m.ApplyTheme()
}

func (m *Model) SwitchView(v ViewKind) {
	m.ViewKind = v
	m.Sidebar.SetSelectedView(v)
	m.Footer.SetView(v)
}

func (m *Model) NextView() {
	m.SwitchView(ViewKind((int(m.ViewKind) + 1) % 7))
}

func (m *Model) PrevView() {
	v := int(m.ViewKind) - 1
	if v < 0 {
		v = 6
	}
	m.SwitchView(ViewKind(v))
}

func (m *Model) AppendHistory(view, action, detail string, ok bool) {
	entry := HistoryEntry{
		Time:    time.Now(),
		View:    view,
		Action:  action,
		Detail:  detail,
		Success: ok,
	}
	m.History = append(m.History, entry)
	if len(m.History) > 200 {
		m.History = m.History[len(m.History)-200:]
	}
}

func (m *Model) filterPalette(query string) {
	if query == "" {
		m.Palette.Filter = m.Palette.Items
		m.Palette.Sel = 0
		return
	}
	q := strings.ToLower(query)
	filtered := make([]string, 0, len(m.Palette.Items))
	for _, item := range m.Palette.Items {
		if strings.Contains(strings.ToLower(item), q) {
			filtered = append(filtered, item)
		}
	}
	m.Palette.Filter = filtered
	if m.Palette.Sel >= len(filtered) {
		m.Palette.Sel = 0
	}
}

func (m *Model) OpenPalette() {
	m.Palette.Open = true
	m.Palette.Query = ""
	m.Palette.Sel = 0
	m.Palette.Filter = m.Palette.Items
	m.Mode = ModePalette
}

func (m *Model) ClosePalette() {
	m.Palette.Open = false
	m.Mode = ModeNormal
}

func (m *Model) OpenSubagents() {
	m.Mode = ModeSubagents
}

func (m *Model) CloseSubagents() {
	if m.Mode == ModeSubagents {
		m.Mode = ModeNormal
	}
}

func (m *Model) OpenArgInput(cmd string) {
	m.ArgInput.Open = true
	m.ArgInput.Cmd = cmd
	m.ArgInput.Value = ""
	m.ArgInput.Input.SetValue("")
	m.ArgInput.Input.Focus()
	m.Mode = ModeArgInput
}

func (m *Model) CloseArgInput() {
	m.ArgInput.Open = false
	m.ArgInput.Input.Blur()
	m.Mode = ModeNormal
}

func (m *Model) RunSelected() {
	tool := m.Sidebar.SelectedTool()
	if tool == nil {
		return
	}
	if tool.Runnable {
		m.runTool(tool.Name, nil)
		return
	}
	m.OpenArgInput(tool.Name)
}

func (m *Model) runTool(name string, args []string) {
	m.AppendHistory(m.ViewKind.String(), "run:"+name, strings.Join(args, " "), true)
	if isSkillName(name) {
		skillArgs := strings.Join(args, " ")
		if skillArgs == "" {
			skillArgs = "perform the requested action"
		}
		cmd := m.runAgentSkillPrompt(name, skillArgs)
		if cmd != nil {
			_ = cmd
		}
	}
	if m.OnRun != nil {
		if err := m.OnRun(name, args); err != nil {
			m.AppendHistory(m.ViewKind.String(), "run:"+name, err.Error(), false)
		}
	}
}

func isSkillName(name string) bool {
	switch name {
	case "websearch", "browser", "scheduler", "goal-mode", "grill-me",
		"doc-coauthoring", "codocs", "marketplace", "brain",
		"context-bridge":
		return true
	}
	return false
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = true
		m.Footer.Width = msg.Width
		m.Tabs.Width = msg.Width
		left := 22
		right := 0
		if m.RightPanel {
			if msg.Width > 100 {
				right = 32
			} else if msg.Width > 60 {
				right = 24
			}
		}
		center := msg.Width - left - right
		if center < 20 {
			center = 20
		}
		m.ToolList.SetSize(center-4, m.Height-8)
		if m.Sidebar.Collapsed {
			m.Sidebar.Width = 6
		} else {
			m.Sidebar.Width = left
		}
		return m, nil

	case SpinnerTickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case NotificationMsg:
		m.SetBanner(&NotificationItem{
			ID:      msg.N.GetID(),
			Title:   msg.N.GetTitle(),
			Message: msg.N.GetMessage(),
			Type:    msg.N.GetType(),
		})
		cmds = append(cmds, ListenForNotifications())
		return m, tea.Batch(cmds...)

	case CountsMsg:
		m.Sidebar.TodoOpen = msg.Open
		m.Sidebar.TodoBlocked = msg.Blocked
		m.Sidebar.TodoOverdue = msg.Overdue
		m.Sidebar.TodoReady = msg.Ready
		return m, nil

	case TodosLoadedMsg:
		m.TodoItems = msg.Items
		if m.TodoSel >= len(m.TodoItems) {
			m.TodoSel = 0
		}
		return m, nil

	case BannerKeyMsg:
		return m, nil

	case chat.ChatResponseMsg:
		m.handleChatResponse(msg)
		return m, nil

	case ChatChunkMsg:
		if msg.Idx >= 0 && msg.Idx < len(m.ChatHistory) {
			m.ChatHistory[msg.Idx].Kind = chatAssistant
			m.ChatHistory[msg.Idx].Text += msg.Text
		}
		return m, nil

	case ChatCopyMsg:
		return m, nil

	case AgentRunnerMsg:
		m.handleAgentRunnerEvent(msg)
		if !msg.Closed && m.AgentRunner != nil {
			cmds = append(cmds, listenAgentRunnerCmd(m.AgentRunner))
		}
		return m, tea.Batch(cmds...)

	case GitRefreshMsg:
		HandleGitRefresh(m, msg)
		return m, nil

	case FilePreviewMsg:
		HandleFilePreviewMsg(m, msg)
		return m, nil

	case DiffPopupMsg:
		m.DiffPopupOpen = !m.DiffPopupOpen
		return m, nil

	case tea.KeyPressMsg:
		if m.NotificationBanner != nil {
			k := msg.String()
			if k == "o" || k == "d" || k == "n" {
				return m.handleKey(msg)
			}
		}
		if m.Mode != ModeNormal {
			return m.handleKey(msg)
		}
		if isGlobalHotkey(msg) {
			return m.handleKey(msg)
		}
		if m.ViewKind == ViewChat {
			cmd := m.updateChat(msg)
			return m, cmd
		}
		return m.handleKey(msg)

	case tea.MouseMsg:
		action := ResolveMouse(msg, m.Width, m.Height, m.Sidebar.Width, m.rightWidth())
		return m, m.handleMouseAction(action)
	}

	return m, tea.Batch(cmds...)
}

func isGlobalHotkey(msg tea.KeyMsg) bool {
	key := msg.String()
	// Chat input keys — must go to textarea, not handleKey
	if key == "ctrl+s" || key == "ctrl+enter" || key == "ctrl+d" {
		return false
	}
	// In chat mode, tab/shift+tab should stay in the textarea
	// (indentation), not switch views. Use Ctrl+Tab for view switching.
	if key == "tab" || key == "shift+tab" {
		return false
	}
	switch {
	case strings.HasPrefix(key, "ctrl+"):
		return true
	case key == "esc", key == "q":
		return true
	case key >= "0" && key <= "9":
		return true // view jump keys (0-9)
	case key == "y" || key == "n":
		return true // permission dialog (when pendingAsk is set)
	}
	return false
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	if m.Mode == ModePalette {
		return m.handlePaletteKey(msg)
	}
	if m.Mode == ModeSubagents {
		if keyStr == "esc" || keyStr == "ctrl+x" {
			m.CloseSubagents()
		}
		return m, nil
	}
	if m.Mode == ModeArgInput {
		return m.handleArgInputKey(msg)
	}
	if m.Mode == ModeSessionSwitcher {
		return m.handleSessionSwitcherKey(msg)
	}
	if m.Mode == ModeModelSelector {
		return m.handleModelSelectorKey(msg)
	}
	if m.Mode == ModePermissionDialog {
		return m.handlePermissionDialogKey(msg)
	}
	if m.Mode == ModeSearch {
		return m.handleSearchKey(msg)
	}

	switch {
	case key.Matches(msg, keymap.Quit):
		m.saveChatHistory()
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
		m.AppendHistory(m.ViewKind.String(), "interrupt", "Esc pressed", true)
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
	case key.Matches(msg, keymap.Palette):
		m.OpenPalette()
		return m, nil
	case key.Matches(msg, keymap.Search):
		m.OpenSearch()
		return m, nil
	case key.Matches(msg, keymap.Subagents):
		m.OpenSubagents()
		return m, nil
	case keyStr == "ctrl+d":
		m.DiffPopupOpen = !m.DiffPopupOpen
		return m, nil
	case keyStr == "ctrl+f" && m.FilePreview != "":
		m.ClearFilePreview()
		return m, nil
	case key.Matches(msg, keymap.SessionSwitch):
		m.OpenSessionSwitcher()
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
	case key.Matches(msg, keymap.CycleTheme):
		m.CycleTheme()
		m.AppendHistory(m.ViewKind.String(), "theme", Themes[m.ThemeIdx].Name, true)
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
	case keyStr == "pgup":
		if m.ViewKind == ViewChat {
			m.ChatViewport.PageUp()
			m.updateChatFocusFromViewport()
			return m, nil
		}
	case keyStr == "pgdn":
		if m.ViewKind == ViewChat {
			m.ChatViewport.PageDown()
			m.updateChatFocusFromViewport()
			return m, nil
		}
	case key.Matches(msg, keymap.ToolUp):
		if m.ViewKind == ViewChat && !m.ChatViewport.AtBottom() {
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
		}
		return m, nil
	case key.Matches(msg, keymap.ToolDown):
		if m.ViewKind == ViewChat && !m.ChatViewport.AtBottom() {
			m.ChatViewport.ScrollDown(1)
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
		}
		return m, nil
	case keyStr == "up_left" || keyStr == "left" || keyStr == "h":
		return m, nil
	case keyStr == "right" || keyStr == "l":
		return m, nil
	case keyStr == "o":
		if m.NotificationBanner != nil {
			m.AppendHistory(ViewTodos.String(), "banner-open", m.NotificationBanner.Title, true)
		}
		return m, nil
	case keyStr == "d":
		if m.NotificationBanner != nil {
			m.DismissBanner()
			m.AppendHistory(ViewTodos.String(), "banner-dismiss", "", true)
		}
		return m, nil
	case keyStr == "n":
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
		}
		return m, nil
	case key.Matches(msg, keymap.CloseSession):
		if m.ViewKind == ViewSessions {
			m.Tabs.Close(m.Tabs.ActiveIdx)
			m.AppendHistory(ViewSessions.String(), "session-close", "", true)
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) PreviousView() {
	v := int(m.ViewKind) - 1
	if v < 0 {
		v = 6
	}
	m.SwitchView(ViewKind(v))
}

func (m *Model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "ctrl+p":
		m.ClosePalette()
		return m, nil
	case "enter":
		if m.Palette.Sel < len(m.Palette.Filter) {
			choice := m.Palette.Filter[m.Palette.Sel]
			m.ClosePalette()
			m.executePaletteChoice(choice)
		}
		return m, nil
	case "up":
		if m.Palette.Sel > 0 {
			m.Palette.Sel--
		}
		return m, nil
	case "down":
		if m.Palette.Sel < len(m.Palette.Filter)-1 {
			m.Palette.Sel++
		}
		return m, nil
	case "backspace":
		if len(m.Palette.Query) > 0 {
			m.Palette.Query = m.Palette.Query[:len(m.Palette.Query)-1]
			m.filterPalette(m.Palette.Query)
		}
		return m, nil
	default:
		m.Palette.Query += msg.String()
		m.filterPalette(m.Palette.Query)
	}
	return m, nil
}

func (m *Model) executePaletteChoice(choice string) {
	switch choice {
	case "theme: next":
		m.CycleTheme()
	case "agent: cycle":
		m.Footer.CycleAgent()
	case "view: tools":
		m.SwitchView(ViewTools)
	case "view: sessions":
		m.SwitchView(ViewSessions)
	case "view: efm":
		m.SwitchView(ViewEFM)
	case "view: config":
		m.SwitchView(ViewConfig)
	case "view: history":
		m.SwitchView(ViewHistory)
	case "sidebar: toggle":
		m.Sidebar.Toggle()
	case "quit":
		m.Quitting = true
	default:
		m.AppendHistory(ViewTools.String(), "palette", choice, true)
	}
}

func (m *Model) handleArgInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.CloseArgInput()
		return m, nil
	case "enter":
		cmd := m.ArgInput.Cmd
		args := splitArgs(m.ArgInput.Input.Value())
		m.CloseArgInput()
		m.runTool(cmd, args)
		return m, nil
	}
	var cmd tea.Cmd
	m.ArgInput.Input, cmd = m.ArgInput.Input.Update(msg)
	return m, cmd
}

func splitArgs(s string) []string {
	out := []string{}
	cur := ""
	inQuote := false
	for _, r := range s {
		switch r {
		case '"', '\'':
			inQuote = !inQuote
			cur += string(r)
		case ' ':
			if inQuote {
				cur += " "
			} else if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		default:
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

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
		content = m.renderChat(m.Styles, m.contentWidth(), contentHeight)
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
	if m.Mode == ModePermissionDialog {
		popup := RenderPermissionDialog(m.PermissionDialog, m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}

	if m.DiffPopupOpen {
		popup := RenderDiffPopupView(m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}

	if m.FilePreview != "" {
		popup := RenderFilePreview(m, m.Styles, m.Width, m.Height)
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, popup)
	}

	v := tea.NewView(layout)
	v.AltScreen = true
	v.WindowTitle = "sin-code tui"
	v.ReportFocus = true
	v.MouseMode = tea.MouseModeCellMotion
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

func (m *Model) handleChatResponse(msg chat.ChatResponseMsg) {
	m.setStreaming(false)
	if msg.Tokens > 0 {
		m.Footer.Tokens += msg.Tokens
		m.Footer.TokensPct = float64(m.Footer.Tokens) / 128000.0
		if m.Footer.TokensPct > 1.0 {
			m.Footer.TokensPct = 1.0
		}
	}
	if len(m.ChatHistory) == 0 {
		return
	}
	idx := len(m.ChatHistory) - 1
	last := m.ChatHistory[idx]
	if last.Kind != chatThinking {
		if msg.Error != nil {
			m.appendChat(ChatMessage{Kind: chatError, Text: msg.Error.Error(), Error: msg.Error})
		} else if msg.Text == "" {
			m.appendChat(ChatMessage{Kind: chatAssistant, Text: "(empty response)"})
		} else {
			m.appendChat(ChatMessage{Kind: chatAssistant, Text: msg.Text})
		}
		m.updateSessionPreview()
		return
	}
	if msg.Error != nil {
		m.ChatHistory[idx] = ChatMessage{Kind: chatError, Text: msg.Error.Error(), Error: msg.Error}
		m.updateSessionPreview()
		return
	}
	text := msg.Text
	if text == "" {
		text = "(empty response)"
	}
	m.ChatHistory[idx] = ChatMessage{Kind: chatAssistant, Text: text}
	m.updateSessionPreview()
}

func (m *Model) OpenSearch() {
	m.Mode = ModeSearch
	m.SearchQuery = ""
	m.SearchMatches = nil
	m.SearchInput.SetValue("")
	m.SearchInput.Placeholder = "Search chat..."
	m.SearchInput.Focus()
}

func (m *Model) CloseSearch() {
	m.Mode = ModeNormal
	m.SearchInput.Blur()
	m.SearchQuery = ""
	m.SearchMatches = nil
}

func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.CloseSearch()
		return m, nil
	case "enter":
		if len(m.SearchMatches) > 0 {
			idx := m.ChatFocusIdx
			found := -1
			for _, mi := range m.SearchMatches {
				if mi > idx {
					found = mi
					break
				}
			}
			if found < 0 {
				found = m.SearchMatches[0]
			}
			m.ChatFocusIdx = found
		}
		return m, nil
	case "backspace":
		val := m.SearchInput.Value()
		if len(val) > 0 {
			m.SearchInput.SetValue(val[:len(val)-1])
		}
		m.updateSearchMatches()
		return m, nil
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			m.SearchInput.SetValue(m.SearchInput.Value() + key)
			m.updateSearchMatches()
		}
		return m, nil
	}
}

func (m *Model) updateSearchMatches() {
	m.SearchQuery = m.SearchInput.Value()
	m.SearchMatches = nil
	if m.SearchQuery == "" {
		return
	}
	q := strings.ToLower(m.SearchQuery)
	for i, msg := range m.ChatHistory {
		text := strings.ToLower(msg.Text + " " + msg.Detail + " " + msg.Tool)
		if strings.Contains(text, q) {
			m.SearchMatches = append(m.SearchMatches, i)
		}
	}
}

func (m *Model) updateChatFocusFromViewport() {
	yOffset := m.ChatViewport.YOffset()
	if yOffset < 0 {
		yOffset = 0
	}
	if yOffset < len(m.ChatHistory) {
		m.ChatFocusIdx = yOffset
	}
}

func copyToClipboard(text string) {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
}

func (m *Model) handleMouseAction(action MouseAction) tea.Cmd {
	switch action.Target {
	case "sidebar":
		return nil
	case "chat":
		if m.ViewKind == ViewChat && m.ChatInput != nil {
			return m.ChatInput.Focus()
		}
		return nil
	case "tabs":
		return nil
	case "footer":
		return nil
	case "right_panel":
		return nil
	}

	if action.Kind == "scroll_up" {
		if m.ViewKind == ViewChat {
			m.ChatViewport.ScrollUp(1)
		}
	}
	if action.Kind == "scroll_down" {
		if m.ViewKind == ViewChat {
			m.ChatViewport.ScrollDown(1)
		}
	}
	return nil
}
