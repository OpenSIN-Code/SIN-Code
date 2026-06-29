// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

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

func (m *Model) PreviousView() {
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
