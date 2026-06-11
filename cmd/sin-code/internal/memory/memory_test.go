// SPDX-License-Identifier: MIT
// Purpose: tests for the memory package: store CRUD, embedding, search,
// graph traversal, cosine similarity, prime context.
package memory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMemoryGenerateID(t *testing.T) {
	id := GenerateID("hello world")
	if !strings.HasPrefix(id, "mem-") {
		t.Errorf("expected mem- prefix, got %q", id)
	}
	if len(id) != len("mem-")+12 {
		t.Errorf("expected 12 hex chars, got %q", id)
	}
}

func TestMemoryNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{"  a ", "b", "a", "", "c", "a"})
	want := []string{"a", "b", "c"}
	if !equalStrSlices(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMemoryAddAndGet(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "test insight", Project: "p1", Tags: []string{"a", "b"}, Actor: "alice"}
	if err := s.Add(m); err != nil {
		t.Fatal(err)
	}
	if m.ID == "" {
		t.Error("ID should be set")
	}
	got, err := s.Get(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Insight != "test insight" {
		t.Errorf("roundtrip: %s", got.Insight)
	}
	if got.Project != "p1" {
		t.Errorf("project: %s", got.Project)
	}
	if !equalStrSlices(got.Tags, []string{"a", "b"}) {
		t.Errorf("tags: %v", got.Tags)
	}
	if got.Created.IsZero() || got.Updated.IsZero() {
		t.Error("timestamps should be set")
	}
}

func TestMemoryAddValidation(t *testing.T) {
	s := tempStore(t)
	if err := s.Add(nil); err == nil {
		t.Error("expected error for nil")
	}
	if err := s.Add(&Memory{}); err == nil {
		t.Error("expected error for empty insight")
	}
}

func TestMemoryAddDedupeTags(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "x", Tags: []string{"a", "a", "b", "  b  ", ""}}
	if err := s.Add(m); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(m.ID)
	if !equalStrSlices(got.Tags, []string{"a", "b"}) {
		t.Errorf("tags: %v", got.Tags)
	}
}

func TestMemoryListFilters(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "A", Project: "p1", Tags: []string{"x"}})
	_ = s.Add(&Memory{Insight: "B", Project: "p2", Tags: []string{"y"}})
	_ = s.Add(&Memory{Insight: "C", Project: "p1", Tags: []string{"x"}, Actor: "alice"})

	all, _ := s.List(ListFilter{})
	if len(all) != 3 {
		t.Errorf("all: got %d", len(all))
	}

	p1, _ := s.List(ListFilter{Project: "p1"})
	if len(p1) != 2 {
		t.Errorf("project: got %d", len(p1))
	}

	tagX, _ := s.List(ListFilter{Tag: "x"})
	if len(tagX) != 2 {
		t.Errorf("tag: got %d", len(tagX))
	}

	alice, _ := s.List(ListFilter{Actor: "alice"})
	if len(alice) != 1 {
		t.Errorf("actor: got %d", len(alice))
	}
}

func TestMemoryListTagsAll(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "AB", Tags: []string{"a", "b"}})
	_ = s.Add(&Memory{Insight: "A", Tags: []string{"a"}})
	_ = s.Add(&Memory{Insight: "B", Tags: []string{"b"}})
	got, _ := s.List(ListFilter{TagsAll: []string{"a", "b"}})
	if len(got) != 1 {
		t.Errorf("expected 1 with both tags, got %d", len(got))
	}
}

func TestMemoryListTagsAny(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "AB", Tags: []string{"a", "b"}})
	_ = s.Add(&Memory{Insight: "C", Tags: []string{"c"}})
	got, _ := s.List(ListFilter{TagsAny: []string{"a", "b"}})
	if len(got) != 1 {
		t.Errorf("expected 1, got %d", len(got))
	}
}

func TestMemoryListLimit(t *testing.T) {
	s := tempStore(t)
	for i := 0; i < 10; i++ {
		_ = s.Add(&Memory{Insight: fmt.Sprintf("mem %d", i)})
	}
	got, _ := s.List(ListFilter{Limit: 3})
	if len(got) != 3 {
		t.Errorf("limit: got %d", len(got))
	}
}

