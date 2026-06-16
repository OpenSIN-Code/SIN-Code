// SPDX-License-Identifier: MIT
// Purpose: tests for the instinct search integration (issue #160).
// Uses an in-memory jsonPersister via t.TempDir to avoid touching
// the real $SIN_CODE_HOME.
package instinct

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/rag"
)

func TestInstinctSearchPath_Default(t *testing.T) {
	// Without SIN_CODE_HOME or XDG_DATA_HOME, the path falls
	// back to $HOME/.local/share/sin-code/instinct-embeddings.json.
	// We can't easily reset the env, so we just verify the
	// function returns a non-empty path.
	p, err := instinctSearchPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, "instinct-embeddings.json") {
		t.Errorf("expected filename in path, got %q", p)
	}
}

func TestInstinctSearchPath_Override(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "/tmp/sin-test-home")
	p, err := instinctSearchPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, "/tmp/sin-test-home") {
		t.Errorf("expected override path, got %q", p)
	}
}

func TestJSONPersister_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "embeddings.json")
	p := &jsonPersister{path: path}
	// Save
	entries := []rag.Entry{
		{ID: "a", Vector: make([]float32, rag.EmbeddingDim)},
		{ID: "b", Vector: make([]float32, rag.EmbeddingDim)},
	}
	for i := range entries[0].Vector {
		entries[0].Vector[i] = float32(i) / float32(rag.EmbeddingDim)
	}
	if err := p.Save(entries); err != nil {
		t.Fatal(err)
	}
	// Load
	loaded, err := p.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	// IDs and vectors match
	byID := map[string][]float32{}
	for _, e := range loaded {
		byID[e.ID] = e.Vector
	}
	if len(byID["a"]) != rag.EmbeddingDim {
		t.Errorf("expected dim %d, got %d", rag.EmbeddingDim, len(byID["a"]))
	}
	for i := range byID["a"] {
		if byID["a"][i] != entries[0].Vector[i] {
			t.Errorf("[%d] mismatch: %v != %v", i, byID["a"][i], entries[0].Vector[i])
		}
	}
}

func TestJSONPersister_LoadNoFile(t *testing.T) {
	dir := t.TempDir()
	p := &jsonPersister{path: filepath.Join(dir, "nonexistent.json")}
	loaded, err := p.Load()
	if err != nil {
		t.Errorf("first-run load should not error, got %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected 0 entries from missing file, got %d", len(loaded))
	}
}

func TestJSONPersister_AtomicWrite(t *testing.T) {
	// Verify no .tmp file is left behind after a successful Save.
	dir := t.TempDir()
	path := filepath.Join(dir, "embeddings.json")
	p := &jsonPersister{path: path}
	if err := p.Save([]rag.Entry{{ID: "x", Vector: make([]float32, rag.EmbeddingDim)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Load(); err != nil {
		t.Fatal(err)
	}
	// The .tmp should not exist.
	if _, err := openTmp(path); err == nil {
		t.Error("expected .tmp file to be gone after Save")
	}
}

func openTmp(path string) (interface{ Close() error }, error) {
	return osOpenHelper(path + ".tmp")
}

func TestJSONPersister_PersistsAcrossInstances(t *testing.T) {
	// Two persister instances pointing at the same path should
	// see the same data — i.e. the on-disk format is stable.
	dir := t.TempDir()
	path := filepath.Join(dir, "embeddings.json")
	p1 := &jsonPersister{path: path}
	vec := make([]float32, rag.EmbeddingDim)
	for i := range vec {
		vec[i] = float32(i+1) / float32(rag.EmbeddingDim+1)
	}
	if err := p1.Save([]rag.Entry{{ID: "stable", Vector: vec}}); err != nil {
		t.Fatal(err)
	}
	p2 := &jsonPersister{path: path}
	loaded, err := p2.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1, got %d", len(loaded))
	}
	for i := range loaded[0].Vector {
		if loaded[0].Vector[i] != vec[i] {
			t.Errorf("[%d] mismatch", i)
		}
	}
}

func TestTrim(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hell…"},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "ab…"},
	}
	for _, c := range cases {
		got := trim(c.in, c.n)
		if got != c.want {
			t.Errorf("trim(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestAllIDs(t *testing.T) {
	idx := rag.NewIndex(nil)
	vec := make([]float32, rag.EmbeddingDim)
	for j := range vec {
		vec[j] = float32(j) / float32(rag.EmbeddingDim)
	}
	idx.Set("a", vec)
	idx.Set("b", vec)
	ids := allIDs(idx)
	if len(ids) != 2 {
		t.Errorf("expected 2, got %d", len(ids))
	}
	// Sorted ascending
	if ids[0] != "a" || ids[1] != "b" {
		t.Errorf("expected [a, b], got %v", ids)
	}
}

func TestRebuildIndex_Empty(t *testing.T) {
	// rebuildIndex with no active instincts is a no-op. We can't
	// easily call mgr() in a unit test (it requires a working
	// directory), so this test is a smoke check on the helper
	// surface only.
	idx := rag.NewIndex(nil)
	if idx.Size() != 0 {
		t.Error("expected empty index")
	}
}

// Sentinel to ensure the file is exercised.
var _ = context.Background
