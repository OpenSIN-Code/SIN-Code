package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	agentrunner "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/tui"
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
	keyDiffPopup     = key.NewBinding(key.WithKeys("f7"), key.WithHelp("F7", "diff popup"))
	keyToolTree      = key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("^t", "tool tree"))
	keySessionTree   = key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^y", "session tree"))
	keyVerifyFull    = key.NewBinding(key.WithKeys("ctrl+v"), key.WithHelp("^v", "verify panel"))
	keyDebugLayout   = key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("^l", "debug layout"))
	keyInlineDiff    = key.NewBinding(key.WithKeys("ctrl+i"), key.WithHelp("^i", "inline diff"))
	keyClosePreview  = key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("^f", "close preview"))
	keyDiffApproval  = key.NewBinding(key.WithKeys("f8"), key.WithHelp("F8", "approve diff"))
	keyBlockToggle   = key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("^g", "toggle block"))
	keySplitPane     = key.NewBinding(key.WithKeys("f2"), key.WithHelp("F2", "split pane"))
	keyCopyMode      = key.NewBinding(key.WithKeys("f9"), key.WithHelp("F9", "copy mode"))
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
		if m.TypewriterBuf != nil {
			m.TypewriterBuf.Append(msg.Text)
		}
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
		if m.TypewriterBuf != nil {
			m.TypewriterBuf.Tick()
		}
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
		case VerifyRunning, VerifyPending:
			m.Footer.SetVerifyGate(VerifyGateRunning, msg.Mode)
		case VerifyPassed:
			m.Footer.SetVerifyGate(VerifyGatePassed, msg.Mode)
			m.Footer.ShowToast(ToastSuccess, "Verified")
		case VerifyFailed:
			m.Footer.SetVerifyGate(VerifyGateFailed, msg.Mode)
			m.Footer.ShowToast(ToastError, "Verification failed")
		case VerifyBlocked:
			m.Footer.SetVerifyGate(VerifyGateFailed, msg.Mode)
		case VerifyIdle:
			m.Footer.SetVerifyGate(VerifyGateIdle, msg.Mode)
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
	// ctrl+d and ctrl+e are standard text-editing shortcuts (delete-forward,
	// end-of-line) that must reach the textarea, not be intercepted as global
	// hotkeys. ctrl+a is still used by keymap.Subagents so it stays global.
	if key == "ctrl+s" || key == "ctrl+enter" || key == "ctrl+d" || key == "ctrl+x" || key == "ctrl+c" || key == "ctrl+e" {
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

func (m *Model) OpenDiffApprovalFromInlineDiff() {
	if m.DiffApproval == nil {
		m.DiffApproval = NewDiffApproval(m.Styles)
	}
	diffs := RecentDiffs()
	if len(diffs) == 0 {
		return
	}
	last := diffs[len(diffs)-1]
	diffText := computeUnifiedDiffText(last.Before, last.After, last.Path)
	filePath := last.Path
	m.DiffApproval.Styles = m.Styles
	m.DiffApproval.Width = min(m.Width-4, 80)
	m.DiffApproval.Height = min(m.Height-4, 24)
	m.DiffApproval.Show(filePath, diffText)
	m.Mode = ModeDiffApproval
}

func (m *Model) handleDiffApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.DiffApproval == nil || !m.DiffApproval.Open {
		m.Mode = ModeNormal
		return m, nil
	}
	k := msg.String()
	switch k {
	case "enter":
		choice := m.DiffApproval.Choice()
		m.DiffApproval.Close()
		m.Mode = ModeNormal
		switch choice {
		case "approve":
			st := PendingDiff()
			if st.Pending {
				_ = ApplyDiff(st.FilePath, st.NewContent)
			}
			ClearPendingDiff()
			m.Footer.ShowToast(ToastSuccess, "Diff approved")
		case "reject":
			ClearPendingDiff()
			m.Footer.ShowToast(ToastInfo, "Diff rejected")
		case "edit":
			m.Footer.ShowToast(ToastInfo, "Edit mode — return to chat")
		}
		return m, nil
	case "esc":
		m.DiffApproval.Close()
		m.Mode = ModeNormal
		ClearPendingDiff()
		return m, nil
	case "tab", "right", "l":
		m.DiffApproval.Next()
		return m, nil
	case "shift+tab", "left", "h":
		m.DiffApproval.Prev()
		return m, nil
	case "up", "k":
		m.DiffApproval.Prev()
		return m, nil
	case "down", "j":
		m.DiffApproval.Next()
		return m, nil
	}
	return m, nil
}

// EnterCopyMode activates copy mode with the current chat history flattened
// into lines.
func (m *Model) EnterCopyMode() {
	lines := m.flattenChatLines()
	if m.CopyMode == nil {
		m.CopyMode = NewCopyMode(m.Styles)
	}
	m.CopyMode.Styles = m.Styles
	m.CopyMode.Enter(lines)
	m.Mode = ModeCopy
}

// ExitCopyMode deactivates copy mode and returns to normal mode.
func (m *Model) ExitCopyMode() {
	if m.CopyMode != nil {
		m.CopyMode.Exit()
	}
	m.Mode = ModeNormal
}

