// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

type TokenBar struct {
	mu        sync.RWMutex
	used      int
	maxTokens int
	cost      float64
	model     string
}

func NewTokenBar(maxTokens int) *TokenBar {
	if maxTokens <= 0 {
		maxTokens = 128000
	}
	return &TokenBar{maxTokens: maxTokens}
}

func (b *TokenBar) Update(used int, cost float64, model string) {
	b.mu.Lock()
	b.used = used
	b.cost = cost
	b.model = model
	b.mu.Unlock()
}

func (b *TokenBar) Render(styles Styles, width int) string {
	if width < 20 {
		width = 20
	}
	b.mu.RLock()
	used := b.used
	maxTokens := b.maxTokens
	cost := b.cost
	model := b.model
	b.mu.RUnlock()

	if maxTokens <= 0 {
		maxTokens = 128000
	}
	pct := 0.0
	if maxTokens > 0 {
		pct = float64(used) / float64(maxTokens)
	}
	if pct < 0 {
		pct = 0
	} else if pct > 1 {
		pct = 1
	}

	modelStr := styles.AccentText.Render(model)
	pctStr := fmt.Sprintf("%.0f%%", pct*100)
	tokStr := fmt.Sprintf("%s/%s tok", FormatTokens(used), FormatTokens(maxTokens))
	costStr := fmt.Sprintf("$%.2f", cost)
	if cost < 0 {
		costStr = "$0.00"
	}

	sep := styles.Muted.Render(" · ")
	sepW := lipgloss.Width(sep)

	fixedW := lipgloss.Width(modelStr) + 2 + 2 + len(pctStr) + sepW + len(tokStr) + sepW + len(costStr)

	var sug string
	if pct > 0.80 {
		sug = styles.StatusWarn.Render("⚠ context compaction recommended")
		fixedW += 2 + lipgloss.Width(sug)
	}

	barW := width - fixedW
	if barW > 30 {
		barW = 30
	}
	if barW < 4 {
		barW = 4
	}

	filled := int(float64(barW) * pct)
	if filled > barW {
		filled = barW
	}
	if filled < 0 {
		filled = 0
	}

	var colorStr string
	switch {
	case pct > 0.95:
		colorStr = styles.Theme.Error
	case pct > 0.80:
		colorStr = styles.Theme.Warn
	default:
		colorStr = styles.Theme.Accent
	}
	filledStyle := lipgloss.NewStyle().Foreground(c(colorStr))
	bar := filledStyle.Render(strings.Repeat("█", filled)) + styles.Muted.Render(strings.Repeat("░", barW-filled))

	var b2 strings.Builder
	b2.WriteString(modelStr)
	b2.WriteString("  ")
	b2.WriteString(bar)
	b2.WriteString("  ")
	b2.WriteString(styles.Muted.Render(pctStr))
	b2.WriteString(sep)
	b2.WriteString(styles.Muted.Render(tokStr))
	b2.WriteString(sep)
	b2.WriteString(styles.FooterVal.Render(costStr))
	if sug != "" {
		b2.WriteString("  ")
		b2.WriteString(sug)
	}
	return b2.String()
}

func (b *TokenBar) Percent() float64 {
	b.mu.RLock()
	used := b.used
	maxTokens := b.maxTokens
	b.mu.RUnlock()
	if maxTokens <= 0 {
		return 0
	}
	pct := float64(used) / float64(maxTokens)
	if pct < 0 {
		return 0
	}
	if pct > 1 {
		return 1
	}
	return pct
}

func (b *TokenBar) IsWarning() bool {
	return b.Percent() > 0.80
}

func (b *TokenBar) IsCritical() bool {
	return b.Percent() > 0.95
}

func FormatTokens(n int) string {
	if n < 0 {
		n = 0
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

func parseCostStr(s string) float64 {
	s = strings.TrimPrefix(s, "$")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
