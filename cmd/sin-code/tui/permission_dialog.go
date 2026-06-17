package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type PermissionDialogState struct {
	Open       bool
	ToolName   string
	Detail     string
	Diff       string
	AllowKey   string
	DenyKey    string
}

func (m *Model) OpenPermissionDialog(toolName, detail, diff string) {
	m.PermissionDialog.Open = true
	m.PermissionDialog.ToolName = toolName
	m.PermissionDialog.Detail = detail
	m.PermissionDialog.Diff = diff
	m.PermissionDialog.AllowKey = "y"
	m.PermissionDialog.DenyKey = "n"
	m.Mode = ModePermissionDialog
}

func (m *Model) ClosePermissionDialog() {
	m.PermissionDialog.Open = false
	m.PermissionDialog.ToolName = ""
	m.PermissionDialog.Detail = ""
	m.PermissionDialog.Diff = ""
	m.Mode = ModeNormal
}

func (m *Model) handlePermissionDialogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "y", "Y":
		m.answerPendingAsk(true)
		m.ClosePermissionDialog()
		return m, nil
	case "n", "N", "esc":
		m.answerPendingAsk(false)
		m.ClosePermissionDialog()
		return m, nil
	case "up", "k":
		// Scroll diff up (future enhancement)
		return m, nil
	case "down", "j":
		// Scroll diff down (future enhancement)
		return m, nil
	}
	return m, nil
}

func renderDiffLine(line string, styles Styles) string {
	if strings.HasPrefix(line, "+") {
		return styles.StatusOK.Render(line)
	}
	if strings.HasPrefix(line, "-") {
		return styles.StatusErr.Render(line)
	}
	if strings.HasPrefix(line, "@@") {
		return styles.AccentText.Render(line)
	}
	return styles.Content.Render(line)
}

func RenderPermissionDialog(state PermissionDialogState, styles Styles, width, height int) string {
	if !state.Open {
		return ""
	}

	var b strings.Builder

	b.WriteString(styles.StatusWarn.Render("⚠ Permission Required"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", min(width-2, 70))))
	b.WriteString("\n\n")

	b.WriteString(styles.Bold.Render("Tool: "))
	b.WriteString(styles.Content.Render(state.ToolName))
	b.WriteString("\n\n")

	b.WriteString(styles.Bold.Render("Action: "))
	b.WriteString(styles.Content.Render(state.Detail))
	b.WriteString("\n\n")

	if state.Diff != "" {
		b.WriteString(styles.Bold.Render("Changes:"))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render(strings.Repeat("─", min(width-2, 70))))
		b.WriteString("\n")

		lines := strings.Split(state.Diff, "\n")
		maxLines := min(15, len(lines))
		for i := 0; i < maxLines; i++ {
			b.WriteString(renderDiffLine(lines[i], styles))
			b.WriteString("\n")
		}
		if len(lines) > maxLines {
			b.WriteString(styles.Muted.Render("... (truncated)"))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(styles.StatusWarn.Render("Allow this action?"))
	b.WriteString("\n")
	b.WriteString(styles.StatusOK.Render("  [y] Allow"))
	b.WriteString("  ")
	b.WriteString(styles.StatusErr.Render("[n] Deny"))
	b.WriteString("\n\n")
	b.WriteString(styles.Muted.Render("Press y to allow, n to deny"))

	return b.String()
}