// flattenChatLines converts the chat history into a flat list of text lines
// suitable for the copy mode overlay.
func (m *Model) flattenChatLines() []string {
	var lines []string
	for _, msg := range m.ChatHistory {
		var label string
		switch msg.Kind {
		case chatUser:
			label = "USER"
		case chatAssistant:
			label = "ASSISTANT"
		case chatTool:
			label = "TOOL: " + msg.Tool
		case chatError:
			label = "ERROR"
		case chatSystem:
			label = "SYSTEM"
		default:
			label = "MSG"
		}
		lines = append(lines, "["+label+"]")
		text := msg.Text
		if text == "" {
			text = msg.Detail
		}
		if text == "" && msg.Tool != "" {
			text = msg.ToolInput
			if msg.ToolOutput != "" {
				text += "\n" + msg.ToolOutput
			}
		}
		if text != "" {
			lines = append(lines, strings.Split(text, "\n")...)
		}
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		lines = []string{"(no chat history)"}
	}
	return lines
}

func (m *Model) handleCopyModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.CopyMode == nil || !m.CopyMode.Active {
		m.Mode = ModeNormal
		return m, nil
	}
	k := msg.String()
	switch k {
	case "q", "esc":
		m.ExitCopyMode()
		return m, nil
	case "j", "down":
		m.CopyMode.Down()
		return m, nil
	case "k", "up":
		m.CopyMode.Up()
		return m, nil
	case "pgdown", "ctrl+d":
		m.CopyMode.PageDown()
		return m, nil
	case "pgup", "ctrl+u":
		m.CopyMode.PageUp()
		return m, nil
	case "g":
		m.CopyMode.Top()
		return m, nil
	case "G":
		m.CopyMode.Bottom()
		return m, nil
	case "v":
		m.CopyMode.ToggleVisual()
		return m, nil
	case "y":
		text := m.CopyMode.Yank()
		m.ExitCopyMode()
		if text != "" {
			m.SetBanner(&NotificationItem{
				ID:      fmt.Sprintf("yank-%d", time.Now().UnixNano()),
				Title:   "Yanked!",
				Message: "Selected text copied to clipboard",
				Type:    "info",
			})
		}
		return m, nil
	case "Y":
		text := m.CopyMode.YankAll()
		m.ExitCopyMode()
		if text != "" {
			m.SetBanner(&NotificationItem{
				ID:      fmt.Sprintf("yank-all-%d", time.Now().UnixNano()),
				Title:   "Yanked!",
				Message: "All text copied to clipboard",
				Type:    "info",
			})
		}
		return m, nil
	}
	return m, nil
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

// Merged from chat_view.go (issue #426).

