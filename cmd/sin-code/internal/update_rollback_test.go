// SPDX-License-Identifier: MIT
// Purpose: Unit tests for rollback logic.
package internal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunRollback_NoSnapshot(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}
	// No snapshots created
	opts := UpdateOptions{StateRoot: td}
	// Override NewBackupManager by using our own
	results, err := runRollback(context.Background(), opts)
	if err != nil {
		t.Fatalf("runRollback should not error on missing snapshot: %v", err)
	}
	if results != nil {
		t.Error("results should be nil when no snapshot exists")
	}
	_ = bm
}

func TestRunRollback_WithSnapshot(t *testing.T) {
	td := t.TempDir()
	// Create a snapshot with manifest
	bm := &BackupManager{StateRoot: td}
	bm.Now = func() string { return "snap1" }
	dir, err := bm.Create()
	if err != nil {
		t.Fatal(err)
	}
	m := NewManifest("v1.0.0")
	m.Pre = UpdateSnapshot{
		GoBins: map[string]string{"discover": "v1.0.0-pre"},
	}
	if err := m.Write(dir); err != nil {
		t.Fatal(err)
	}
	// Create a fake backup file
	backupFile := filepath.Join(dir, "discover")
	if err := os.WriteFile(backupFile, []byte("fake-binary-pre"), 0755); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	t.Setenv("SIN_CODE_BIN_DIR", binDir)
	opts := UpdateOptions{StateRoot: td}
	results, err := runRollback(context.Background(), opts)
	if err != nil {
		t.Fatalf("runRollback failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Failed > 0 {
		t.Errorf("rollback had %d failures", results[0].Failed)
	}
	if results[0].Updated != 1 {
		t.Errorf("expected 1 update, got %d", results[0].Updated)
	}
	// Verify file was restored
	data, err := os.ReadFile(filepath.Join(binDir, "discover"))
	if err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
	if string(data) != "fake-binary-pre" {
		t.Errorf("restored content = %q", string(data))
	}
}

func TestCopyFile(t *testing.T) {
	td := t.TempDir()
	src := filepath.Join(td, "src")
	dst := filepath.Join(td, "dst")
	if err := os.WriteFile(src, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("copied data = %q", string(data))
	}
}

func TestRunRollback_MissingBackupFile(t *testing.T) {
	td := t.TempDir()
	bm := &BackupManager{StateRoot: td}
	bm.Now = func() string { return "snap1" }
	dir, err := bm.Create()
	if err != nil {
		t.Fatal(err)
	}
	m := NewManifest("v1.0.0")
	m.Pre = UpdateSnapshot{
		GoBins: map[string]string{"scout": "v2.0.0-pre"},
	}
	if err := m.Write(dir); err != nil {
		t.Fatal(err)
	}
	// No actual backup file created for "scout"

	binDir := t.TempDir()
	t.Setenv("SIN_CODE_BIN_DIR", binDir)
	opts := UpdateOptions{StateRoot: td}
	results, err := runRollback(context.Background(), opts)
	if err != nil {
		t.Fatalf("runRollback failed: %v", err)
	}
	if results[0].Failed != 1 {
		t.Errorf("expected 1 failure for missing backup, got %d", results[0].Failed)
	}
}
