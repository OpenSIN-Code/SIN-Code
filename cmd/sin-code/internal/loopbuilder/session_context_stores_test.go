// SPDX-License-Identifier: MIT
// Purpose: unit tests for the SessionContextBuilder store adapters (issue #379).
package loopbuilder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

func TestTodoStoreReader_Open(t *testing.T) {
	db := filepath.Join(t.TempDir(), "todo.db")
	store, err := todo.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Add(&todo.Todo{Title: "open task", Priority: todo.PriorityP1}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(&todo.Todo{Title: "done task", Priority: todo.PriorityP2, Status: todo.StatusDone}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(&todo.Todo{Title: "blocked task", Priority: todo.PriorityP0, Status: todo.StatusBlocked}); err != nil {
		t.Fatal(err)
	}

	reader := &TodoStoreReader{Store: store}
	items, err := reader.Open(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 open todos, got %d: %v", len(items), items)
	}
	var hasOpen, hasBlocked bool
	for _, it := range items {
		if it == "done task" {
			t.Fatalf("done task should not appear: %v", items)
		}
		if strings.Contains(it, "open task") {
			hasOpen = true
		}
		if strings.Contains(it, "blocked task") {
			hasBlocked = true
		}
	}
	if !hasOpen || !hasBlocked {
		t.Fatalf("expected open and blocked tasks, got %v", items)
	}

	blocked, err := reader.Open(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || !strings.Contains(blocked[0], "blocked task") {
		t.Fatalf("expected exactly one blocked task, got %v", blocked)
	}
}

func TestInMemoryTodoReader_Open(t *testing.T) {
	reader := &InMemoryTodoReader{Items: []string{"a", "b"}}
	items, err := reader.Open(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0] != "a" || items[1] != "b" {
		t.Fatalf("unexpected items: %v", items)
	}
}

func TestFileAutoMemoryReader_IndexBytes_Workspace(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, "MEMORY.md"), []byte("# Workspace Memory\n- rule 1")); err != nil {
		t.Fatal(err)
	}
	reader := &FileAutoMemoryReader{Workspace: dir}
	data, err := reader.IndexBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Workspace Memory\n- rule 1" {
		t.Fatalf("unexpected memory: %q", string(data))
	}
}

func TestFileAutoMemoryReader_IndexBytes_Fallback(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, "MEMORY.md"), []byte("# Home Memory")); err != nil {
		t.Fatal(err)
	}
	reader := &FileAutoMemoryReader{Workspace: t.TempDir(), HomeDir: dir}
	data, err := reader.IndexBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Home Memory" {
		t.Fatalf("unexpected memory: %q", string(data))
	}
}

func TestFileAutoMemoryReader_IndexBytes_Missing(t *testing.T) {
	reader := &FileAutoMemoryReader{Workspace: t.TempDir()}
	data, err := reader.IndexBytes()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty data when MEMORY.md missing, got %q", string(data))
	}
}

func TestNewDefaultSessionContextBuilder_NilStores(t *testing.T) {
	b := NewDefaultSessionContextBuilder("/tmp/ws", nil, "", nil, nil, nil, nil, "")
	if b == nil {
		t.Fatal("expected non-nil builder")
	}
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