func TestMemoryDeleteSoft(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "soft delete me"}
	_ = s.Add(m)
	if err := s.Delete(m.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(m.ID)
	if !strings.HasPrefix(got.Insight, "[forgotten]") {
		t.Errorf("soft delete: %s", got.Insight)
	}
}

func TestMemoryDeleteHard(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "hard delete me"}
	_ = s.Add(m)
	if err := s.Delete(m.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryAddLink(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	_ = s.Add(a)
	_ = s.Add(b)
	if err := s.AddLink(Link{From: a.ID, To: b.ID, Rel: "extends"}); err != nil {
		t.Fatal(err)
	}
	links, _ := s.GetLinks(a.ID)
	if len(links) != 1 {
		t.Errorf("expected 1 link, got %d", len(links))
	}
}

func TestMemoryAddLinkValidation(t *testing.T) {
	s := tempStore(t)
	if err := s.AddLink(Link{From: "", To: "b", Rel: "extends"}); err == nil {
		t.Error("expected error for empty from")
	}
	if err := s.AddLink(Link{From: "a", To: "a", Rel: "extends"}); err == nil {
		t.Error("expected error for self-link")
	}
	if err := s.AddLink(Link{From: "a", To: "b", Rel: "invalid-type"}); err == nil {
		t.Error("expected error for invalid rel type")
	}
}

func TestMemoryLinkBidirectional(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	c := &Memory{Insight: "C"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.Add(c)
	_ = s.AddLink(Link{From: a.ID, To: b.ID, Rel: "references"})
	_ = s.AddLink(Link{From: c.ID, To: a.ID, Rel: "supports"})

	bLinks, _ := s.GetLinks(b.ID)
	if len(bLinks) != 1 {
		t.Errorf("B should see 1 link, got %d", len(bLinks))
	}

	aLinks, _ := s.GetLinks(a.ID)
	if len(aLinks) != 2 {
		t.Errorf("A should see 2 links (1 out + 1 in), got %d", len(aLinks))
	}

	cLinks, _ := s.GetLinks(c.ID)
	if len(cLinks) != 1 {
		t.Errorf("C should see 1 link, got %d", len(cLinks))
	}
}

func TestMemoryRemoveLink(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.AddLink(Link{From: a.ID, To: b.ID, Rel: "extends"})
	if err := s.RemoveLink(a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	links, _ := s.GetLinks(a.ID)
	if len(links) != 0 {
		t.Errorf("expected 0 after remove, got %d", len(links))
	}
}

func TestMemoryGraphBFS(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	c := &Memory{Insight: "C"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.Add(c)
	_ = s.AddLink(Link{From: a.ID, To: b.ID, Rel: "extends"})
	_ = s.AddLink(Link{From: b.ID, To: c.ID, Rel: "references"})

	tree, err := s.Graph(a.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 {
		t.Errorf("expected 3 nodes in graph, got %d", len(tree))
	}
}

func TestMemoryGraphDepthLimit(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	c := &Memory{Insight: "C"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.Add(c)
	_ = s.AddLink(Link{From: a.ID, To: b.ID, Rel: "extends"})
	_ = s.AddLink(Link{From: b.ID, To: c.ID, Rel: "references"})

	tree, _ := s.Graph(a.ID, 1)
	if len(tree) != 2 {
		t.Errorf("depth 1: expected 2 nodes, got %d", len(tree))
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if s := CosineSimilarity(a, b); math.Abs(s-1.0) > 0.001 {
		t.Errorf("identical: got %f", s)
	}
	c := []float32{0, 1, 0}
	if s := CosineSimilarity(a, c); math.Abs(s) > 0.001 {
		t.Errorf("orthogonal: got %f", s)
	}
	d := []float32{-1, 0, 0}
	if s := CosineSimilarity(a, d); math.Abs(s+1.0) > 0.001 {
		t.Errorf("opposite: got %f", s)
	}
	if s := CosineSimilarity([]float32{1}, []float32{1, 2}); s != 0 {
		t.Errorf("different lengths: got %f", s)
	}
	if s := CosineSimilarity([]float32{}, []float32{}); s != 0 {
		t.Errorf("empty: got %f", s)
	}
}

func TestMemoryStats(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "a"})
	_ = s.Add(&Memory{Insight: "b"})
	_ = s.Add(&Memory{Insight: "c"})
	stats, _ := s.Stats()
	if stats["total"] != 3 {
		t.Errorf("total: %d", stats["total"])
	}
	if stats["links"] != 0 {
		t.Errorf("links: %d", stats["links"])
	}
}

func TestMemoryEmbeddingStatusDefault(t *testing.T) {
	s := tempStore(t)
	enabled, dim := s.EmbeddingStatus()
	if enabled {
		t.Error("expected disabled by default in test env")
	}
	_ = dim
}

func TestMemoryPrimeEmpty(t *testing.T) {
	s := tempStore(t)
	text, err := s.Prime("anything", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		t.Errorf("expected empty prime, got %q", text)
	}
}

func TestMemorySearchFallback(t *testing.T) {
	old, _ := GetEmbedder()
	defer SetEmbedder(old, 0)

	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "the quick brown fox", Tags: []string{"animal"}})
	_ = s.Add(&Memory{Insight: "hello world", Tags: []string{"greeting"}})
	_ = s.Add(&Memory{Insight: "completely unrelated"})

	results, err := s.Search("fox", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected fallback results")
	}
	if results[0].Insight != "the quick brown fox" {
		t.Errorf("expected 'fox' result first, got %q", results[0].Insight)
	}
}

func TestMemorySearchWithEmbeddings(t *testing.T) {
	old, _ := GetEmbedder()
	defer SetEmbedder(old, 0)

	SetEmbedder(func(text string) ([]float32, error) {
		v := make([]float32, 4)
		switch {
		case strings.Contains(text, "apple"):
			v[0] = 1.0
		case strings.Contains(text, "orange"):
			v[0] = 0.9
			v[1] = 0.1
		case strings.Contains(text, "banana"):
			v[2] = 1.0
		}
		return v, nil
	}, 4)

	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "apple is red and sweet"})
	_ = s.Add(&Memory{Insight: "orange is citrus"})
	_ = s.Add(&Memory{Insight: "banana is yellow"})

	results, err := s.Search("apple", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].Insight != "apple is red and sweet" {
		t.Errorf("expected apple first, got %q (score %f)", results[0].Insight, results[0].Score)
	}
	if results[0].Score < 0.9 {
		t.Errorf("apple score too low: %f", results[0].Score)
	}
}

func TestMemoryPrimeWithResults(t *testing.T) {
	old, _ := GetEmbedder()
	defer SetEmbedder(old, 0)

	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "always use go modules", Project: "go", Tags: []string{"go"}})
	_ = s.Add(&Memory{Insight: "use gorm for db", Project: "go", Tags: []string{"go"}})

	text, err := s.Prime("go best practices", "go", 5)
	if err != nil {
		t.Fatal(err)
	}
	if text == "" {
		t.Error("expected prime text")
	}
	if !strings.Contains(text, "go modules") && !strings.Contains(text, "gorm") {
		t.Errorf("prime missing content: %s", text)
	}
}

