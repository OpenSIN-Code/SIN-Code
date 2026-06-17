package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func DefaultAgentDashboardState() AgentDashboardState {
	return AgentDashboardState{
		Sessions: []AgentSessionRow{},
		Selected: 0,
	}
}

func sessionStatusColor(status string, styles Styles) string {
	switch status {
	case "running":
		return styles.StatusOK.Render("● " + status)
	case "blocked":
		return styles.StatusWarn.Render("● " + status)
	case "done":
		return styles.Muted.Render("● " + status)
	case "error":
		return styles.StatusErr.Render("● " + status)
	default:
		return styles.Muted.Render("● " + status)
	}
}

func formatSessionDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func RenderAgentDashboardView(ds AgentDashboardState, styles Styles, width, height int) string {
	if width < 30 {
		width = 30
	}
	if height < 10 {
		height = 10
	}

	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Agent Dashboard"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n\n")

	if len(ds.Sessions) == 0 {
		b.WriteString(styles.Muted.Render("  No active agent sessions."))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  Sessions will appear here when agents are running."))
		b.WriteString("\n")
		return b.String()
	}

	cardWidth := width - 6
	if cardWidth < 30 {
		cardWidth = 30
	}

	maxCards := height - 7
	if maxCards < 1 {
		maxCards = 1
	}

	totalCost := 0.0
	totalTokens := 0
	running := 0
	blocked := 0
	done := 0

	start := 0
	if len(ds.Sessions) > maxCards {
		start = len(ds.Sessions) - maxCards
	}

	for i := start; i < len(ds.Sessions); i++ {
		sess := ds.Sessions[i]
		totalCost += sess.Cost
		totalTokens += sess.Tokens
		switch sess.Status {
		case "running":
			running++
		case "blocked":
			blocked++
		case "done":
			done++
		}

		isSelected := i == ds.Selected

		var card strings.Builder
		card.WriteString(fmt.Sprintf("%s  %s", sess.AgentName, sessionStatusColor(sess.Status, styles)))
		card.WriteString("\n")
		taskLine := sess.Task
		if len(taskLine) > cardWidth-4 {
			taskLine = taskLine[:cardWidth-7] + "..."
		}
		card.WriteString(styles.Muted.Render(taskLine))
		card.WriteString("\n")
		card.WriteString(styles.Muted.Render(fmt.Sprintf("%s  %s  $%.4f",
			formatSessionDuration(sess.Duration),
			formatTokens(sess.Tokens),
			sess.Cost)))

		borderColor := c(styles.Theme.TextDim)
		if isSelected {
			borderColor = c(styles.Theme.Accent)
		}

		cardStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1).
			Width(cardWidth)

		b.WriteString(cardStyle.Render(card.String()))
		b.WriteString("\n")
	}

	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n")
	b.WriteString(styles.Bold.Render(fmt.Sprintf("  %d session(s)", len(ds.Sessions))))
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  |  %d running  %d blocked  %d done", running, blocked, done)))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  Total: %s tokens  |  $%.4f", formatTokens(totalTokens), totalCost)))
	b.WriteString("\n")

	return b.String()
}
