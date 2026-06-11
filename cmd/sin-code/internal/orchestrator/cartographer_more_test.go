// SPDX-License-Identifier: MIT
// Purpose: lightweight tests for the Cartographer (impact-aware symbol index).

package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCartographer_NoRepoRoot(t *testing.T) {
	c := NewCartographer("")
	if c == nil {
		t.Fatal("nil cartographer")
	}
	// No repo root -> Invalidate is a safe no-op
	c.Invalidate([]string{"foo.go"})
	if c.SymbolCount() != 0 {
		t.Fatal("count must be 0 with no repo")
	}
}

func TestNewCartographer_BuildsIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"),
		[]byte("package x\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.SymbolCount() < 1 {
		t.Fatalf("expected at least one symbol, got %d", c.SymbolCount())
	}
}

func TestCartographer_InvalidateDropsAndReindexes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"),
		[]byte("package x\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := c.SymbolCount()
	if before < 1 {
		t.Fatal("setup failed")
	}
	c.Invalidate([]string{"x.go"})
	if c.SymbolCount() < 1 {
		t.Fatal("re-index should restore symbols")
	}
}

func TestCartographer_SliceForEmptyImpact(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"),
		[]byte("package x\nfunc A() {}\nfunc B() {}\nfunc C() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	items := c.SliceFor(nil, 2)
	if len(items) == 0 {
		t.Fatal("expected non-empty slice")
	}
	if len(items) > 2 {
		t.Fatalf("slice should be capped at k=2, got %d", len(items))
	}
	for _, it := range items {
		if it.Relevance < 0 || it.Relevance > 1 {
			t.Fatalf("relevance out of [0,1]: %f", it.Relevance)
		}
	}
}

func TestCartographer_IndexAllEmptyDir(t *testing.T) {
	dir := t.TempDir()
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatalf("IndexAll on empty dir: %v", err)
	}
	if c.SymbolCount() != 0 {
		t.Fatal("empty dir must produce no symbols")
	}
}

func TestCartographer_InvalidateOnNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	c := NewCartographer(dir)
	// Invalidate on a file that was never indexed must be safe
	c.Invalidate([]string{"ghost.go"})
	if c.SymbolCount() != 0 {
		t.Fatal("no symbols must exist")
	}
}
