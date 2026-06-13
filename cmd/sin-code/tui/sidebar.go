package tui

import "strings"

type SidebarItem struct {
	View     ViewKind
	Icon     string
	Label    string
	Shortcut string
}

func DefaultSidebarItems() []SidebarItem {
	return []SidebarItem{
		{View: ViewTools, Icon: "⚒", Label: "Tools", Shortcut: "1"},
		{View: ViewSessions, Icon: "❒", Label: "Sessions", Shortcut: "2"},
		{View: ViewEFM, Icon: "⚡", Label: "EFM", Shortcut: "3"},
		{View: ViewConfig, Icon: "⚙", Label: "Config", Shortcut: "4"},
		{View: ViewHistory, Icon: "⏱", Label: "History", Shortcut: "5"},
		{View: ViewTodos, Icon: "☐", Label: "Todos", Shortcut: "6"},
		{View: ViewChat, Icon: "💬", Label: "Chat", Shortcut: "7"},
	}
}

type Sidebar struct {
	Items        []SidebarItem
	Selected     int
	Width        int
	Collapsed    bool
	ToolSubItems []ToolSubItem
	ToolSel      int

	TodoOpen    int
	TodoBlocked int
	TodoOverdue int
	TodoReady   int
}

type ToolSubItem struct {
	Name        string
	Description string
	Runnable    bool
}

func DefaultToolSubItems() []ToolSubItem {
	return []ToolSubItem{
		{Name: "discover", Description: "Discover files with relevance scoring", Runnable: false},
		{Name: "execute", Description: "Safe shell execution with redaction", Runnable: false},
		{Name: "map", Description: "Architecture map + dependency graph", Runnable: false},
		{Name: "grasp", Description: "Deep single-file analysis", Runnable: false},
		{Name: "scout", Description: "Regex/semantic/symbol code search", Runnable: false},
		{Name: "harvest", Description: "URL fetch + cache + structure extract", Runnable: false},
		{Name: "orchestrate", Description: "Task management with dependencies", Runnable: true},
		{Name: "ibd", Description: "Intent-based diffing", Runnable: false},
		{Name: "poc", Description: "Proof-of-correctness verification", Runnable: false},
		{Name: "sckg", Description: "Semantic codebase knowledge graph", Runnable: false},
		{Name: "adw", Description: "Architectural debt watchdogs", Runnable: false},
		{Name: "oracle", Description: "Verification oracle", Runnable: false},
		{Name: "efm", Description: "Ephemeral full-stack mocking", Runnable: false},
		{Name: "serve", Description: "Start MCP server (stdio)", Runnable: true},
		{Name: "security", Description: "Security scan (Go/Python/Node)", Runnable: true},
		{Name: "sbom", Description: "SBOM generation (SPDX/CycloneDX)", Runnable: false},
		{Name: "config", Description: "Configuration management", Runnable: false},
		{Name: "self-update", Description: "Update to latest release", Runnable: false},
		{Name: "tui", Description: "Launch this TUI", Runnable: true},
	}
}

func NewSidebar() Sidebar {
	return Sidebar{
		Items:        DefaultSidebarItems(),
		Selected:     0,
		Width:        22,
		Collapsed:    false,
		ToolSubItems: DefaultToolSubItems(),
		ToolSel:      0,
	}
}

func (s *Sidebar) Toggle() { s.Collapsed = !s.Collapsed }

func (s *Sidebar) MoveUp() {
	if s.Selected > 0 {
		s.Selected--
	}
}

func (s *Sidebar) MoveDown() {
	if s.Selected < len(s.Items)-1 {
		s.Selected++
	}
}

func (s *Sidebar) SelectedView() ViewKind {
	if s.Selected < 0 || s.Selected >= len(s.Items) {
		return ViewTools
	}
	return s.Items[s.Selected].View
}

func (s *Sidebar) SetSelectedView(v ViewKind) {
	for i, it := range s.Items {
		if it.View == v {
			s.Selected = i
			return
		}
	}
}

func (s *Sidebar) ToolMoveUp() {
	if s.ToolSel > 0 {
		s.ToolSel--
	}
}

func (s *Sidebar) ToolMoveDown() {
	if s.ToolSel < len(s.ToolSubItems)-1 {
		s.ToolSel++
	}
}

func (s *Sidebar) SelectedTool() *ToolSubItem {
	if s.ToolSel < 0 || s.ToolSel >= len(s.ToolSubItems) {
		return nil
	}
	return &s.ToolSubItems[s.ToolSel]
}

func (s Sidebar) View(styles Styles) string {
	if s.Collapsed {
		var b strings.Builder
		for i, it := range s.Items {
			if i == s.Selected {
				b.WriteString(styles.SidebarSel.Render(" " + it.Icon + " "))
			} else {
				b.WriteString(styles.Sidebar.Render(" " + it.Icon + " "))
			}
			b.WriteString("\n")
		}
		return b.String()
	}

	width := s.Width
	if width < 18 {
		width = 18
	}
	var b strings.Builder
	b.WriteString(styles.SidebarHdr.Render("  ⚡ sin-code"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n")

	for i, it := range s.Items {
		label := " " + it.Icon + "  " + it.Label
		if it.View == ViewTodos && (s.TodoOpen > 0 || s.TodoBlocked > 0 || s.TodoOverdue > 0) {
			label += badgeFor(s)
		}
		if i == s.Selected {
			padded := padRight(label, width-2)
			b.WriteString(styles.SidebarSel.Render(padded))
		} else {
			padded := padRight(label, width-2)
			b.WriteString(styles.Sidebar.Render(padded))
		}
		b.WriteString("\n")
	}

	if s.SelectedView() == ViewTools {
		b.WriteString(styles.Muted.Render(strings.Repeat("─", width-2)))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render(" Subcommands"))
		b.WriteString("\n")
		for i, t := range s.ToolSubItems {
			line := "  " + t.Name
			if i == s.ToolSel {
				padded := padRight(" ▸ "+line, width-2)
				b.WriteString(styles.SidebarSel.Render(padded))
			} else {
				padded := padRight("  "+line, width-2)
				b.WriteString(styles.Sidebar.Render(padded))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

func badgeFor(s Sidebar) string {
	if s.TodoOpen == 0 && s.TodoBlocked == 0 && s.TodoOverdue == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("  ")
	if s.TodoOpen > 0 {
		b.WriteString("🔵")
		b.WriteString(itoa(s.TodoOpen))
	}
	if s.TodoBlocked > 0 {
		b.WriteString(" 🟡")
		b.WriteString(itoa(s.TodoBlocked))
	}
	if s.TodoOverdue > 0 {
		b.WriteString(" 🔴")
		b.WriteString(itoa(s.TodoOverdue))
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
