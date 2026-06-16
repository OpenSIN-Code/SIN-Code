// SPDX-License-Identifier: MIT
// Purpose: tests for issue #194 — checkpoint store.
package checkpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStore_CaptureRestoreRoundTrip(t *testing.T) {
	ws := t.TempDir()
	st, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	original := []byte("original content\n")
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), original, 0o644); err != nil {
		t.Fatal(err)
	}

	id, err := st.Capture(context.Background(), ws, "sess-1", "before-edit", []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := st.Restore(context.Background(), ws, id); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(ws, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("expected %q, got %q", original, got)
	}
}

func TestStore_TombstoneRemovesNewFile(t *testing.T) {
	ws := t.TempDir()
	st, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	id, err := st.Capture(context.Background(), ws, "sess-1", "no-file-yet", []string{"ghost.txt"})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(ws, "ghost.txt"), []byte("spooky"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := st.Restore(context.Background(), ws, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws, "ghost.txt")); !os.IsNotExist(err) {
		t.Errorf("expected ghost.txt to be removed by rewind, stat err = %v", err)
	}
}

func TestStore_BlobDedup(t *testing.T) {
	ws := t.TempDir()
	st, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	content := []byte("hello world")
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "b.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Capture(context.Background(), ws, "s1", "dup",
		[]string{"a.txt", "b.txt"}); err != nil {
		t.Fatal(err)
	}

	blobs, err := os.ReadDir(filepath.Join(ws, ".sin-code", "checkpoints", "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(blobs) != 1 {
		t.Errorf("expected 1 blob (dedup), got %d", len(blobs))
	}
}

func TestStore_ListNewestFirst(t *testing.T) {
	ws := t.TempDir()
	st, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("1"), 0o644)
	id1, err := st.Capture(context.Background(), ws, "sess-1", "first", []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("2"), 0o644)
	id2, err := st.Capture(context.Background(), ws, "sess-1", "second", []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("3"), 0o644)
	id3, err := st.Capture(context.Background(), ws, "sess-1", "third", []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}

	list, err := st.List(context.Background(), "sess-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(list))
	}
	if list[0].ID != id3 || list[2].ID != id1 {
		t.Errorf("expected newest-first [id3, id2, id1], got [%s, %s, %s]",
			list[0].ID, list[1].ID, list[2].ID)
	}
	_ = id2
}

func TestStore_RestoreUnknownID(t *testing.T) {
	ws := t.TempDir()
	st, err := Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.Restore(context.Background(), ws, "ckpt-nope"); err == nil {
		t.Error("expected error for unknown id")
	}
}
