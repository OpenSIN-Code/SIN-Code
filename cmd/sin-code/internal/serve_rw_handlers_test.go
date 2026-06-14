// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the direct MCP read/write/edit handlers.
// (st-cov1)
package internal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleRead_RequiresPath(t *testing.T) {
	if _, err := handleRead(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing path must error")
	}
}

func TestHandleRead_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() {}\n"), 0o644)

	out, err := handleRead(context.Background(), map[string]any{
		"path": p,
		"mode": "raw",
	})
	if err != nil {
		t.Fatalf("handleRead failed: %v", err)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected output to contain 'Hello', got %q", out)
	}
}

func TestHandleWrite_RequiresPathAndContent(t *testing.T) {
	if _, err := handleWrite(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing path/content must error")
	}
	if _, err := handleWrite(context.Background(), map[string]any{"path": "x.go"}); err == nil {
		t.Fatal("missing content must error")
	}
}

func TestHandleWrite_WritesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")

	out, err := handleWrite(context.Background(), map[string]any{
		"path":    p,
		"content": "package main\n",
	})
	if err != nil {
		t.Fatalf("handleWrite failed: %v", err)
	}
	if !strings.Contains(out, "created") {
		t.Errorf("expected output to mention 'created', got %q", out)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "package main\n" {
		t.Errorf("file content = %q, want %q", string(data), "package main\n")
	}
}

func TestHandleEdit_RequiresPath(t *testing.T) {
	if _, err := handleEdit(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing path must error")
	}
}

func TestHandleEdit_AppliesEdit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() {}\n"), 0o644)

	out, err := handleEdit(context.Background(), map[string]any{
		"path":       p,
		"old_string": "func Hello() {}",
		"new_string": "func Hello() string {}",
	})
	if err != nil {
		t.Fatalf("handleEdit failed: %v", err)
	}
	if !strings.Contains(out, "replace") {
		t.Errorf("expected output to mention replace, got %q", out)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "func Hello() string {}") {
		t.Errorf("file content missing new string: %q", string(data))
	}
}

func TestHandleEdit_DryRun(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() {}\n"), 0o644)

	_, err := handleEdit(context.Background(), map[string]any{
		"path":       p,
		"old_string": "func Hello() {}",
		"new_string": "func Hello() string {}",
		"dry_run":    true,
	})
	if err != nil {
		t.Fatalf("handleEdit dry_run failed: %v", err)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "func Hello() {}") {
		t.Errorf("file changed during dry run: %q", string(data))
	}
}

func TestHandleRead_InvalidMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() {}\n"), 0o644)

	if _, err := handleRead(context.Background(), map[string]any{
		"path": p,
		"mode": "unknown",
	}); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestHandleRead_Directory(t *testing.T) {
	dir := t.TempDir()
	if _, err := handleRead(context.Background(), map[string]any{
		"path": dir,
	}); err == nil {
		t.Fatal("expected error for directory")
	}
}

func TestHandleEdit_EditFails(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() {}\n"), 0o644)

	// Ambiguous old_string should fail the string edit.
	_, err := handleEdit(context.Background(), map[string]any{
		"path":       p,
		"old_string": "func",
		"new_string": "FUNC",
	})
	if err == nil {
		t.Fatal("expected error for ambiguous edit")
	}
}

func TestHandleWrite_ParentDirMissing(t *testing.T) {
	_, err := handleWrite(context.Background(), map[string]any{
		"path":    "/nonexistent/dir/file.go",
		"content": "test",
	})
	if err == nil {
		t.Fatal("expected error when parent directory is missing")
	}
}
