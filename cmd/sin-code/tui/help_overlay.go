// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"
)

type helpEntry struct {
	key    string
	action string
}

type helpGroup struct {
	name    string
	entries []helpEntry
}

type HelpOverlay struct {
	keymapCfg KeymapConfig
	groups    []helpGroup
	active    bool
	scroll    int
}

func NewHelpOverlay(keymap KeymapConfig) *HelpOverlay {
	ho := &HelpOverlay{keymapCfg: keymap}
	ho.buildGroups()
	return ho
}

func (ho *HelpOverlay) buildGroups() {
	ho.groups = []helpGroup{
		ho.buildGroup("Global", ho.keymapCfg.Global),
		ho.buildGroup("Navigation", filterNavBindings(ho.keymapCfg.Global)),
		ho.buildGroup("Tools", ho.keymapCfg.Tools),
		ho.buildGroup("Chat", ho.keymapCfg.Chat),
		ho.buildGroup("Sessions", ho.keymapCfg.Sessions),
	}
}

func (ho *HelpOverlay) buildGroup(name string, bindings ContextBindings) helpGroup {
	g := helpGroup{name: name}
	keys := make([]string, 0, len(bindings))
	for k := range bindings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g.entries = append(g.entries, helpEntry{
			key:    formatKeys(bindings[k]),
			action: formatActionLabel(k),
		})
	}
	return g
}

func filterNavBindings(global ContextBindings) ContextBindings {
	nav := ContextBindings{}
	navActions := []string{"next_view", "prev_view", "view_tools", "view_sessions",
		"view_efm", "view_config", "view_history", "view_todos",
		"view_chat", "view_dag", "view_context", "view_dashboard"}
	for _, a := range navActions {
		if keys, ok := global[a]; ok {
			nav[a] = keys
		}
	}
	return nav
}

func formatKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return strings.Join(keys, ", ")
}

func formatActionLabel(action string) string {
	labels := map[string]string{
		"quit":           "Quit",
		"help":           "Help overlay",
		"palette":        "Command palette",
		"toggle_sidebar": "Toggle sidebar",
		"cycle_theme":    "Cycle theme",
		"cycle_agent":    "Cycle agent",
		"interrupt":      "Interrupt / Esc",
		"model_select":   "Model selector",
		"subagents":      "Subagents popup",
		"next_view":      "Next view",
		"prev_view":      "Previous view",
		"view_tools":     "Jump to Tools",
		"view_sessions":  "Jump to Sessions",
		"view_efm":       "Jump to EFM",
		"view_config":    "Jump to Config",
		"view_history":   "Jump to History",
		"view_todos":     "Jump to Todos",
		"view_chat":      "Jump to Chat",
		"view_dag":       "Jump to DAG",
		"view_context":   "Jump to Context",
		"view_dashboard": "Jump to Dashboard",
		"run_tool":       "Run tool",
		"show_help":      "Show help / expand",
		"tool_up":        "Move up",
		"tool_down":      "Move down",
		"submit":         "Submit message",
		"cancel":         "Cancel",
		"search":         "Search chat",
		"copy_message":   "Copy message",
		"scroll_up":      "Scroll up",
		"scroll_down":    "Scroll down",
		"compact_toggle": "Toggle compact mode",
		"new_session":    "New session",
		"close_session":  "Close session",
		"session_switch": "Switch session",
	}
	if label, ok := labels[action]; ok {
		return label
	}
	return strings.ReplaceAll(action, "_", " ")
}

func (ho *HelpOverlay) Open() {
	ho.active = true
	ho.scroll = 0
}

func (ho *HelpOverlay) Close() {
	ho.active = false
}

func (ho *HelpOverlay) IsActive() bool {
	return ho.active
}

func (ho *HelpOverlay) ScrollUp() {
	if ho.scroll > 0 {
		ho.scroll--
	}
}

func (ho *HelpOverlay) ScrollDown(maxScroll int) {
	if ho.scroll < maxScroll {
		ho.scroll++
	}
}

func (ho *HelpOverlay) Render(styles Styles, width, height int) string {
	if !ho.active {
		return ""
	}
	if width < 30 {
		width = 30
	}
	if height < 10 {
		height = 10
	}
	contentWidth := width - 4
	keyColWidth := 18
	actionColWidth := contentWidth - keyColWidth - 4
	if actionColWidth < 10 {
		actionColWidth = 10
	}
	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Keybindings"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("-", contentWidth)))
	b.WriteString("\n\n")
	visibleLines := 0
	maxLines := height - 6
	skipped := 0
	for _, g := range ho.groups {
		if visibleLines >= maxLines {
			break
		}
		if skipped < ho.scroll {
			skipped++
			continue
		}
		b.WriteString(styles.Header.Render(g.name))
		b.WriteString("\n")
		for _, e := range g.entries {
			if visibleLines >= maxLines {
				break
			}
			keyStr := padRight(e.key, keyColWidth)
			actionStr := truncateString(e.action, actionColWidth)
			line := fmt.Sprintf("  %s  %s", styles.AccentText.Render(keyStr), styles.Content.Render(actionStr))
			b.WriteString(line)
			b.WriteString("\n")
			visibleLines++
		}
		b.WriteString("\n")
		visibleLines++
	}
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(padRight("[Esc] close", contentWidth)))
	b.WriteString("\n")
	overlayStyle := lipgloss.NewStyle().
		Foreground(c(styles.Theme.Text)).
		Background(c(styles.Theme.Background)).
		Width(width).
		Height(height).
		Padding(1, 2)
	return overlayStyle.Render(b.String())
}

func (ho *HelpOverlay) HandleKey(msg tea.KeyMsg) bool {
	key := msg.String()
	switch key {
	case "esc", "?", "q":
		ho.Close()
		return true
	case "up", "k":
		ho.ScrollUp()
		return true
	case "down", "j":
		ho.ScrollDown(100)
		return true
	}
	return false
}

func (m *Model) handleHelpOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.HelpOverlay != nil {
		m.HelpOverlay.HandleKey(msg)
	}
	if m.HelpOverlay == nil || !m.HelpOverlay.IsActive() {
		m.Mode = ModeNormal
	}
	return m, nil
}
