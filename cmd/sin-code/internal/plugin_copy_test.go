// SPDX-License-Identifier: MIT
// Purpose: tests for the plugin installation utilities.

package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDir_BasicTree(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	// build a small tree
	if err := os.WriteFile(filepath.Join(src, "a.txt"),
		[]byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"),
		[]byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "a.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("a.txt not copied: %v %q", err, data)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt")); err != nil || string(data) != "world" {
		t.Fatalf("sub/b.txt not copied: %v %q", err, data)
	}
}

func TestCopyDir_EmptySrc(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	// dst should remain empty
	entries, _ := os.ReadDir(dst)
	if len(entries) != 0 {
		t.Fatalf("expected empty dst, got %d entries", len(entries))
	}
}
