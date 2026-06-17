// SPDX-License-Identifier: MIT
// Purpose: Kanban view — renders todos as a board with columns for each
// todo state. Supports navigation with arrow keys and moving items
// between columns (which changes their status).
package tui

import (
	"fmt"
	"strings"
)

// KanbanColumn represents a single lane in the Kanban board.
type KanbanColumn struct {
	Title string
	Color string
	Items []KanbanItem
}

// KanbanItem represents a single card in a Kanban column.
type KanbanItem struct {
	ID       string
	Title    string
	Priority string
	Assignee string
	Status   string
}

// KanbanView is the full Kanban board state with columns and selection.
type KanbanView struct {
	Columns       []KanbanColumn
	ColIdx        int // selected column
	ItemIdx       int // selected item within column
	columnStatus  []string
}

// Column status constants — each column maps to a todo status string.
const (
	kanbanStatusBacklog     = "open"
	kanbanStatusReady       = "ready"
	kanbanStatusInProgress  = "in_progress"
	kanbanStatusBlocked     = "blocked"
	kanbanStatusDone        = "done"
)

// NewKanbanView creates a KanbanView with 5 empty columns:
// Backlog, Ready, In Progress, Blocked, Done.
func NewKanbanView() *KanbanView {
	return &KanbanView{
		Columns: []KanbanColumn{
			{Title: "Backlog", Color: "muted"},
			{Title: "Ready", Color: "green"},
			{Title: "In Progress", Color: "yellow"},
			{Title: "Blocked", Color: "red"},
			{Title: "Done", Color: "green"},
		},
		ColIdx:       0,
		ItemIdx:      0,
		columnStatus: []string{kanbanStatusBacklog, kanbanStatusReady, kanbanStatusInProgress, kanbanStatusBlocked, kanbanStatusDone},
	}
}

// SetTodos distributes a slice of TodoRow into the appropriate columns
// based on each todo's Status field. Clears all existing items first.
func (k *KanbanView) SetTodos(todos []TodoRow) {
	for i := range k.Columns {
		k.Columns[i].Items = k.Columns[i].Items[:0]
	}

	for _, t := range todos {
		item := KanbanItem{
			ID:       t.ID,
			Title:    t.Title,
			Priority: t.Priority,
			Assignee: t.Assignee,
			Status:   t.Status,
		}
		col := k.columnForStatus(t.Status)
		k.Columns[col].Items = append(k.Columns[col].Items, item)
	}

	// Clamp selection.
	if k.ColIdx >= len(k.Columns) {
		k.ColIdx = 0
	}
	k.clampItemIdx()
}

// columnForStatus returns the column index for a given status string.
// Unknown statuses default to Backlog (column 0).
func (k *KanbanView) columnForStatus(status string) int {
	for i, s := range k.columnStatus {
		if s == status {
			return i
		}
	}
	if status == "cancelled" {
		return 4 // Done
	}
	return 0 // Backlog
}

// statusForColumn returns the status string for a given column index.
func (k *KanbanView) statusForColumn(col int) string {
	if col < 0 || col >= len(k.columnStatus) {
		return kanbanStatusBacklog
	}
	return k.columnStatus[col]
}

// clampItemIdx ensures ItemIdx is within bounds for the current column.
func (k *KanbanView) clampItemIdx() {
	if k.ItemIdx < 0 {
		k.ItemIdx = 0
	}
	if k.ColIdx < 0 || k.ColIdx >= len(k.Columns) {
		return
	}
	if k.ItemIdx >= len(k.Columns[k.ColIdx].Items) {
		k.ItemIdx = len(k.Columns[k.ColIdx].Items) - 1
	}
}

// MoveUp moves the selection up within the current column.
func (k *KanbanView) MoveUp() {
	if k.ItemIdx > 0 {
		k.ItemIdx--
	}
}

// MoveDown moves the selection down within the current column.
func (k *KanbanView) MoveDown() {
	if k.ColIdx < 0 || k.ColIdx >= len(k.Columns) {
		return
	}
	if k.ItemIdx < len(k.Columns[k.ColIdx].Items)-1 {
		k.ItemIdx++
	}
}

// MoveLeft moves the selection to the previous column.
func (k *KanbanView) MoveLeft() {
	if k.ColIdx > 0 {
		k.ColIdx--
		k.ItemIdx = 0
	}
}

