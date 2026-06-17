// SPDX-License-Identifier: MIT
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFileBrowserLoad(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.py"), []byte("print(1)"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("hello"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !b.Loaded() {
		t.Error("expected loaded=true")
	}
	if b.FlatCount() < 3 {
		t.Errorf("expected at least 3 flat entries, got %d", b.FlatCount())
	}
}

func TestFileBrowserRender(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	os.Mkdir(filepath.Join(dir, "pkg"), 0o755)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	styles := NewStyles(Themes[0])
	out := b.Render(styles, 40, 20)
	if out == "" {
		t.Error("expected non-empty render")
	}
	if !strings.Contains(out, "📁") && !strings.Contains(out, "📂") {
		t.Error("expected directory icon in render")
	}
	if !strings.Contains(out, "🐹") {
		t.Error("expected Go file icon in render")
	}
}

func TestFileBrowserMoveUp(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	b.MoveDown()
	b.MoveDown()
	cur := b.Cursor()
	if cur != 2 {
		t.Fatalf("expected cursor at 2, got %d", cur)
	}
	b.MoveUp()
	if b.Cursor() != 1 {
		t.Errorf("expected cursor at 1, got %d", b.Cursor())
	}
}

func TestFileBrowserMoveDown(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	b.MoveDown()
	b.MoveDown()
	if b.Cursor() > b.FlatCount()-1 {
		t.Errorf("cursor %d exceeded flat count %d", b.Cursor(), b.FlatCount())
	}
}

func TestFileBrowserMoveUpClamped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	b.MoveUp()
	if b.Cursor() != 0 {
		t.Errorf("expected cursor clamped at 0, got %d", b.Cursor())
	}
}

func TestFileBrowserToggleExpand(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "inner.go"), []byte("package sub"), 0o644)
	os.WriteFile(filepath.Join(dir, "top.go"), []byte("package main"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	initialCount := b.FlatCount()

	b.MoveDown()
	for i := 0; i < b.FlatCount(); i++ {
		if b.SelectedIsDir() && b.SelectedPath() != dir {
			break
		}
		b.MoveDown()
	}
	if !b.SelectedIsDir() {
		t.Fatal("expected to find a subdirectory")
	}
	if b.SelectedPath() == dir {
		t.Fatal("expected subdirectory, not root")
	}

	b.ToggleExpand()
	expandedCount := b.FlatCount()
	if expandedCount <= initialCount {
		t.Errorf("expected more entries after expand, initial=%d expanded=%d", initialCount, expandedCount)
	}

	b.ToggleExpand()
	collapsedCount := b.FlatCount()
	if collapsedCount != initialCount {
		t.Errorf("expected collapse to restore count, initial=%d collapsed=%d", initialCount, collapsedCount)
	}
}

func TestFileBrowserSelectedPath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.go"), []byte("package main"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	for i := 0; i < b.FlatCount(); i++ {
		p := b.SelectedPath()
		if strings.HasSuffix(p, "file.go") {
			return
		}
		b.MoveDown()
	}
	t.Errorf("selected path never matched file.go, last=%s", b.SelectedPath())
}

func TestFileBrowserHiddenFilesDimmed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("secret"), 0o644)
	os.WriteFile(filepath.Join(dir, "visible.go"), []byte("package main"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	styles := NewStyles(Themes[0])
	out := b.Render(styles, 40, 20)
	if !strings.Contains(out, ".hidden") {
		t.Error("expected hidden file in render")
	}
}

func TestFileBrowserEmptyDir(t *testing.T) {
	dir := t.TempDir()
	emptyDir := filepath.Join(dir, "empty")
	os.MkdirAll(emptyDir, 0o755)
	b := NewFileBrowser(emptyDir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	styles := NewStyles(Themes[0])
	out := b.Render(styles, 40, 10)
	if !strings.Contains(out, "empty") {
		t.Errorf("expected empty indicator, got: %s", out)
	}
}

func TestFileBrowserDeepNesting(t *testing.T) {
	dir := t.TempDir()
	deep := dir
	for i := 0; i < 5; i++ {
		deep = filepath.Join(deep, "level"+string(rune('0'+i)))
		os.MkdirAll(deep, 0o755)
	}
	os.WriteFile(filepath.Join(deep, "deep.go"), []byte("package deep"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if b.FlatCount() < 2 {
		t.Errorf("expected at least 2 entries for nested dirs, got %d", b.FlatCount())
	}

	for i := 0; i < b.FlatCount(); i++ {
		if b.SelectedIsDir() {
			b.ToggleExpand()
		}
		b.MoveDown()
	}
}

func TestFileBrowserSetRoot(t *testing.T) {
	dir1 := t.TempDir()
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("a"), 0o644)
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "b.go"), []byte("package b"), 0o644)

	b := NewFileBrowser(dir1)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if b.Root() != dir1 {
		t.Errorf("expected root %s, got %s", dir1, b.Root())
	}

	b.SetRoot(dir2)
	if b.Root() != dir2 {
		t.Errorf("expected root %s, got %s", dir2, b.Root())
	}
	if b.FlatCount() < 1 {
		t.Error("expected entries after SetRoot")
	}
}

func TestFileBrowserNonExistentRoot(t *testing.T) {
	b := NewFileBrowser("/nonexistent/path/that/does/not/exist")
	err := b.Load()
	if err == nil {
		t.Error("expected error for non-existent root")
	}
	styles := NewStyles(Themes[0])
	out := b.Render(styles, 40, 10)
	if out == "" {
		t.Error("expected non-empty render even on error")
	}
}

func TestFileBrowserConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.py"), []byte("print(1)"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("c"), 0o644)

	b := NewFileBrowser(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	styles := NewStyles(Themes[0])
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.MoveUp()
			b.MoveDown()
			b.ToggleExpand()
			_ = b.Render(styles, 40, 20)
			_ = b.SelectedPath()
			_ = b.SelectedIsDir()
			_ = b.Cursor()
			_ = b.FlatCount()
		}()
	}
	wg.Wait()
}