func (m *Model) renderChat(styles Styles, width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 6 {
		height = 6
	}

	textHeight := 3
	if m.ChatInput != nil {
		raw := m.ChatInput.RawValue()
		lines := strings.Count(raw, "\n") + 1
		if lines > 3 {
			textHeight = min(lines+2, 10)
		}
	}
	chatHeight := height - textHeight - 3
	if chatHeight < 3 {
		chatHeight = 3
	}

	modelName := m.Footer.ModelName
	if modelName == "" {
		modelName = m.Footer.AgentName()
	}
	m.SessionInfo.Update(shortSessionID(m.Tabs.Active().Name), modelName, countUserTurns(m.ChatHistory), m.VerifyPanel.State == VerifyPassed)
	m.TokenBar.Update(m.Footer.Tokens, parseCostStr(m.Footer.Cost), modelName)

	if len(m.ChatHistory) == 0 {
		inputEmpty := true
		if m.ChatInput != nil {
			inputEmpty = m.ChatInput.RawValue() == ""
		}

		var viewportContent string
		if inputEmpty {
			info := WelcomeInfo{
				ModelName:  modelName,
				Session:    "new",
				Workspace:  m.Workspace,
				VerifyMode: m.AgentConfig.VerifyMode,
			}
			viewportContent = RenderWelcome(styles, info, width, chatHeight)
		}

		m.ChatViewport.SetWidth(width)
		m.ChatViewport.SetHeight(chatHeight)
		m.ChatViewport.SetContent(viewportContent)
		m.ChatViewport.GotoTop()

		var b strings.Builder
		b.WriteString(m.SessionInfo.Render(styles, width))
		b.WriteString("\n")
		b.WriteString(m.ChatViewport.View())
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render(strings.Repeat("─", width)))
		b.WriteString("\n")
		b.WriteString(m.TokenBar.Render(styles, width))
		b.WriteString("\n")
		if m.ContextMeter != nil {
			m.ContextMeter.SetUsage(m.Footer.Tokens, m.Footer.EstimatedTokens)
			m.ContextMeter.Width = min(30, width/3)
			meterLine := m.ContextMeter.Render()
			if meterLine != "" {
				b.WriteString(meterLine)
				b.WriteString("\n")
			}
		}
		if m.ChatInput != nil {
			m.ChatInput.SetSize(width, textHeight)
			b.WriteString(m.ChatInput.View())
		}
		return b.String()
	}

	highlighter := NewSyntaxHighlighter(styles.Theme)

	var content strings.Builder
	isStreaming := m.Footer.Streaming
	compact := m.CompactMode != nil && m.CompactMode.Active()

	for i, msg := range m.ChatHistory {
		if compact {
			content.WriteString(renderCompactMessage(msg, styles, width, i == m.ChatFocusIdx, m.Spinner))
			continue
		}
		isLast := i == len(m.ChatHistory)-1
		msgStreaming := isLast && isStreaming && (msg.Kind == chatAssistant || msg.Kind == chatThinking)

		if msgStreaming && msg.Kind == chatAssistant && m.TypewriterBuf != nil {
			visible := m.TypewriterBuf.Visible()
			if visible != "" {
				msg.Text = visible
			}
		}

		block := ChatBlock{
			Role:      chatKindString(msg.Kind),
			Timestamp: msg.Timestamp,
			Collapsed: false,
			Width:     width,
		}
		if msg.Kind == chatVerify {
			if strings.Contains(msg.Detail, "PASS") {
				block.VerifyResult = "pass"
			} else if strings.Contains(msg.Detail, "FAIL") {
				block.VerifyResult = "fail"
			}
		}
		if msg.Kind == chatTool {
			block.ToolCalls = 1
		}
		content.WriteString(RenderBlockHeader(block, styles, width))
		content.WriteString("\n")

		rendered := renderChatMessageV2(msg, highlighter, styles, width, i == m.ChatFocusIdx, m.Spinner, msgStreaming)
		content.WriteString(rendered)

		if i < len(m.ChatHistory)-1 {
			content.WriteString("\n")
		}
	}

	m.ChatViewport.SetWidth(width)
	m.ChatViewport.SetHeight(chatHeight)
	m.ChatViewport.SetContent(content.String())
	if !m.ChatViewport.AtBottom() {
		m.ChatViewport.GotoBottom()
	}

	var b strings.Builder
	b.WriteString(m.SessionInfo.Render(styles, width))
	b.WriteString("\n")
	b.WriteString(m.ChatViewport.View())

	if m.Mode == ModeSearch {
		b.WriteString("\n")
		if m.ChatSearch != nil {
			b.WriteString(m.ChatSearch.RenderBar(styles))
		} else {
			b.WriteString(styles.AccentText.Render("/" + m.SearchInput.View()))
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width)))
	b.WriteString("\n")
	b.WriteString(m.TokenBar.Render(styles, width))
	b.WriteString("\n")
	if m.ContextMeter != nil {
		m.ContextMeter.SetUsage(m.Footer.Tokens, m.Footer.EstimatedTokens)
		m.ContextMeter.Width = min(30, width/3)
		meterLine := m.ContextMeter.Render()
		if meterLine != "" {
			b.WriteString(meterLine)
			b.WriteString("\n")
		}
	}
	if m.ChatInput != nil {
		m.ChatInput.SetSize(width, textHeight)
		b.WriteString(m.ChatInput.View())
	}

	return b.String()
}

func renderChatMessageV2(msg ChatMessage, highlighter *SyntaxHighlighter, styles Styles, width int, focused bool, spinner Spinner, streaming bool) string {
	var b strings.Builder

	focusPrefix := ""
	if focused {
		focusPrefix = "▸ "
	}

	switch msg.Kind {
	case chatUser:
		b.WriteString(renderUserBubble(msg, styles, width))
		b.WriteString("\n")

	case chatAssistant:
		b.WriteString(renderAssistantBubble(msg, highlighter, styles, width, streaming, spinner))
		b.WriteString("\n")

	case chatTool:
		b.WriteString(renderToolCard(msg, styles, width, focused))
		if isFileModifyingTool(msg.Tool) {
			diffText := extractDiffFromOutput(msg.ToolOutput)
			if diffText == "" {
				diffText = extractDiffFromOutput(msg.Detail)
			}
			if diffText != "" {
				renderer := NewDiffRenderer(styles)
				compact := renderer.RenderCompact(diffText, styles, width-4)
				if compact != "" {
					b.WriteString(compact)
					b.WriteString("\n")
				}
			}
		}
		b.WriteString("\n")

	case chatVerify:
		status := "pending"
		if strings.Contains(msg.Detail, "PASS") {
			status = "pass"
		} else if strings.Contains(msg.Detail, "FAIL") {
			status = "fail"
		}
		b.WriteString(renderVerificationCompact(status, msg.Detail, styles))
		b.WriteString("\n")

	case chatAsk:
		b.WriteString(styles.StatusWarn.Render(focusPrefix + "🔒 " + msg.Detail))
		b.WriteString("\n")

	case chatDone:
		detail := msg.Detail
		if len(detail) > 80 {
			detail = detail[:77] + "..."
		}
		b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + detail))
		b.WriteString("\n")

	case chatError:
		b.WriteString(renderErrorBubble(msg, styles, width))

	case chatThinking:
		anim := NewThinkingAnimation()
		anim.SetFrame(spinner.Frame() % len(thinkingFrames))
		anim.SetStart(msg.Timestamp)
		b.WriteString(anim.Render(styles))
		b.WriteString("\n")

	case chatSystem:
		b.WriteString(renderSystemBubble(msg, styles, width))

	case chatAgent:
		b.WriteString(styles.Muted.Render(focusPrefix + "⟳ " + msg.Text))
		b.WriteString("\n")
	}

	return b.String()
}

