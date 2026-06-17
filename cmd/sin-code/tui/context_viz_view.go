package tui

import (
	"fmt"
	"strings"
)

func DefaultContextState() ContextState {
	return ContextState{
		MaxTokens: 200000,
		Categories: []ContextCategory{
			{Name: "System Prompt", Tokens: 6000, Color: "muted"},
			{Name: "Tools", Tokens: 11600, Color: "accent"},
			{Name: "Memory", Tokens: 743, Color: "dim"},
			{Name: "Conversation", Tokens: 15000, Color: "warn"},
			{Name: "Current Response", Tokens: 8000, Color: "accent"},
		},
		CacheHit:  0.0,
		CostUSD:   0.0,
		Compacted: false,
	}
}

func (cs *ContextState) Recompute() {
	total := 0
	for _, c := range cs.Categories {
		total += c.Tokens
	}
	cs.UsedTokens = total
}

func contextBarWidth(tokens, maxTokens, barWidth int) int {
	if maxTokens <= 0 || barWidth <= 0 {
		return 0
	}
	w := int(float64(barWidth) * float64(tokens) / float64(maxTokens))
	if w > barWidth {
		w = barWidth
	}
	if w < 0 {
		w = 0
	}
	return w
}

func RenderContextVizView(cs ContextState, styles Styles, width, height int) string {
	if width < 30 {
		width = 30
	}
	if height < 10 {
		height = 10
	}

	cs.Recompute()

	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Context Usage Visualizer"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n\n")

	used := cs.UsedTokens
	max := cs.MaxTokens
	if max <= 0 {
		max = 200000
	}
	pct := float64(used) / float64(max)
	if pct > 1.0 {
		pct = 1.0
	}

	headerLabel := fmt.Sprintf("  %s / %s  (%.1f%%)", formatTokens(used), formatTokens(max), pct*100)
	b.WriteString(styles.Bold.Render(headerLabel))
	b.WriteString("\n")

	barWidth := width - 8
	if barWidth < 10 {
		barWidth = 10
	}
	filled := contextBarWidth(used, max, barWidth)
	freeWidth := barWidth - filled

	var bar strings.Builder
	bar.WriteString(styles.Progress.Render(strings.Repeat("█", filled)))
	if freeWidth > 0 {
		bar.WriteString(styles.Muted.Render(strings.Repeat("░", freeWidth)))
	}
	b.WriteString("  ")
	b.WriteString(bar.String())
	b.WriteString("\n\n")

	b.WriteString(styles.AccentText.Render("  Breakdown by Category"))
	b.WriteString("\n")

	catBarWidth := width - 24
	if catBarWidth < 8 {
		catBarWidth = 8
	}

	for _, cat := range cs.Categories {
		catPct := float64(cat.Tokens) / float64(max)
		if catPct > 1.0 {
			catPct = 1.0
		}
		catFilled := contextBarWidth(cat.Tokens, max, catBarWidth)
		catFree := catBarWidth - catFilled

		var catBar strings.Builder
		catBar.WriteString(strings.Repeat("█", catFilled))
		catBar.WriteString(strings.Repeat("░", catFree))

		label := fmt.Sprintf("  %-18s %5s  ", cat.Name, formatTokens(cat.Tokens))
		b.WriteString(styles.Content.Render(label))
		b.WriteString(styles.Muted.Render(catBar.String()))
		b.WriteString(fmt.Sprintf("  %4.1f%%", catPct*100))
		b.WriteString("\n")
	}

	freeTokens := max - used
	if freeTokens < 0 {
		freeTokens = 0
	}
	freePct := float64(freeTokens) / float64(max) * 100
	b.WriteString("\n")
	b.WriteString(styles.StatusOK.Render(fmt.Sprintf("  Free Space: %s (%.1f%%)", formatTokens(freeTokens), freePct)))
	b.WriteString("\n\n")

	b.WriteString(styles.AccentText.Render("  Metrics"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  Cost (USD):     $%.4f", cs.CostUSD)))
	b.WriteString("\n")

	cacheIcon := "⏸️"
	cacheLabel := "inactive"
	if cs.CacheHit > 0 {
		cacheIcon = "🔄"
		cacheLabel = fmt.Sprintf("%.0f%% hit", cs.CacheHit*100)
	}
	b.WriteString(styles.Muted.Render(fmt.Sprintf("  Cache:          %s %s", cacheIcon, cacheLabel)))
	b.WriteString("\n")

	if cs.Compacted {
		b.WriteString(styles.StatusWarn.Render("  ⚠ Auto-Compaction triggered"))
		b.WriteString("\n")
	} else if pct >= 0.85 {
		b.WriteString(styles.StatusWarn.Render(fmt.Sprintf("  ⚠ Near compaction threshold (%.0f%%)", pct*100)))
		b.WriteString("\n")
	} else {
		b.WriteString(styles.Muted.Render("  Compaction:     not needed"))
		b.WriteString("\n")
	}

	return b.String()
}
