// SPDX-License-Identifier: MIT

package internal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleIndex_DefaultRoot(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package x\n"), 0644)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	os.Chdir(root)
	defer os.Chdir(oldWd)

	res, err := handleIndex(context.Background(), map[string]any{
		"action": "build",
	})
	if err != nil {
		t.Fatalf("handleIndex: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if int(m["files"].(float64)) != 1 {
		t.Fatalf("expected 1 file, got %v", m["files"])
	}
}

func TestHandleIndex_AbsError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	filepathAbsFn = func(path string) (string, error) {
		return "", errors.New("abs fail")
	}

	_, err := handleIndex(context.Background(), map[string]any{
		"root": root,
	})
	if err == nil {
		t.Fatal("expected error when root abs fails")
	}
}

func TestHandleIndex_BuildIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	buildIndexOverride = func(root string) (*inMemoryIndex, error) {
		return nil, errors.New("build fail")
	}
	_, err := handleIndex(context.Background(), map[string]any{
		"action": "build",
		"root":   root,
	})
	if err == nil {
		t.Fatal("expected error when buildIndex fails")
	}
}

func TestHandleIndex_SaveIndexErrorAfterBuild(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package x\n"), 0644)

	saveIndexCreate = func(name string) (*os.File, error) {
		return nil, errors.New("save fail")
	}
	_, err := handleIndex(context.Background(), map[string]any{
		"action": "build",
		"root":   root,
	})
	if err == nil {
		t.Fatal("expected error when saveIndex fails after build")
	}
}

func TestHandleIndex_RefreshIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package x\n"), 0644)
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if err := saveIndex(idx); err != nil {
		t.Fatalf("saveIndex: %v", err)
	}
	setFileIndex(idx)

	refreshIndexOverride = func(idx *inMemoryIndex) (*inMemoryIndex, int, int, error) {
		return nil, 0, 0, errors.New("refresh fail")
	}
	_, err = handleIndex(context.Background(), map[string]any{
		"action": "refresh",
		"root":   root,
	})
	if err == nil {
		t.Fatal("expected error when refreshIndex fails")
	}
}

func TestHandleIndex_SaveIndexErrorAfterRefresh(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package x\n"), 0644)
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if err := saveIndex(idx); err != nil {
		t.Fatalf("saveIndex: %v", err)
	}
	setFileIndex(idx)

	saveIndexCreate = func(name string) (*os.File, error) {
		return nil, errors.New("save fail")
	}
	_, err = handleIndex(context.Background(), map[string]any{
		"action": "refresh",
		"root":   root,
	})
	if err == nil {
		t.Fatal("expected error when saveIndex fails after refresh")
	}
}

func TestHandleIndex_GetFileIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	p := indexPath(root)
	os.MkdirAll(filepath.Dir(p), 0750)
	os.WriteFile(p, []byte("x"), 0000)
	defer os.Chmod(p, 0644)

	_, err := handleIndex(context.Background(), map[string]any{
		"action": "status",
		"root":   root,
	})
	if err == nil {
		t.Fatal("expected error when getFileIndex fails")
	}
}

func TestHandleIndex_ClearRemoveError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package x\n"), 0644)
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if err := saveIndex(idx); err != nil {
		t.Fatalf("saveIndex: %v", err)
	}

	p := indexPath(root)
	os.Remove(p)
	os.Mkdir(p, 0755)
	os.WriteFile(filepath.Join(p, "block"), []byte("x"), 0644)
	defer os.RemoveAll(p)

	_, err = handleIndex(context.Background(), map[string]any{
		"action": "clear",
		"root":   root,
	})
	if err == nil {
		t.Fatal("expected error when clear cannot remove index path")
	}
}

func TestHandleIndex_RefreshGetFileIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	p := indexPath(root)
	os.MkdirAll(filepath.Dir(p), 0750)
	os.WriteFile(p, []byte("x"), 0000)
	defer os.Chmod(p, 0644)

	_, err := handleIndex(context.Background(), map[string]any{
		"action": "refresh",
		"root":   root,
	})
	if err == nil {
		t.Fatal("expected error when getFileIndex fails in refresh action")
	}
}

func TestHandleIndexSearch_DefaultRoot(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	os.Chdir(root)
	defer os.Chdir(oldWd)

	res, err := handleIndexSearch(context.Background(), map[string]any{
		"query": "hello",
	})
	if err != nil {
		t.Fatalf("handleIndexSearch: %v", err)
	}
	var results []scoutResult
	if err := json.Unmarshal([]byte(res), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results with default root and search_type")
	}
}

func TestHandleIndexSearch_AbsError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	filepathAbsFn = func(path string) (string, error) {
		return "", errors.New("abs fail")
	}

	_, err := handleIndexSearch(context.Background(), map[string]any{
		"query": "hello",
		"root":  root,
	})
	if err == nil {
		t.Fatal("expected error when root abs fails")
	}
}

func TestHandleIndexSearch_GetFileIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	p := indexPath(root)
	os.MkdirAll(filepath.Dir(p), 0750)
	os.WriteFile(p, []byte("x"), 0000)
	defer os.Chmod(p, 0644)

	_, err := handleIndexSearch(context.Background(), map[string]any{
		"query": "hello",
		"root":  root,
	})
	if err == nil {
		t.Fatal("expected error when getFileIndex fails")
	}
}

func TestHandleIndexSearch_BuildIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	buildIndexOverride = func(root string) (*inMemoryIndex, error) {
		return nil, errors.New("build fail")
	}
	_, err := handleIndexSearch(context.Background(), map[string]any{
		"query": "hello",
		"root":  root,
	})
	if err == nil {
		t.Fatal("expected error when buildIndex fails")
	}
}

func TestHandleIndexSearch_SaveIndexErrorAfterBuild(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	saveIndexCreate = func(name string) (*os.File, error) {
		return nil, errors.New("save fail")
	}
	_, err := handleIndexSearch(context.Background(), map[string]any{
		"query": "hello",
		"root":  root,
	})
	if err == nil {
		t.Fatal("expected error when saveIndex fails after build")
	}
}

func TestHandleIndexSearch_RefreshIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if err := saveIndex(idx); err != nil {
		t.Fatalf("saveIndex: %v", err)
	}
	setFileIndex(idx)

	refreshIndexOverride = func(idx *inMemoryIndex) (*inMemoryIndex, int, int, error) {
		return nil, 0, 0, errors.New("refresh fail")
	}
	_, err = handleIndexSearch(context.Background(), map[string]any{
		"query": "hello",
		"root":  root,
	})
	if err == nil {
		t.Fatal("expected error when refreshIndex fails")
	}
}

func TestHandleIndexSearch_SaveIndexErrorAfterRefresh(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	if err := saveIndex(idx); err != nil {
		t.Fatalf("saveIndex: %v", err)
	}
	setFileIndex(idx)

	saveIndexCreate = func(name string) (*os.File, error) {
		return nil, errors.New("save fail")
	}
	_, err = handleIndexSearch(context.Background(), map[string]any{
		"query": "hello",
		"root":  root,
	})
	if err == nil {
		t.Fatal("expected error when saveIndex fails after refresh")
	}
}

func TestHandleIndexSearch_SearchError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	_, err := handleIndexSearch(context.Background(), map[string]any{
		"query":       "[invalid",
		"root":        root,
		"search_type": "regex",
	})
	if err == nil {
		t.Fatal("expected error when searchWithIndex fails")
	}
}
