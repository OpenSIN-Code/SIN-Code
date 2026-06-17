// SPDX-License-Identifier: MIT
// Purpose: Memory browser TUI view — list, search, inspect memories
// (issue #355).
package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// MemoryRow is a single row in the memory browser list.
type MemoryRow struct {
	ID         string
	Tags       []string
	Content    string
	Created    time.Time
	Importance float64
}

// MemoryBrowser holds the state for the TUI memory browser view.
type MemoryBrowser struct {
	all      []MemoryRow
	filtered []MemoryRow
	sel      int
	query    string
}

// NewMemoryBrowser creates a ready-to-use memory browser with empty state.
func NewMemoryBrowser() *MemoryBrowser {
	return &MemoryBrowser{
		all:      nil,
		filtered: nil,
		sel:      0,
		query:    "",
	}
}

// SetMemories replaces the full memory list and re-applies the current filter.
func (b *MemoryBrowser) SetMemories(memories []MemoryRow) {
	if b == nil {
		return
	}
	b.all = memories
	b.applyFilter()
	if b.sel >= len(b.filtered) {
		b.sel = max(0, len(b.filtered)-1)
	}
}

// MoveUp moves the selection cursor up by one row (clamped at 0).
func (b *MemoryBrowser) MoveUp() {
	if b == nil {
		return
	}
	if b.sel > 0 {
		b.sel--
	}
}

// MoveDown moves the selection cursor down by one row (clamped at last row).
func (b *MemoryBrowser) MoveDown() {
	if b == nil {
		return
	}
	if b.sel < len(b.filtered)-1 {
		b.sel++
	}
}

// Selected returns the currently selected row, or nil if the list is empty.
func (b *MemoryBrowser) Selected() *MemoryRow {
	if b == nil || b.sel < 0 || b.sel >= len(b.filtered) {
		return nil
	}
	row := b.filtered[b.sel]
	return &row
}

// Search sets the query and re-filters the list. A query starting with "#"
// filters by tag; otherwise it matches content substring.
func (b *MemoryBrowser) Search(query string) {
	if b == nil {
		return
	}
	b.query = query
	b.applyFilter()
	if b.sel >= len(b.filtered) {
		b.sel = max(0, len(b.filtered)-1)
	}
}

func (b *MemoryBrowser) applyFilter() {
	if b.query == "" {
		b.filtered = b.all
		return
	}

	if strings.HasPrefix(b.query, "#") {
		tag := strings.ToLower(strings.TrimSpace(b.query[1:]))
		b.filtered = nil
		for _, m := range b.all {
			for _, t := range m.Tags {
				if strings.ToLower(t) == tag {
					b.filtered = append(b.filtered, m)
					break
				}
			}
		}
		return
	}

	needle := strings.ToLower(b.query)
	b.filtered = nil
	for _, m := range b.all {
		if strings.Contains(strings.ToLower(m.Content), needle) {
			b.filtered = append(b.filtered, m)
		}
	}
}

// uniqueTags returns the count of distinct tags across all memories.
func (b *MemoryBrowser) uniqueTags() int {
	seen := map[string]bool{}
	for _, m := range b.all {
		for _, t := range m.Tags {
			seen[t] = true
		}
	}
	return len(seen)
}

// truncateContent shortens content to maxRunes runes, appending "…" if truncated.
func truncateContent(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// formatDate returns a YYYY-MM-DD string from a time.Time.
func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// Render produces the full memory browser view: search bar, list, and footer count.
func (b *MemoryBrowser) Render(styles Styles, width, height int) string {
	if width < 20 {
		width = 20
	}
	if height < 5 {
		height = 5
	}

	var out strings.Builder

	// Search bar
	searchDisplay := b.query
	if searchDisplay == "" {
		searchDisplay = "(type to filter, #tag to filter by tag)"
	}
	out.WriteString(styles.AccentText.Render("Search: "))
	out.WriteString(styles.Muted.Render("[" + searchDisplay + "]"))
	out.WriteString("\n")
	out.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	out.WriteString("\n")

	// List
	listHeight := height - 5
	if listHeight < 1 {
		listHeight = 1
	}

	if len(b.filtered) == 0 {
		out.WriteString("\n")
		out.WriteString(styles.Muted.Render("  (no memories)"))
		out.WriteString("\n")
	} else {
		maxShow := listHeight
		if maxShow > len(b.filtered) {
			maxShow = len(b.filtered)
		}
		contentWidth := width - 16
		if contentWidth < 10 {
			contentWidth = 10
		}
		for i := 0; i < maxShow; i++ {
			row := b.filtered[i]
			tagStr := strings.Join(row.Tags, ",")
			if tagStr == "" {
				tagStr = "—"
			}
			preview := truncateContent(row.Content, contentWidth)
			dateStr := formatDate(row.Created)

			line := fmt.Sprintf("  %s · %s · %s",
				styles.AccentText.Render(tagStr),
				styles.Content.Render(preview),
				styles.Muted.Render(dateStr),
			)
			if i == b.sel {
				line = styles.SidebarSel.Render("▸ " + truncateContent(row.Content, width-6))
			}
			out.WriteString(line)
			out.WriteString("\n")
		}
	}

	// Count footer
	out.WriteString("\n")
	out.WriteString(styles.Muted.Render(
		fmt.Sprintf("  %d memories · %d tags", len(b.all), b.uniqueTags()),
	))
	out.WriteString("\n")

	return out.String()
}

// DetailView renders the full detail of a single memory row.
func (b *MemoryBrowser) DetailView(row *MemoryRow, styles Styles, width int) string {
	if row == nil {
		return styles.Muted.Render("  (no memory selected)")
	}
	if width < 20 {
		width = 20
	}

	var out strings.Builder
	out.WriteString(styles.AccentText.Render("Memory Detail"))
	out.WriteString("\n")
	out.WriteString(styles.Muted.Render(strings.Repeat("─", max(width-2, 10))))
	out.WriteString("\n\n")

	out.WriteString(styles.AccentText.Render("ID"))
	out.WriteString("\n")
	out.WriteString(styles.Content.Render("  " + row.ID))
	out.WriteString("\n\n")

	out.WriteString(styles.AccentText.Render("Tags"))
	out.WriteString("\n")
	if len(row.Tags) == 0 {
		out.WriteString(styles.Muted.Render("  (none)"))
	} else {
		sort.Strings(row.Tags)
		out.WriteString(styles.AccentText.Render("  " + strings.Join(row.Tags, ", ")))
	}
	out.WriteString("\n\n")

	out.WriteString(styles.AccentText.Render("Content"))
	out.WriteString("\n")
	out.WriteString(styles.Content.Render("  " + row.Content))
	out.WriteString("\n\n")

	out.WriteString(styles.AccentText.Render("Created"))
	out.WriteString("\n")
	out.WriteString(styles.Muted.Render("  " + formatDate(row.Created)))
	out.WriteString("\n\n")

	out.WriteString(styles.AccentText.Render("Importance"))
	out.WriteString("\n")
	out.WriteString(styles.Muted.Render(fmt.Sprintf("  %.2f", row.Importance)))
	out.WriteString("\n")

	return out.String()
}
