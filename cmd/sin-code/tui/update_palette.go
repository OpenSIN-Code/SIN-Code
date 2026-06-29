// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	tea "charm.land/bubbletea/v2"
)

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
