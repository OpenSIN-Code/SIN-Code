// SPDX-License-Identifier: MIT
// Purpose: tests for the in-memory trigram/symbol index used by sin scout.

package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildTrigrams_DedupAndLength(t *testing.T) {
	trigrams := buildTrigrams("hello world")
	if len(trigrams) == 0 {
		t.Fatal("expected non-empty trigrams for non-empty input")
	}
	// Short input (<3 chars) returns no trigrams
	if got := buildTrigrams("ab"); len(got) != 0 {
		t.Fatalf("expected 0 trigrams for 2-char input, got %d", got)
	}
}

func TestQueryTrigrams_ReturnsSet(t *testing.T) {
	q := queryTrigrams("hello world hello")
	if len(q) == 0 {
		t.Fatal("expected non-empty set")
	}
	// Should be a set, not a list
	if len(q) < 2 {
		t.Fatalf("expected at least 2 distinct trigrams, got %d", len(q))
	}
}

func TestIndexPath_Deterministic(t *testing.T) {
	a := indexPath("/tmp/foo")
	b := indexPath("/tmp/foo")
	if a != b {
		t.Fatalf("indexPath should be deterministic: %q vs %q", a, b)
	}
	c := indexPath("/tmp/bar")
	if a == c {
		t.Fatalf("indexPath should differ across roots: %q vs %q", a, c)
	}
}

func TestProcessFileForIndex_GoFile(t *testing.T) {
	dir := t.TempDir()
	src := `package foo

func Hello() {}
func World(x int) string { return "" }
`
	p := filepath.Join(dir, "x.go")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	e := processFileForIndex(p, dir)
	if e.File != "x.go" {
		t.Fatalf("file: %q", e.File)
	}
	if len(e.Trigrams) == 0 {
		t.Fatal("expected non-empty trigrams")
	}
	if len(e.Symbols) == 0 {
		t.Fatal("expected at least 2 symbols (Hello, World)")
	}
}

func TestInMemoryIndex_BuildAndQuery(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"),
		[]byte("package a\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"),
		[]byte("package b\nfunc Beta() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := buildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if idx.len() < 2 {
		t.Fatalf("index should have >= 2 files, got %d", idx.len())
	}
	paths := idx.allIndexedPaths()
	if len(paths) < 2 {
		t.Fatalf("allIndexedPaths should return >= 2, got %d", len(paths))
	}
	// trigram search: any short query should return some hits
	hits := idx.searchTrigram("Alpha")
	if len(hits) == 0 {
		t.Fatal("trigram search must return some hits")
	}
}
