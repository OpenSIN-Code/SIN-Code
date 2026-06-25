// SPDX-License-Identifier: MIT
// Purpose: TUI views — EFM stack listing and DAG visualizer.
// EFM view: live rendering of ephemeral full-stack mocking environments.
// DAG view (merged from dag_view.go): live rendering of the orchestrator
// DAG with Running / Blocked / Done / Pre-warmed / Predicted tasks
// (issue #286). Renders as a topological-sorted list with status icons,
// probability scores, and dependency arrows. Integrates with the
// DeepPlanner (issue #282) and PreWarmManager (issue #285).
package tui

import (
	"fmt"
	"strings"
	"time"
)

func RenderEFMView(stacks []EFMStack, styles Styles, width, height int, spinner Spinner) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("⚡ EFM — Ephemeral Full-Stack Mocking"))
	b.WriteString(" ")
	b.WriteString(spinner.View(styles.Spinner))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	if len(stacks) == 0 {
		b.WriteString(styles.Muted.Render("  No active stacks."))
		b.WriteString("\n\n")
		b.WriteString(styles.Muted.Render("  Press n to spin up a new ephemeral stack."))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  Press r to refresh from the EFM backend."))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(styles.AccentText.Render(fmt.Sprintf("  %-22s  %-12s  %-32s  %s", "Name", "Status", "URL", "TTL")))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n")

	for _, st := range stacks {
		ttl := fmt.Sprintf("%ds", st.TTL)
		if st.TTL == 0 {
			ttl = "—"
		}
		status := st.Status
		line := fmt.Sprintf("  %-22s  %-12s  %-32s  %s", truncate(st.Name, 22), status, truncate(st.URL, 32), ttl)
		switch status {
		case "running", "up":
			b.WriteString(styles.StatusOK.Render(line))
		case "starting":
			b.WriteString(styles.StatusWarn.Render(line))
		case "down", "error":
			b.WriteString(styles.StatusErr.Render(line))
		default:
			b.WriteString(styles.Content.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  %d stack(s)  ·  refreshed %s", len(stacks), time.Now().Format("15:04:05"))))
	b.WriteString("\n")

	return b.String()
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// ── DAG view (merged from dag_view.go) ─────────────────────────────────

// DAG task status icons
const (
	dagIconGreen     = "✓"
	dagIconRunning   = "●"
	dagIconPreWarmed = "○"
	dagIconPredicted = "·"
	dagIconFailed    = "✗"
	dagIconSkipped   = "⊘"
	dagIconPending   = "…"
)

func RenderDAGView(state DAGState, styles Styles, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("◊ Orchestrator DAG"))
	b.WriteString("\n")

	if state.Prompt != "" {
		promptLine := state.Prompt
		if len(promptLine) > width-6 {
			promptLine = promptLine[:width-9] + "..."
		}
		b.WriteString(styles.Muted.Render("  " + promptLine))
		b.WriteString("\n")
	}
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	if len(state.Tasks) == 0 {
		b.WriteString(styles.Muted.Render("  No active plan. Use the orchestrator to start a DAG."))
		b.WriteString("\n")
		return b.String()
	}

	// Group tasks by status for summary.
	counts := map[string]int{}
	for _, t := range state.Tasks {
		counts[t.Status]++
	}

	// Render task rows.
	for i, task := range state.Tasks {
		selected := i == state.Selected
		icon := dagIconForStatus(task.Status, task.PreWarmed)
		probStr := ""
		if task.Probability > 0 {
			probStr = fmt.Sprintf(" P=%.0f%%", task.Probability*100)
		}
		preWarmStr := ""
		if task.PreWarmed {
			preWarmStr = " 🔥"
		}

		label := fmt.Sprintf("  %s %s%s%s  %s", icon, task.Type, probStr, preWarmStr, truncateDag(task.Description, width-20))
		if selected {
			b.WriteString(styles.SidebarSel.Render(padRight(label, width-4)))
		} else {
			b.WriteString(styles.Content.Render(padRight(label, width-4)))
		}
		b.WriteString("\n")
	}

	// Summary line.
	b.WriteString("\n")
	summaryParts := make([]string, 0, 6)
	if counts["completed"] > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d green", counts["completed"]))
	}
	if counts["running"] > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d running", counts["running"]))
	}
	if counts["pending"] > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d pending", counts["pending"]))
	}
	if counts["failed"] > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d failed", counts["failed"]))
	}
	preWarmedCount := 0
	for _, t := range state.Tasks {
		if t.PreWarmed && t.Status == "pending" {
			preWarmedCount++
		}
	}
	if preWarmedCount > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d pre-warmed", preWarmedCount))
	}
	b.WriteString(styles.Muted.Render("  " + strings.Join(summaryParts, " · ")))
	b.WriteString("\n")

	// Selected task details.
	if state.Selected >= 0 && state.Selected < len(state.Tasks) {
		b.WriteString("\n")
		b.WriteString(renderDAGTaskDetail(state.Tasks[state.Selected], styles, width))
	}

	return b.String()
}

