// SPDX-License-Identifier: MIT
// Purpose: tests for TODO/FIXME auto-discovery (issue #330).
package todo

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanFileTODO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	writeTestFile(t, path, "package x\n\n// TODO: fix this\nfunc a() {}\n")
	d := NewAutoDiscover()
	markers, err := d.ScanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(markers))
	}
	m := markers[0]
	if m.Type != "TODO" {
		t.Errorf("Type = %q, want TODO", m.Type)
	}
	if m.Text != "fix this" {
		t.Errorf("Text = %q, want 'fix this'", m.Text)
	}
	if m.Priority != PriorityP2 {
		t.Errorf("Priority = %q, want P2", m.Priority)
	}
	if m.Line != 3 {
		t.Errorf("Line = %d, want 3", m.Line)
	}
}

func TestScanFileFIXME(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.go")
	writeTestFile(t, path, "// FIXME: urgent bug\n")
	d := NewAutoDiscover()
	markers, _ := d.ScanFile(path)
	if len(markers) != 1 || markers[0].Type != "FIXME" || markers[0].Text != "urgent bug" {
		t.Fatalf("unexpected: %+v", markers)
	}
	if markers[0].Priority != PriorityP1 {
		t.Errorf("Priority = %q, want P1", markers[0].Priority)
	}
}

func TestScanFileHACK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.go")
	writeTestFile(t, path, "// HACK: temporary workaround\n")
	d := NewAutoDiscover()
	markers, _ := d.ScanFile(path)
	if len(markers) != 1 || markers[0].Type != "HACK" || markers[0].Priority != PriorityP3 {
		t.Fatalf("unexpected: %+v", markers)
	}
}

func TestScanFilePythonStyle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.py")
	writeTestFile(t, path, "# TODO: implement feature\nx = 1\n")
	d := NewAutoDiscover()
	markers, _ := d.ScanFile(path)
	if len(markers) != 1 || markers[0].Type != "TODO" || markers[0].Text != "implement feature" {
		t.Fatalf("unexpected: %+v", markers)
	}
}

func TestScanFileNoMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.go")
	writeTestFile(t, path, "package x\n\nfunc main() {}\n")
	d := NewAutoDiscover()
	markers, _ := d.ScanFile(path)
	if len(markers) != 0 {
		t.Fatalf("expected 0 markers, got %d", len(markers))
	}
}

func TestScanFileBlockComment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.js")
	writeTestFile(t, path, "/* TODO: fix block */\n")
	d := NewAutoDiscover()
	markers, _ := d.ScanFile(path)
	if len(markers) != 1 || markers[0].Type != "TODO" {
		t.Fatalf("unexpected: %+v", markers)
	}
}

func TestScanDirMultipleFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.go"), "// TODO: one\n")
	writeTestFile(t, filepath.Join(root, "b.py"), "# FIXME: two\n")
	writeTestFile(t, filepath.Join(root, "sub", "c.js"), "// HACK: three\n")
	d := NewAutoDiscover()
	markers, err := d.ScanDir(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 3 {
		t.Fatalf("expected 3 markers, got %d", len(markers))
	}
}

func TestScanDirMaxDepth(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.go"), "// TODO: one\n")
	writeTestFile(t, filepath.Join(root, "sub", "b.go"), "// TODO: two\n")
	writeTestFile(t, filepath.Join(root, "sub", "deep", "c.go"), "// TODO: three\n")
	d := NewAutoDiscover()
	markers, _ := d.ScanDir(root, 1)
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker at depth 1, got %d", len(markers))
	}
}

func TestScanDirSkipsIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a.go"), "// TODO: one\n")
	writeTestFile(t, filepath.Join(root, "node_modules", "b.go"), "// TODO: two\n")
	writeTestFile(t, filepath.Join(root, ".git", "c.go"), "// TODO: three\n")
	d := NewAutoDiscover()
	markers, _ := d.ScanDir(root, 0)
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker (ignored dirs skipped), got %d: %+v", len(markers), markers)
	}
}

func TestMarkersToTodos(t *testing.T) {
	d := NewAutoDiscover()
	markers := []TodoMarker{
		{File: "a.go", Line: 10, Type: "TODO", Text: "fix this", Priority: PriorityP2},
		{File: "b.go", Line: 20, Type: "FIXME", Text: "urgent bug", Priority: PriorityP1},
		{File: "c.go", Line: 30, Type: "HACK", Text: "", Priority: PriorityP3},
	}
	todos := d.MarkersToTodos(markers)
	if len(todos) != 3 {
		t.Fatalf("expected 3 todos, got %d", len(todos))
	}
	if todos[0].Title != "fix this" || todos[0].Type != TypeTask {
		t.Errorf("todo[0]: Title=%q Type=%q", todos[0].Title, todos[0].Type)
	}
	if todos[0].ExternalRef != "file:a.go:10" {
		t.Errorf("ExternalRef = %q", todos[0].ExternalRef)
	}
	if todos[1].Type != TypeBug {
		t.Errorf("todo[1] Type = %q, want bug", todos[1].Type)
	}
	if todos[2].Title != "HACK" {
		t.Errorf("todo[2] Title = %q, want 'HACK'", todos[2].Title)
	}
}

func TestAutoDiscoverConcurrent(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("file%d.go", i)), "// TODO: task\n")
	}
	d := NewAutoDiscover()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.ScanDir(root, 0)
		}()
	}
	wg.Wait()
}
