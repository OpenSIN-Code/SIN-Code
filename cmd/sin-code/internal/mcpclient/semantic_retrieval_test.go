// SPDX-License-Identifier: MIT
// Purpose: tests for semantic tool retrieval with offline TF-IDF features
// (issue #364). Runs with -race to satisfy mandate M7.
package mcpclient

import (
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
)

func testSemanticSpecs() []ToolSpec {
	return []ToolSpec{
		{
			Name:        "sin_read",
			Description: "Read a file (UTF-8, capped at 64KB).",
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "sin_write",
			Description: "Atomically write content to a file, creating parent dirs.",
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "sin_bash",
			Description: "Run a shell command in the workspace (120s timeout).",
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "sin_git_commit",
			Description: "Create a git commit from staged changes.",
			InputSchema: map[string]any{"type": "object"},
		},
	}
}

func TestNewSemanticIndex_DoesNotMutateInput(t *testing.T) {
	want := testSemanticSpecs()
	got := NewSemanticIndex(want).All()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NewSemanticIndex mutated input: got %+v, want %+v", got, want)
	}
}

func TestSemanticIndex_CountAndAll(t *testing.T) {
	idx := NewSemanticIndex(testSemanticSpecs())
	if got, want := idx.Count(), 4; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
	if len(idx.All()) != 4 {
		t.Errorf("All() returned %d specs, want 4", len(idx.All()))
	}
}

func TestSemanticIndex_SearchEmptyOrInvalid(t *testing.T) {
	idx := NewSemanticIndex(testSemanticSpecs())
	if idx.Search("", 5) != nil {
		t.Error("Search(\"\", k) should return nil")
	}
	if idx.Search("read", 0) != nil {
		t.Error("Search(query, 0) should return nil")
	}
	if idx.Search("  ", -1) != nil {
		t.Error("Search(whitespace, -1) should return nil")
	}
}

func TestSemanticIndex_SearchNameMatch(t *testing.T) {
	idx := NewSemanticIndex(testSemanticSpecs())
	cases := []struct {
		query string
		want  string
	}{
		{"read file", "sin_read"},
		{"write file", "sin_write"},
		{"bash", "sin_bash"},
		{"git commit", "sin_git_commit"},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			got := idx.Search(c.query, 1)
			if len(got) == 0 {
				t.Fatalf("Search(%q, 1) returned no results", c.query)
			}
			if got[0].Name != c.want {
				t.Errorf("Search(%q, 1)[0].Name = %q, want %q", c.query, got[0].Name, c.want)
			}
		})
	}
}

func TestSemanticIndex_SearchDescriptionMatch(t *testing.T) {
	idx := NewSemanticIndex(testSemanticSpecs())
	got := idx.Search("shell command", 5)
	names := make([]string, len(got))
	for i, s := range got {
		names[i] = s.Name
	}
	if len(names) == 0 || names[0] != "sin_bash" {
		t.Errorf("Search(\"shell command\") = %v, want sin_bash first", names)
	}
}

func TestSemanticIndex_SearchKLimit(t *testing.T) {
	idx := NewSemanticIndex(testSemanticSpecs())
	got := idx.Search("file", 1)
	if len(got) != 1 {
		t.Fatalf("Search(\"file\", 1) returned %d results, want 1", len(got))
	}
	gotAll := idx.Search("file", 100)
	if len(gotAll) < 2 {
		t.Errorf("Search(\"file\", 100) returned %d results, want at least 2", len(gotAll))
	}
}

func TestSemanticIndex_SearchNoMatch(t *testing.T) {
	idx := NewSemanticIndex(testSemanticSpecs())
	if got := idx.Search("xyz123abc", 5); got != nil {
		t.Errorf("Search(\"xyz123abc\") = %v, want nil", got)
	}
}

func TestSemanticIndex_ConcurrentSearchAndIndex(t *testing.T) {
	idx := NewSemanticIndex(testSemanticSpecs())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = idx.Search("file", 5)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		idx.Index(testSemanticSpecs())
	}()
	wg.Wait()
}

