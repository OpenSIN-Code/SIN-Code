// SPDX-License-Identifier: MIT

package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIndexBuildAndSearch(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	os.WriteFile(filepath.Join(root, "b.go"), []byte("package main\n\nfunc world() {}\n"), 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if idx.len() != 2 {
		t.Fatalf("expected 2 files, got %d", idx.len())
	}

	matches := idx.searchTrigram("hello")
	if len(matches) != 1 || !strings.Contains(matches[0], "a.go") {
		t.Fatalf("expected a.go match, got %v", matches)
	}

	matches = idx.searchSymbols("hello", "")
	if len(matches) != 1 || !strings.Contains(matches[0], "a.go") {
		t.Fatalf("expected symbol match for hello, got %v", matches)
	}
}

func TestIndexSaveAndLoad(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n\nfunc foo() {}\n"), 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if err := saveIndex(idx); err != nil {
		t.Fatalf("saveIndex: %v", err)
	}

	idx2, err := loadIndex(root)
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}
	if idx2.len() != 1 {
		t.Fatalf("expected 1 file after load, got %d", idx2.len())
	}
	if _, ok := idx2.fileModTime("x.go"); !ok {
		t.Fatal("expected x.go in loaded index")
	}
}

func TestIndexRefresh(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "old.go"), []byte("package old\n"), 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if err := saveIndex(idx); err != nil {
		t.Fatalf("saveIndex: %v", err)
	}

	// Add new file
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(filepath.Join(root, "new.go"), []byte("package new\n"), 0644)

	idx2, added, removed, err := refreshIndex(idx)
	if err != nil {
		t.Fatalf("refreshIndex: %v", err)
	}
	if added != 1 {
		t.Fatalf("expected 1 added, got %d", added)
	}
	if removed != 0 {
		t.Fatalf("expected 0 removed, got %d", removed)
	}
	if idx2.len() != 2 {
		t.Fatalf("expected 2 files after refresh, got %d", idx2.len())
	}
}

func TestIndexRefreshRemove(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0644)
	os.WriteFile(filepath.Join(root, "b.go"), []byte("package b\n"), 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	os.Remove(filepath.Join(root, "b.go"))
	idx2, added, removed, err := refreshIndex(idx)
	if err != nil {
		t.Fatalf("refreshIndex: %v", err)
	}
	if added != 0 {
		t.Fatalf("expected 0 added, got %d", added)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}
	if idx2.len() != 1 {
		t.Fatalf("expected 1 file after refresh, got %d", idx2.len())
	}
}

func TestIndexBinarySkip(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "img.png"), []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x00}, 0644)
	os.WriteFile(filepath.Join(root, "code.go"), []byte("package main\n"), 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if idx.len() != 1 {
		t.Fatalf("expected 1 file (binary skipped), got %d", idx.len())
	}

	// code.go should have trigrams
	idx.mu.RLock()
	fi := idx.files["code.go"]
	idx.mu.RUnlock()
	if fi == nil {
		t.Fatal("expected code.go in index")
	}
	if len(fi.trigrams) == 0 {
		t.Fatalf("expected trigrams for code.go, got 0")
	}
}

func TestTrigrams(t *testing.T) {
	tris := buildTrigrams("hello world")
	if len(tris) == 0 {
		t.Fatal("expected trigrams")
	}
	q := queryTrigrams("hello")
	if len(q) == 0 {
		t.Fatal("expected query trigrams")
	}
}

func TestSearchWithIndex(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() string { return \"hi\" }\n"), 0644)
	os.WriteFile(filepath.Join(root, "b.go"), []byte("package main\n\nfunc goodbye() {}\n"), 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	results, err := searchWithIndex(idx, root, "hello", "regex", 10, false)
	if err != nil {
		t.Fatalf("searchWithIndex: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for hello")
	}
	found := false
	for _, r := range results {
		if strings.Contains(r.File, "a.go") && strings.Contains(r.Match, "hello") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a.go match, got %+v", results)
	}
}

func TestSearchWithIndexSymbol(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	results, err := searchWithIndex(idx, root, "hello", "symbol", 10, false)
	if err != nil {
		t.Fatalf("searchWithIndex symbol: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected symbol results")
	}
}

func TestHandleIndexBuild(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)

	res, err := handleIndex(context.Background(), map[string]any{
		"action": "build",
		"root":   root,
	})
	if err != nil {
		t.Fatalf("handleIndex build: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if int(m["files"].(float64)) != 1 {
		t.Fatalf("expected 1 file, got %v", m["files"])
	}
}

func TestHandleIndexStatus(t *testing.T) {
	root := t.TempDir()
	res, err := handleIndex(context.Background(), map[string]any{
		"action": "status",
		"root":   root,
	})
	if err != nil {
		t.Fatalf("handleIndex status: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["exists"].(bool) {
		t.Fatal("expected no index")
	}
}

func TestHandleIndexClear(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)

	res, err := handleIndex(context.Background(), map[string]any{
		"action": "clear",
		"root":   root,
	})
	if err != nil {
		t.Fatalf("handleIndex clear: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !m["cleared"].(bool) {
		t.Fatal("expected cleared")
	}
}

func TestHandleIndexSearch(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	res, err := handleIndexSearch(context.Background(), map[string]any{
		"query":       "hello",
		"root":        root,
		"search_type": "regex",
	})
	if err != nil {
		t.Fatalf("handleIndexSearch: %v", err)
	}
	var results []scoutResult
	if err := json.Unmarshal([]byte(res), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	found := false
	for _, r := range results {
		if strings.Contains(r.File, "a.go") && strings.Contains(r.Match, "hello") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a.go match, got %+v", results)
	}
}

// TestInMemoryIndex_HelperMethods verifies the small helper methods
// on inMemoryIndex: rootPath, hasFile, allIndexedPaths, clear, remove.
// (st-cov1)
func TestInMemoryIndex_HelperMethods(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}

	// rootPath
	if got := idx.rootPath(); got != root {
		t.Errorf("rootPath() = %q, want %q", got, root)
	}

	// hasFile
	if !idx.hasFile("x.go") {
		t.Error("expected hasFile('x.go') = true")
	}
	if idx.hasFile("nonexistent.go") {
		t.Error("expected hasFile('nonexistent.go') = false")
	}

	// allIndexedPaths
	paths := idx.allIndexedPaths()
	if len(paths) != 1 || paths[0] != "x.go" {
		t.Errorf("expected allIndexedPaths() = [x.go], got %v", paths)
	}

	// remove
	idx.remove("x.go")
	if idx.hasFile("x.go") {
		t.Error("expected x.go to be removed")
	}

	// clear
	idx.add(indexEntry{File: "y.go", ModTime: time.Now(), Size: 100, Trigrams: nil, IsBinary: false, Lines: 5})
	if !idx.hasFile("y.go") {
		t.Error("expected y.go after add")
	}
	idx.clear()
	if len(idx.allIndexedPaths()) != 0 {
		t.Errorf("expected empty after clear, got %v", idx.allIndexedPaths())
	}
}
