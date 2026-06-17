// SPDX-License-Identifier: MIT
// Purpose: tests for issue #328 — Kanban view with lanes for todo states.
package tui

import (
	"strings"
	"testing"
)

func TestNewKanbanView(t *testing.T) {
	k := NewKanbanView()
	if k == nil {
		t.Fatal("expected non-nil KanbanView")
	}
	if len(k.Columns) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(k.Columns))
	}
	expected := []string{"Backlog", "Ready", "In Progress", "Blocked", "Done"}
	for i, title := range expected {
		if k.Columns[i].Title != title {
			t.Errorf("column %d: expected %q, got %q", i, title, k.Columns[i].Title)
		}
	}
	if k.ColIdx != 0 {
		t.Errorf("expected ColIdx=0, got %d", k.ColIdx)
	}
	if k.ItemIdx != 0 {
		t.Errorf("expected ItemIdx=0, got %d", k.ItemIdx)
	}
}

func TestKanbanSetTodos(t *testing.T) {
	k := NewKanbanView()
	todos := []TodoRow{
		{ID: "st-1", Title: "Open task", Priority: "P0", Status: "open", Type: "bug"},
		{ID: "st-2", Title: "In progress", Priority: "P1", Status: "in_progress", Type: "task"},
		{ID: "st-3", Title: "Blocked", Priority: "P2", Status: "blocked", Type: "task"},
		{ID: "st-4", Title: "Done", Priority: "P3", Status: "done", Type: "feature"},
		{ID: "st-5", Title: "Cancelled", Priority: "P0", Status: "cancelled", Type: "chore"},
	}
	k.SetTodos(todos)

	if len(k.Columns[0].Items) != 1 {
		t.Errorf("Backlog: expected 1 item, got %d", len(k.Columns[0].Items))
	}
	if len(k.Columns[2].Items) != 1 {
		t.Errorf("In Progress: expected 1 item, got %d", len(k.Columns[2].Items))
	}
	if len(k.Columns[3].Items) != 1 {
		t.Errorf("Blocked: expected 1 item, got %d", len(k.Columns[3].Items))
	}
	if len(k.Columns[4].Items) != 2 {
		t.Errorf("Done: expected 2 items (done+cancelled), got %d", len(k.Columns[4].Items))
	}
}

func TestKanbanMoveUpDown(t *testing.T) {
	k := NewKanbanView()
	k.SetTodos([]TodoRow{
		{ID: "st-1", Title: "A", Status: "open"},
		{ID: "st-2", Title: "B", Status: "open"},
		{ID: "st-3", Title: "C", Status: "open"},
	})

	if k.ItemIdx != 0 {
		t.Fatalf("expected ItemIdx=0, got %d", k.ItemIdx)
	}

	k.MoveDown()
	if k.ItemIdx != 1 {
		t.Errorf("after MoveDown: expected 1, got %d", k.ItemIdx)
	}

	k.MoveDown()
	if k.ItemIdx != 2 {
		t.Errorf("after MoveDown: expected 2, got %d", k.ItemIdx)
	}

	// Clamp at last item.
	k.MoveDown()
	if k.ItemIdx != 2 {
		t.Errorf("after MoveDown at last: expected 2, got %d", k.ItemIdx)
	}

	k.MoveUp()
	if k.ItemIdx != 1 {
		t.Errorf("after MoveUp: expected 1, got %d", k.ItemIdx)
	}

	k.MoveUp()
	if k.ItemIdx != 0 {
		t.Errorf("after MoveUp: expected 0, got %d", k.ItemIdx)
	}

	// Clamp at first item.
	k.MoveUp()
	if k.ItemIdx != 0 {
		t.Errorf("after MoveUp at first: expected 0, got %d", k.ItemIdx)
	}
}

func TestKanbanMoveLeftRight(t *testing.T) {
	k := NewKanbanView()

	// Move right without items — just moves the cursor.
	k.MoveRight()
	if k.ColIdx != 1 {
		t.Errorf("after MoveRight (no items): expected ColIdx=1, got %d", k.ColIdx)
	}

	k.MoveLeft()
	if k.ColIdx != 0 {
		t.Errorf("after MoveLeft: expected ColIdx=0, got %d", k.ColIdx)
	}

	// Can't move left from first column.
	k.MoveLeft()
	if k.ColIdx != 0 {
		t.Errorf("after MoveLeft at first: expected ColIdx=0, got %d", k.ColIdx)
	}
}

