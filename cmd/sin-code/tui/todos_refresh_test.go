// SPDX-License-Identifier: MIT
// Purpose: tests for issue #325 — RefreshTodosCmd queries real bbolt
// store instead of returning hardcoded zeros.
package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

// tempTodoDB creates a temp bbolt DB for the todo store and returns a
// cleanup function that restores the original hook.
func tempTodoDB(t *testing.T) (*todo.Store, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.db")
	store, err := todo.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	orig := todoOpenHook
	todoOpenHook = func(p string) (*todo.Store, error) {
		if p == "" {
			return todo.Open(path)
		}
		return todo.Open(p)
	}
	return store, func() {
		todoOpenHook = orig
		_ = store.Close()
	}
}

// TestRefreshTodosCmdReturnsRealCounts verifies that RefreshTodosCmd
// queries the real store and returns non-zero counts when todos exist.
func TestRefreshTodosCmdReturnsRealCounts(t *testing.T) {
	store, cleanup := tempTodoDB(t)
	defer cleanup()

	// Seed the store with real todos.
	if err := store.Add(&todo.Todo{Title: "Task A", Priority: todo.PriorityP0, Status: todo.StatusOpen}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(&todo.Todo{Title: "Task B", Priority: todo.PriorityP1, Status: todo.StatusOpen}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(&todo.Todo{Title: "Task C", Priority: todo.PriorityP2, Status: todo.StatusBlocked}); err != nil {
		t.Fatal(err)
	}

	// The default todoDataHook uses todoOpenHook, which is overridden
	// above to point at our temp DB. No need to reset todoDataHook.

	cmd := RefreshTodosCmd()
	msg := cmd()

	refresh, ok := msg.(TodosRefreshMsg)
	if !ok {
		// Graceful degradation might return CountsMsg if store is nil.
		// But with our hook, it should return TodosRefreshMsg.
		t.Fatalf("expected TodosRefreshMsg, got %T", msg)
	}

	if refresh.Counts.Open != 3 {
		t.Errorf("expected Open=3, got %d", refresh.Counts.Open)
	}
	if refresh.Counts.Blocked != 1 {
		t.Errorf("expected Blocked=1, got %d", refresh.Counts.Blocked)
	}
}

// TestRefreshTodosCmdReturnsRealItems verifies that RefreshTodosCmd
// returns real todo items from the store, not an empty list.
func TestRefreshTodosCmdReturnsRealItems(t *testing.T) {
	store, cleanup := tempTodoDB(t)
	defer cleanup()

	if err := store.Add(&todo.Todo{Title: "Real Todo", Priority: todo.PriorityP0, Status: todo.StatusOpen, Type: todo.TypeBug}); err != nil {
		t.Fatal(err)
	}

	cmd := RefreshTodosCmd()
	msg := cmd()

	refresh, ok := msg.(TodosRefreshMsg)
	if !ok {
		t.Fatalf("expected TodosRefreshMsg, got %T", msg)
	}

	if len(refresh.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(refresh.Items))
	}
	if refresh.Items[0].Title != "Real Todo" {
		t.Errorf("expected title 'Real Todo', got %q", refresh.Items[0].Title)
	}
	if refresh.Items[0].Priority != "P0" {
		t.Errorf("expected priority 'P0', got %q", refresh.Items[0].Priority)
	}
}

// TestRefreshTodosCmdOverdueComputed verifies that overdue todos (open
// with DueAt in the past) are counted correctly.
func TestRefreshTodosCmdOverdueComputed(t *testing.T) {
	store, cleanup := tempTodoDB(t)
	defer cleanup()

	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	// Overdue: open with past DueAt.
	if err := store.Add(&todo.Todo{Title: "Overdue", Priority: todo.PriorityP0, Status: todo.StatusOpen, DueAt: &past}); err != nil {
		t.Fatal(err)
	}
	// Not overdue: open with future DueAt.
	if err := store.Add(&todo.Todo{Title: "Future", Priority: todo.PriorityP1, Status: todo.StatusOpen, DueAt: &future}); err != nil {
		t.Fatal(err)
	}
	// Not overdue: done with past DueAt (closed todos don't count).
	if err := store.Add(&todo.Todo{Title: "Closed", Priority: todo.PriorityP2, Status: todo.StatusDone, DueAt: &past}); err != nil {
		t.Fatal(err)
	}

	cmd := RefreshTodosCmd()
	msg := cmd()

	refresh, ok := msg.(TodosRefreshMsg)
	if !ok {
		t.Fatalf("expected TodosRefreshMsg, got %T", msg)
	}

	if refresh.Counts.Overdue != 1 {
		t.Errorf("expected Overdue=1, got %d", refresh.Counts.Overdue)
	}
	if refresh.Counts.Open != 2 {
		t.Errorf("expected Open=2, got %d", refresh.Counts.Open)
	}
}

// TestRefreshTodosCmdGracefulDegradation verifies that RefreshTodosCmd
// returns CountsMsg{} (zeros) when the store is unavailable, rather
// than crashing.
func TestRefreshTodosCmdGracefulDegradation(t *testing.T) {
	// Point the hook at a non-existent path to force an error.
	origOpen := todoOpenHook
	todoOpenHook = func(p string) (*todo.Store, error) {
		return nil, os.ErrNotExist
	}
	defer func() { todoOpenHook = origOpen }()

	cmd := RefreshTodosCmd()
	msg := cmd()

	// Should return CountsMsg{} (graceful degradation, not a crash).
	c, ok := msg.(CountsMsg)
	if !ok {
		t.Fatalf("expected CountsMsg on error, got %T", msg)
	}
	if c.Open != 0 || c.Blocked != 0 || c.Overdue != 0 || c.Ready != 0 {
		t.Errorf("expected all zeros on error, got %+v", c)
	}
}

// TestTodosRefreshMsgUpdatesModel verifies that the Update handler
// correctly applies TodosRefreshMsg to the model's sidebar counts and
// todo items.
func TestTodosRefreshMsgUpdatesModel(t *testing.T) {
	m := NewModel()

	items := []TodoRow{
		{ID: "st-1", Title: "Alpha", Priority: "P0", Status: "open", Type: "bug"},
		{ID: "st-2", Title: "Beta", Priority: "P1", Status: "blocked", Type: "task"},
	}
	refresh := TodosRefreshMsg{
		Counts: CountsMsg{Open: 2, Ready: 1, Blocked: 1, Overdue: 0},
		Items:  items,
	}

	_, _ = m.Update(refresh)

	if m.Sidebar.TodoOpen != 2 {
		t.Errorf("expected TodoOpen=2, got %d", m.Sidebar.TodoOpen)
	}
	if m.Sidebar.TodoBlocked != 1 {
		t.Errorf("expected TodoBlocked=1, got %d", m.Sidebar.TodoBlocked)
	}
	if m.Sidebar.TodoReady != 1 {
		t.Errorf("expected TodoReady=1, got %d", m.Sidebar.TodoReady)
	}
	if len(m.TodoItems) != 2 {
		t.Errorf("expected 2 todo items, got %d", len(m.TodoItems))
	}
	if m.TodoItems[0].ID != "st-1" {
		t.Errorf("expected first item ID 'st-1', got %q", m.TodoItems[0].ID)
	}

	// Kanban view should also be populated.
	if m.KanbanView == nil {
		t.Fatal("expected KanbanView to be initialized")
	}
}
