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

func TestHandleIndexRefresh_NoExisting(t *testing.T) {
	root := t.TempDir()
	if _, err := handleIndex(context.Background(), map[string]any{
		"action": "refresh",
		"root":   root,
	}); err == nil {
		t.Fatal("expected error for refresh without existing index")
	}
}

func TestHandleIndexRefresh_Existing(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	setFileIndex(idx)

	res, err := handleIndex(context.Background(), map[string]any{
		"action": "refresh",
		"root":   root,
	})
	if err != nil {
		t.Fatalf("handleIndex refresh: %v", err)
	}
	var m map[string]any
	json.Unmarshal([]byte(res), &m)
	if _, ok := m["total"]; !ok {
		t.Errorf("expected total in response, got %v", m)
	}
}

func TestHandleIndexStatus_Existing(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	setFileIndex(idx)

	res, err := handleIndex(context.Background(), map[string]any{
		"action": "status",
		"root":   root,
	})
	if err != nil {
		t.Fatalf("handleIndex status existing: %v", err)
	}
	var m map[string]any
	json.Unmarshal([]byte(res), &m)
	if !m["exists"].(bool) {
		t.Errorf("expected exists=true, got %v", m)
	}
}

func TestHandleIndex_InvalidAction(t *testing.T) {
	root := t.TempDir()
	if _, err := handleIndex(context.Background(), map[string]any{
		"action": "bad",
		"root":   root,
	}); err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestHandleIndexSearch_WithExistingIndex(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)

	res, err := handleIndexSearch(context.Background(), map[string]any{
		"query":       "hello",
		"root":        root,
		"search_type": "regex",
	})
	if err != nil {
		t.Fatalf("handleIndexSearch existing: %v", err)
	}
	var results []scoutResult
	json.Unmarshal([]byte(res), &results)
	if len(results) == 0 {
		t.Fatal("expected results with existing index")
	}
}

