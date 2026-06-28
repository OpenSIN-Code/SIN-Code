// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// ContextMeter shows a visual representation of context window usage.
// It displays: [████████░░░░░░░░] 45% · 18k/40k tokens
type ContextMeter struct {
	UsedTokens    int
	MaxTokens     int
	CompactedOnce bool
	Styles        Styles
	Width         int
}

// NewContextMeter creates a ContextMeter with the given styles and bar width.
func NewContextMeter(styles Styles, width int) *ContextMeter {
	if width < 20 {
		width = 20
	}
	return &ContextMeter{
		MaxTokens: 128000,
		Styles:    styles,
		Width:     width,
	}
}

// SetUsage updates the used and max token counts.
func (cm *ContextMeter) SetUsage(used, max int) {
	cm.UsedTokens = used
	cm.MaxTokens = max
}

// SetCompacted marks whether the context has been compacted at least once.
func (cm *ContextMeter) SetCompacted(b bool) {
	cm.CompactedOnce = b
}

// Percentage returns the usage as a fraction of max (0.0–1.0+).
func (cm *ContextMeter) Percentage() float64 {
	max := cm.MaxTokens
	if max <= 0 {
		return 0
	}
	return float64(cm.UsedTokens) / float64(max)
}

// Status returns the color zone name: "green" <60%, "yellow" 60–80%, "red" >80%.
func (cm *ContextMeter) Status() string {
	pct := cm.Percentage()
	switch {
	case pct > 0.80:
		return "red"
	case pct > 0.60:
		return "yellow"
	default:
		return "green"
	}
}

// Render produces the meter string.
//
//	⑂[████████░░░░░░░░░░] 45% · 18k/40k
//
// The ⑂ prefix appears only when CompactedOnce is true.
// Color zones: green <60%, yellow 60–80%, red >80%. Over 100% shows COMPACTED.
func (cm *ContextMeter) Render() string {
	pct := cm.Percentage()
	if pct < 0 {
		pct = 0
	}

	barW := 20
	if cm.Width > 0 && cm.Width < barW {
		barW = cm.Width
	}
	if barW < 4 {
		barW = 4
	}

	displayPct := pct
	if displayPct > 1.0 {
		displayPct = 1.0
	}
	filled := int(float64(barW) * displayPct)
	if filled > barW {
		filled = barW
	}
	if filled < 0 {
		filled = 0
	}

	colorStr := cm.Styles.Theme.Success
	switch cm.Status() {
	case "yellow":
		colorStr = cm.Styles.Theme.Warn
	case "red":
		colorStr = cm.Styles.Theme.Error
	}
	filledStyle := lipgloss.NewStyle().Foreground(c(colorStr))
	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		cm.Styles.Muted.Render(strings.Repeat("░", barW-filled))

	pctStr := fmt.Sprintf("%.0f%%", pct*100)
	tokStr := fmt.Sprintf("%s/%s", FormatTokens(cm.UsedTokens), FormatTokens(cm.MaxTokens))

	var b strings.Builder
	if cm.CompactedOnce {
		b.WriteString(cm.Styles.StatusWarn.Render("⑂"))
	}
	b.WriteString("[")
	b.WriteString(bar)
	b.WriteString("] ")
	b.WriteString(cm.Styles.Muted.Render(pctStr))
	b.WriteString(cm.Styles.Muted.Render(" · "))
	b.WriteString(cm.Styles.Muted.Render(tokStr))

	if pct > 1.0 {
		b.WriteString(" ")
		b.WriteString(cm.Styles.StatusErr.Render("COMPACTED"))
	}

	return b.String()
}
