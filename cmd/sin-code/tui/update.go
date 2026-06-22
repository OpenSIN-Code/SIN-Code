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

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/usage"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
)

var keymap = DefaultKeymap()

// clamp returns v constrained to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// streamTickInterval is how often the live footer updates while streaming.
const streamTickInterval = 250 * time.Millisecond

// streamTickCmd returns a command that waits for the streaming tick interval
// and then sends a StreamTickMsg. The Update loop keeps re-scheduling it while
// IsStreaming() is true.
func streamTickCmd() tea.Cmd {
	return tea.Tick(streamTickInterval, func(time.Time) tea.Msg { return StreamTickMsg{} })
}

// Additional bindings not in the keymap struct
var (
	keyBannerOpen    = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open banner"))
	keyBannerDismiss = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "dismiss banner"))
	keyBannerNext    = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next banner"))
	keyPgUp          = key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "scroll up"))
	keyPgDn          = key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "scroll down"))
	keyDiffPopup     = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "diff popup"))
	keyToolTree      = key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("^t", "tool tree"))
	keySessionTree   = key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^y", "session tree"))
	keyVerifyFull    = key.NewBinding(key.WithKeys("ctrl+v"), key.WithHelp("^v", "verify panel"))
	keyDebugLayout   = key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("^l", "debug layout"))
	keyInlineDiff    = key.NewBinding(key.WithKeys("ctrl+i"), key.WithHelp("^i", "inline diff"))
	keyClosePreview  = key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("^f", "close preview"))
	keySplitPane     = key.NewBinding(key.WithKeys("f2"), key.WithHelp("F2", "split pane"))
	keyLeft          = key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left"))
	keyRight         = key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right"))
	keyUp            = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	keyDown          = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
)

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.Spinner.Init(),
		ListenForNotifications(),
		RefreshTodosCmd(),
		RefreshSessionTreeCmd(),
		RefreshLSPCmd(),
		InitGitRefresh(),
	}
	return tea.Batch(cmds...)
}

func SetKeymap(k Keymap) { keymap = k }

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
	old := m.ViewKind
	m.ViewKind = v
	m.Sidebar.SetSelectedView(v)
	m.Footer.SetView(v)
	if m.Footer.Transition != nil && old != v {
		fwd := (int(v) - int(old) + viewCount) % viewCount
		bwd := (int(old) - int(v) + viewCount) % viewCount
		if fwd <= bwd {
			m.Footer.Transition.Start(TransitionSlideLeft)
		} else {
			m.Footer.Transition.Start(TransitionSlideRight)
		}
	}
}

func (m *Model) NextView() {
	m.SwitchView(ViewKind((int(m.ViewKind) + 1) % viewCount))
}

