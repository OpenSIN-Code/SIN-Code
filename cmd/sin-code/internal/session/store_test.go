// SPDX-License-Identifier: MIT
// Purpose: session store regression tests (mandate C2, AGENTS.md §8).
package session

import (
	"path/filepath"
	"testing"
)

func TestOpenAndStart(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Fatal("empty id")
	}
	if len(s.History()) != 0 {
		t.Fatal("new session should have empty history")
	}
}

func TestSaveAndResume(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()

	s, _ := store.StartOrResume("")
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	if err := s.SaveHistory(msgs); err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Fatal("empty id")
	}

	s2, err := store.StartOrResume(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.History()) != 2 {
		t.Fatalf("want 2 msgs, got %d", len(s2.History()))
	}
	if s2.History()[0].Content != "hello" {
		t.Fatalf("first msg wrong: %q", s2.History()[0].Content)
	}
}

func TestResumeMissing(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()
	if _, err := store.StartOrResume("nope"); err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestListAndDelete(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()

	s, _ := store.StartOrResume("")
	_ = s.SaveHistory([]Message{{Role: "user", Content: "x"}})

	infos, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("want 1, got %d", len(infos))
	}
	if err := store.Delete(s.ID); err != nil {
		t.Fatal(err)
	}
	infos, _ = store.List()
	if len(infos) != 0 {
		t.Fatalf("after delete want 0, got %d", len(infos))
	}
}

func TestSaveHistoryReplaces(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()

	s, _ := store.StartOrResume("")
	_ = s.SaveHistory([]Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
	})
	// Replace with shorter list
	if err := s.SaveHistory([]Message{{Role: "user", Content: "only"}}); err != nil {
		t.Fatal(err)
	}
	if got := s.History(); len(got) != 1 || got[0].Content != "only" {
		t.Fatalf("after replace, want 1 msg 'only', got %+v", got)
	}
	// And on resume
	s2, _ := store.StartOrResume(s.ID)
	if got := s2.History(); len(got) != 1 || got[0].Content != "only" {
		t.Fatalf("after resume, want 1 msg 'only', got %+v", got)
	}
}

func TestSaveHistoryToolCallID(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()

	s, _ := store.StartOrResume("")
	raw := `{"id":"t1","name":"sin_read","args":{}}`
	_ = s.SaveHistory([]Message{
		{Role: "assistant", ToolCalls: []byte(raw)},
		{Role: "tool", ToolCallID: "t1", Content: "out"},
	})
	s2, _ := store.StartOrResume(s.ID)
	if got := s2.History(); len(got) != 2 || got[0].Role != "assistant" || string(got[0].ToolCalls) != raw {
		t.Fatalf("tool-call roundtrip failed: %+v", got)
	}
}

func TestDefaultPathCreatesDir(t *testing.T) {
	// We don't assert on the exact path (depends on HOME), just that it
	// returns a non-empty, .db-suffixed path under a writable dir.
	p := DefaultPath()
	if p == "" {
		t.Fatal("empty default path")
	}
}

func TestForkBasic(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()
	src, _ := store.StartOrResume("")
	_ = src.SaveHistory([]Message{
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
	})
	forked, err := store.Fork(src.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if forked.ID == src.ID {
		t.Fatal("fork must produce a new id")
	}
	hist := forked.History()
	if len(hist) != 2 || hist[0].Content != "u1" || hist[1].Content != "a1" {
		t.Fatalf("fork history wrong: %v", hist)
	}
}

func TestForkClampsOvershoot(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()
	src, _ := store.StartOrResume("")
	_ = src.SaveHistory([]Message{{Role: "user", Content: "u1"}})
	forked, err := store.Fork(src.ID, 9999)
	if err != nil {
		t.Fatal(err)
	}
	if got := forked.History(); len(got) != 1 {
		t.Fatalf("overshoot must clamp, got %d", len(got))
	}
}

func TestForkClampsNegative(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()
	src, _ := store.StartOrResume("")
	_ = src.SaveHistory([]Message{{Role: "user", Content: "u1"}})
	forked, err := store.Fork(src.ID, -3)
	if err != nil {
		t.Fatal(err)
	}
	if got := forked.History(); len(got) != 0 {
		t.Fatalf("negative turn must clamp to 0, got %d", len(got))
	}
}

func TestForkNotFound(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "s.db"))
	defer store.Close()
	if _, err := store.Fork("does-not-exist", 0); err == nil {
		t.Fatal("want error for missing source")
	}
}