func renderThinkingIndicator(spinner Spinner, styles Styles) string {
	anim := NewThinkingAnimation()
	anim.SetFrame(spinner.Frame() % len(thinkingFrames))
	return anim.Render(styles)
}

func renderMarkdownWithCodeBlocks(text string, highlighter *SyntaxHighlighter, styles Styles, width int) string {
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	var b strings.Builder
	var textBuf strings.Builder
	inBlock := false
	var blockLang string
	var blockBuf strings.Builder

	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		rendered := renderMarkdownSimple(textBuf.String(), styles, width)
		rendered = LinkifyText(rendered, styles)
		b.WriteString(strings.TrimRight(rendered, "\n"))
		textBuf.Reset()
	}

	flushCode := func() {
		code := strings.TrimSuffix(blockBuf.String(), "\n")
		code = strings.TrimPrefix(code, "\n")
		if code != "" {
			rendered := renderCodeBlock(code, blockLang, highlighter, styles, width, false)
			b.WriteString(rendered)
		}
		blockBuf.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inBlock {
				flushText()
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				inBlock = true
				blockLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			} else {
				flushCode()
				b.WriteString("\n")
				inBlock = false
				blockLang = ""
			}
		} else if inBlock {
			blockBuf.WriteString(line)
			blockBuf.WriteString("\n")
		} else {
			textBuf.WriteString(line)
			textBuf.WriteString("\n")
		}
	}

	if inBlock {
		flushCode()
	} else {
		flushText()
	}

	return b.String()
}

func renderMarkdownSimple(text string, styles Styles, width int) string {
	if text == "" {
		return ""
	}
	if !hasMarkdownSyntax(text) {
		return strings.TrimRight(text, "\n")
	}
	r := getCachedRenderer(width)
	if r == nil {
		return strings.TrimRight(text, "\n")
	}
	rendered, err := r.Render(text)
	if err != nil {
		return strings.TrimRight(text, "\n")
	}
	return strings.TrimSpace(rendered)
}

func renderChatMessageCompact(msg ChatMessage, md *markdownRenderer, styles Styles, width int, focused bool, spinner Spinner) string {
	highlighter := NewSyntaxHighlighter(styles.Theme)
	isStreaming := msg.Kind == chatThinking
	return renderChatMessageV2(msg, highlighter, styles, width, focused, spinner, isStreaming)
}

func renderVerificationCompact(status, message string, styles Styles) string {
	switch status {
	case "pass":
		return styles.StatusOK.Render("✓ " + message)
	case "fail":
		return styles.StatusErr.Render("✗ " + message)
	default:
		return styles.StatusWarn.Render("⏳ " + message)
	}
}

func (m *Model) chatViewHelp() string {
	if m.ChatInput == nil {
		return "Ctrl+S submit · /attach <path> · /clear"
	}
	return fmt.Sprintf("Ctrl+S submit · /attach <path> · /clear · %d attachments", len(m.ChatInput.Attachments()))
}

func (m *Model) renderChatFooter(styles Styles, width int) string {
	if width < 20 {
		width = 20
	}

	var b strings.Builder

	tokens := m.Footer.Tokens
	tokensPct := m.Footer.TokensPct
	cost := m.Footer.Cost
	agent := m.Footer.AgentName()

	left := styles.FooterKey.Render(" " + agent + " ")
	mid := styles.Muted.Render(fmt.Sprintf("tokens %d (%.0f%%)", tokens, tokensPct*100))
	right := styles.FooterVal.Render(cost)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap > lipgloss.Width(mid) {
		mid += strings.Repeat(" ", gap-lipgloss.Width(mid))
	}

	b.WriteString(styles.Footer.Render(left + mid + right))
	return b.String()
}

func (m *Model) updateChatMetrics(tokens int, cost float64, duration time.Duration) {
	m.Footer.Tokens = tokens
	m.Footer.Cost = fmt.Sprintf("$%.2f", cost)
	m.Footer.Duration = duration
	m.Footer.TokensPct = float64(tokens) / 128000.0
	if m.Footer.TokensPct > 1.0 {
		m.Footer.TokensPct = 1.0
	}

	if m.Footer.TokensPct >= 0.8 && !m.Footer.Compacted {
		m.autoCompactContext()
	}
}

func (m *Model) autoCompactContext() {
	if len(m.ChatHistory) <= 50 {
		return
	}

	summary := fmt.Sprintf("[context compacted: %d messages removed to free up space]", len(m.ChatHistory)-20)
	m.ChatHistory = append([]ChatMessage{{Kind: chatSystem, Text: summary}}, m.ChatHistory[len(m.ChatHistory)-20:]...)
	m.Footer.Compacted = true

	m.SetBanner(&NotificationItem{
		ID:      "auto-compact",
		Title:   "Context Compacted",
		Message: fmt.Sprintf("Reduced from %d to 20 messages to stay within token limit", len(m.ChatHistory)),
		Type:    "info",
	})

	m.AppendHistory(ViewChat.String(), "auto-compact", summary, true)
}

func (m *Model) setStreaming(streaming bool) {
	m.Footer.Streaming = streaming
	if streaming && m.TypewriterBuf != nil {
		m.TypewriterBuf.Reset()
	}
}

// IsStreaming reports whether the model is currently receiving a streaming
// response or waiting for the agent loop to produce output.
func (m *Model) IsStreaming() bool {
	return m.Footer.Streaming
}