// MoveRight moves the selection to the next column.
// When the current item exists, it moves the item to the new column
// (changing its status). When no item is selected, just moves the cursor.
func (k *KanbanView) MoveRight() {
	if k.ColIdx < len(k.Columns)-1 {
		// If there's a selected item, move it to the next column.
		if k.ColIdx >= 0 && k.ColIdx < len(k.Columns) && k.ItemIdx >= 0 && k.ItemIdx < len(k.Columns[k.ColIdx].Items) {
			item := k.Columns[k.ColIdx].Items[k.ItemIdx]
			newStatus := k.statusForColumn(k.ColIdx + 1)
			item.Status = newStatus

			// Remove from current column.
			k.Columns[k.ColIdx].Items = append(
				k.Columns[k.ColIdx].Items[:k.ItemIdx],
				k.Columns[k.ColIdx].Items[k.ItemIdx+1:]...,
			)

			// Add to next column.
			k.ColIdx++
			k.Columns[k.ColIdx].Items = append(k.Columns[k.ColIdx].Items, item)
			k.ItemIdx = len(k.Columns[k.ColIdx].Items) - 1
		} else {
			k.ColIdx++
			k.ItemIdx = 0
		}
		k.clampItemIdx()
	}
}

// Selected returns the currently selected KanbanItem, or nil if no item
// is selected.
func (k *KanbanView) Selected() *KanbanItem {
	if k.ColIdx < 0 || k.ColIdx >= len(k.Columns) {
		return nil
	}
	items := k.Columns[k.ColIdx].Items
	if k.ItemIdx < 0 || k.ItemIdx >= len(items) {
		return nil
	}
	return &items[k.ItemIdx]
}

// Render renders the Kanban board as horizontal columns within the given
// width and height.
func (k *KanbanView) Render(styles Styles, width, height int) string {
	if width < 20 {
		width = 20
	}
	if height < 5 {
		height = 5
	}
	var b strings.Builder
	b.WriteString(styles.ContentHdr.Render("Kanban Board"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n\n")

	numCols := len(k.Columns)
	colWidth := width / numCols
	if colWidth < 12 {
		colWidth = 12
	}

	// Render column headers.
	for i, col := range k.Columns {
		hdr := padRight(col.Title, colWidth)
		if i == k.ColIdx {
			b.WriteString(styles.AccentText.Render(hdr))
		} else {
			b.WriteString(styles.Bold.Render(hdr))
		}
		if i < numCols-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("\n")

	// Render separator line under headers.
	for i := range k.Columns {
		b.WriteString(styles.Muted.Render(strings.Repeat("─", colWidth)))
		if i < numCols-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("\n")

	// Render items row by row.
	maxRows := height - 5
	if maxRows < 1 {
		maxRows = 1
	}

	// Find the maximum number of items across columns.
	maxItems := 0
	for _, col := range k.Columns {
		if len(col.Items) > maxItems {
			maxItems = len(col.Items)
		}
	}
	if maxItems > maxRows {
		maxItems = maxRows
	}

	for row := 0; row < maxItems; row++ {
		for colIdx, col := range k.Columns {
			var cell string
			if row < len(col.Items) {
				item := col.Items[row]
				priDot := priorityDot(item.Priority)
				cell = fmt.Sprintf("%s %s %s", priDot, item.ID, truncateKanban(item.Title, colWidth-12))
			}
			cell = padRight(cell, colWidth)

			if colIdx == k.ColIdx && row == k.ItemIdx {
				b.WriteString(styles.SidebarSel.Render(cell))
			} else {
				b.WriteString(styles.Content.Render(cell))
			}
			if colIdx < numCols-1 {
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}

	// Footer hint.
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("  ↑/↓ navigate · ←/→ move card · ^k toggle"))
	b.WriteString("\n")

	return b.String()
}

// priorityDot returns a colored dot string for a priority level.
func priorityDot(priority string) string {
	switch priority {
	case "P0":
		return "🔴"
	case "P1":
		return "🟠"
	case "P2":
		return " " // P2 is default, no dot
	case "P3":
		return "⚪"
	}
	return " "
}

// truncateKanban shortens a string to maxLen characters, appending "…".
func truncateKanban(s string, maxLen int) string {
	if maxLen < 1 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}
