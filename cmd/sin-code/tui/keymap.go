// SPDX-License-Identifier: MIT
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type Keymap struct {
	Quit          key.Binding
	Help          key.Binding
	Palette       key.Binding
	ToggleSidebar key.Binding
	CycleTheme    key.Binding
	CycleAgent    key.Binding
	Interrupt     key.Binding

	NextView     key.Binding
	PrevView     key.Binding
	ViewTools    key.Binding
	ViewSessions key.Binding
	ViewEFM      key.Binding
	ViewConfig   key.Binding
	ViewHistory  key.Binding
	ViewTodos    key.Binding
	ViewChat     key.Binding
	ViewDAG      key.Binding
	ViewContext  key.Binding
	ViewDashboard key.Binding

	RunTool  key.Binding
	ShowHelp key.Binding
	ToolUp   key.Binding
	ToolDown key.Binding

	Submit      key.Binding
	Cancel      key.Binding
	Search      key.Binding
	CopyMessage key.Binding
	ScrollUp    key.Binding
	ScrollDown  key.Binding

	CompactToggle key.Binding

	NewSession    key.Binding
	CloseSession  key.Binding
	SessionSwitch key.Binding

	ModelSelect key.Binding

	Subagents key.Binding
}

func DefaultKeymap() Keymap {
	return Keymap{
		Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c", "ctrl+x"), key.WithHelp("q/^x", "quit")),
		Help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Palette:       key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^p", "palette")),
		ToggleSidebar: key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("^b", "sidebar")),
		CycleTheme:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		CycleAgent:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "agent")),
		Interrupt:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "interrupt")),
		NextView:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		PrevView:      key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "prev view")),
		ViewTools:     key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "tools")),
		ViewSessions:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "sessions")),
		ViewEFM:       key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "efm")),
		ViewConfig:    key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "config")),
		ViewHistory:   key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "history")),
		ViewTodos:     key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "todos")),
		ViewChat:       key.NewBinding(key.WithKeys("7"), key.WithHelp("7", "chat")),
		ViewDAG:        key.NewBinding(key.WithKeys("8"), key.WithHelp("8", "dag")),
		ViewContext:    key.NewBinding(key.WithKeys("9"), key.WithHelp("9", "context")),
		ViewDashboard:  key.NewBinding(key.WithKeys("0"), key.WithHelp("0", "dashboard")),
		RunTool:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run")),
		ShowHelp:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "help")),
		ToolUp:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("up/k", "up")),
		ToolDown:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("dn/j", "down")),
		Submit:        key.NewBinding(key.WithKeys("ctrl+s", "ctrl+enter"), key.WithHelp("^s", "submit")),
		Cancel:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Search:        key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("^f", "search")),
		CopyMessage:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy")),
		ScrollUp:      key.NewBinding(key.WithKeys("pgup", "up"), key.WithHelp("pgup", "scroll up")),
		ScrollDown:    key.NewBinding(key.WithKeys("pgdown", "down"), key.WithHelp("pgdn", "scroll down")),
		CompactToggle: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "compact")),
		NewSession:    key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "new session")),
		CloseSession:  key.NewBinding(key.WithKeys("-"), key.WithHelp("-", "close session")),
		SessionSwitch: key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("^g", "switch session")),
		ModelSelect:   key.NewBinding(key.WithKeys("ctrl+m"), key.WithHelp("^m", "model")),
		Subagents:     key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("^a", "subagents")),
	}
}

