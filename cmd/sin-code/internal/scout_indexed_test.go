// SPDX-License-Identifier: MIT

package internal

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func resetIndexState(t *testing.T) {
	oldBuild := buildIndexOverride
	oldRefresh := refreshIndexOverride
	oldSaveCreate := saveIndexCreate
	oldSaveEncode := saveIndexEncode
	oldSaveClose := saveIndexClose
	oldSearchFile := searchFileFn
	oldFilepathAbs := filepathAbsFn
	setFileIndex(nil)

	t.Cleanup(func() {
		buildIndexOverride = oldBuild
		refreshIndexOverride = oldRefresh
		saveIndexCreate = oldSaveCreate
		saveIndexEncode = oldSaveEncode
		saveIndexClose = oldSaveClose
		searchFileFn = oldSearchFile
		filepathAbsFn = oldFilepathAbs
		setFileIndex(nil)
	})
}


func TestScoutIndexed_BuildIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	buildIndexOverride = func(root string) (*inMemoryIndex, error) {
		return nil, errors.New("build fail")
	}
	_, err := scoutSearchAuto(root, "hello", "regex", 10, true)
	if err == nil {
		t.Fatal("expected error when buildIndex fails")
	}
}

func TestScoutIndexed_SaveIndexErrorAfterBuild(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	saveIndexCreate = func(name string) (*os.File, error) {
		return nil, errors.New("save fail")
	}
	_, err := scoutSearchAuto(root, "hello", "regex", 10, true)
	if err == nil {
		t.Fatal("expected error when saveIndex fails after build")
	}
}

func TestScoutIndexed_RefreshIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	setFileIndex(idx)

	refreshIndexOverride = func(idx *inMemoryIndex) (*inMemoryIndex, int, int, error) {
		return nil, 0, 0, errors.New("refresh fail")
	}
	_, err = scoutSearchAuto(root, "hello", "regex", 10, true)
	if err == nil {
		t.Fatal("expected error when refreshIndex fails")
	}
}

func TestScoutIndexed_SaveIndexErrorAfterRefresh(t *testing.T) {
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
	_, err = scoutSearchAuto(root, "hello", "regex", 10, true)
	if err == nil {
		t.Fatal("expected error when saveIndex fails after refresh")
	}
}

func TestScoutIndexed_GetFileIndexError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	p := indexPath(root)
	os.MkdirAll(filepath.Dir(p), 0750)
	os.WriteFile(p, []byte("x"), 0000)
	defer os.Chmod(p, 0644)

	_, err := scoutSearchAuto(root, "hello", "regex", 10, true)
	if err == nil {
		t.Fatal("expected error when getFileIndex fails")
	}
}

func TestScoutIndexed_SearchWithIndexSymbolWithType(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	// The line contains the literal query after the symbol keyword so the
	// symbol regex built from the full query still matches. The candidate
	// selection is what we are really exercising: it splits "func hello"
	// into stype="func" and name="hello".
	os.WriteFile(filepath.Join(root, "a.go"), []byte("func func hello\n"), 0644)

	idx := &inMemoryIndex{
		root:  root,
		files: map[string]*fileIndex{},
	}
	idx.files["a.go"] = &fileIndex{
		path:    "a.go",
		symbols: []symbolIndex{{Name: "hello", Type: "func"}},
	}

	results, err := searchWithIndex(idx, root, "func hello", "symbol", 10, true)
	if err != nil {
		t.Fatalf("searchWithIndex: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected symbol results")
	}
}

func TestScoutIndexed_SearchWithIndexCompileError(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	idx := &inMemoryIndex{
		root:  root,
		files: map[string]*fileIndex{},
	}
	_, err := searchWithIndex(idx, root, "[invalid", "regex", 10, true)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestScoutIndexed_SearchWithIndexCandidateErrors(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	os.WriteFile(filepath.Join(root, "binary.go"), []byte{0, 1, 2}, 0644)
	os.WriteFile(filepath.Join(root, "bad.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)

	tri := queryTrigrams("hello")
	idx := &inMemoryIndex{
		root: root,
		files: map[string]*fileIndex{
			"a.go":      {path: "a.go", trigrams: tri},
			"binary.go": {path: "binary.go", trigrams: tri},
			"bad.go":    {path: "bad.go", trigrams: tri},
			"missing.go": {path: "missing.go", trigrams: tri},
		},
	}

	searchFileFn = func(path, rel, root string, re *regexp.Regexp, searchType string) ([]scoutResult, error) {
		if rel == "bad.go" {
			return nil, errors.New("bad")
		}
		return searchFile(path, rel, root, re, searchType)
	}

	results, err := searchWithIndex(idx, root, "hello", "regex", 10, true)
	if err != nil {
		t.Fatalf("searchWithIndex: %v", err)
	}
	found := false
	for _, r := range results {
		if r.File == "a.go" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a.go result, got %+v", results)
	}
}

func TestScoutIndexed_RegexMetaFallback(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	results, err := scoutSearchAuto(root, "h.llo", "regex", 10, true)
	if err != nil {
		t.Fatalf("scoutSearchAuto: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results from regex metachar fallback")
	}
}

func TestScoutIndexed_SuccessPath(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	results, err := scoutSearchAuto(root, "hello", "regex", 10, true)
	if err != nil {
		t.Fatalf("scoutSearchAuto: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
}

func TestScoutIndexed_SearchWithIndexMaxResultsZero(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package main\n\nfunc hello() {}\n"), 0644)
	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	results, err := searchWithIndex(idx, root, "hello", "regex", 0, true)
	if err != nil {
		t.Fatalf("searchWithIndex: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results with maxResults=0")
	}
}

func TestScoutIndexed_RefreshSuccess(t *testing.T) {
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

	results, err := scoutSearchAuto(root, "hello", "regex", 10, true)
	if err != nil {
		t.Fatalf("scoutSearchAuto: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results after refresh")
	}
}

func TestScoutIndexed_SearchWithIndexUnknownType(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()
	idx := &inMemoryIndex{
		root:  root,
		files: map[string]*fileIndex{},
	}
	_, err := searchWithIndex(idx, root, "hello", "bogus", 10, true)
	if err == nil {
		t.Fatal("expected error for unknown search type")
	}
}

func TestScoutIndexed_SearchWithIndexOverscanAndTruncate(t *testing.T) {
	resetIndexState(t)
	root := t.TempDir()

	var lines []byte
	for i := 0; i < 30; i++ {
		lines = append(lines, []byte("hello\n")...)
	}
	os.WriteFile(filepath.Join(root, "a.go"), lines, 0644)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex: %v", err)
	}
	results, err := searchWithIndex(idx, root, "hello", "regex", 5, true)
	if err != nil {
		t.Fatalf("searchWithIndex: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results after truncation, got %d", len(results))
	}
}
