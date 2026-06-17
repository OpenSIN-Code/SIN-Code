// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func withGotoHook(t *testing.T, fn func(file string, line, col int) (DefinitionResult, error)) func() {
	orig := lspGotoDefinitionHook
	lspGotoDefinitionHook = fn
	return func() { lspGotoDefinitionHook = orig }
}

func TestGotoDefinitionNewHasNoResult(t *testing.T) {
	g := NewGotoDefinition()
	if g.IsPending() {
		t.Error("expected not pending on new")
	}
	if _, err := g.Result(); err == nil {
		t.Error("expected error when no result available")
	}
}

func TestGotoDefinitionRequestInvalidArgs(t *testing.T) {
	g := NewGotoDefinition()
	if err := g.Request("", 10, 5); err == nil {
		t.Error("expected error for empty file")
	}
	if err := g.Request("a.go", 0, 5); err == nil {
		t.Error("expected error for zero line")
	}
	if err := g.Request("a.go", 5, 0); err == nil {
		t.Error("expected error for zero col")
	}
}

func TestGotoDefinitionRequestStoresResult(t *testing.T) {
	restore := withGotoHook(t, func(file string, line, col int) (DefinitionResult, error) {
		return DefinitionResult{File: "target.go", Line: 99, Col: 5}, nil
	})
	defer restore()

	g := NewGotoDefinition()
	if err := g.Request("src.go", 10, 3); err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	res, err := g.Result()
	if err != nil {
		t.Fatalf("Result failed: %v", err)
	}
	if res.File != "target.go" || res.Line != 99 || res.Col != 5 {
		t.Errorf("unexpected result %+v", res)
	}
}

func TestGotoDefinitionRequestPropagatesError(t *testing.T) {
	restore := withGotoHook(t, func(file string, line, col int) (DefinitionResult, error) {
		return DefinitionResult{}, fmt.Errorf("no definition found")
	})
	defer restore()

	g := NewGotoDefinition()
	if err := g.Request("src.go", 10, 3); err == nil {
		t.Error("expected error from hook")
	}
	if _, err := g.Result(); err == nil {
		t.Error("expected Result to return the stored error")
	}
}

func TestGotoDefinitionRenderStates(t *testing.T) {
	styles := NewStyles(Themes[0])
	g := NewGotoDefinition()

	out := g.Render(styles, 60)
	if !strings.Contains(out, "no definition requested") {
		t.Errorf("expected empty-state render, got: %s", out)
	}

	restore := withGotoHook(t, func(file string, line, col int) (DefinitionResult, error) {
		return DefinitionResult{}, fmt.Errorf("boom")
	})
	defer restore()
	_ = g.Request("a.go", 1, 1)
	out = g.Render(styles, 60)
	if !strings.Contains(out, "boom") {
		t.Errorf("expected error in render, got: %s", out)
	}

	g.Reset()
	restore2 := withGotoHook(t, func(file string, line, col int) (DefinitionResult, error) {
		return DefinitionResult{File: "def.go", Line: 42, Col: 1}, nil
	})
	defer restore2()
	_ = g.Request("a.go", 1, 1)
	out = g.Render(styles, 60)
	if !strings.Contains(out, "def.go:42:1") {
		t.Errorf("expected def.go:42:1 in render, got: %s", out)
	}
}

func TestGotoDefinitionJumpToResult(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.go")
	if err := os.WriteFile(target, []byte("package main\nfunc helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := withGotoHook(t, func(file string, line, col int) (DefinitionResult, error) {
		return DefinitionResult{File: target, Line: 2, Col: 6}, nil
	})
	defer restore()

	g := NewGotoDefinition()
	v := NewFileViewer()
	g.SetViewer(v)

	if err := g.JumpToResult(); err == nil {
		t.Error("expected error when no result yet")
	}

	if err := g.Request("src.go", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := g.JumpToResult(); err != nil {
		t.Fatalf("JumpToResult failed: %v", err)
	}
	if v.CurrentPath() != target {
		t.Errorf("expected viewer at %s, got %s", target, v.CurrentPath())
	}
	if v.Cursor() != 2 {
		t.Errorf("expected cursor at line 2, got %d", v.Cursor())
	}
}

func TestGotoDefinitionJumpWithoutViewer(t *testing.T) {
	restore := withGotoHook(t, func(file string, line, col int) (DefinitionResult, error) {
		return DefinitionResult{File: "x.go", Line: 1, Col: 1}, nil
	})
	defer restore()

	g := NewGotoDefinition()
	if err := g.Request("a.go", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := g.JumpToResult(); err == nil {
		t.Error("expected error when no viewer attached")
	}
}

func TestGotoDefinitionReset(t *testing.T) {
	restore := withGotoHook(t, func(file string, line, col int) (DefinitionResult, error) {
		return DefinitionResult{File: "x.go", Line: 1, Col: 1}, nil
	})
	defer restore()

	g := NewGotoDefinition()
	if err := g.Request("a.go", 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Result(); err != nil {
		t.Error("expected result before reset")
	}
	g.Reset()
	if _, err := g.Result(); err == nil {
		t.Error("expected no result after reset")
	}
}

func TestGotoDefinitionConcurrentAccess(t *testing.T) {
	restore := withGotoHook(t, func(file string, line, col int) (DefinitionResult, error) {
		return DefinitionResult{File: "x.go", Line: col, Col: line}, nil
	})
	defer restore()

	g := NewGotoDefinition()
	styles := NewStyles(Themes[0])
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = g.Request("a.go", n%10+1, n%5+1)
			_, _ = g.Result()
			_ = g.Render(styles, 60)
			_ = g.IsPending()
			g.Reset()
		}(i)
	}
	wg.Wait()
}
