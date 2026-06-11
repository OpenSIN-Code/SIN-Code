// SPDX-License-Identifier: MIT
// Purpose: tests for the Todo store CRUD operations.

package todo

import (
	"path/filepath"
	"testing"
)

func TestStore_AddListAndGet(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	t1 := &Todo{ID: "T01", Title: "first", Type: TypeTask, Priority: PriorityP1}
	if err := s.Add(t1); err != nil {
		t.Fatal(err)
	}
	t2 := &Todo{ID: "T02", Title: "second", Type: TypeBug, Priority: PriorityP2}
	if err := s.Add(t2); err != nil {
		t.Fatal(err)
	}

	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("List: %d items", len(got))
	}
	one, err := s.Get("T01")
	if err != nil {
		t.Fatal(err)
	}
	if one.Title != "first" {
		t.Fatalf("Get: %q", one.Title)
	}
}

func TestStore_AddRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(filepath.Join(dir, "t.db"))
	defer s.Close()

	if err := s.Add(nil); err == nil {
		t.Fatal("nil must error")
	}
	if err := s.Add(&Todo{ID: "T"}); err == nil {
		t.Fatal("empty title must error")
	}
}

func TestStore_Complete(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(filepath.Join(dir, "t.db"))
	defer s.Close()

	if err := s.Add(&Todo{ID: "T01", Title: "x", Priority: PriorityP2}); err != nil {
		t.Fatal(err)
	}
	// Update the status to done
	if err := s.Update(&Todo{ID: "T01", Title: "x", Priority: PriorityP2, Status: StatusDone}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("T01")
	if got.Status != StatusDone {
		t.Fatalf("status: %q", got.Status)
	}
}

func TestStore_Delete(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(filepath.Join(dir, "t.db"))
	defer s.Close()

	s.Add(&Todo{ID: "T01", Title: "x", Priority: PriorityP2})
	if err := s.Delete("T01", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("T01"); err == nil {
		t.Fatal("Get after Delete should error")
	}
}
