// SPDX-License-Identifier: MIT
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFileViewerLoadText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	os.WriteFile(path, []byte(content), 0o644)

	v := NewFileViewer()
	if err := v.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if v.IsBinary() {
		t.Error("expected non-binary")
	}
	if v.LineCount() < 4 {
		t.Errorf("expected at least 4 lines, got %d", v.LineCount())
	}
}

func TestFileViewerRenderLineNumbers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

	v := NewFileViewer()
	if err := v.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	styles := NewStyles(Themes[0])
	out := v.Render(styles, 50, 20)
	if !strings.Contains(out, "1") {
		t.Error("expected line number 1 in render")
	}
	if !strings.Contains(out, "line1") {
		t.Error("expected line1 content in render")
	}
	if !strings.Contains(out, "│") {
		t.Error("expected line number separator │ in render")
	}
}

func TestFileViewerBinaryDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.dat")
	binaryData := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A, 0x1A, 0x0A}
	os.WriteFile(path, binaryData, 0o644)

	v := NewFileViewer()
	if err := v.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !v.IsBinary() {
		t.Error("expected binary file")
	}
	styles := NewStyles(Themes[0])
	out := v.Render(styles, 50, 10)
	if !strings.Contains(out, "binary") {
		t.Errorf("expected 'binary' in render, got: %s", out)
	}
}

func TestFileViewerLargeFileTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	var b strings.Builder
	for i := 0; i < 600; i++ {
		b.WriteString("line\n")
	}
	os.WriteFile(path, []byte(b.String()), 0o644)

	v := NewFileViewer()
	if err := v.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !v.IsTruncated() {
		t.Error("expected truncated=true")
	}
	if v.LineCount() > maxViewerLines {
		t.Errorf("expected at most %d lines, got %d", maxViewerLines, v.LineCount())
	}
	styles := NewStyles(Themes[0])
	out := v.Render(styles, 50, 30)
	if !strings.Contains(out, "more lines") {
		t.Error("expected truncation indicator in render")
	}
}

func TestFileViewerCurrentPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.go")
	os.WriteFile(path, []byte("package main\n"), 0o644)

	v := NewFileViewer()
	if v.CurrentPath() != "" {
		t.Error("expected empty path initially")
	}
	if err := v.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if v.CurrentPath() != path {
		t.Errorf("expected %s, got %s", path, v.CurrentPath())
	}
}

func TestFileViewerScrollUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scroll.txt")
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line\n")
	}
	os.WriteFile(path, []byte(b.String()), 0o644)

	v := NewFileViewer()
	if err := v.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	v.ScrollDown(10)
	v.ScrollUp(5)
}

func TestFileViewerConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.go")
	os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o644)

	v := NewFileViewer()
	if err := v.Load(path); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	styles := NewStyles(Themes[0])
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = v.Render(styles, 50, 20)
			_ = v.CurrentPath()
			_ = v.IsBinary()
			_ = v.LineCount()
			v.ScrollDown(1)
			v.ScrollUp(1)
		}()
	}
	wg.Wait()
}

func TestFileViewerLoadNonExistent(t *testing.T) {
	v := NewFileViewer()
	err := v.Load("/nonexistent/file/path.go")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	styles := NewStyles(Themes[0])
	out := v.Render(styles, 50, 10)
	if !strings.Contains(out, "Error") {
		t.Error("expected error message in render")
	}
}
