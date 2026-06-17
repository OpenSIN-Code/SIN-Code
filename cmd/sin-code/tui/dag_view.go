// SPDX-License-Identifier: MIT
// Purpose: DAG Visualizer — live rendering of the orchestrator DAG with
// Running / Blocked / Done / Pre-warmed / Predicted tasks (issue #286).
// Renders as a topological-sorted list with status icons, probability
// scores, and dependency arrows. Integrates with the DeepPlanner (issue
// #282) and PreWarmManager (issue #285).
package tui

import (
	"fmt"
	"strings"
)

// DAG task status icons
const (
	dagIconGreen      = "✓"
	dagIconRunning    = "●"
	dagIconPreWarmed  = "○"
	dagIconPredicted  = "·"
	dagIconFailed     = "✗"
	dagIconSkipped    = "⊘"
	dagIconPending    = "…"
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