func TestHandleIndexSearch_EmptyQuery(t *testing.T) {
	root := t.TempDir()
	if _, err := handleIndexSearch(context.Background(), map[string]any{
		"root": root,
	}); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestHandleIndexSearch_Symbol(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	res, err := handleIndexSearch(context.Background(), map[string]any{
		"query":       "hello",
		"root":        root,
		"search_type": "symbol",
	})
	if err != nil {
		t.Fatalf("handleIndexSearch symbol: %v", err)
	}
	var results []scoutResult
	json.Unmarshal([]byte(res), &results)
	if len(results) == 0 {
		t.Fatal("expected symbol results")
	}
}

func TestHandleIndexSearch_Usage(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\nfunc call() { hello() }\n"), 0644)

	res, err := handleIndexSearch(context.Background(), map[string]any{
		"query":       "hello",
		"root":        root,
		"search_type": "usage",
		"max_results": float64(10),
	})
	if err != nil {
		t.Fatalf("handleIndexSearch usage: %v", err)
	}
	var results []scoutResult
	json.Unmarshal([]byte(res), &results)
	if len(results) == 0 {
		t.Fatal("expected usage results")
	}
}

func TestHandleIndexSearch_Semantic(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	res, err := handleIndexSearch(context.Background(), map[string]any{
		"query":       "hello",
		"root":        root,
		"search_type": "semantic",
	})
	if err != nil {
		t.Fatalf("handleIndexSearch semantic: %v", err)
	}
	var results []scoutResult
	json.Unmarshal([]byte(res), &results)
	if len(results) == 0 {
		t.Fatal("expected semantic results")
	}
}

func TestHandleIndexSearch_InvalidRoot(t *testing.T) {
	if _, err := handleIndexSearch(context.Background(), map[string]any{
		"query": "hello",
		"root":  "/dev/null/nonexistent-dir-xyz/abc",
	}); err == nil {
		t.Fatal("expected error for invalid root")
	}
}

func TestHandleIndexSearch_MaxResults(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	res, err := handleIndexSearch(context.Background(), map[string]any{
		"query":       "hello",
		"root":        root,
		"max_results": float64(0),
	})
	if err != nil {
		t.Fatalf("handleIndexSearch max_results: %v", err)
	}
	var results []scoutResult
	json.Unmarshal([]byte(res), &results)
	if len(results) == 0 {
		t.Fatal("expected results with max_results=0")
	}
}

func TestHandleIndex_DefaultAction(t *testing.T) {
	root := t.TempDir()
	res, err := handleIndex(context.Background(), map[string]any{
		"root": root,
	})
	if err != nil {
		t.Fatalf("handleIndex default action: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["exists"].(bool) {
		t.Fatal("expected no index for default status action")
	}
}

func TestHandleIndexSearch_RefreshExistingInMemory(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	idx, _ := buildIndex(root)
	setFileIndex(idx)

	res, err := handleIndexSearch(context.Background(), map[string]any{
		"query":       "hello",
		"root":        root,
		"search_type": "regex",
	})
	if err != nil {
		t.Fatalf("handleIndexSearch refresh in-memory: %v", err)
	}
	var results []scoutResult
	json.Unmarshal([]byte(res), &results)
	if len(results) == 0 {
		t.Fatal("expected results with in-memory index refresh")
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

func TestIndexBuildCmd_Default(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	oldWd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(oldWd)
	if err := indexBuildCmd.RunE(indexBuildCmd, nil); err != nil {
		t.Fatalf("indexBuildCmd default: %v", err)
	}
}

func TestIndexBuildCmd_Root(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	if err := indexBuildCmd.RunE(indexBuildCmd, []string{root}); err != nil {
		t.Fatalf("indexBuildCmd root: %v", err)
	}
}

func TestIndexRefreshCmd_NoExisting(t *testing.T) {
	root := t.TempDir()
	getOut := captureStdout(t)
	indexRefreshCmd.RunE(indexRefreshCmd, []string{root})
	buf := getOut()
	if !strings.Contains(buf, "No existing index") {
		t.Fatalf("expected no existing index message, got %q", buf)
	}
}

func TestIndexRefreshCmd_Existing(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	setFileIndex(idx)
	if err := indexRefreshCmd.RunE(indexRefreshCmd, []string{root}); err != nil {
		t.Fatalf("indexRefreshCmd existing: %v", err)
	}
}

func TestIndexStatusCmd_NoExisting(t *testing.T) {
	root := t.TempDir()
	getOut := captureStdout(t)
	indexStatusCmd.RunE(indexStatusCmd, []string{root})
	buf := getOut()
	if !strings.Contains(buf, "No index found") {
		t.Fatalf("expected no index found, got %q", buf)
	}
}

func TestIndexStatusCmd_Existing(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	setFileIndex(idx)
	getOut := captureStdout(t)
	if err := indexStatusCmd.RunE(indexStatusCmd, []string{root}); err != nil {
		t.Fatalf("indexStatusCmd existing: %v", err)
	}
	buf := getOut()
	if !strings.Contains(buf, "Index:") {
		t.Fatalf("expected index status, got %q", buf)
	}
}

func TestIndexWatchCmd_BuildNewIndex(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	oldInterval := indexWatchInterval
	oldMax := indexWatchMaxIterations
	indexWatchInterval = 0
	indexWatchMaxIterations = 1
	defer func() {
		indexWatchInterval = oldInterval
		indexWatchMaxIterations = oldMax
	}()
	if err := indexWatchCmd.RunE(indexWatchCmd, []string{root}); err != nil {
		t.Fatalf("indexWatchCmd build: %v", err)
	}
}

func TestIndexWatchCmd_RefreshExisting(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	oldInterval := indexWatchInterval
	oldMax := indexWatchMaxIterations
	indexWatchInterval = 0
	indexWatchMaxIterations = 1
	defer func() {
		indexWatchInterval = oldInterval
		indexWatchMaxIterations = oldMax
	}()
	if err := indexWatchCmd.RunE(indexWatchCmd, []string{root}); err != nil {
		t.Fatalf("indexWatchCmd refresh: %v", err)
	}
}

func TestIndexClearCmd_Success(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	if err := indexClearCmd.RunE(indexClearCmd, []string{root}); err != nil {
		t.Fatalf("indexClearCmd: %v", err)
	}
}

func TestIndexClearCmd_RemoveError(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644)
	idx, _ := buildIndex(root)
	saveIndex(idx)
	p := indexPath(root)
	os.Remove(p)
	os.Mkdir(p, 0755)
	os.WriteFile(filepath.Join(p, "block"), []byte("x"), 0644)
	defer os.RemoveAll(p)
	if err := indexClearCmd.RunE(indexClearCmd, []string{root}); err == nil {
		t.Fatal("expected error when index path is a non-empty directory")
	}
}