func (m *Model) updateSessionPreview() {
	if len(m.ChatHistory) == 0 {
		return
	}

	for i := len(m.ChatHistory) - 1; i >= 0; i-- {
		msg := m.ChatHistory[i]
		if msg.Kind == chatAssistant {
			preview := msg.Text
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			m.Tabs.UpdatePreview(m.Tabs.ActiveIdx, preview)
			return
		}
	}
}

func renderToolCall(name, args string, styles Styles) string {
	var b strings.Builder
	b.WriteString(styles.AccentText.Render("⚡ "))
	b.WriteString(styles.Bold.Render(name))
	if args != "" {
		b.WriteString(styles.Muted.Render(" " + args))
	}
	return b.String()
}

func renderVerification(status, message string, styles Styles) string {
	var b strings.Builder
	switch status {
	case "pass":
		b.WriteString(styles.StatusOK.Render("✅ "))
		b.WriteString(styles.StatusOK.Render(message))
	case "fail":
		b.WriteString(styles.StatusErr.Render("❌ "))
		b.WriteString(styles.StatusErr.Render(message))
	default:
		b.WriteString(styles.StatusWarn.Render("⏳ "))
		b.WriteString(styles.StatusWarn.Render(message))
	}
	return b.String()
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderError(err string, styles Styles) string {
	return styles.StatusErr.Render("❌ " + err)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderErrorExpanded(msg ChatMessage, styles Styles, width int) string {
	return renderErrorBubble(msg, styles, width)
}

func countUserTurns(history []ChatMessage) int {
	n := 0
	for _, msg := range history {
		if msg.Kind == chatUser {
			n++
		}
	}
	return n
}

func looksLikeGoCode(s string) bool {
	indicators := []string{"func ", "package ", "import ", "type ", "var ", "const "}
	count := 0
	for _, ind := range indicators {
		if strings.Contains(s, ind) {
			count++
		}
	}
	return count >= 2 || (strings.Contains(s, "func ") && strings.Contains(s, "{"))
}

func renderToolOutput(output string, styles Styles, width int) string {
	if output == "" {
		return ""
	}
	if width < 10 {
		width = 10
	}

	highlighter := NewSyntaxHighlighter(styles.Theme)

	if looksLikeGoCode(output) {
		return renderCodeBlock(output, "go", highlighter, styles, width, false) + "\n"
	}

	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c(styles.Theme.Accent)).
		Padding(0, 1).
		Width(width - 2)

	return panelStyle.Render(styles.Content.Render(output))
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderSpinner(text string, styles Styles) string {
	return styles.AccentText.Render("⏳ " + text)
}

type chatBubbleStyles struct {
	user      lipgloss.Style
	assistant lipgloss.Style
	tool      lipgloss.Style
	system    lipgloss.Style
	error     lipgloss.Style
	success   lipgloss.Style
	warning   lipgloss.Style
	timestamp lipgloss.Style
}

func newChatBubbleStyles(styles Styles) chatBubbleStyles {
	t := styles.Theme
	return chatBubbleStyles{
		user: lipgloss.NewStyle().
			Foreground(c(t.Background)).
			Background(c(t.Accent)).
			Bold(true).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()),

		assistant: lipgloss.NewStyle().
			Foreground(c(t.Text)).
			PaddingLeft(2).
			PaddingRight(2),

		tool: lipgloss.NewStyle().
			Foreground(c(t.AccentDim)).
			PaddingLeft(2).
			PaddingRight(2),

		system: lipgloss.NewStyle().
			Foreground(c(t.TextDim)).
			Italic(true).
			Padding(0, 2),

		error: lipgloss.NewStyle().
			Foreground(c(t.Error)).
			BorderLeft(true).
			BorderLeftForeground(c(t.Error)).
			Background(lipgloss.Color(t.Background)).
			Padding(0, 1).
			Bold(true),

		success: lipgloss.NewStyle().
			Foreground(c(t.Success)).
			Bold(true).
			Padding(0, 2),

		warning: lipgloss.NewStyle().
			Foreground(c(t.Warn)).
			Bold(true).
			Padding(0, 2),

		timestamp: lipgloss.NewStyle().
			Foreground(c(t.TextDim)).
			Faint(true),
	}
}

func renderChatBubble(role, content string, styles chatBubbleStyles, width int) string {
	var b strings.Builder

	switch role {
	case "user":
		label := styles.user.Render(" You ")
		ts := styles.timestamp.Render(time.Now().Format("15:04:05"))
		b.WriteString(label)
		b.WriteString("  ")
		b.WriteString(ts)
		b.WriteString("\n")
		wrapped := wrapText(content, width-6)
		b.WriteString(wrapped)
	case "assistant":
		label := styles.assistant.Render("Assistant")
		ts := styles.timestamp.Render(time.Now().Format("15:04:05"))
		b.WriteString(label)
		b.WriteString("  ")
		b.WriteString(ts)
		b.WriteString("\n")
		wrapped := wrapText(content, width-6)
		b.WriteString(wrapped)
	case "tool":
		b.WriteString(styles.tool.Render("⚡ " + content))
	case "system":
		b.WriteString(styles.system.Render("⚠ " + content))
	case "error":
		b.WriteString(styles.error.Render("❌ " + content))
	case "success":
		b.WriteString(styles.success.Render("✅ " + content))
	case "warning":
		b.WriteString(styles.warning.Render("⚠ " + content))
	default:
		b.WriteString(content)
	}

	return b.String()
}

func wrapText(text string, width int) string {
	if width <= 0 {
		width = 80
	}

	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if len(paragraph) <= width {
			lines = append(lines, paragraph)
			continue
		}

		words := strings.Fields(paragraph)
		var currentLine strings.Builder
		for _, word := range words {
			if currentLine.Len()+len(word)+1 > width {
				if currentLine.Len() > 0 {
					lines = append(lines, currentLine.String())
					currentLine.Reset()
				}
			}
			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
		}
		if currentLine.Len() > 0 {
			lines = append(lines, currentLine.String())
		}
	}

	return strings.Join(lines, "\n")
}

