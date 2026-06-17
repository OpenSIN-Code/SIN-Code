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

func withReferencesHook(t *testing.T, fn func(file string, line, col int) ([]ReferenceResult, error)) func() {
	orig := lspFindReferencesHook
	lspFindReferencesHook = fn
	return func() { lspFindReferencesHook = orig }
}

func sampleRefs() []ReferenceResult {
	return []ReferenceResult{
		{File: "b.go", Line: 30, Col: 5, Preview: "  foo()"},
		{File: "a.go", Line: 12, Col: 2, Preview: "\tfoo()"},
		{File: "a.go", Line: 5, Col: 1, Preview: "func foo() {}"},
	}
}

func TestFindReferencesNewIsEmpty(t *testing.T) {
	r := NewFindReferences()
	if r.Count() != 0 {
		t.Errorf("expected 0, got %d", r.Count())
	}
	if s := r.Selected(); s != nil {
		t.Error("expected nil selected on new")
	}
	if r.IsPending() {
		t.Error("expected not pending")
	}
}

func TestFindReferencesRequestInvalidArgs(t *testing.T) {
	r := NewFindReferences()
	if err := r.Request("", 10, 5); err == nil {
		t.Error("expected error for empty file")
	}
	if err := r.Request("a.go", 0, 5); err == nil {
		t.Error("expected error for zero line")
	}
	if err := r.Request("a.go", 5, -1); err == nil {
		t.Error("expected error for negative col")
	}
}

func TestFindReferencesRequestStoresSorted(t *testing.T) {
	restore := withReferencesHook(t, func(file string, line, col int) ([]ReferenceResult, error) {
		return sampleRefs(), nil
	})
	defer restore()

	r := NewFindReferences()
	if err := r.Request("a.go", 5, 1); err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	refs := r.Results()
	if len(refs) != 3 {
		t.Fatalf("expected 3, got %d", len(refs))
	}
	if refs[0].File != "a.go" || refs[0].Line != 5 {
		t.Errorf("expected a.go:5 first, got %s:%d", refs[0].File, refs[0].Line)
	}
	if refs[1].File != "a.go" || refs[1].Line != 12 {
		t.Errorf("expected a.go:12 second, got %s:%d", refs[1].File, refs[1].Line)
	}
	if refs[2].File != "b.go" {
		t.Errorf("expected b.go last, got %s", refs[2].File)
	}
}

func TestFindReferencesRequestPropagatesError(t *testing.T) {
	restore := withReferencesHook(t, func(file string, line, col int) ([]ReferenceResult, error) {
		return nil, fmt.Errorf("server unreachable")
	})
	defer restore()

	r := NewFindReferences()
	if err := r.Request("a.go", 5, 1); err == nil {
		t.Error("expected error from hook")
	}
	if r.Count() != 0 {
		t.Errorf("expected 0 results on error, got %d", r.Count())
	}
}

func TestFindReferencesRenderStates(t *testing.T) {
	styles := NewStyles(Themes[0])
	r := NewFindReferences()

	restore := withReferencesHook(t, func(file string, line, col int) ([]ReferenceResult, error) {
		return sampleRefs(), nil
	})
	defer restore()

	if err := r.Request("a.go", 5, 1); err != nil {
		t.Fatal(err)
	}
	out := r.Render(styles, 100, 20)
	if !strings.Contains(out, "3 references") {
		t.Errorf("expected count in render, got: %s", out)
	}
	if !strings.Contains(out, "a.go") {
		t.Error("expected file in render")
	}
	if !strings.Contains(out, "foo()") {
		t.Error("expected preview in render")
	}

	r.Reset()
	out = r.Render(styles, 100, 20)
	if !strings.Contains(out, "no references") {
		t.Errorf("expected empty render after reset, got: %s", out)
	}
}

func TestFindReferencesNavigation(t *testing.T) {
	restore := withReferencesHook(t, func(file string, line, col int) ([]ReferenceResult, error) {
		return sampleRefs(), nil
	})
	defer restore()

	r := NewFindReferences()
	if err := r.Request("a.go", 5, 1); err != nil {
		t.Fatal(err)
	}
	if s := r.Selected(); s == nil || s.Line != 5 {
		t.Errorf("expected a.go:5 selected, got %+v", s)
	}
	r.MoveDown()
	if s := r.Selected(); s == nil || s.Line != 12 {
		t.Errorf("expected a.go:12 after MoveDown, got %+v", s)
	}
	r.MoveDown()
	if s := r.Selected(); s == nil || s.File != "b.go" {
		t.Errorf("expected b.go after MoveDown, got %+v", s)
	}
	r.MoveDown()
	if s := r.Selected(); s == nil || s.File != "b.go" {
		t.Error("MoveDown should clamp at last")
	}
	r.MoveUp()
	r.MoveUp()
	if s := r.Selected(); s == nil || s.Line != 5 {
		t.Errorf("expected back at a.go:5, got %+v", s)
	}
	r.MoveUp()
	if s := r.Selected(); s == nil || s.Line != 5 {
		t.Error("MoveUp should clamp at zero")
	}
}

func TestFindReferencesJumpToSelected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ref.go")
	if err := os.WriteFile(target, []byte("package main\nfunc foo() {}\nfoo()\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := withReferencesHook(t, func(file string, line, col int) ([]ReferenceResult, error) {
		return []ReferenceResult{{File: target, Line: 3, Col: 1, Preview: "foo()"}}, nil
	})
	defer restore()

	r := NewFindReferences()
	v := NewFileViewer()
	r.SetViewer(v)

	if err := r.JumpToSelected(); err == nil {
		t.Error("expected error when no results yet")
	}

	if err := r.Request("a.go", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := r.JumpToSelected(); err != nil {
		t.Fatalf("JumpToSelected failed: %v", err)
	}
	if v.CurrentPath() != target {
		t.Errorf("expected viewer at %s, got %s", target, v.CurrentPath())
	}
	if v.Cursor() != 3 {
		t.Errorf("expected cursor at 3, got %d", v.Cursor())
	}
}

func TestFindReferencesResultsReturnsCopy(t *testing.T) {
	restore := withReferencesHook(t, func(file string, line, col int) ([]ReferenceResult, error) {
		return sampleRefs(), nil
	})
	defer restore()

	r := NewFindReferences()
	if err := r.Request("a.go", 5, 1); err != nil {
		t.Fatal(err)
	}
	refs := r.Results()
	refs[0].Preview = "mutated"
	again := r.Results()
	if again[0].Preview == "mutated" {
		t.Error("Results() should return a copy")
	}
}

func TestFindReferencesConcurrentAccess(t *testing.T) {
	restore := withReferencesHook(t, func(file string, line, col int) ([]ReferenceResult, error) {
		return sampleRefs(), nil
	})
	defer restore()

	r := NewFindReferences()
	styles := NewStyles(Themes[0])
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = r.Request("a.go", n%5+1, 1)
			_ = r.Render(styles, 80, 20)
			_ = r.Results()
			_ = r.Selected()
			_ = r.Count()
			r.MoveUp()
			r.MoveDown()
			r.Reset()
		}(i)
	}
	wg.Wait()
}
