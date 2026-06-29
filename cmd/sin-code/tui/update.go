// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/key"

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

	if m.CrashRecovery != nil {
		if state, err := m.CrashRecovery.Load(); err == nil && state != nil && len(state.ChatHistory) > 0 {
			m.ChatHistory = state.ChatHistory
			if state.ViewKind >= 0 && state.ViewKind < viewCount {
				m.ViewKind = ViewKind(state.ViewKind)
			}
			m.appendChat(ChatMessage{Kind: chatSystem, Text: "↻ Restored from previous session (crash recovery)"})
			_ = m.CrashRecovery.Clear()
		}
	}

	return tea.Batch(cmds...)
}

func SetKeymap(k Keymap) { keymap = k }

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
		m.SaveCrashState()
		m.Quitting = true
		return m, tea.Quit
	}
	if msg.Code == 'c' && msg.Mod&tea.ModCtrl != 0 {
		m.saveChatHistory()
		m.SaveCrashState()
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