func TestSemanticIndex_CacheRoundTrip(t *testing.T) {
	specs := testSemanticSpecs()
	idx := NewSemanticIndex(specs)
	before := idx.Search("file", 10)

	dir := t.TempDir()
	path := filepath.Join(dir, "tool-embeddings.bin")
	if err := SaveCachedSemanticIndex(path, idx); err != nil {
		t.Fatalf("SaveCachedSemanticIndex: %v", err)
	}

	loaded, ok, err := LoadCachedSemanticIndex(path, specs)
	if err != nil {
		t.Fatalf("LoadCachedSemanticIndex: %v", err)
	}
	if !ok {
		t.Fatal("LoadCachedSemanticIndex returned ok=false for matching specs")
	}

	after := loaded.Search("file", 10)
	if !sameNames(before, after) {
		t.Errorf("cached results differ: before=%v after=%v", namesOf(before), namesOf(after))
	}
}

func TestSemanticIndex_CacheHashMismatch(t *testing.T) {
	specs := testSemanticSpecs()
	idx := NewSemanticIndex(specs)

	dir := t.TempDir()
	path := filepath.Join(dir, "tool-embeddings.bin")
	if err := SaveCachedSemanticIndex(path, idx); err != nil {
		t.Fatalf("SaveCachedSemanticIndex: %v", err)
	}

	other := append(testSemanticSpecs(), ToolSpec{Name: "sin_extra", Description: "Extra tool.", InputSchema: map[string]any{"type": "object"}})
	loaded, ok, err := LoadCachedSemanticIndex(path, other)
	if err != nil {
		t.Fatalf("LoadCachedSemanticIndex: %v", err)
	}
	if ok {
		t.Fatal("LoadCachedSemanticIndex returned ok=true for mismatched specs")
	}
	if loaded != nil {
		t.Errorf("LoadCachedSemanticIndex returned non-nil index on mismatch")
	}
}

func TestLazyToolLoader_UseSemantic(t *testing.T) {
	specs := testSemanticSpecs()
	loader := NewLazyToolLoader(specs)
	if loader.IsSemantic() {
		t.Error("new loader should not be semantic by default")
	}

	loader.UseSemantic(true, "")
	if !loader.IsSemantic() {
		t.Error("UseSemantic(true) should enable semantic mode")
	}

	got := loader.Search("read file", 1)
	if len(got) == 0 || got[0].Name != "sin_read" {
		t.Errorf("semantic Search(\"read file\") = %v, want sin_read first", namesOf(got))
	}

	loader.UseSemantic(false, "")
	if loader.IsSemantic() {
		t.Error("UseSemantic(false) should disable semantic mode")
	}
	got = loader.Search("sin_read", 1)
	if len(got) == 0 || got[0].Name != "sin_read" {
		t.Errorf("keyword Search(\"sin_read\") = %v, want sin_read first", namesOf(got))
	}
}

func TestLazyToolLoader_UseSemanticCache(t *testing.T) {
	specs := testSemanticSpecs()
	dir := t.TempDir()
	path := filepath.Join(dir, "tool-embeddings.bin")

	loader1 := NewLazyToolLoader(specs)
	loader1.UseSemantic(true, path)
	want := loader1.Search("file", 10)

	loader2 := NewLazyToolLoader(specs)
	loader2.UseSemantic(true, path)
	got := loader2.Search("file", 10)
	if !sameNames(want, got) {
		t.Errorf("cached semantic results differ: want=%v got=%v", namesOf(want), namesOf(got))
	}
}

func namesOf(specs []ToolSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Name
	}
	return out
}

func sameNames(a, b []ToolSpec) bool {
	if len(a) != len(b) {
		return false
	}
	x := namesOf(a)
	y := namesOf(b)
	sort.Strings(x)
	sort.Strings(y)
	return reflect.DeepEqual(x, y)
}