func (k Keymap) HelpView(styles Styles) string {
	var b strings.Builder
	categories := []struct {
		title    string
		bindings []key.Binding
	}{
		{"Global", []key.Binding{k.Quit, k.Help, k.Palette, k.ToggleSidebar, k.CycleTheme, k.CycleAgent, k.Interrupt}},
		{"Navigation", []key.Binding{k.NextView, k.PrevView, k.ViewTools, k.ViewSessions, k.ViewEFM, k.ViewConfig, k.ViewHistory, k.ViewTodos, k.ViewChat, k.ViewDAG, k.ViewContext, k.ViewDashboard}},
		{"Tools", []key.Binding{k.RunTool, k.ShowHelp, k.ToolUp, k.ToolDown}},
		{"Chat", []key.Binding{k.Submit, k.Cancel, k.Search, k.CopyMessage, k.ScrollUp, k.ScrollDown, k.CompactToggle}},
		{"Sessions", []key.Binding{k.NewSession, k.CloseSession, k.SessionSwitch}},
		{"Model", []key.Binding{k.ModelSelect}},
		{"Subagents", []key.Binding{k.Subagents}},
	}
	for _, cat := range categories {
		b.WriteString(styles.Header.Render(cat.title))
		b.WriteString("\n")
		for _, binding := range cat.bindings {
			if !binding.Enabled() {
				continue
			}
			help := binding.Help()
			line := fmt.Sprintf("  %-10s %s", help.Key, help.Desc)
			b.WriteString(styles.Muted.Render(line))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

type KeyOverrides struct {
	Quit          []string `json:"quit,omitempty"`
	Help          []string `json:"help,omitempty"`
	Palette       []string `json:"palette,omitempty"`
	ToggleSidebar []string `json:"toggle_sidebar,omitempty"`
	CycleTheme    []string `json:"cycle_theme,omitempty"`
	CycleAgent    []string `json:"cycle_agent,omitempty"`
	Interrupt     []string `json:"interrupt,omitempty"`
	NextView      []string `json:"next_view,omitempty"`
	PrevView      []string `json:"prev_view,omitempty"`
	ViewTools     []string `json:"view_tools,omitempty"`
	ViewSessions  []string `json:"view_sessions,omitempty"`
	ViewEFM       []string `json:"view_efm,omitempty"`
	ViewConfig    []string `json:"view_config,omitempty"`
	ViewHistory   []string `json:"view_history,omitempty"`
	ViewTodos     []string `json:"view_todos,omitempty"`
	ViewChat     []string `json:"view_chat,omitempty"`
	ViewDAG      []string `json:"view_dag,omitempty"`
	ViewContext  []string `json:"view_context,omitempty"`
	ViewDashboard []string `json:"view_dashboard,omitempty"`
	RunTool       []string `json:"run_tool,omitempty"`
	ShowHelp      []string `json:"show_help,omitempty"`
	ToolUp        []string `json:"tool_up,omitempty"`
	ToolDown      []string `json:"tool_down,omitempty"`
	Submit        []string `json:"submit,omitempty"`
	Cancel        []string `json:"cancel,omitempty"`
	Search        []string `json:"search,omitempty"`
	CopyMessage   []string `json:"copy_message,omitempty"`
	ScrollUp      []string `json:"scroll_up,omitempty"`
	ScrollDown    []string `json:"scroll_down,omitempty"`
	CompactToggle []string `json:"compact_toggle,omitempty"`
	NewSession    []string `json:"new_session,omitempty"`
	CloseSession  []string `json:"close_session,omitempty"`
	SessionSwitch []string `json:"session_switch,omitempty"`
	ModelSelect   []string `json:"model_select,omitempty"`
	Subagents     []string `json:"subagents,omitempty"`
}

// KeyOverridesExampleJSON is a valid example for
// ~/.config/sin-code/tui-keys.json. Every field is OPTIONAL (omitempty); a
// field that is absent or set to an empty array leaves the DefaultKeymap
// binding untouched. Each value is a list of key strings as understood by
// charm.land/bubbles (e.g. "ctrl+x", "shift+enter", "pgup", "up", "k").
//
// The full set of overridable field names mirrors the KeyOverrides struct
// JSON tags: quit, help, palette, toggle_sidebar, cycle_theme, cycle_agent,
// interrupt, next_view, prev_view, view_tools, view_sessions, view_efm,
// view_config, view_history, view_todos, view_chat, view_dag, view_context,
// view_dashboard, run_tool, show_help,
// tool_up, tool_down, submit, cancel, search, copy_message, scroll_up,
// scroll_down, new_session, close_session, session_switch, model_select,
// subagents.
//
// Load with LoadKeyOverrides(path) and apply with Keymap.ApplyOverrides(ov).
const KeyOverridesExampleJSON = `{
  "quit":            ["ctrl+x", "ctrl+c"],
  "submit":          ["ctrl+s", "ctrl+enter"],
  "search":          ["ctrl+f"],
  "session_switch":  ["ctrl+g"],
  "cycle_theme":     ["t"],
  "copy_message":    ["y"],
  "scroll_up":       ["pgup", "ctrl+u"],
  "scroll_down":     ["pgdown", "ctrl+d"],
  "next_view":       ["tab"],
  "prev_view":       ["shift+tab"],
  "new_session":     ["+"],
  "close_session":   ["-"],
  "model_select":    ["ctrl+m"],
  "subagents":       ["ctrl+a"]
}`

func LoadKeyOverrides(path string) (KeyOverrides, error) {
	var ov KeyOverrides
	data, err := os.ReadFile(path)
	if err != nil {
		return ov, err
	}
	if err := json.Unmarshal(data, &ov); err != nil {
		return ov, fmt.Errorf("parse key overrides: %w", err)
	}
	return ov, nil
}

func (k *Keymap) ApplyOverrides(ov KeyOverrides) {
	k.applyOverride(&k.Quit, ov.Quit)
	k.applyOverride(&k.Help, ov.Help)
	k.applyOverride(&k.Palette, ov.Palette)
	k.applyOverride(&k.ToggleSidebar, ov.ToggleSidebar)
	k.applyOverride(&k.CycleTheme, ov.CycleTheme)
	k.applyOverride(&k.CycleAgent, ov.CycleAgent)
	k.applyOverride(&k.Interrupt, ov.Interrupt)
	k.applyOverride(&k.NextView, ov.NextView)
	k.applyOverride(&k.PrevView, ov.PrevView)
	k.applyOverride(&k.ViewTools, ov.ViewTools)
	k.applyOverride(&k.ViewSessions, ov.ViewSessions)
	k.applyOverride(&k.ViewEFM, ov.ViewEFM)
	k.applyOverride(&k.ViewConfig, ov.ViewConfig)
	k.applyOverride(&k.ViewHistory, ov.ViewHistory)
	k.applyOverride(&k.ViewTodos, ov.ViewTodos)
	k.applyOverride(&k.ViewChat, ov.ViewChat)
	k.applyOverride(&k.ViewDAG, ov.ViewDAG)
	k.applyOverride(&k.ViewContext, ov.ViewContext)
	k.applyOverride(&k.ViewDashboard, ov.ViewDashboard)
	k.applyOverride(&k.RunTool, ov.RunTool)
	k.applyOverride(&k.ShowHelp, ov.ShowHelp)
	k.applyOverride(&k.ToolUp, ov.ToolUp)
	k.applyOverride(&k.ToolDown, ov.ToolDown)
	k.applyOverride(&k.Submit, ov.Submit)
	k.applyOverride(&k.Cancel, ov.Cancel)
	k.applyOverride(&k.Search, ov.Search)
	k.applyOverride(&k.CopyMessage, ov.CopyMessage)
	k.applyOverride(&k.ScrollUp, ov.ScrollUp)
	k.applyOverride(&k.ScrollDown, ov.ScrollDown)
	k.applyOverride(&k.CompactToggle, ov.CompactToggle)
	k.applyOverride(&k.NewSession, ov.NewSession)
	k.applyOverride(&k.CloseSession, ov.CloseSession)
	k.applyOverride(&k.SessionSwitch, ov.SessionSwitch)
	k.applyOverride(&k.ModelSelect, ov.ModelSelect)
	k.applyOverride(&k.Subagents, ov.Subagents)
}

func (k *Keymap) applyOverride(b *key.Binding, keys []string) {
	if len(keys) == 0 {
		return
	}
	help := b.Help()
	*b = key.NewBinding(key.WithKeys(keys...), key.WithHelp(help.Key, help.Desc))
}

type MouseAction struct {
	Kind   string
	X      int
	Y      int
	Target string
}

func ResolveMouse(msg tea.MouseMsg, width, height int, sidebarWidth int, rightPanelWidth int) MouseAction {
	var x, y int
	kind := "click"
	switch evt := msg.(type) {
	case tea.MouseClickMsg:
		x, y = evt.X, evt.Y
		kind = "click"
	case tea.MouseWheelMsg:
		x, y = evt.X, evt.Y
		switch evt.Button {
		case tea.MouseWheelUp:
			kind = "scroll_up"
		case tea.MouseWheelDown:
			kind = "scroll_down"
		default:
			kind = "click"
		}
	default:
		m := msg.Mouse()
		x, y = m.X, m.Y
		kind = "click"
	}
	return MouseAction{
		Kind:   kind,
		X:      x,
		Y:      y,
		Target: resolveMouseTarget(x, y, width, height, sidebarWidth, rightPanelWidth),
	}
}

func resolveMouseTarget(x, y, width, height, sidebarWidth, rightPanelWidth int) string {
	const tabBarHeight = 3
	const footerHeight = 3
	if height <= tabBarHeight+footerHeight {
		return "tabs"
	}
	if y < tabBarHeight {
		return "tabs"
	}
	if y >= height-footerHeight {
		return "footer"
	}
	if sidebarWidth > 0 && x < sidebarWidth {
		return "sidebar"
	}
	if rightPanelWidth > 0 && x >= width-rightPanelWidth {
		return "right_panel"
	}
	return "chat"
}