func indentText(text string, indent int) string {
	prefix := strings.Repeat(" ", indent)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatDivider(styles Styles, width int) string {
	return styles.Muted.Render(strings.Repeat("─", width))
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatHeader(title string, styles Styles) string {
	return styles.ContentHdr.Render("💬 " + title)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func renderChatStatus(status string, styles Styles) string {
	return styles.Muted.Render(status)
}

type chatMsgKind int

const (
	chatUser chatMsgKind = iota
	chatAssistant
	chatAgent
	chatTool
	chatVerify
	chatAsk
	chatDone
	chatError
	chatThinking
	chatSystem
)

func chatKindString(k chatMsgKind) string {
	switch k {
	case chatUser:
		return "user"
	case chatAssistant:
		return "assistant"
	case chatAgent:
		return "agent"
	case chatTool:
		return "tool"
	case chatVerify:
		return "verify"
	case chatAsk:
		return "ask"
	case chatDone:
		return "done"
	case chatError:
		return "error"
	case chatThinking:
		return "thinking"
	case chatSystem:
		return "system"
	default:
		return "msg"
	}
}

type ChatMessage struct {
	ID         int64
	Kind       chatMsgKind
	Text       string
	Tool       string
	ToolInput  string
	ToolOutput string
	Detail     string
	Result     bool
	Timestamp  time.Time
	Tokens     int
	Error      error
	Expanded   bool
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05")
}

func renderUserBubble(msg ChatMessage, styles Styles, width int) string {
	var b strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Background)).
		Background(c(styles.Theme.Accent)).
		Bold(true).
		Padding(0, 1)

	tsStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Faint(true)

	label := labelStyle.Render("You")
	ts := tsStyle.Render(formatTimestamp(msg.Timestamp))

	prefixWidth := lipgloss.Width(label + "  " + ts)
	_ = prefixWidth
	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}
	wrapped := wrapText(msg.Text, bodyWidth)

	bodyStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		Background(lipgloss.Color(styles.Theme.Background)).
		Padding(0, 1).
		Width(bodyWidth)

	body := bodyStyle.Render(wrapped)

	rightAligned := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Right)

	content := label + "  " + ts + "\n" + body
	b.WriteString(rightAligned.Render(content))
	b.WriteString("\n")
	return b.String()
}

func renderAssistantBubble(msg ChatMessage, highlighter *SyntaxHighlighter, styles Styles, width int, streaming bool, spinner Spinner) string {
	var b strings.Builder

	labelStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Accent)).
		Bold(true)

	tsStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Faint(true)

	label := labelStyle.Render("Assistant")
	ts := tsStyle.Render(formatTimestamp(msg.Timestamp))

	headerLine := label + "  " + ts

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	rendered := renderMarkdownWithCodeBlocks(msg.Text, highlighter, styles, bodyWidth)

	if streaming {
		cursor := renderStreamingCursor(spinner, styles)
		rendered = strings.TrimRight(rendered, "\n") + cursor
	}

	bodyStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		PaddingLeft(2).
		PaddingRight(2).
		Width(bodyWidth)

	body := bodyStyle.Render(rendered)

	b.WriteString(headerLine)
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}

func renderSystemBubble(msg ChatMessage, styles Styles, width int) string {
	text := msg.Text
	if text == "" {
		text = msg.Detail
	}

	style := lipgloss.NewStyle().
		Foreground(c(styles.Theme.TextDim)).
		Italic(true).
		Align(lipgloss.Center).
		Width(width)

	return style.Render(text) + "\n"
}

func renderErrorBubble(msg ChatMessage, styles Styles, width int) string {
	errText := msg.Text
	if errText == "" && msg.Error != nil {
		errText = msg.Error.Error()
	}

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	style := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Error)).
		BorderLeft(true).
		BorderLeftForeground(c(styles.Theme.Error)).
		Background(lipgloss.Color(fmt.Sprintf("%s", styles.Theme.Background))).
		Padding(0, 1).
		Width(bodyWidth)

	if !msg.Expanded {
		short := truncateString(errText, bodyWidth-4)
		return style.Render("❌ "+short) + "\n"
	}

	body := style.Render(errText) + "\n" + styles.Muted.Render("  Press enter to collapse")
	return body + "\n"
}