func (m *Model) PrevView() {
	v := int(m.ViewKind) - 1
	if v < 0 {
		v = viewCount - 1
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
	matches := fuzzyFilter(m.Palette.Items, query)
	filtered := make([]string, 0, len(matches))
	for _, fm := range matches {
		filtered = append(filtered, fm.Item)
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
		m.Footer.TickOverlays()
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
		if m.KanbanView != nil {
			m.KanbanView.SetTodos(msg.Items)
		}
		return m, nil

	case TodosRefreshMsg:
		m.Sidebar.TodoOpen = msg.Counts.Open
		m.Sidebar.TodoBlocked = msg.Counts.Blocked
		m.Sidebar.TodoOverdue = msg.Counts.Overdue
		m.Sidebar.TodoReady = msg.Counts.Ready
		m.TodoItems = msg.Items
		if m.TodoSel >= len(m.TodoItems) {
			m.TodoSel = 0
		}
		if m.KanbanView != nil {
			m.KanbanView.SetTodos(msg.Items)
		}
		return m, nil

	case BannerKeyMsg:
		return m, nil

	case chat.ChatResponseMsg:
		m.handleChatResponse(msg)
		return m, nil

	case ChatChunkMsg:
		m.updatePromptDuration()
		if msg.Idx >= 0 && msg.Idx < len(m.ChatHistory) {
			m.ChatHistory[msg.Idx].Kind = chatAssistant
			m.ChatHistory[msg.Idx].Text += msg.Text
		}
		if msg.EstimatedTokens > 0 {
			m.Footer.Tokens = msg.EstimatedTokens
			m.Footer.TokensPct = clamp(float64(msg.EstimatedTokens)/128000.0, 0, 1)
			m.Footer.Cost = fmt.Sprintf("$%.2f", usage.ComputeCost(m.Footer.ModelName, msg.EstimatedTokens))
		}
		return m, nil

	case ChatCopyMsg:
		return m, nil

	case StreamTickMsg:
		m.updatePromptDuration()
		if m.IsStreaming() {
			return m, streamTickCmd()
		}
		return m, nil

	case AgentRunnerMsg:
		m.handleAgentRunnerEvent(msg)
		if msg.Closed {
			m.Footer.ShowToast(ToastSuccess, "Agent run complete")
		}
		if !msg.Closed && m.AgentRunner != nil {
			cmds = append(cmds, listenAgentRunnerCmd(m.AgentRunner))
		}
		// Once streaming starts, schedule the live ticker for the footer.
		if m.IsStreaming() {
			cmds = append(cmds, streamTickCmd())
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

	case ReloadMsg:
		HandleReload(m)
		m.Footer.ShowToast(ToastInfo, "Session saved")
		return m, nil

	case VerifyUpdateMsg:
		HandleVerifyUpdate(&m.VerifyPanel, msg)
		switch msg.State {
		case VerifyPassed:
			m.Footer.ShowToast(ToastSuccess, "Verified")
		case VerifyFailed:
			m.Footer.ShowToast(ToastError, "Verification failed")
		}
		return m, nil

	case ToolCallTreeMsg:
		if m.ToolTree == nil {
			m.ToolTree = &ToolCallTree{}
		}
		m.ToolTree.AddNode(msg.ParentID, msg.Node)
		return m, nil

	case ToolCallUpdateMsg:
		if m.ToolTree != nil {
			m.ToolTree.UpdateNode(msg.ID, msg.Status, msg.Output, msg.Duration, msg.Error)
		}
		return m, nil

	case SessionTreeMsg:
		m.SessionTree = BuildSessionTree(msg.Sessions)
		return m, nil

	case LSPDiagnosticsMsg:
		HandleLSPDiagnostics(&m.LSPState, msg)
		return m, nil

	case DiffAcceptMsg:
		st := PendingDiff()
		if st.Pending {
			_ = ApplyDiff(st.FilePath, st.NewContent)
		}
		ClearPendingDiff()
		m.DiffPopupOpen = false
		return m, nil

	case DiffRejectMsg:
		ClearPendingDiff()
		m.DiffPopupOpen = false
		return m, nil

	case tea.KeyPressMsg:
		// Ctrl+X and Ctrl+C quit immediately from any view/mode
		if msg.Code == 'x' && msg.Mod&tea.ModCtrl != 0 {
			m.saveChatHistory()
			m.Quitting = true
			return m, tea.Quit
		}
		if msg.Code == 'c' && msg.Mod&tea.ModCtrl != 0 {
			m.saveChatHistory()
			m.Quitting = true
			return m, tea.Quit
		}
		if m.NotificationBanner != nil {
			k := msg.String()
			if k == "o" || k == "d" || k == "n" {
				return m.handleKey(msg)
			}
		}
		if m.DiffPopupOpen {
			k := msg.String()
			if k == "y" {
				return m, HandleDiffAccept()
			}
			if k == "n" {
				return m, HandleDiffReject()
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
	// Quit keys handled at top of Update — never route to handleKey
	if key == "ctrl+s" || key == "ctrl+enter" || key == "ctrl+d" || key == "ctrl+x" || key == "ctrl+c" {
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
	case key == "?":
		return true
	case key == "f2":
		return true
	case key >= "0" && key <= "9":
		return true // view jump keys (0-9)
	case key == "y" || key == "n":
		return true // permission dialog (when pendingAsk is set)
	}
	return false
}

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
	case key.Matches(msg, keymap.Palette):
		m.OpenPalette()
		return m, nil
	case key.Matches(msg, keymap.Search):
		m.OpenSearch()
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
			m.ChatViewport.PageUp()
			m.updateChatFocusFromViewport()
			return m, nil
		}
	case key.Matches(msg, keyPgDn):
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

func (m *Model) PreviousView() {
	v := int(m.ViewKind) - 1
	if v < 0 {
		v = viewCount - 1
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
	case "view: dag":
		m.SwitchView(ViewDAG)
	case "view: context":
		m.SwitchView(ViewContextViz)
	case "view: dashboard":
		m.SwitchView(ViewAgentDashboard)
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

	if m.ViewKind == ViewChat && m.SlashAutocomplete != nil && m.SlashAutocomplete.Active() {
		popup := m.SlashAutocomplete.Render(m.Styles, min(m.Width-8, 70))
		layout = lipgloss.Place(m.Width, m.Height, lipgloss.Left, lipgloss.Bottom, popup)
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

func (m *Model) handleChatResponse(msg chat.ChatResponseMsg) {
	m.updatePromptDuration()
	m.setStreaming(false)
	if msg.Tokens > 0 {
		m.Footer.Tokens += msg.Tokens
		m.Footer.TokensPct = clamp(float64(m.Footer.Tokens)/128000.0, 0, 1)
		m.Footer.Cost = fmt.Sprintf("$%.2f", usage.ComputeCost(m.Footer.ModelName, m.Footer.Tokens))
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
	m.OpenChatSearch()
}

func (m *Model) CloseSearch() {
	m.CloseChatSearch()
}

func (m *Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.CloseChatSearch()
		return m, nil
	case "enter":
		if m.ChatSearch != nil && m.ChatSearch.CurrentResult() != nil {
			r := m.ChatSearch.CurrentResult()
			m.ChatFocusIdx = r.MessageIdx
			m.SearchQuery = m.SearchInput.Value()
		} else if len(m.SearchMatches) > 0 {
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
	case "up":
		if m.ChatSearch != nil {
			m.ChatSearch.Prev()
		}
		return m, nil
	case "down":
		if m.ChatSearch != nil {
			m.ChatSearch.Next()
		}
		return m, nil
	case "backspace":
		val := m.SearchInput.Value()
		if len(val) > 0 {
			m.SearchInput.SetValue(val[:len(val)-1])
		}
		m.updateSearchMatches()
		if m.ChatSearch != nil {
			m.ChatSearch.Search(m.ChatHistory, m.SearchInput.Value())
		}
		return m, nil
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			m.SearchInput.SetValue(m.SearchInput.Value() + key)
			m.updateSearchMatches()
			if m.ChatSearch != nil {
				m.ChatSearch.Search(m.ChatHistory, m.SearchInput.Value())
			}
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
		m.ChatViewport.ScrollUp(3)
	}
	return nil
}

func (m *Model) handleMouseScrollDown(action MouseResolution) tea.Cmd {
	if m.ViewKind == ViewChat {
		m.ChatViewport.ScrollDown(3)
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