func renderDAGTaskDetail(task DAGTaskRow, styles Styles, width int) string {
	var b strings.Builder
	b.WriteString(styles.AccentText.Render("  Task Details"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n")
	b.WriteString(styles.Content.Render(fmt.Sprintf("  Agent:    %s", task.AgentName)))
	b.WriteString("\n")
	b.WriteString(styles.Content.Render(fmt.Sprintf("  Type:     %s", task.Type)))
	b.WriteString("\n")
	b.WriteString(styles.Content.Render(fmt.Sprintf("  Status:   %s", task.Status)))
	b.WriteString("\n")
	if task.Probability > 0 {
		b.WriteString(styles.Content.Render(fmt.Sprintf("  P:        %.0f%%", task.Probability*100)))
		b.WriteString("\n")
	}
	if task.PreWarmed {
		b.WriteString(styles.StatusOK.Render("  PreWarm:  yes 🔥"))
		b.WriteString("\n")
	}
	if len(task.DependsOn) > 0 {
		b.WriteString(styles.Content.Render(fmt.Sprintf("  Deps:     %s", strings.Join(task.DependsOn, ", "))))
		b.WriteString("\n")
	}
	if task.ExpectedOutput != "" {
		b.WriteString(styles.Content.Render("  Expected: " + truncateDag(task.ExpectedOutput, width-14)))
		b.WriteString("\n")
	}
	if task.TokensUsed > 0 {
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  Tokens:   %d  Cost: $%.4f", task.TokensUsed, task.Cost)))
		b.WriteString("\n")
	}
	return b.String()
}

func dagIconForStatus(status string, preWarmed bool) string {
	switch status {
	case "completed":
		return dagIconGreen
	case "running":
		return dagIconRunning
	case "failed":
		return dagIconFailed
	case "skipped", "cancelled":
		return dagIconSkipped
	case "pending":
		if preWarmed {
			return dagIconPreWarmed
		}
		return dagIconPending
	default:
		return dagIconPredicted
	}
}

func truncateDag(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ── Todos view (merged from todos_view.go) ──────────────────────────────

func (m *Model) RenderTodos(styles Styles, width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 5 {
		height = 5
	}
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("Todos"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n\n")

	open := m.Sidebar.TodoOpen
	ready := m.Sidebar.TodoReady
	blocked := m.Sidebar.TodoBlocked
	overdue := m.Sidebar.TodoOverdue

	countLine := fmt.Sprintf("  🔵 %d open  🟢 %d ready  🟡 %d blocked  🔴 %d overdue",
		open, ready, blocked, overdue)
	b.WriteString(styles.Content.Render(countLine))
	b.WriteString("\n\n")

	if len(m.TodoItems) == 0 {
		b.WriteString(styles.Muted.Render("  (no todos yet — press 'a' to add)"))
		b.WriteString("\n")
		return b.String()
	}

	header := fmt.Sprintf("  %-8s %-3s %-10s %-7s %s", "ID", "PRI", "STATUS", "TYPE", "TITLE")
	b.WriteString(styles.AccentText.Render(header))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", width-4)))
	b.WriteString("\n")

	limit := height - 8
	if limit < 1 {
		limit = 1
	}
	if limit > len(m.TodoItems) {
		limit = len(m.TodoItems)
	}
	for i := 0; i < limit; i++ {
		row := m.TodoItems[i]
		priStyle := styles.Muted
		switch row.Priority {
		case "P0":
			priStyle = styles.Bold
		case "P1":
			priStyle = styles.AccentText
		}
		line := fmt.Sprintf("  %-8s %-3s %-10s %-7s %s",
			row.ID, row.Priority, row.Status, row.Type, row.Title)
		if i == m.TodoSel {
			b.WriteString(styles.SidebarSel.Render(line))
		} else {
			b.WriteString(priStyle.Render(line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ── Tools view (merged from tools_view.go) ──────────────────────────────

func RenderToolsView(sidebar Sidebar, styles Styles, width, height int) string {
	sel := sidebar.SelectedTool()
	if sel == nil {
		return styles.Muted.Render("No tool selected")
	}

	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("⚒ Tools (all " + fmt.Sprintf("%d", len(sidebar.ToolSubItems)) + " subcommands)"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	for i, t := range sidebar.ToolSubItems {
		icon := " "
		if t.Runnable {
			icon = "▶"
		}
		prefix := fmt.Sprintf("  %s %-14s", icon, t.Name)
		desc := t.Description
		line := prefix + "  " + desc
		if i == sidebar.ToolSel {
			b.WriteString(styles.SidebarSel.Render(padRight(line, width-4)))
		} else {
			b.WriteString(styles.Content.Render(padRight(line, width-4)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.AccentText.Render("▸ Selected: "))
	b.WriteString(styles.Bold.Render(sel.Name))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + sel.Description))
	b.WriteString("\n")
	if sel.Runnable {
		b.WriteString(styles.StatusOK.Render("  ✓ Runnable without args — press r to run"))
	} else {
		b.WriteString(styles.Muted.Render("  Press r to run with arguments"))
	}
	b.WriteString("\n")

	return b.String()
}

// ── History view (merged from history_view.go) ──────────────────────────

func RenderHistoryView(entries []HistoryEntry, selected int, styles Styles, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("⏱ History — last actions"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	if len(entries) == 0 {
		b.WriteString(styles.Muted.Render("  No actions yet."))
		b.WriteString("\n")
		return b.String()
	}

	start := 0
	maxRows := max(height-10, 5)
	if len(entries) > maxRows {
		start = len(entries) - maxRows
	}

	for i := start; i < len(entries); i++ {
		e := entries[i]
		icon := "✓"
		style := styles.StatusOK
		if !e.Success {
			icon = "✗"
			style = styles.StatusErr
		}
		line := fmt.Sprintf("  %s  %s  %-9s  %-14s  %s", e.Time.Format("15:04:05"), icon, e.View, truncate(e.Action, 14), truncate(e.Detail, 40))
		if i == selected {
			b.WriteString(styles.SidebarSel.Render(padRight(line, width-4)))
		} else {
			b.WriteString(style.Render(padRight(line, width-4)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  %d entries", len(entries))))
	b.WriteString(" ")
	b.WriteString(styles.Muted.Render("· press c to clear"))
	b.WriteString("\n")

	return b.String()
}
