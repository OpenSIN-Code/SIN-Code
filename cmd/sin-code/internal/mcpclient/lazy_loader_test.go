// SPDX-License-Identifier: MIT
// Purpose: tests for LazyToolLoader (issue #270 — lazy tool loading).
// All tests must pass under `go test -race -count=1` (mandate M7).
package mcpclient

import (
	"sync"
	"testing"
)

func sampleSpecs() []ToolSpec {
	return []ToolSpec{
		{Name: "sin_read", Description: "Read a file from disk."},
		{Name: "sin_write", Description: "Write content to a file atomically."},
		{Name: "sin_edit", Description: "Replace text in a file."},
		{Name: "sin_bash", Description: "Execute a shell command."},
		{Name: "sin_search", Description: "Search files for a substring."},
		{Name: "websearch__search", Description: "Search the web using SerpAPI."},
		{Name: "scheduler__schedule_job", Description: "Schedule a cron or interval job."},
		{Name: "grillme__start", Description: "Start a grilling session for design review."},
		{Name: "sckg__build", Description: "Build a semantic codebase knowledge graph."},
		{Name: "poc__verify", Description: "Proof of correctness verification."},
	}
}

func TestSearchByName(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	results := l.Search("sin_read", 5)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "sin_read" {
		t.Fatalf("expected sin_read, got %s", results[0].Name)
	}
}

func TestSearchByDescription(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	results := l.Search("shell", 5)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'shell'")
	}
	found := false
	for _, r := range results {
		if r.Name == "sin_bash" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected sin_bash in results for 'shell'")
	}
}

func TestSearchTopKLimiting(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	all := l.Search("sin", 100)
	if len(all) <= 1 {
		t.Fatalf("expected multiple results for 'sin', got %d", len(all))
	}
	limited := l.Search("sin", 2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 results with k=2, got %d", len(limited))
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	if results := l.Search("", 10); results != nil {
		t.Fatalf("expected nil for empty query, got %d results", len(results))
	}
	if results := l.Search("   ", 10); results != nil {
		t.Fatalf("expected nil for whitespace-only query, got %d results", len(results))
	}
}

func TestSearchNoMatches(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	results := l.Search("nonexistent_xyz", 10)
	if results != nil {
		t.Fatalf("expected nil for no matches, got %d results", len(results))
	}
}

func TestAllReturnsAllSpecs(t *testing.T) {
	specs := sampleSpecs()
	l := NewLazyToolLoader(specs)
	all := l.All()
	if len(all) != len(specs) {
		t.Fatalf("expected %d specs, got %d", len(specs), len(all))
	}
	for i, s := range specs {
		if all[i].Name != s.Name {
			t.Errorf("index %d: expected %s, got %s", i, s.Name, all[i].Name)
		}
	}
}

func TestAllReturnsDefensiveCopy(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	all := l.All()
	if len(all) == 0 {
		t.Fatal("expected non-empty All()")
	}
	all[0].Name = "mutated"
	again := l.All()
	if again[0].Name == "mutated" {
		t.Fatal("All() must return a defensive copy")
	}
}

func TestCount(t *testing.T) {
	specs := sampleSpecs()
	l := NewLazyToolLoader(specs)
	if l.Count() != len(specs) {
		t.Fatalf("expected count %d, got %d", len(specs), l.Count())
	}
}

func TestConcurrentAccessRaceFree(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = l.Search("sin", 5)
		}()
		go func() {
			defer wg.Done()
			_ = l.All()
		}()
		go func() {
			defer wg.Done()
			_ = l.Count()
		}()
	}
	wg.Wait()
}

func TestSearchOrderingByRelevance(t *testing.T) {
	specs := []ToolSpec{
		{Name: "foo", Description: "search for things"},
		{Name: "search_tool", Description: "generic utility"},
		{Name: "sin_search", Description: "search files"},
	}
	l := NewLazyToolLoader(specs)
	results := l.Search("search", 3)
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].Name != "sin_search" {
		t.Fatalf("expected sin_search first (exact name match scores highest), got %s", results[0].Name)
	}
}

func TestSearchMultiWordQuery(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	results := l.Search("search files", 5)
	if len(results) == 0 {
		t.Fatal("expected results for multi-word query")
	}
	found := false
	for _, r := range results {
		if r.Name == "sin_search" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected sin_search in multi-word results")
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	upper := l.Search("SIN_READ", 5)
	lower := l.Search("sin_read", 5)
	if len(upper) != len(lower) {
		t.Fatalf("case-insensitive: expected same count, got upper=%d lower=%d", len(upper), len(lower))
	}
	if len(upper) == 0 {
		t.Fatal("expected results for uppercase query")
	}
	if upper[0].Name != lower[0].Name {
		t.Fatalf("case-insensitive: expected same first result, got upper=%s lower=%s", upper[0].Name, lower[0].Name)
	}
}

func TestSearchPartialMatch(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	results := l.Search("sched", 5)
	if len(results) == 0 {
		t.Fatal("expected results for partial match 'sched'")
	}
	found := false
	for _, r := range results {
		if r.Name == "scheduler__schedule_job" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected scheduler__schedule_job in partial match results")
	}
}

func TestSearchKZeroReturnsNil(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	if results := l.Search("sin", 0); results != nil {
		t.Fatalf("expected nil for k=0, got %d results", len(results))
	}
}

func TestSearchKNegativeReturnsNil(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	if results := l.Search("sin", -1); results != nil {
		t.Fatalf("expected nil for k=-1, got %d results", len(results))
	}
}

func TestSearchKExceedsResults(t *testing.T) {
	l := NewLazyToolLoader(sampleSpecs())
	results := l.Search("grill", 100)
	if len(results) != 1 {
		t.Fatalf("expected 1 result even with large k, got %d", len(results))
	}
}

func TestNewLazyToolLoaderDoesNotMutateInput(t *testing.T) {
	specs := sampleSpecs()
	original := make([]ToolSpec, len(specs))
	copy(original, specs)
	_ = NewLazyToolLoader(specs)
	for i, s := range specs {
		if s.Name != original[i].Name {
			t.Fatalf("input slice was mutated at index %d: expected %s, got %s", i, original[i].Name, s.Name)
		}
	}
}

func TestToolSearchSpec(t *testing.T) {
	spec := ToolSearchSpec()
	if spec.Name != "tool_search" {
		t.Fatalf("expected name tool_search, got %s", spec.Name)
	}
	if spec.Description == "" {
		t.Fatal("expected non-empty description")
	}
	if spec.InputSchema == nil {
		t.Fatal("expected non-nil InputSchema")
	}
	props, ok := spec.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map in InputSchema")
	}
	if _, ok := props["query"]; !ok {
		t.Fatal("expected 'query' property in tool_search schema")
	}
	if _, ok := props["limit"]; !ok {
		t.Fatal("expected 'limit' property in tool_search schema")
	}
	required, ok := spec.InputSchema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "query" {
		t.Fatalf("expected required=[query], got %v", spec.InputSchema["required"])
	}
}

func TestSearchEmptyLoader(t *testing.T) {
	l := NewLazyToolLoader(nil)
	if l.Count() != 0 {
		t.Fatalf("expected count 0, got %d", l.Count())
	}
	if results := l.Search("anything", 10); results != nil {
		t.Fatalf("expected nil from empty loader, got %d results", len(results))
	}
	if all := l.All(); len(all) != 0 {
		t.Fatalf("expected empty All(), got %d", len(all))
	}
}
