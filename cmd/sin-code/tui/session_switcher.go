package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func RenderSessionSwitcher(state SessionSwitcherState, tabs Tabs, styles Styles, width, height int) string {
	if !state.Open {
		return ""
	}

	var b strings.Builder

	b.WriteString(styles.AccentText.Render(" Session Switcher"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", min(width-2, 60))))
	b.WriteString("\n")

	if state.Query != "" {
		b.WriteString(styles.Content.Render(" 🔍 " + state.Query))
	} else {
		b.WriteString(styles.Muted.Render(" 🔍 (type to filter)"))
	}
	b.WriteString("\n\n")

	if len(state.Indices) == 0 {
		b.WriteString(styles.Muted.Render("  No sessions found"))
		b.WriteString("\n")
		return b.String()
	}

	maxVisible := min(10, len(state.Indices))
	start := 0
	if state.Sel >= maxVisible {
		start = state.Sel - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(state.Indices) {
		end = len(state.Indices)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	for displayIdx, sessIdx := range state.Indices[start:end] {
		sess := tabs.Sessions[sessIdx]
		
		marker := " "
		if sess.Dirty {
			marker = "●"
		}
		
		name := sess.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}
		
		preview := sess.Preview
		if len(preview) > 40 {
			preview = preview[:37] + "..."
		}
		if preview == "" {
			preview = "(empty session)"
		}
		
		line := fmt.Sprintf("  %s  %-30s  %s", marker, name, preview)
		
		if displayIdx+start == state.Sel {
			b.WriteString(styles.SidebarSel.Render(padRight(line, width-4)))
		} else {
			b.WriteString(styles.Content.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(" ↑/↓ navigate · enter select · esc close"))
	b.WriteString("\n")

	return b.String()
}

func (m *Model) OpenSessionSwitcher() {
	m.SessionSwitcher.Open = true
	m.SessionSwitcher.Query = ""
	m.SessionSwitcher.Sel = 0
	m.SessionSwitcher.Indices = m.Tabs.SortedByRecency()
	m.Mode = ModeSessionSwitcher
}

func (m *Model) CloseSessionSwitcher() {
	m.SessionSwitcher.Open = false
	m.SessionSwitcher.Query = ""
	m.SessionSwitcher.Sel = 0
	m.Mode = ModeNormal
}

func (m *Model) SessionSwitcherNavigate(direction int) {
	if len(m.SessionSwitcher.Indices) == 0 {
		return
	}
	
	m.SessionSwitcher.Sel += direction
	if m.SessionSwitcher.Sel < 0 {
		m.SessionSwitcher.Sel = len(m.SessionSwitcher.Indices) - 1
	}
	if m.SessionSwitcher.Sel >= len(m.SessionSwitcher.Indices) {
		m.SessionSwitcher.Sel = 0
	}
}

func (m *Model) SessionSwitcherSelect() {
	if len(m.SessionSwitcher.Indices) == 0 {
		return
	}
	
	if m.SessionSwitcher.Sel >= 0 && m.SessionSwitcher.Sel < len(m.SessionSwitcher.Indices) {
		sessIdx := m.SessionSwitcher.Indices[m.SessionSwitcher.Sel]
		m.Tabs.Select(sessIdx)
	}
	
	m.CloseSessionSwitcher()
}

func (m *Model) SessionSwitcherFilter(query string) {
	m.SessionSwitcher.Query = query
	
	if query == "" {
		m.SessionSwitcher.Indices = m.Tabs.SortedByRecency()
		m.SessionSwitcher.Sel = 0
		return
	}
	
	var filtered []int
	queryLower := strings.ToLower(query)
	for i, sess := range m.Tabs.Sessions {
		if strings.Contains(strings.ToLower(sess.Name), queryLower) ||
		   strings.Contains(strings.ToLower(sess.Preview), queryLower) {
			filtered = append(filtered, i)
		}
	}
	
	m.SessionSwitcher.Indices = filtered
	m.SessionSwitcher.Sel = 0
}

func (m *Model) handleSessionSwitcherKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "ctrl+g":
		m.CloseSessionSwitcher()
		return m, nil
	case "enter":
		m.SessionSwitcherSelect()
		return m, nil
	case "up":
		m.SessionSwitcherNavigate(-1)
		return m, nil
	case "down":
		m.SessionSwitcherNavigate(1)
		return m, nil
	case "backspace":
		if len(m.SessionSwitcher.Query) > 0 {
			m.SessionSwitcherFilter(m.SessionSwitcher.Query[:len(m.SessionSwitcher.Query)-1])
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.SessionSwitcherFilter(m.SessionSwitcher.Query + msg.String())
		}
		return m, nil
	}
}
