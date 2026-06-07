package tui

import (
	"fmt"
	"strings"
)

type ConfigEntry struct {
	Key   string
	Value string
	Hint  string
	Kind  string
}

func DefaultConfigEntries() []ConfigEntry {
	return []ConfigEntry{
		{Key: "theme", Value: "default", Hint: "default|Dracula|Nord|Solarized|Monokai", Kind: "select"},
		{Key: "agent", Value: "Build", Hint: "Build|Audit|Stats", Kind: "select"},
		{Key: "sidebar", Value: "visible", Hint: "visible|collapsed", Kind: "select"},
		{Key: "default-view", Value: "Tools", Hint: "Tools|Sessions|EFM|Config|History", Kind: "select"},
		{Key: "cooldown-seconds", Value: "60", Hint: "1-3600", Kind: "number"},
		{Key: "efm-default-ttl", Value: "3600", Hint: "60-86400", Kind: "number"},
		{Key: "output-format", Value: "json", Hint: "json|text|yaml", Kind: "select"},
		{Key: "verbosity", Value: "info", Hint: "debug|info|warn|error", Kind: "select"},
	}
}

func RenderConfigView(entries []ConfigEntry, selected int, styles Styles, width, height int) string {
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("⚙ Config — sin-code runtime settings"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	b.WriteString("\n\n")

	b.WriteString(styles.AccentText.Render(fmt.Sprintf("  %-22s  %-16s  %-24s  %s", "Key", "Value", "Allowed", "Kind")))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  " + strings.Repeat("─", max(width-6, 10))))
	b.WriteString("\n")

	for i, e := range entries {
		line := fmt.Sprintf("  %-22s  %-16s  %-24s  %s", truncate(e.Key, 22), truncate(e.Value, 16), truncate(e.Hint, 24), e.Kind)
		if i == selected {
			b.WriteString(styles.SidebarSel.Render(padRight(line, width-4)))
		} else {
			b.WriteString(styles.Content.Render(padRight(line, width-4)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if selected >= 0 && selected < len(entries) {
		e := entries[selected]
		b.WriteString(styles.AccentText.Render("▸ Selected: "))
		b.WriteString(styles.Bold.Render(e.Key))
		b.WriteString(" = ")
		b.WriteString(styles.ContentHdr.Render(e.Value))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  allowed: " + e.Hint + "  (" + e.Kind + ")"))
		b.WriteString("\n")
	}
	b.WriteString(styles.Muted.Render("  Press e to edit · s to save"))
	b.WriteString("\n")

	return b.String()
}
