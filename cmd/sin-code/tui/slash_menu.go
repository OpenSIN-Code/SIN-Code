// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

// DefaultSlashCommands is the default command list shown in the slash menu.
var DefaultSlashCommands = []SlashCommand{
	{Name: "/help", Description: "Show help and available commands", Category: "general"},
	{Name: "/clear", Description: "Clear chat history", Category: "general"},
	{Name: "/compact", Description: "Compact context window", Category: "general"},
	{Name: "/commit", Description: "Create a git commit with conventional format", Category: "git"},
	{Name: "/diff", Description: "Show working tree diff", Category: "git"},
	{Name: "/pr", Description: "Create a pull request", Category: "git"},
	{Name: "/session", Description: "Switch or create session", Category: "session", Args: "[name]"},
	{Name: "/model", Description: "Switch LLM model", Category: "session", Args: "[name]"},
	{Name: "/verify", Description: "Run verification gate", Category: "tools"},
	{Name: "/tools", Description: "List available tools", Category: "tools"},
	{Name: "/agent", Description: "Spawn a sub-agent for parallel work", Category: "agent", Args: "<prompt>"},
	{Name: "/orchestrate", Description: "Run multi-agent orchestrator", Category: "agent", Args: "<prompt>"},
	{Name: "/todo", Description: "Manage todos", Category: "general", Args: "[add|list|done]"},
	{Name: "/memory", Description: "Search agent memory", Category: "general", Args: "<query>"},
	{Name: "/theme", Description: "Cycle color theme", Category: "general"},
	{Name: "/config", Description: "View or edit configuration", Category: "general", Args: "[key]"},
}

// SlashMenu is a popup menu for browsing slash commands with descriptions,
// category color-coding, and fuzzy filtering.
type SlashMenu struct {
	mu       sync.Mutex
	Open     bool
	Commands []SlashCommand
	Filter   string
	Sel      int
	Styles   Styles
	Width    int
	filtered []SlashCommand
}

// NewSlashMenu creates a SlashMenu populated with DefaultSlashCommands.
func NewSlashMenu(styles Styles) *SlashMenu {
	sm := &SlashMenu{
		Styles:   styles,
		Commands: make([]SlashCommand, len(DefaultSlashCommands)),
		Width:    60,
	}
	copy(sm.Commands, DefaultSlashCommands)
	sm.filtered = make([]SlashCommand, len(sm.Commands))
	copy(sm.filtered, sm.Commands)
	return sm
}

// OpenMenu opens the menu and resets the filter and selection.
func (m *SlashMenu) OpenMenu() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Open = true
	m.Filter = ""
	m.Sel = 0
	m.filtered = make([]SlashCommand, len(m.Commands))
	copy(m.filtered, m.Commands)
}

// Close closes the menu and resets state.
func (m *SlashMenu) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Open = false
	m.Filter = ""
	m.Sel = 0
}

// Filter_ filters the command list by fuzzy-matching name, description, and
// category against the query. The leading "/" is stripped automatically.
func (m *SlashMenu) Filter_(query string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	q := strings.ToLower(strings.TrimSpace(query))
	q = strings.TrimPrefix(q, "/")
	m.Filter = q

	if q == "" {
		m.filtered = make([]SlashCommand, len(m.Commands))
		copy(m.filtered, m.Commands)
		m.Sel = 0
		return
	}

	var matched []SlashCommand
	for _, cmd := range m.Commands {
		nameLower := strings.ToLower(cmd.Name)
		descLower := strings.ToLower(cmd.Description)
		catLower := strings.ToLower(cmd.Category)

		if ok, _ := fuzzySubsequenceMatch(nameLower, q); ok {
			matched = append(matched, cmd)
			continue
		}
		if ok, _ := fuzzySubsequenceMatch(descLower, q); ok {
			matched = append(matched, cmd)
			continue
		}
		if ok, _ := fuzzySubsequenceMatch(catLower, q); ok {
			matched = append(matched, cmd)
		}
	}
	m.filtered = matched
	if m.Sel >= len(m.filtered) {
		m.Sel = 0
	}
}

