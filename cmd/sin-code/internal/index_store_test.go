// SPDX-License-Identifier: MIT
// Purpose: tests for the in-memory trigram/symbol index used by sin scout.

package internal

import (
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// TestMockFileInfo verifies that mockFileInfo satisfies the os.FileInfo
// interface and returns the zero values used by the index. (st-cov1)
func TestMockFileInfo(t *testing.T) {
	m := &mockFileInfo{}
	if m.Name() != "" {
		t.Errorf("Name() = %q, want empty", m.Name())
	}
	if m.Size() != 0 {
		t.Errorf("Size() = %d, want 0", m.Size())
	}
	if m.Mode() != 0 {
		t.Errorf("Mode() = %v, want 0", m.Mode())
	}
	if !m.ModTime().IsZero() {
		t.Errorf("ModTime() = %v, want zero", m.ModTime())
	}
	if m.IsDir() {
		t.Error("IsDir() = true, want false")
	}
	if m.Sys() != nil {
		t.Errorf("Sys() = %v, want nil", m.Sys())
	}
}

func TestIndexStore_QueryTrigramsShort(t *testing.T) {
	if got := queryTrigrams("ab"); got != nil {
		t.Fatalf("expected nil for short query, got %v", got)
	}
	idx := &inMemoryIndex{root: t.TempDir(), files: make(map[string]*fileIndex)}
	if got := idx.searchTrigram("ab"); got != nil {
		t.Fatalf("expected nil when query has no trigrams, got %v", got)
	}
}

func TestIndexStore_SearchTrigramFilters(t *testing.T) {
	idx := &inMemoryIndex{root: t.TempDir(), files: make(map[string]*fileIndex)}
	idx.add(indexEntry{File: "hit.go", Trigrams: buildTrigrams("abcdef"), IsBinary: false})
	idx.add(indexEntry{File: "binary.go", Trigrams: buildTrigrams("abcdef"), IsBinary: true})
	idx.add(indexEntry{File: "empty.go", Trigrams: []uint32{}, IsBinary: false})
	idx.add(indexEntry{File: "miss.go", Trigrams: buildTrigrams("xyz"), IsBinary: false})

	hits := idx.searchTrigram("abc")
	if len(hits) != 1 || hits[0] != "hit.go" {
		t.Fatalf("expected only hit.go, got %v", hits)
	}
}

func TestIndexStore_SearchSymbolsFilters(t *testing.T) {
	idx := &inMemoryIndex{root: t.TempDir(), files: make(map[string]*fileIndex)}
	idx.add(indexEntry{File: "a.go", Symbols: []symbolIndex{{Name: "Alpha", Type: "func"}}, IsBinary: false})
	idx.add(indexEntry{File: "b.go", Symbols: []symbolIndex{{Name: "Alpha", Type: "method"}}, IsBinary: true})

	// Binary entries are skipped even when the symbol name matches.
	// Non-binary entries with a mismatched type are skipped.
	matches := idx.searchSymbols("Alpha", "method")
	if len(matches) != 0 {
		t.Fatalf("expected no matches, got %v", matches)
	}
}

func TestIndexStore_FileModTimeMissing(t *testing.T) {
	idx := &inMemoryIndex{root: t.TempDir(), files: make(map[string]*fileIndex)}
	if _, ok := idx.fileModTime("missing.go"); ok {
		t.Fatal("expected false for missing file")
	}
}

func TestIndexStore_LoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, ".sin-code")
	if err := os.MkdirAll(binDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "index.bin"), []byte("not a gob"), 0644); err != nil {
		t.Fatal(err)
	}
	idx, err := loadIndex(dir)
	if err != nil {
		t.Fatalf("loadIndex should not error on corrupt file: %v", err)
	}
	if idx.len() != 0 {
		t.Fatalf("expected empty index after corrupt load, got %d", idx.len())
	}
}

