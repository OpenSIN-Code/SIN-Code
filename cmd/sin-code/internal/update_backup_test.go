// SPDX-License-Identifier: MIT
// Purpose: Unit tests for BackupManager lifecycle.
package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupManager_Create(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}
	bm.Now = func() string { return "20260613T120000Z" }

	snapshotDir, err := bm.Create()
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if info, err := os.Stat(snapshotDir); err != nil {
		t.Fatalf("snapshot dir missing: %v", err)
	} else if !info.IsDir() {
		t.Error("snapshot path is not a directory")
	}
	// Verify it was created under StateRoot/updates/<ts>
	if dir := filepath.Dir(snapshotDir); filepath.Base(dir) != "updates" {
		t.Errorf("snapshot parent directory = %s, want 'updates'", filepath.Base(dir))
	}
	if filepath.Base(snapshotDir) != "20260613T120000Z" {
		t.Errorf("snapshot name = %s, want 20260613T120000Z", filepath.Base(snapshotDir))
	}
}

func TestBackupManager_List(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}

	for _, ts := range []string{"1", "2", "3"} {
		bm.Now = func() string { return ts }
		if _, err := bm.Create(); err != nil {
			t.Fatalf("Create %s failed: %v", ts, err)
		}
	}

	dirs, err := bm.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(dirs) != 3 {
		t.Fatalf("List count = %d, want 3", len(dirs))
	}
	// Newest first (reverse sort): "3" should be first
	if filepath.Base(dirs[0]) != "3" {
		t.Errorf("first dir = %s, want '3'", filepath.Base(dirs[0]))
	}
	if filepath.Base(dirs[2]) != "1" {
		t.Errorf("last dir = %s, want '1'", filepath.Base(dirs[2]))
	}
}

func TestBackupManager_Latest(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}
	bm.Now = func() string { return "1" }
	bm.Create()
	bm.Now = func() string { return "2" }
	bm.Create()

	latestDir, err := bm.Latest()
	if err != nil {
		t.Fatalf("Latest failed: %v", err)
	}
	if filepath.Base(latestDir) != "2" {
		t.Errorf("latest = %s, want '2'", filepath.Base(latestDir))
	}
}

func TestBackupManager_LatestEmpty(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}
	latestDir, err := bm.Latest()
	if err != nil {
		t.Fatalf("Latest failed: %v", err)
	}
	if latestDir != "" {
		t.Errorf("latest should be empty, got %s", latestDir)
	}
}

func TestBackupManager_Prune(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}

	stamps := []string{"01","02","03","04","05","06","07","08","09","10","11","12","13","14","15"}
	for _, ts := range stamps {
		stamp := ts
		bm.Now = func() string { return stamp }
		if _, err := bm.Create(); err != nil {
			t.Fatalf("Create %s failed: %v", stamp, err)
		}
	}

	if err := bm.Prune(10); err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	dirs, err := bm.List()
	if err != nil {
		t.Fatalf("List after prune failed: %v", err)
	}
	if len(dirs) != 10 {
		t.Errorf("after prune count = %d, want 10", len(dirs))
	}
}

func TestBackupManager_PruneLessThanKeep(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}
	bm.Now = func() string { return "s1" }
	bm.Create()
	if err := bm.Prune(10); err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	dirs, err := bm.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(dirs) != 1 {
		t.Errorf("should keep 1, got %d", len(dirs))
	}
}

func TestBackupManager_EmptyList(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}
	dirs, err := bm.List()
	if err != nil {
		t.Fatalf("List on empty dir failed: %v", err)
	}
	if dirs != nil {
		t.Errorf("expected nil, got %v", dirs)
	}
}

func TestSnapshotDir(t *testing.T) {
	dir := SnapshotDir("/root", "ts")
	expected := filepath.Join("/root", "updates", "ts")
	if dir != expected {
		t.Errorf("SnapshotDir = %q, want %q", dir, expected)
	}
}
