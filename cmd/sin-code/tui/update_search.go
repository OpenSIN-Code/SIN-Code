// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when TUI is rewritten

package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

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
		matchIdx := -1
		if m.ChatSearch != nil && m.ChatSearch.CurrentResult() != nil {
			r := m.ChatSearch.CurrentResult()
			m.ChatFocusIdx = r.MessageIdx
			m.SearchQuery = m.SearchInput.Value()
			matchIdx = r.MessageIdx
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
			matchIdx = found
		}
		if matchIdx >= 0 {
			m.ScrollToMatchIdx = matchIdx
		}
		m.CloseChatSearch()
		return m, nil
	case "n":
		if m.ChatSearch != nil {
			r := m.ChatSearch.Next()
			if r != nil {
				m.ChatFocusIdx = r.MessageIdx
				m.ScrollToMatchIdx = r.MessageIdx
			}
		}
		return m, nil
	case "N", "shift+n":
		if m.ChatSearch != nil {
			r := m.ChatSearch.Prev()
			if r != nil {
				m.ChatFocusIdx = r.MessageIdx
				m.ScrollToMatchIdx = r.MessageIdx
			}
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