// A path whose parent "directory" is actually a file fails with ENOTDIR on
// every platform regardless of user privileges (unlike absolute paths under
// "/", which are creatable in containers/CI running with a writable root).
func TestMemoryOpenInvalidDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(blocker, "sub", "memory.db")); err == nil {
		t.Error("expected error for invalid dir")
	}
}

func TestMemoryOpenEmptyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := os.Stat(s.Path()); err != nil {
		t.Errorf("file should exist: %v", err)
	}
}

func TestNIMEmbedderSetup(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "")
	SetupNIMEmbedder()
	enabled, _ := GetEmbedder()
	if enabled != nil {
		t.Error("expected nil embedder without API key")
	}

	t.Setenv("SIN_NIM_API_KEY", "test")
	SetupNIMEmbedder()
	enabled2, _ := GetEmbedder()
	if enabled2 == nil {
		t.Error("expected non-nil embedder with API key")
	}
}

func TestNIMEmbedderMissingKey(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "")
	e := NewNIMEmbedder()
	if _, err := e.EmbedOne(context.Background(), "hello"); err == nil {
		t.Error("expected error without key")
	}
}

func TestNIMEmbedderMockServer(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintln(w, `{"data":[{"embedding":[0.1,0.2,0.3],"index":0}],"usage":{"prompt_tokens":1,"total_tokens":2}}`)
	}))
	defer srv.Close()

	e := &Embedder{BaseURL: srv.URL, Model: "test", APIKey: "k", HTTP: srv.Client()}
	vec, err := e.EmbedOne(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 {
		t.Errorf("expected 3-dim vector, got %d", len(vec))
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 server call, got %d", calls.Load())
	}
}

func TestNIMEmbedderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprintln(w, "unauthorized")
	}))
	defer srv.Close()

	e := &Embedder{BaseURL: srv.URL, Model: "test", APIKey: "k", HTTP: srv.Client()}
	_, err := e.EmbedOne(context.Background(), "hello")
	if err == nil {
		t.Error("expected error for 401")
	}
}

func TestMemoryEncodeDecodeEmbedding(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	v := make([]float32, 8)
	for i := range v {
		v[i] = r.Float32()
	}
	encoded := encodeEmbedding(v)
	decoded := decodeEmbedding(encoded)
	if len(decoded) != len(v) {
		t.Fatalf("length mismatch: %d vs %d", len(decoded), len(v))
	}
	for i := range v {
		if math.Abs(float64(v[i]-decoded[i])) > 0.0001 {
			t.Errorf("idx %d: %f != %f", i, v[i], decoded[i])
		}
	}
}

func TestTextHash(t *testing.T) {
	if textHash("hello") != textHash("hello") {
		t.Error("deterministic hash")
	}
	if textHash("hello") == textHash("world") {
		t.Error("different inputs should produce different hashes")
	}
}

func TestMemorySearchLimit(t *testing.T) {
	old, _ := GetEmbedder()
	defer SetEmbedder(old, 0)

	s := tempStore(t)
	for i := 0; i < 10; i++ {
		_ = s.Add(&Memory{Insight: fmt.Sprintf("item %d", i)})
	}
	results, err := s.Search("item", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) > 3 {
		t.Errorf("limit not respected: got %d", len(results))
	}
}

func TestMemoryLinkValidTypes(t *testing.T) {
	for _, lt := range []LinkType{LinkReferences, LinkSupports, LinkContradicts, LinkExtends, LinkCauses} {
		if !lt.Valid() {
			t.Errorf("expected %q valid", lt)
		}
	}
	if LinkType("nope").Valid() {
		t.Error("expected 'nope' invalid")
	}
}

func TestPrimeWithProjectFilter(t *testing.T) {
	old, _ := GetEmbedder()
	defer SetEmbedder(old, 0)

	SetEmbedder(func(text string) ([]float32, error) {
		v := make([]float32, 2)
		if strings.Contains(text, "cobra") || strings.Contains(text, "CLI") {
			v[0] = 1.0
		}
		if strings.Contains(text, "gin") {
			v[1] = 1.0
		}
		return v, nil
	}, 2)

	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "use cobra for CLI", Project: "sin-code"})
	_ = s.Add(&Memory{Insight: "use gin for HTTP", Project: "other"})

	text, _ := s.Prime("cli framework", "sin-code", 5)
	if !strings.Contains(text, "cobra") {
		t.Errorf("project filter failed: %s", text)
	}
	if strings.Contains(text, "gin") {
		t.Errorf("project filter let through: %s", text)
	}
}

func TestScoredMemorySort(t *testing.T) {
	results := []ScoredMemory{
		{Memory: &Memory{ID: "a"}, Score: 0.5},
		{Memory: &Memory{ID: "b"}, Score: 0.9},
		{Memory: &Memory{ID: "c"}, Score: 0.1},
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if results[0].ID != "b" || results[1].ID != "a" || results[2].ID != "c" {
		t.Error("sort failed")
	}
}

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
