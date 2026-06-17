// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

type VerifyState int

const (
	VerifyIdle VerifyState = iota
	VerifyPending
	VerifyRunning
	VerifyPassed
	VerifyFailed
	VerifyBlocked
)

func (s VerifyState) String() string {
	switch s {
	case VerifyIdle:
		return "idle"
	case VerifyPending:
		return "pending"
	case VerifyRunning:
		return "running"
	case VerifyPassed:
		return "passed"
	case VerifyFailed:
		return "failed"
	case VerifyBlocked:
		return "blocked"
	}
	return "unknown"
}

func (s VerifyState) Icon() string {
	switch s {
	case VerifyIdle:
		return "○"
	case VerifyPending:
		return "⏳"
	case VerifyRunning:
		return "⟳"
	case VerifyPassed:
		return "✅"
	case VerifyFailed:
		return "❌"
	case VerifyBlocked:
		return "⛔"
	}
	return "?"
}

type VerifyPanel struct {
	State     VerifyState
	Mode      string
	Target    string
	Evidence  string
	StartTime time.Time
	EndTime   time.Time
	Attempts  int
	History   []VerifyAttempt
}

type VerifyAttempt struct {
	Timestamp time.Time
	State     VerifyState
	Target    string
	Evidence  string
	Duration  time.Duration
}

type VerifyUpdateMsg struct {
	State    VerifyState
	Mode     string
	Target   string
	Evidence string
}

func RenderVerifyPanel(panel VerifyPanel, styles Styles, width int) string {
	if panel.State == VerifyIdle {
		return ""
	}

	var b strings.Builder

	stateStyle := styleForVerifyState(panel.State, styles)
	header := fmt.Sprintf(" %s Verification Gate — %s ",
		panel.State.Icon(),
		stateStyle.Render(strings.ToUpper(panel.State.String())))

	border := styles.Muted.Render(strings.Repeat("─", max(width-2, 10)))

	b.WriteString(styles.ContentHdr.Render(header))
	b.WriteString("\n")
	b.WriteString(border)
	b.WriteString("\n")

	if panel.Mode != "" {
		b.WriteString(styles.Muted.Render("  Mode: "))
		b.WriteString(styles.Bold.Render(panel.Mode))
		b.WriteString("\n")
	}
	if panel.Target != "" {
		b.WriteString(styles.Muted.Render("  Target: "))
		b.WriteString(styles.Content.Render(panel.Target))
		b.WriteString("\n")
	}

	if panel.Evidence != "" {
		b.WriteString(styles.Muted.Render("  Evidence:\n"))
		lines := strings.Split(panel.Evidence, "\n")
		for i, line := range lines {
			if i >= 3 {
				b.WriteString(styles.Muted.Render("    ... (truncated)\n"))
				break
			}
			b.WriteString(styles.Content.Render("    " + line + "\n"))
		}
	}

	if !panel.StartTime.IsZero() {
		duration := panel.EndTime.Sub(panel.StartTime)
		if duration > 0 {
			b.WriteString(styles.Muted.Render(fmt.Sprintf("  Duration: %s", formatDuration(duration))))
			b.WriteString("\n")
		}
	}

	if panel.Attempts > 1 {
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  Attempt: #%d", panel.Attempts)))
		b.WriteString("\n")
	}

	return b.String()
}

func RenderVerifyHistory(panel VerifyPanel, styles Styles, width int) string {
	if len(panel.History) == 0 {
		return styles.Muted.Render("  No verification history")
	}

	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render(" Verification History"))
	b.WriteString("\n")

	for _, attempt := range panel.History {
		icon := attempt.State.Icon()
		ts := attempt.Timestamp.Format("15:04:05")
		line := fmt.Sprintf("  %s %s  %s  %s", icon, ts, attempt.Target, formatDuration(attempt.Duration))
		b.WriteString(styles.Content.Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

func RenderVerifyStatusBar(panel VerifyPanel, styles Styles) string {
	if panel.State == VerifyIdle {
		return ""
	}
	stateStyle := styleForVerifyState(panel.State, styles)
	return stateStyle.Render(panel.State.Icon() + " verify:" + panel.State.String())
}

func styleForVerifyState(state VerifyState, styles Styles) lipgloss.Style {
	switch state {
	case VerifyPassed:
		return styles.StatusOK
	case VerifyFailed:
		return styles.StatusErr
	case VerifyBlocked:
		return styles.StatusWarn
	case VerifyRunning, VerifyPending:
		return styles.AccentText
	default:
		return styles.Muted
	}
}

func HandleVerifyUpdate(panel *VerifyPanel, msg VerifyUpdateMsg) {
	panel.State = msg.State
	panel.Mode = msg.Mode
	panel.Target = msg.Target
	panel.Evidence = msg.Evidence

	if msg.State == VerifyRunning && panel.StartTime.IsZero() {
		panel.StartTime = time.Now()
	}
	if msg.State == VerifyPassed || msg.State == VerifyFailed || msg.State == VerifyBlocked {
		panel.EndTime = time.Now()
		panel.History = append(panel.History, VerifyAttempt{
			Timestamp: time.Now(),
			State:     msg.State,
			Target:    msg.Target,
			Evidence:  msg.Evidence,
			Duration:  panel.EndTime.Sub(panel.StartTime),
		})
		if len(panel.History) > 20 {
			panel.History = panel.History[len(panel.History)-20:]
		}
		panel.StartTime = time.Time{}
	}
}

func ResetVerifyPanel(panel *VerifyPanel) {
	panel.State = VerifyIdle
	panel.Mode = ""
	panel.Target = ""
	panel.Evidence = ""
	panel.StartTime = time.Time{}
	panel.EndTime = time.Time{}
	panel.Attempts = 0
}