func renderToolCard(msg ChatMessage, styles Styles, width int, focused bool) string {
	focusPrefix := ""
	if focused {
		focusPrefix = "▸ "
	}

	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}

	if msg.Expanded {
		var b strings.Builder

		iconStyle := lipgloss.NewStyle().
			Foreground(c(styles.Theme.Accent)).
			Bold(true)

		hdr := iconStyle.Render(focusPrefix + "⚡ " + msg.Tool)
		b.WriteString(hdr)
		b.WriteString("\n")

		if msg.ToolInput != "" {
			inputText := msg.ToolInput
			if len(inputText) > bodyWidth-10 {
				inputText = truncateString(inputText, bodyWidth-13)
			}
			b.WriteString(styles.Muted.Render("  in: "))
			b.WriteString(styles.Muted.Render(inputText))
			b.WriteString("\n")
		}

		output := msg.ToolOutput
		if output == "" {
			output = msg.Detail
		}
		if output != "" {
			rendered := renderToolOutput(output, styles, bodyWidth)
			b.WriteString(rendered)
		}

		cardStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(c(styles.Theme.AccentDim)).
			Padding(0, 1).
			Width(bodyWidth)

		return cardStyle.Render(b.String()) + "\n"
	}

	var b strings.Builder
	if msg.Result {
		b.WriteString(styles.StatusOK.Render(focusPrefix + "✓ " + msg.Tool))
	} else {
		b.WriteString(styles.AccentText.Render(focusPrefix + "⚡ " + msg.Tool))
	}
	if msg.Detail != "" {
		detail := msg.Detail
		if len(detail) > 60 {
			detail = detail[:57] + "..."
		}
		b.WriteString(styles.Muted.Render(" → " + detail))
	}
	b.WriteString("\n")
	return b.String()
}

func renderStreamingCursor(spinner Spinner, styles Styles) string {
	visible := spinner.pulse%2 == 0
	if visible {
		return styles.AccentText.Render("▋")
	}
	return " "
}

func renderTypingDots(spinner Spinner, styles Styles) string {
	phase := spinner.frame % 3
	switch phase {
	case 0:
		return styles.Muted.Render("·  ")
	case 1:
		return styles.Muted.Render("·· ")
	default:
		return styles.Muted.Render("···")
	}
}

// ============================================================================
// Agent runner adapter (merged from agent_runner_adapter.go)
// ============================================================================

type AgentRunnerMsg struct {
	Event  agentrunner.AgentEvent
	Closed bool
}

var newAgentRunnerHook = func(ctx context.Context, cfg agentrunner.Config) (*agentrunner.AgentRunner, error) {
	return agentrunner.NewAgentRunner(ctx, cfg)
}

var submitAgentRunnerHook = func(r *agentrunner.AgentRunner, ctx context.Context, prompt string) (<-chan struct{}, error) {
	return r.Submit(ctx, prompt)
}

func (m *Model) initAgentRunner() *agentrunner.AgentRunner {
	if m.AgentRunner != nil {
		return m.AgentRunner
	}
	ws := m.Workspace
	if ws == "" {
		ws = "."
	}
	r, err := newAgentRunnerHook(m.ctx(), agentrunner.Config{
		Workspace:   ws,
		Headless:    false,
		Yolo:        m.AgentConfig.Yolo,
		MaxTurns:    m.AgentConfig.MaxTurns,
		ToolFactory: tuiToolFactory(ws),
	})
	if err != nil {
		return nil
	}
	m.AgentRunner = r
	return r
}

func listenAgentRunnerCmd(r *agentrunner.AgentRunner) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-r.EventsChannel()
		if !ok {
			return AgentRunnerMsg{Closed: true}
		}
		return AgentRunnerMsg{Event: ev}
	}
}

