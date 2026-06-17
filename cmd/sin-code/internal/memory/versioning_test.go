// SPDX-License-Identifier: MIT
// Purpose: tests for issue #358 — memory editing/versioning history.
package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/testutil"
)

func newVersioningStore(t *testing.T) *VersioningStore {
	t.Helper()
	db := testutil.IsolatedSQLite(t)
	vs, err := NewVersioningStore(db)
	if err != nil {
		t.Fatalf("NewVersioningStore: %v", err)
	}
	return vs
}

func TestNewVersioningStoreNilDB(t *testing.T) {
	_, err := NewVersioningStore(nil)
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestSaveVersionAndHistory(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()

	if err := vs.SaveVersion(ctx, "mem-1", "original content", "edited content", "first edit"); err != nil {
		t.Fatal(err)
	}
	if err := vs.SaveVersion(ctx, "mem-1", "edited content", "final content", "second edit"); err != nil {
		t.Fatal(err)
	}

	history, err := vs.History(ctx, "mem-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(history))
	}
	if history[0].Version != 2 {
		t.Errorf("expected newest version 2, got %d", history[0].Version)
	}
	if history[0].Content != "edited content" {
		t.Errorf("expected 'edited content', got %q", history[0].Content)
	}
	if history[1].Version != 1 {
		t.Errorf("expected version 1, got %d", history[1].Version)
	}
	if history[1].Content != "original content" {
		t.Errorf("expected 'original content', got %q", history[1].Content)
	}
}

func TestSaveVersionEmptyMemID(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()
	if err := vs.SaveVersion(ctx, "", "old", "new", "test"); err == nil {
		t.Error("expected error for empty memory id")
	}
}

func TestSaveVersionDefaultReason(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()
	if err := vs.SaveVersion(ctx, "mem-1", "old", "new", ""); err != nil {
		t.Fatal(err)
	}
	history, _ := vs.History(ctx, "mem-1")
	if history[0].EditReason != "edit" {
		t.Errorf("expected default reason 'edit', got %q", history[0].EditReason)
	}
}

func TestHistoryEmpty(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()
	history, err := vs.History(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 versions for nonexistent memory, got %d", len(history))
	}
}

func TestRestore(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()

	_ = vs.SaveVersion(ctx, "mem-1", "v1 content", "v2 content", "first edit")
	_ = vs.SaveVersion(ctx, "mem-1", "v2 content", "v3 content", "second edit")

	if err := vs.Restore(ctx, "mem-1", 1); err != nil {
		t.Fatal(err)
	}

	history, _ := vs.History(ctx, "mem-1")
	if len(history) != 3 {
		t.Fatalf("expected 3 versions after restore, got %d", len(history))
	}
	if history[0].Content != "v1 content" {
		t.Errorf("expected restored content 'v1 content', got %q", history[0].Content)
	}
	if !strings.Contains(history[0].EditReason, "restore from version 1") {
		t.Errorf("expected restore reason, got %q", history[0].EditReason)
	}
}

func TestRestoreNotFound(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()
	err := vs.Restore(ctx, "mem-1", 99)
	if err == nil {
		t.Error("expected error for nonexistent version")
	}
}

func TestDiff(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()

	_ = vs.SaveVersion(ctx, "mem-1", "line1\nline2\nline3", "line1\nmodified\nline3", "edit")
	_ = vs.SaveVersion(ctx, "mem-1", "line1\nmodified\nline3", "line1\nmodified\nline4", "edit2")

	diff, err := vs.Diff(ctx, "mem-1", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "- line2") {
		t.Errorf("expected '- line2' in diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+ modified") {
		t.Errorf("expected '+ modified' in diff, got:\n%s", diff)
	}
}

func TestDiffVersionNotFound(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()
	_ = vs.SaveVersion(ctx, "mem-1", "content", "new", "edit")
	_, err := vs.Diff(ctx, "mem-1", 1, 99)
	if err == nil {
		t.Error("expected error for nonexistent version in diff")
	}
}
