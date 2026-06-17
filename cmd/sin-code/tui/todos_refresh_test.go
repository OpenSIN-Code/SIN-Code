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

// setupTodoDB creates a temp bbolt DB, seeds it with the given todos,
// closes it, and wires todoOpenHook to reopen it. Returns a cleanup fn.
// The store must be closed before RefreshTodosCmd is called to avoid
// bbolt lock contention.
func setupTodoDB(t *testing.T, seed ...*todo.Todo) func() {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.db")
	store, err := todo.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, td := range seed {
		if err := store.Add(td); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	_ = store.Close()

	orig := todoOpenHook
	todoOpenHook = func(p string) (*todo.Store, error) {
		if p == "" {
			return todo.Open(path)
		}
		return todo.Open(p)
	}
	return func() {
		todoOpenHook = orig
	}
}

// TestRefreshTodosCmdReturnsRealCounts verifies that RefreshTodosCmd
// queries the real store and returns non-zero counts when todos exist.
func TestRefreshTodosCmdReturnsRealCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.db")
	store, err := todo.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Create two todos: A (open) blocks B (open) → B is "blocked".
	todoA := &todo.Todo{Title: "Task A", Priority: todo.PriorityP0, Status: todo.StatusOpen}
	todoB := &todo.Todo{Title: "Task B", Priority: todo.PriorityP1, Status: todo.StatusOpen}
	if err := store.Add(todoA); err != nil {
		t.Fatalf("Add A: %v", err)
	}
	if err := store.Add(todoB); err != nil {
		t.Fatalf("Add B: %v", err)
	}
	if err := store.AddDep(todo.Dependency{From: todoB.ID, To: todoA.ID, Type: todo.DepBlocks}); err != nil {
		t.Fatalf("AddDep: %v", err)
	}
	_ = store.Close()

	orig := todoOpenHook
	todoOpenHook = func(p string) (*todo.Store, error) {
		if p == "" {
			return todo.Open(path)
		}
		return todo.Open(p)
	}
	defer func() { todoOpenHook = orig }()

	cmd := RefreshTodosCmd()
	msg := cmd()

	refresh, ok := msg.(TodosRefreshMsg)
	if !ok {
		t.Fatalf("expected TodosRefreshMsg, got %T", msg)
	}

	if refresh.Counts.Open != 2 {
		t.Errorf("expected Open=2, got %d", refresh.Counts.Open)
	}
	if refresh.Counts.Blocked != 1 {
		t.Errorf("expected Blocked=1, got %d", refresh.Counts.Blocked)
	}
	if refresh.Counts.Ready != 1 {
		t.Errorf("expected Ready=1, got %d", refresh.Counts.Ready)
	}
}

// TestRefreshTodosCmdReturnsRealItems verifies that RefreshTodosCmd
// returns real todo items from the store, not an empty list.
func TestRefreshTodosCmdReturnsRealItems(t *testing.T) {
	cleanup := setupTodoDB(t,
		&todo.Todo{Title: "Real Todo", Priority: todo.PriorityP0, Status: todo.StatusOpen, Type: todo.TypeBug},
	)
	defer cleanup()

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
	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	cleanup := setupTodoDB(t,
		&todo.Todo{Title: "Overdue", Priority: todo.PriorityP0, Status: todo.StatusOpen, DueAt: &past},
		&todo.Todo{Title: "Future", Priority: todo.PriorityP1, Status: todo.StatusOpen, DueAt: &future},
		&todo.Todo{Title: "Closed", Priority: todo.PriorityP2, Status: todo.StatusDone, DueAt: &past},
	)
	defer cleanup()

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
	origOpen := todoOpenHook
	todoOpenHook = func(p string) (*todo.Store, error) {
		return nil, os.ErrNotExist
	}
	defer func() { todoOpenHook = origOpen }()

	cmd := RefreshTodosCmd()
	msg := cmd()

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

	if m.KanbanView == nil {
		t.Fatal("expected KanbanView to be initialized")
	}
}