func (m *Model) handleAgentRunnerEvent(msg AgentRunnerMsg) {
	m.updatePromptDuration()
	if msg.Closed {
		m.AgentRunner = nil
		m.resetPromptContext()
		m.setStreaming(false)
		return
	}
	// Ensure tool tree exists before sending any tool-call messages.
	if m.ToolTree == nil {
		m.ToolTree = &ToolCallTree{}
	}
	ev := msg.Event
	var cm ChatMessage
	switch ev.Kind {
	case agentrunner.EventTurn:
		cm = ChatMessage{Kind: chatAgent, Text: "turn start", Detail: ev.Detail}
	case agentrunner.EventTool:
		isResult := strings.HasPrefix(ev.Detail, "tool result")
		cm = ChatMessage{
			Kind:   chatTool,
			Tool:   ev.ToolName,
			Detail: ev.Detail,
			Result: isResult,
		}
		toolName := ev.ToolName
		if toolName == "" && strings.HasPrefix(ev.Detail, "tool: ") {
			toolName = strings.TrimPrefix(ev.Detail, "tool: ")
		}
		if ev.ToolCallID != "" {
			if ev.Detail == "tool start" {
				if m.Program != nil {
					m.Program.Send(ToolCallTreeMsg{
						ParentID: "",
						Node: &ToolCallNode{
							ID:        ev.ToolCallID,
							Tool:      ev.ToolName,
							Status:    "running",
							StartTime: ev.StartTime,
							Expanded:  false,
						},
					})
				}
			} else if ev.Detail == "tool result" {
				status := "success"
				errMsg := ""
				if ev.Err != nil {
					status = "error"
					errMsg = ev.Err.Error()
				}
				if m.Program != nil {
					m.Program.Send(ToolCallUpdateMsg{
						ID:       ev.ToolCallID,
						Status:   status,
						Duration: ev.Duration,
						Output:   ev.Result,
						Error:    errMsg,
					})
				}
			}
		} else if !isResult && toolName != "" {
			// New tool call starting — add node to tree.
			nodeID := fmt.Sprintf("tool-%d-%s", time.Now().UnixNano(), toolName)
			if m.Program != nil {
				m.Program.Send(ToolCallTreeMsg{
					ParentID: "",
					Node: &ToolCallNode{
						ID:        nodeID,
						Tool:      toolName,
						Status:    "running",
						StartTime: time.Now(),
						Expanded:  false,
					},
				})
			}
		} else if isResult && toolName != "" {
			// Tool call result — best-effort update by tool name.
			if m.Program != nil {
				m.Program.Send(ToolCallUpdateMsg{
					ID:     toolName,
					Status: "success",
					Output: ev.Detail,
				})
			}
		}
	case agentrunner.EventVerify:
		cm = ChatMessage{Kind: chatVerify, Detail: ev.Detail}
		vState := VerifyRunning
		d := ev.Detail
		if strings.Contains(d, "PASSED") || strings.Contains(d, "pass") {
			vState = VerifyPassed
		} else if strings.Contains(d, "FAILED") || strings.Contains(d, "fail") {
			vState = VerifyFailed
		} else if strings.Contains(d, "BLOCKED") || strings.Contains(d, "blocked") {
			vState = VerifyBlocked
		}
		if m.Program != nil {
			m.Program.Send(VerifyUpdateMsg{
				State:    vState,
				Mode:     "poc",
				Target:   ev.Detail,
				Evidence: ev.Result,
			})
		}
	case agentrunner.EventAsk:
		m.pendingAsk = ev.AskReply
		m.OpenPermissionDialog(ev.ToolName, ev.Detail, "")
		m.setStreaming(false)
		// Don't add to chat history — the permission dialog IS the
		// visual feedback. A lock entry would clutter the scrollback.
		return
	case agentrunner.EventUsage:
		m.Footer.Tokens = ev.Tokens
		m.Footer.TokensPct = clamp(float64(ev.Tokens)/128000.0, 0, 1)
		m.Footer.Cost = fmt.Sprintf("$%.2f", usage.ComputeCost(m.Footer.ModelName, ev.Tokens))
		return
	case agentrunner.EventDone:
		cm = ChatMessage{Kind: chatDone, Detail: ev.Result}
		m.setStreaming(false)
		m.resetPromptContext()
		if ev.Tokens > 0 {
			m.Footer.Tokens += ev.Tokens
			m.Footer.TokensPct = clamp(float64(m.Footer.Tokens)/128000.0, 0, 1)
			m.Footer.Cost = fmt.Sprintf("$%.2f", usage.ComputeCost(m.Footer.ModelName, m.Footer.Tokens))
		}
		if strings.Contains(strings.ToLower(ev.Result), "verified") && m.Program != nil {
			m.Program.Send(VerifyUpdateMsg{
				State:    VerifyPassed,
				Mode:     "poc",
				Target:   "agent run complete",
				Evidence: ev.Result,
			})
		}
	case agentrunner.EventError:
		cm = ChatMessage{Kind: chatError, Text: ev.Detail, Error: ev.Err}
		m.setStreaming(false)
		m.resetPromptContext()
	default:
		cm = ChatMessage{Kind: chatSystem, Text: ev.Detail}
	}
	m.appendChat(cm)
	m.AppendHistory(ViewChat.String(), "agent-event", cm.Detail, ev.Err == nil)
}

func (m *Model) answerPendingAsk(allow bool) {
	if m.pendingAsk == nil {
		return
	}
	ch := m.pendingAsk
	m.pendingAsk = nil
	select {
	case ch <- allow:
	case <-time.After(3 * time.Second):
	}
}

func (m *Model) submitAgentPrompt(prompt string) tea.Cmd {
	r := m.initAgentRunner()
	if r == nil {
		return nil
	}
	ctx := m.startPromptContext()
	if _, err := submitAgentRunnerHook(r, ctx, prompt); err != nil {
		m.resetPromptContext()
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "(agent runner unavailable: " + err.Error() + ")"})
		return nil
	}
	return listenAgentRunnerCmd(r)
}

func (m *Model) runAgentSkillPrompt(skill, args string) tea.Cmd {
	r := m.initAgentRunner()
	if r == nil {
		hint := fmt.Sprintf("run: sin-code mcp call %s %q", skill, args)
		m.appendChat(ChatMessage{Kind: chatAssistant, Text: hint})
		return nil
	}
	ctx := m.startPromptContext()
	prompt := fmt.Sprintf("use the %s tool to %s", skill, args)
	if _, err := submitAgentRunnerHook(r, ctx, prompt); err != nil {
		m.resetPromptContext()
		m.appendChat(ChatMessage{Kind: chatSystem, Text: "(agent runner error: " + err.Error() + ")"})
		return nil
	}
	return listenAgentRunnerCmd(r)
}