// Next moves the selection down by one, wrapping to the top.
func (m *SlashMenu) Next() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filtered) == 0 {
		return
	}
	m.Sel++
	if m.Sel >= len(m.filtered) {
		m.Sel = 0
	}
}

// Prev moves the selection up by one, wrapping to the bottom.
func (m *SlashMenu) Prev() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filtered) == 0 {
		return
	}
	m.Sel--
	if m.Sel < 0 {
		m.Sel = len(m.filtered) - 1
	}
}

// Selected returns the currently selected command. Returns a zero-value
// SlashCommand when nothing is selected.
func (m *SlashMenu) Selected() SlashCommand {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filtered) == 0 || m.Sel < 0 || m.Sel >= len(m.filtered) {
		return SlashCommand{}
	}
	return m.filtered[m.Sel]
}

// Filtered returns a copy of the current filtered command list.
func (m *SlashMenu) Filtered() []SlashCommand {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SlashCommand, len(m.filtered))
	copy(out, m.filtered)
	return out
}

// categoryStyle returns the lipgloss style for a command's category.
func (m *SlashMenu) categoryStyle(category string) lipgloss.Style {
	switch category {
	case "git":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.Styles.Theme.Success))
	case "session":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.Styles.Theme.Warn))
	case "tools":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.Styles.Theme.TextDim))
	case "agent", "general":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.Styles.Theme.Accent))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.Styles.Theme.Text))
	}
}

// Render renders the slash menu popup.
func (m *SlashMenu) Render() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	width := m.Width
	if width < 30 {
		width = 30
	}

	innerWidth := width - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	var b strings.Builder

	b.WriteString(m.Styles.AccentText.Render(" Slash Commands"))
	b.WriteString("\n")
	b.WriteString(m.Styles.Muted.Render("  " + strings.Repeat("─", max(innerWidth, 10))))
	b.WriteString("\n")

	filtered := m.filtered
	if len(filtered) == 0 {
		b.WriteString(m.Styles.Muted.Render("  (no matches)"))
		b.WriteString("\n")
	} else {
		nameWidth := 14
		for _, cmd := range filtered {
			label := cmd.Name
			if cmd.Args != "" {
				label += " " + cmd.Args
			}
			if len(label) > nameWidth {
				nameWidth = len(label)
			}
		}
		if nameWidth > innerWidth-10 {
			nameWidth = innerWidth - 10
		}

		for i, cmd := range filtered {
			marker := "  "
			if i == m.Sel {
				marker = "▸ "
			}

			label := cmd.Name
			if cmd.Args != "" {
				label += " " + cmd.Args
			}
			label = padRight(label, nameWidth)

			desc := cmd.Description
			descMax := innerWidth - nameWidth - 3
			if descMax < 4 {
				descMax = 4
			}
			if len(desc) > descMax {
				desc = truncateString(desc, descMax)
			}

			catStyle := m.categoryStyle(cmd.Category)
			nameRendered := catStyle.Render(label)

			line := marker + nameRendered + "  " + desc

			if i == m.Sel {
				selStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color(m.Styles.Theme.Background)).
					Background(lipgloss.Color(m.Styles.Theme.Accent)).
					Bold(true)
				line = selStyle.Render(padRight(line, innerWidth+2))
			} else {
				line = padRight(line, innerWidth+2)
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.Styles.Muted.Render("  ↑/↓ navigate · Enter select · Esc close"))

	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.Styles.Theme.Accent)).
		Foreground(lipgloss.Color(m.Styles.Theme.Text)).
		Background(lipgloss.Color(m.Styles.Theme.Background)).
		Padding(0, 1)

	return popupStyle.Render(b.String())
}
