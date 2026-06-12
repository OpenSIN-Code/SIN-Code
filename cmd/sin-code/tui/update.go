package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui/chat"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.Spinner.Init(),
		ListenForNotifications(),
		RefreshTodosCmd(),
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
	if m.OnRun != nil {
		if err := m.OnRun(name, args); err != nil {
			m.AppendHistory(m.ViewKind.String(), "run:"+name, err.Error(), false)
		}
	}
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

	case tea.KeyPressMsg:
		if m.ViewKind == ViewChat {
			cmd := m.updateChat(msg)
			return m, cmd
		}
		return m.handleKey(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.Mode == ModePalette {
		return m.handlePaletteKey(msg)
	}
	if m.Mode == ModeSubagents {
		if key == "esc" || key == "ctrl+x" {
			m.CloseSubagents()
		}
		return m, nil
	}
	if m.Mode == ModeArgInput {
		return m.handleArgInputKey(msg)
	}

	switch key {
	case "ctrl+c", "q":
		m.Quitting = true
		return m, tea.Quit
	case "esc":
		m.AppendHistory(m.ViewKind.String(), "interrupt", "Esc pressed", true)
		return m, nil
	case "tab":
		m.NextView()
		return m, nil
	case "shift+tab":
		m.PreviousView()
		return m, nil
	case "ctrl+b":
		m.Sidebar.Toggle()
		return m, nil
	case "ctrl+p":
		m.OpenPalette()
		return m, nil
	case "ctrl+x":
		m.OpenSubagents()
		return m, nil
	case "1":
		m.SwitchView(ViewTools)
		return m, nil
	case "2":
		m.SwitchView(ViewSessions)
		return m, nil
	case "3":
		m.SwitchView(ViewEFM)
		return m, nil
	case "4":
		m.SwitchView(ViewConfig)
		return m, nil
	case "5":
		m.SwitchView(ViewHistory)
		return m, nil
	case "6":
		m.SwitchView(ViewTodos)
		return m, nil
	case "7":
		m.SwitchView(ViewChat)
		return m, nil
	case "t":
		m.CycleTheme()
		m.AppendHistory(m.ViewKind.String(), "theme", Themes[m.ThemeIdx].Name, true)
		return m, nil
	case "a":
		m.Footer.CycleAgent()
		m.AppendHistory(m.ViewKind.String(), "agent", m.Footer.AgentName(), true)
		return m, nil
	case "r":
		if m.ViewKind == ViewTools {
			m.RunSelected()
		}
		return m, nil
	case "enter":
		if m.ViewKind == ViewTools {
			tool := m.Sidebar.SelectedTool()
			if tool != nil {
				m.AppendHistory(ViewTools.String(), "show-help", tool.Name, true)
			}
		}
		return m, nil
	case "up", "k":
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
	case "down", "j":
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
	case "up_left", "left", "h":
		_ = key
		return m, nil
	case "right", "l":
		_ = key
		return m, nil
	case "o":
		if m.NotificationBanner != nil {
			m.AppendHistory(ViewTodos.String(), "banner-open", m.NotificationBanner.Title, true)
		}
		return m, nil
	case "d":
		if m.NotificationBanner != nil {
			m.DismissBanner()
			m.AppendHistory(ViewTodos.String(), "banner-dismiss", "", true)
		}
		return m, nil
	case "n":
		if m.NotificationBanner != nil {
			m.BannerNext()
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) PreviousView() {
	v := int(m.ViewKind) - 1
	if v < 0 {
		v = 4
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
		return v
	}
	if m.Width < 20 || m.Height < 6 {
		v := tea.NewView(m.Styles.Muted.Render("Terminal too small — resize to at least 20x6."))
		v.AltScreen = true
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

	v := tea.NewView(layout)
	v.AltScreen = true
	return v
}

func (m *Model) contentWidth() int {
	left := 0
	if !m.Sidebar.Collapsed {
		left = m.Sidebar.Width
	}
	right := 0
	if m.RightPanel {
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

// handleChatResponse replaces the most recent "assistant: thinking..."
// placeholder in ChatHistory with the real response (or error). The
// placeholder is always the last entry because handleChatSubmit appends
// it immediately before spawning the goroutine, and the Update loop is
// the only writer to ChatHistory in steady state.
func (m *Model) handleChatResponse(msg chat.ChatResponseMsg) {
	if len(m.ChatHistory) == 0 {
		return
	}
	idx := len(m.ChatHistory) - 1
	last := m.ChatHistory[idx]
	if !strings.HasPrefix(last, "assistant: thinking...") {
		// No placeholder — append as a new entry.
		if msg.Error != nil {
			m.ChatHistory = append(m.ChatHistory, "assistant: (error: "+msg.Error.Error()+")")
		} else if msg.Text == "" {
			m.ChatHistory = append(m.ChatHistory, "assistant: (empty response)")
		} else {
			m.ChatHistory = append(m.ChatHistory, "assistant: "+msg.Text)
		}
		if len(m.ChatHistory) > 500 {
			m.ChatHistory = m.ChatHistory[len(m.ChatHistory)-500:]
		}
		return
	}
	if msg.Error != nil {
		m.ChatHistory[idx] = "assistant: (error: " + msg.Error.Error() + ")"
		return
	}
	text := msg.Text
	if text == "" {
		text = "(empty response)"
	}
	m.ChatHistory[idx] = "assistant: " + text
}