func TestIndexStore_SaveIndexErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	idx, err := buildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}

	oldCreate, oldEncode, oldClose, oldRemove := saveIndexCreate, saveIndexEncode, saveIndexClose, saveIndexRemove
	defer func() {
		saveIndexCreate = oldCreate
		saveIndexEncode = oldEncode
		saveIndexClose = oldClose
		saveIndexRemove = oldRemove
	}()

	// Create error: make the index directory read-only.
	binDir := filepath.Join(dir, ".sin-code")
	if err := os.MkdirAll(binDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(binDir, 0000); err != nil {
		t.Fatal(err)
	}
	if err := saveIndex(idx); err == nil {
		t.Fatal("expected error when create fails")
	}
	if err := os.Chmod(binDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Encode error.
	saveIndexEncode = func(enc *gob.Encoder, pi persistentIndex) error { return errors.New("encode fail") }
	if err := saveIndex(idx); err == nil {
		t.Fatal("expected error when encode fails")
	}

	// Close error.
	saveIndexEncode = oldEncode
	saveIndexClose = func(f *os.File) error { f.Close(); return errors.New("close fail") }
	if err := saveIndex(idx); err == nil {
		t.Fatal("expected error when close fails")
	}
}

func TestIndexStore_ProcessFileEdgeCases(t *testing.T) {
	dir := t.TempDir()

	// Missing file: stat error path uses mockFileInfo.
	ie := processFileForIndex(filepath.Join(dir, "missing.go"), dir)
	if ie.File != "missing.go" {
		t.Fatalf("expected File missing.go, got %q", ie.File)
	}
	if ie.Size != 0 {
		t.Fatalf("expected size 0 for missing file, got %d", ie.Size)
	}

	// Binary file: early return before reading content.
	bin := filepath.Join(dir, "bin.png")
	if err := os.WriteFile(bin, []byte{0x00, 0x01}, 0644); err != nil {
		t.Fatal(err)
	}
	ie = processFileForIndex(bin, dir)
	if !ie.IsBinary {
		t.Fatal("expected binary file to be marked binary")
	}
	if len(ie.Trigrams) != 0 {
		t.Fatal("expected no trigrams for binary file")
	}

	// Read error path.
	txt := filepath.Join(dir, "x.go")
	if err := os.WriteFile(txt, []byte("package x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	oldRead := processFileReadFile
	defer func() { processFileReadFile = oldRead }()
	processFileReadFile = func(string) ([]byte, error) { return nil, errors.New("read fail") }
	ie = processFileForIndex(txt, dir)
	if len(ie.Trigrams) != 0 || ie.Lines != 0 {
		t.Fatalf("expected empty trigrams/lines after read error, got %d trigrams, %d lines", len(ie.Trigrams), ie.Lines)
	}
}

func TestIndexStore_BuildIndexIgnores(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored/\n*.log\n.gitignore\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc A() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "ignored"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored", "secret.go"), []byte("package ignored\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "debug.log"), []byte("log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin.png"), []byte{0x00, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	// Hidden directories should be skipped.
	if err := os.MkdirAll(filepath.Join(dir, ".hidden"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden", "secret.go"), []byte("package hidden\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Unknown-language files hit the parseOutline "none" path.
	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("plain text file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(dir, "locked")
	if err := os.MkdirAll(locked, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0755)

	idx, err := buildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if idx.len() != 2 {
		t.Fatalf("expected 2 indexed files, got %d", idx.len())
	}
	if !idx.hasFile("a.go") {
		t.Fatalf("expected a.go to be indexed, got %v", idx.allIndexedPaths())
	}
	if !idx.hasFile("plain.txt") {
		t.Fatalf("expected plain.txt to be indexed, got %v", idx.allIndexedPaths())
	}
}

func TestIndexStore_RefreshIndexIgnores(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored/\n*.log\n.gitignore\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc A() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	idx, err := buildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Add new entries that should be ignored or skipped during refresh.
	if err := os.MkdirAll(filepath.Join(dir, "ignored"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored", "secret.go"), []byte("package ignored\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "debug.log"), []byte("log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin.png"), []byte{0x00, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".hidden"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden", "secret.go"), []byte("package hidden\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("plain text file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(dir, "locked")
	if err := os.MkdirAll(locked, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0755)

	idx2, added, removed, err := refreshIndex(idx)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || removed != 0 {
		t.Fatalf("expected 1 added/0 removed, got %d/%d", added, removed)
	}
	if idx2.len() != 2 {
		t.Fatalf("expected 2 indexed files after refresh, got %d", idx2.len())
	}
	if !idx2.hasFile("a.go") {
		t.Fatalf("expected a.go to remain indexed, got %v", idx2.allIndexedPaths())
	}
	if !idx2.hasFile("plain.txt") {
		t.Fatalf("expected plain.txt to be indexed after refresh, got %v", idx2.allIndexedPaths())
	}
}

func TestIndexStore_IndexHelpers(t *testing.T) {
	idx := &inMemoryIndex{root: "/tmp/root", files: make(map[string]*fileIndex)}
	if got := idx.rootPath(); got != "/tmp/root" {
		t.Errorf("rootPath() = %q, want /tmp/root", got)
	}
	if idx.len() != 0 {
		t.Errorf("len() = %d, want 0", idx.len())
	}
	if idx.hasFile("a") {
		t.Error("expected hasFile('a') = false")
	}

	mtime := time.Now()
	idx.add(indexEntry{
		File:     "a",
		ModTime:  mtime,
		Size:     42,
		Trigrams: []uint32{1},
		Symbols:  []symbolIndex{{Name: "Alpha", Type: "func"}},
		Lines:    5,
	})
	if !idx.hasFile("a") {
		t.Error("expected hasFile('a') = true")
	}
	if mt, ok := idx.fileModTime("a"); !ok || !mt.Equal(mtime) {
		t.Errorf("fileModTime('a') = (%v, %v), want (%v, true)", mt, ok, mtime)
	}
	paths := idx.allIndexedPaths()
	if len(paths) != 1 || paths[0] != "a" {
		t.Errorf("allIndexedPaths() = %v, want [a]", paths)
	}

	idx.remove("a")
	if idx.hasFile("a") {
		t.Error("expected a to be removed")
	}

	idx.add(indexEntry{File: "b"})
	idx.clear()
	if idx.len() != 0 {
		t.Errorf("expected empty after clear, got %d", idx.len())
	}

	// searchSymbols exact match
	idx.add(indexEntry{File: "c.go", Symbols: []symbolIndex{{Name: "Alpha", Type: "func"}}})
	if got := idx.searchSymbols("Alpha", "func"); len(got) != 1 || got[0] != "c.go" {
		t.Errorf("expected exact symbol match, got %v", got)
	}
	// searchSymbols substring match (len >= 3)
	idx.add(indexEntry{File: "d.go", Symbols: []symbolIndex{{Name: "Alphabet", Type: "func"}}})
	if got := idx.searchSymbols("Alp", "func"); len(got) != 2 {
		t.Errorf("expected substring symbol matches, got %v", got)
	}

	// mockFileInfo interface methods
	m := &mockFileInfo{}
	if m.Name() != "" {
		t.Errorf("Name() = %q, want empty", m.Name())
	}
	if m.Mode() != 0 {
		t.Errorf("Mode() = %v, want 0", m.Mode())
	}
	if m.IsDir() {
		t.Error("IsDir() = true, want false")
	}
	if m.Sys() != nil {
		t.Errorf("Sys() = %v, want nil", m.Sys())
	}
}

func TestIndexStore_BuildTrigramsShort(t *testing.T) {
	if got := buildTrigrams("ab"); got != nil {
		t.Fatalf("expected nil for short input, got %v", got)
	}
}

func TestIndexStore_SaveIndexMkdirError(t *testing.T) {
	dir := t.TempDir()
	rootFile := filepath.Join(dir, "rootfile")
	if err := os.WriteFile(rootFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	idx := &inMemoryIndex{
		root:  rootFile,
		files: map[string]*fileIndex{"a": {path: "a"}},
	}
	if err := saveIndex(idx); err == nil {
		t.Fatal("expected error when MkdirAll fails")
	}
}

func TestIndexStore_LoadAndSave(t *testing.T) {
	dir := t.TempDir()
	idx := &inMemoryIndex{
		root:      dir,
		version:   7,
		createdAt: time.Now(),
		files:     make(map[string]*fileIndex),
	}
	idx.add(indexEntry{
		File:     "x.go",
		ModTime:  time.Now(),
		Size:     10,
		Trigrams: buildTrigrams("package x"),
		Lines:    2,
		Symbols:  []symbolIndex{{Name: "X", Type: "func"}},
	})
	if err := saveIndex(idx); err != nil {
		t.Fatalf("saveIndex: %v", err)
	}

	loaded, err := loadIndex(dir)
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}
	if loaded.len() != 1 {
		t.Fatalf("expected 1 loaded file, got %d", loaded.len())
	}
	if loaded.version != 7 {
		t.Errorf("expected version 7, got %d", loaded.version)
	}
	if _, ok := loaded.fileModTime("x.go"); !ok {
		t.Error("expected x.go in loaded index")
	}

	// Missing index file returns an empty index.
	empty, err := loadIndex(t.TempDir())
	if err != nil {
		t.Fatalf("loadIndex missing: %v", err)
	}
	if empty.len() != 0 {
		t.Errorf("expected empty index, got %d", empty.len())
	}

	// Non-NotExist open error is propagated.
	badDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(badDir, ".sin-code"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadIndex(badDir); err == nil {
		t.Fatal("expected error for non-directory index path")
	}
}

func TestIndexStore_RefreshIndexChanges(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".gitignore\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b\n"), 0644); err != nil {
		t.Fatal(err)
	}
	idx, err := buildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if idx.len() != 2 {
		t.Fatalf("expected 2 files, got %d", idx.len())
	}

	// Update a.go, remove b.go, add c.go.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc A() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "b.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.go"), []byte("package c\n"), 0644); err != nil {
		t.Fatal(err)
	}

	idx2, added, removed, err := refreshIndex(idx)
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || removed != 1 {
		t.Fatalf("expected added=2, removed=1, got %d/%d", added, removed)
	}
	if idx2.len() != 2 {
		t.Fatalf("expected 2 files after refresh, got %d", idx2.len())
	}
	if !idx2.hasFile("a.go") || !idx2.hasFile("c.go") || idx2.hasFile("b.go") {
		t.Fatalf("unexpected index contents: %v", idx2.allIndexedPaths())
	}
}

func TestIndexStore_GetSetFileIndex(t *testing.T) {
	setFileIndex(nil)
	dir := t.TempDir()
	idx, existed, err := getFileIndex(dir)
	if err != nil {
		t.Fatalf("getFileIndex: %v", err)
	}
	if existed {
		t.Error("expected no existing index on first call")
	}
	if idx.len() != 0 {
		t.Errorf("expected empty index, got %d", idx.len())
	}

	// Second call with the same root returns the cached index.
	idx2, existed, err := getFileIndex(dir)
	if err != nil {
		t.Fatalf("getFileIndex cached: %v", err)
	}
	if !existed {
		t.Error("expected cached index on second call")
	}
	if idx2 != idx {
		t.Error("expected cached index pointer")
	}

	// setFileIndex replaces the cached index.
	custom := &inMemoryIndex{root: dir, files: map[string]*fileIndex{"a": nil}}
	setFileIndex(custom)
	got, existed, _ := getFileIndex(dir)
	if !existed || got != custom {
		t.Error("setFileIndex did not replace the cached index")
	}

	// Error path: reset and load from a bad root.
	badDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(badDir, ".sin-code"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	setFileIndex(nil)
	if _, _, err := getFileIndex(badDir); err == nil {
		t.Fatal("expected error from bad index path")
	}
}
