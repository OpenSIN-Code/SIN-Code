// SPDX-License-Identifier: MIT
// Purpose: tests for the trivial Notification interface accessors and
// Store.DB() accessor (these are depended on by the TUI banner).

package notifications

import (
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestNotification_Accessors(t *testing.T) {
	n := &Notification{
		ID:      "N1",
		Title:   "T",
		Message: "M",
		Type:    TypeTodoCreated,
	}
	if n.GetID() != "N1" {
		t.Fatalf("GetID: %q", n.GetID())
	}
	if n.GetTitle() != "T" {
		t.Fatalf("GetTitle: %q", n.GetTitle())
	}
	if n.GetMessage() != "M" {
		t.Fatalf("GetMessage: %q", n.GetMessage())
	}
	if n.GetType() != "todo_created" {
		t.Fatalf("GetType: %q", n.GetType())
	}
}

func TestStore_DBReturnsValidHandle(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir + "/n.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db := s.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}
	// Confirm the underlying bbolt handle is usable
	if err := db.View(func(*bolt.Tx) error { return nil }); err != nil {
		t.Fatalf("View failed: %v", err)
	}
}
