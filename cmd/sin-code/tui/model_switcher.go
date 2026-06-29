// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
)

type ModelSwitcher struct {
	mu       sync.Mutex
	open     bool
	models   []string
	sel      int
	current  string
	selected string
}

func NewModelSwitcher() *ModelSwitcher {
	return &ModelSwitcher{}
}

func (s *ModelSwitcher) SetModels(models []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.models = models
	if s.sel >= len(s.models) {
		s.sel = 0
	}
}

func (s *ModelSwitcher) SetCurrent(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = model
}

func (s *ModelSwitcher) Open() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = true
	s.sel = 0
	s.selected = ""
}

func (s *ModelSwitcher) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = false
}

func (s *ModelSwitcher) IsOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open
}

func (s *ModelSwitcher) MoveUp() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.models) == 0 {
		return
	}
	s.sel--
	if s.sel < 0 {
		s.sel = len(s.models) - 1
	}
}

func (s *ModelSwitcher) MoveDown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.models) == 0 {
		return
	}
	s.sel++
	if s.sel >= len(s.models) {
		s.sel = 0
	}
}

func (s *ModelSwitcher) Confirm() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sel < 0 || s.sel >= len(s.models) {
		return ""
	}
	s.selected = s.models[s.sel]
	s.open = false
	return s.selected
}

func (s *ModelSwitcher) Render(styles Styles, width, height int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.open {
		return ""
	}
	popupWidth := 50
	if popupWidth > width-4 {
		popupWidth = width - 4
	}
	if popupWidth < 30 {
		popupWidth = 30
	}
	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Switch Model"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("-", popupWidth-4)))
	b.WriteString("\n\n")
	if len(s.models) == 0 {
		b.WriteString(styles.Muted.Render("  No models available"))
		b.WriteString("\n")
	} else {
		for i, model := range s.models {
			marker := " "
			if model == s.current {
				marker = "+"
			}
			label := fmt.Sprintf("  %s %s", marker, model)
			if i == s.sel {
				b.WriteString(styles.PopupSel.Render(padRight(label, popupWidth-4)))
			} else {
				b.WriteString(styles.PopupItem.Render(padRight(label, popupWidth-4)))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(" up/down navigate | enter select | esc close"))
	b.WriteString("\n")
	return styles.Popup.Render(b.String())
}

type ModelSwitchMsg struct {
	Model string
}

func (m *Model) OpenModelSwitcher() {
	if m.ModelSwitcher == nil {
		m.ModelSwitcher = NewModelSwitcher()
	}
	m.ModelSwitcher.SetModels(buildModelList(m))
	m.ModelSwitcher.SetCurrent(m.Footer.ModelName)
	m.ModelSwitcher.Open()
	m.Mode = ModeModelSwitcher
}

func (m *Model) CloseModelSwitcher() {
	if m.ModelSwitcher != nil {
		m.ModelSwitcher.Close()
	}
	m.Mode = ModeNormal
}

func (m *Model) handleModelSwitcherKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.CloseModelSwitcher()
		return m, nil
	case "enter":
		m.handleModelSwitcherSelect()
		return m, nil
	case "up":
		m.ModelSwitcher.MoveUp()
		return m, nil
	case "down":
		m.ModelSwitcher.MoveDown()
		return m, nil
	}
	return m, nil
}

func switcherModelNames() []string {
	out := make([]string, 0, len(DefaultModels))
	for _, mi := range DefaultModels {
		out = append(out, mi.Name)
	}
	return out
}
