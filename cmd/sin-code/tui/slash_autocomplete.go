// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

type SlashCommand struct {
	Name        string
	Description string
	Category    string
	Args        string
}

type SlashAutocomplete struct {
	mu       sync.Mutex
	active   bool
	commands []SlashCommand
	filtered []SlashCommand
	selected int
	scroll   int
	maxShow  int
}

func NewSlashAutocomplete() *SlashAutocomplete {
	sa := &SlashAutocomplete{
		commands: defaultSlashCommands(),
		maxShow:  8,
	}
	sa.filtered = make([]SlashCommand, len(sa.commands))
	copy(sa.filtered, sa.commands)
	return sa
}

func defaultSlashCommands() []SlashCommand {
	return []SlashCommand{
		{Name: "/clear", Description: "Clear conversation", Category: "chat"},
		{Name: "/help", Description: "Show help", Category: "meta"},
		{Name: "/attach", Description: "Attach file", Category: "chat", Args: "<path>"},
		{Name: "/search", Description: "Search conversation", Category: "chat"},
		{Name: "/btw", Description: "Side question", Category: "chat"},
		{Name: "/undercover", Description: "Toggle undercover mode", Category: "git"},
		{Name: "/model", Description: "Switch model", Category: "meta"},
		{Name: "/theme", Description: "Switch theme", Category: "meta"},
		{Name: "/compact", Description: "Compact context", Category: "meta"},
		{Name: "/tools", Description: "List tools", Category: "meta"},
		{Name: "/sessions", Description: "Manage sessions", Category: "meta"},
		{Name: "/dag", Description: "Show DAG view", Category: "view"},
		{Name: "/ctx-viz", Description: "Context visualizer", Category: "view"},
		{Name: "/dashboard", Description: "Agent dashboard", Category: "view"},
	}
}

func (s *SlashAutocomplete) Commands() []SlashCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SlashCommand, len(s.commands))
	copy(out, s.commands)
	return out
}

func (s *SlashAutocomplete) Filter(query string) []SlashCommand {
	s.mu.Lock()
	defer s.mu.Unlock()

	q := strings.ToLower(strings.TrimSpace(query))
	q = strings.TrimPrefix(q, "/")

	if q == "" {
		s.filtered = make([]SlashCommand, len(s.commands))
		copy(s.filtered, s.commands)
		s.selected = 0
		s.scroll = 0
		return s.copyFilteredLocked()
	}

	var matched []SlashCommand
	for _, cmd := range s.commands {
		nameLower := strings.ToLower(strings.TrimPrefix(cmd.Name, "/"))
		if ok, _ := fuzzySubsequenceMatch(nameLower, q); ok {
			matched = append(matched, cmd)
			continue
		}
		if ok, _ := fuzzySubsequenceMatch(strings.ToLower(cmd.Description), q); ok {
			matched = append(matched, cmd)
			continue
		}
		if ok, _ := fuzzySubsequenceMatch(strings.ToLower(cmd.Category), q); ok {
			matched = append(matched, cmd)
		}
	}
	s.filtered = matched
	if s.selected >= len(s.filtered) {
		s.selected = 0
	}
	s.scroll = 0
	return s.copyFilteredLocked()
}

func (s *SlashAutocomplete) copyFilteredLocked() []SlashCommand {
	out := make([]SlashCommand, len(s.filtered))
	copy(out, s.filtered)
	return out
}

func (s *SlashAutocomplete) Render(styles Styles, width int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if width < 20 {
		width = 20
	}

	innerWidth := width - 6
	if innerWidth < 14 {
		innerWidth = 14
	}

	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Slash Commands"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(innerWidth, 10))))
	b.WriteString("\n")

	count := len(s.filtered)
	if count == 0 {
		b.WriteString(styles.Muted.Render("  (no matches)"))
		b.WriteString("\n")
	} else {
		maxShow := s.maxShow
		if count < maxShow {
			maxShow = count
		}
		for i := 0; i < maxShow; i++ {
			idx := s.scroll + i
			if idx >= count {
				break
			}
			cmd := s.filtered[idx]
			line := formatSlashCommandLine(cmd, innerWidth)
			if idx == s.selected {
				selStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color(styles.Theme.Background)).
					Background(lipgloss.Color(styles.Theme.Accent)).
					Bold(true)
				b.WriteString(selStyle.Render(padRight("  "+line, innerWidth+2)))
			} else {
				b.WriteString(styles.PopupItem.Render(padRight("  "+line, innerWidth+2)))
			}
			b.WriteString("\n")
		}
		if count > s.maxShow {
			b.WriteString(styles.Muted.Render("  ↓ more..."))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  ↑/↓ select · Tab/Enter insert · Esc close"))
	b.WriteString("\n")

	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(styles.Theme.Accent)).
		Foreground(lipgloss.Color(styles.Theme.Text)).
		Background(lipgloss.Color(styles.Theme.Background)).
		Padding(0, 1)
	return popupStyle.Render(b.String())
}

func formatSlashCommandLine(cmd SlashCommand, width int) string {
	name := cmd.Name
	if cmd.Args != "" {
		name += " " + cmd.Args
	}
	desc := cmd.Description
	cat := "(" + cmd.Category + ")"

	nameWidth := 16
	if width < 40 {
		nameWidth = 12
	}
	if len(name) > nameWidth {
		name = truncateString(name, nameWidth)
	}

	remaining := width - nameWidth - len(cat) - 4
	if remaining < 4 {
		remaining = 4
	}
	if len(desc) > remaining {
		desc = truncateString(desc, remaining)
	}

	return padRight(name, nameWidth) + "  " + padRight(desc, remaining) + "  " + cat
}

func (s *SlashAutocomplete) MoveUp() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.filtered) == 0 {
		return
	}
	s.selected--
	if s.selected < 0 {
		s.selected = len(s.filtered) - 1
	}
	s.adjustScroll()
}

func (s *SlashAutocomplete) MoveDown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.filtered) == 0 {
		return
	}
	s.selected++
	if s.selected >= len(s.filtered) {
		s.selected = 0
	}
	s.adjustScroll()
}

func (s *SlashAutocomplete) adjustScroll() {
	if s.selected < s.scroll {
		s.scroll = s.selected
	}
	if s.selected >= s.scroll+s.maxShow {
		s.scroll = s.selected - s.maxShow + 1
	}
	if s.scroll < 0 {
		s.scroll = 0
	}
}

func (s *SlashAutocomplete) Selected() *SlashCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.filtered) == 0 || s.selected < 0 || s.selected >= len(s.filtered) {
		return nil
	}
	cmd := s.filtered[s.selected]
	return &cmd
}

func (s *SlashAutocomplete) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *SlashAutocomplete) SetActive(b bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = b
	if !b {
		s.selected = 0
		s.scroll = 0
	}
}

func (s *SlashAutocomplete) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
	s.selected = 0
	s.scroll = 0
	s.filtered = make([]SlashCommand, len(s.commands))
	copy(s.filtered, s.commands)
}