func TestKanbanMoveRightMovesItem(t *testing.T) {
	k := NewKanbanView()
	k.SetTodos([]TodoRow{
		{ID: "st-1", Title: "Move me", Priority: "P0", Status: "open"},
	})

	// Select the item (column 0, item 0).
	if k.Selected() == nil {
		t.Fatal("expected selected item")
	}
	if k.Selected().ID != "st-1" {
		t.Errorf("expected st-1, got %q", k.Selected().ID)
	}

	// Move right — should move the item to Ready column.
	k.MoveRight()

	if k.ColIdx != 1 {
		t.Errorf("expected ColIdx=1, got %d", k.ColIdx)
	}
	if len(k.Columns[0].Items) != 0 {
		t.Errorf("Backlog should be empty, got %d items", len(k.Columns[0].Items))
	}
	if len(k.Columns[1].Items) != 1 {
		t.Errorf("Ready should have 1 item, got %d", len(k.Columns[1].Items))
	}
	if k.Columns[1].Items[0].Status != "ready" {
		t.Errorf("expected status 'ready', got %q", k.Columns[1].Items[0].Status)
	}
}

func TestKanbanSelected(t *testing.T) {
	k := NewKanbanView()

	// No items — Selected returns nil.
	if k.Selected() != nil {
		t.Error("expected nil when no items")
	}

	k.SetTodos([]TodoRow{
		{ID: "st-1", Title: "A", Status: "open"},
		{ID: "st-2", Title: "B", Status: "open"},
	})

	sel := k.Selected()
	if sel == nil {
		t.Fatal("expected non-nil selected")
	}
	if sel.ID != "st-1" {
		t.Errorf("expected st-1, got %q", sel.ID)
	}

	k.MoveDown()
	sel = k.Selected()
	if sel == nil {
		t.Fatal("expected non-nil selected after MoveDown")
	}
	if sel.ID != "st-2" {
		t.Errorf("expected st-2, got %q", sel.ID)
	}
}

func TestKanbanRender(t *testing.T) {
	k := NewKanbanView()
	k.SetTodos([]TodoRow{
		{ID: "st-1", Title: "Open task", Priority: "P0", Status: "open"},
		{ID: "st-2", Title: "Done task", Priority: "P1", Status: "done"},
	})

	m := NewModel()
	out := k.Render(m.Styles, 80, 20)

	if !strings.Contains(out, "Kanban Board") {
		t.Error("expected 'Kanban Board' header")
	}
	if !strings.Contains(out, "Backlog") {
		t.Error("expected 'Backlog' column header")
	}
	if !strings.Contains(out, "Done") {
		t.Error("expected 'Done' column header")
	}
	if !strings.Contains(out, "st-1") {
		t.Error("expected st-1 in render")
	}
	if !strings.Contains(out, "st-2") {
		t.Error("expected st-2 in render")
	}
}

func TestKanbanRenderEmpty(t *testing.T) {
	k := NewKanbanView()
	m := NewModel()
	out := k.Render(m.Styles, 80, 10)

	if !strings.Contains(out, "Kanban Board") {
		t.Error("expected header even when empty")
	}
	if !strings.Contains(out, "Backlog") {
		t.Error("expected column headers even when empty")
	}
}

func TestKanbanViewKindEnum(t *testing.T) {
	if ViewKanban.String() != "Kanban" {
		t.Errorf("expected 'Kanban', got %q", ViewKanban.String())
	}
	if !strings.Contains(ViewKanban.Short(), "Kanban") {
		t.Errorf("expected Short to contain 'Kanban', got %q", ViewKanban.Short())
	}
}

func TestKanbanInSidebar(t *testing.T) {
	items := DefaultSidebarItems()
	found := false
	for _, it := range items {
		if it.View == ViewKanban {
			found = true
			if it.Icon != "▮" {
				t.Errorf("expected icon ▮, got %q", it.Icon)
			}
			if it.Shortcut == "0" {
				t.Error("shortcut 0 is taken by Dashboard")
			}
		}
	}
	if !found {
		t.Error("expected ViewKanban in default sidebar items")
	}
}

func TestKanbanSetTodosClampsSelection(t *testing.T) {
	k := NewKanbanView()
	k.ColIdx = 3
	k.ItemIdx = 10

	k.SetTodos([]TodoRow{
		{ID: "st-1", Title: "A", Status: "open"},
	})

	// After SetTodos, selection should be clamped.
	if k.ColIdx > len(k.Columns)-1 {
		t.Errorf("ColIdx should be clamped, got %d", k.ColIdx)
	}
	// ItemIdx should be within bounds of current column.
	if k.ColIdx >= 0 && k.ColIdx < len(k.Columns) {
		maxIdx := len(k.Columns[k.ColIdx].Items) - 1
		if k.ItemIdx > maxIdx {
			t.Errorf("ItemIdx should be clamped to %d, got %d", maxIdx, k.ItemIdx)
		}
	}
}
