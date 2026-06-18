// SPDX-License-Identifier: MIT
// Purpose: Coverage gap tests for the memory package.
// Targets uncovered error paths, edge cases, and helper functions.
// All tests pass under `go test -race -count=1` (mandate M7).
package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// auto_observe.go coverage — ObservableTools, ReadOnlyTools (0%)
// ---------------------------------------------------------------------------

func TestObservableTools(t *testing.T) {
	tools := ObservableTools()
	if len(tools) == 0 {
		t.Fatal("expected non-empty observable tools list")
	}
	expected := map[string]bool{
		"edit": true, "write": true, "execute": true, "test": true,
		"sin_edit": true, "sin_write": true, "sin_execute": true, "sin_test": true,
	}
	for _, tool := range tools {
		if !expected[tool] {
			t.Errorf("unexpected observable tool: %q", tool)
		}
	}
}

func TestReadOnlyTools(t *testing.T) {
	tools := ReadOnlyTools()
	if len(tools) == 0 {
		t.Fatal("expected non-empty read-only tools list")
	}
	expected := map[string]bool{
		"discover": true, "scout": true, "map": true, "read": true,
		"sin_discover": true, "sin_scout": true, "sin_map": true, "sin_read": true,
	}
	for _, tool := range tools {
		if !expected[tool] {
			t.Errorf("unexpected read-only tool: %q", tool)
		}
	}
}

// ---------------------------------------------------------------------------
// embedding_cache.go coverage — MaxEntries, TTL, nil safety (0%)
// ---------------------------------------------------------------------------

func TestEmbeddingCache_MaxEntries(t *testing.T) {
	c := NewEmbeddingCache(42, time.Hour)
	if got := c.MaxEntries(); got != 42 {
		t.Errorf("MaxEntries: got %d, want 42", got)
	}
}

func TestEmbeddingCache_TTL(t *testing.T) {
	c := NewEmbeddingCache(10, 30*time.Minute)
	if got := c.TTL(); got != 30*time.Minute {
		t.Errorf("TTL: got %v, want 30m", got)
	}
}

func TestEmbeddingCache_MaxEntries_Nil(t *testing.T) {
	var c *EmbeddingCache
	if got := c.MaxEntries(); got != 0 {
		t.Errorf("nil MaxEntries: got %d, want 0", got)
	}
}

func TestEmbeddingCache_TTL_Nil(t *testing.T) {
	var c *EmbeddingCache
	if got := c.TTL(); got != 0 {
		t.Errorf("nil TTL: got %v, want 0", got)
	}
}

func TestEmbeddingCache_NewEmbeddingCache_Defaults(t *testing.T) {
	c := NewEmbeddingCache(0, time.Hour)
	if got := c.MaxEntries(); got != 10000 {
		t.Errorf("default maxEntries: got %d, want 10000", got)
	}
	c2 := NewEmbeddingCache(10, 0)
	if got := c2.TTL(); got != time.Hour {
		t.Errorf("default TTL: got %v, want 1h", got)
	}
}

func TestEmbeddingCache_Stats_AfterEviction(t *testing.T) {
	c := NewEmbeddingCache(2, time.Hour)
	c.Set("a", []float32{1})
	c.Set("b", []float32{2})
	c.Set("c", []float32{3})
	s := c.Stats()
	if s.Size != 2 {
		t.Errorf("size after eviction: got %d, want 2", s.Size)
	}
	if s.Evictions != 1 {
		t.Errorf("evictions: got %d, want 1", s.Evictions)
	}
}

func TestEmbeddingCache_Clear(t *testing.T) {
	c := NewEmbeddingCache(10, time.Hour)
	c.Set("a", []float32{1})
	c.Set("b", []float32{2})
	c.Clear()
	if _, ok := c.Get("a"); ok {
		t.Error("expected miss after Clear")
	}
	s := c.Stats()
	if s.Size != 0 {
		t.Errorf("size after Clear: got %d, want 0", s.Size)
	}
}

func TestEmbeddingCache_PurgeExpired(t *testing.T) {
	c := NewEmbeddingCache(10, 10*time.Millisecond)
	c.Set("a", []float32{1})
	time.Sleep(20 * time.Millisecond)
	removed := c.PurgeExpired()
	if removed != 1 {
		t.Errorf("expected 1 expired entry purged, got %d", removed)
	}
}

// ---------------------------------------------------------------------------
// autodream.go coverage — WithLLMClient (0%)
// ---------------------------------------------------------------------------

func TestWithLLMClient(t *testing.T) {
	store := tempStore(t)
	ad := NewAutoDream(store, WithLLMClient(nil))
	if ad == nil {
		t.Fatal("expected non-nil AutoDream")
	}
}

// ---------------------------------------------------------------------------
// context_guard.go coverage — String, ratio (83.3%, 80%)
// ---------------------------------------------------------------------------

func TestGuardLevel_String_AllLevels(t *testing.T) {
	cases := []struct {
		level GuardLevel
		want  string
	}{
		{GuardGreen, "green"},
		{GuardYellow, "yellow"},
		{GuardOrange, "orange"},
		{GuardRed, "red"},
		{GuardLevel(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.level.String(); got != c.want {
			t.Errorf("GuardLevel(%d).String() = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestContextGuard_Ratio_ZeroMax(t *testing.T) {
	g := &ContextGuard{maxTokens: 0}
	if got := g.ratio(); got != 1.0 {
		t.Errorf("ratio with maxTokens=0: got %.2f, want 1.0", got)
	}
}

// ---------------------------------------------------------------------------
// governance.go coverage — itoa (85.7%)
// ---------------------------------------------------------------------------

func TestGovernanceItoa(t *testing.T) {
	cases := map[int]string{
		0:   "0",
		1:   "1",
		9:   "9",
		10:  "10",
		99:  "99",
		100: "100",
		42:  "42",
	}
	for n, want := range cases {
		if got := itoa(n); got != want {
			t.Errorf("itoa(%d) = %q, want %q", n, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// unified_query.go coverage — pure functions (40-75%)
// ---------------------------------------------------------------------------

func TestImportanceScore(t *testing.T) {
	cases := []struct {
		imp  float64
		want float64
	}{
		{0, 0},
		{-1, 0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
		{0.3, 0.3},
	}
	for _, c := range cases {
		if got := importanceScore(c.imp); got != c.want {
			t.Errorf("importanceScore(%.2f) = %.2f, want %.2f", c.imp, got, c.want)
		}
	}
}

func TestClamp01(t *testing.T) {
	cases := []struct {
		x    float64
		want float64
	}{
		{-1, 0},
		{0, 0},
		{0.5, 0.5},
		{1, 1},
		{1.5, 1},
		{-0.1, 0},
	}
	for _, c := range cases {
		if got := clamp01(c.x); got != c.want {
			t.Errorf("clamp01(%.2f) = %.2f, want %.2f", c.x, got, c.want)
		}
	}
}

func TestParseTime(t *testing.T) {
	valid := "2026-01-15T10:30:00Z"
	t1 := parseTime(valid)
	if t1.IsZero() {
		t.Error("expected non-zero time for valid RFC3339")
	}
	t2 := parseTime("not-a-date")
	if !t2.IsZero() {
		t.Error("expected zero time for invalid input")
	}
	t3 := parseTime("")
	if !t3.IsZero() {
		t.Error("expected zero time for empty string")
	}
}

func TestRecencyScore(t *testing.T) {
	if got := recencyScore(time.Time{}); got != 0 {
		t.Errorf("recencyScore(zero) = %.2f, want 0", got)
	}
	now := recencyScore(time.Now())
	if now < 0.99 || now > 1.0 {
		t.Errorf("recencyScore(now) = %.4f, want ~1.0", now)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if got := recencyScore(old); got <= 0 || got >= 1 {
		t.Errorf("recencyScore(30d ago) = %.4f, want 0 < x < 1", got)
	}
}

func TestSubstringScore(t *testing.T) {
	if got := substringScore("", "haystack"); got != 0.5 {
		t.Errorf("substringScore('', ...) = %.2f, want 0.5", got)
	}
	if got := substringScore("hello", "hello"); got != 1.0 {
		t.Errorf("substringScore(exact) = %.2f, want 1.0", got)
	}
	if got := substringScore("xyz", "hello"); got != 0 {
		t.Errorf("substringScore(no match) = %.2f, want 0", got)
	}
	// "ell" is a substring of "hello" so returns 1.0; use word-level partial match instead
	score := substringScore("hello world", "hello there")
	if score <= 0 || score >= 1 {
		t.Errorf("substringScore(partial words) = %.2f, want 0 < x < 1", score)
	}
}

func TestDedupeByContent_SameContentHigherScore(t *testing.T) {
	in := []UnifiedResult{
		{Content: "same text", Score: 0.5},
		{Content: "same text", Score: 0.9},
	}
	out := dedupeByContent(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 after dedup, got %d", len(out))
	}
	if out[0].Score != 0.9 {
		t.Errorf("expected higher score 0.9, got %.2f", out[0].Score)
	}
}

func TestDedupeByContent_DifferentContent(t *testing.T) {
	in := []UnifiedResult{
		{Content: "text A", Score: 0.5},
		{Content: "text B", Score: 0.9},
	}
	out := dedupeByContent(in)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

func TestUnifiedStore_HasStore_Nil(t *testing.T) {
	var u *UnifiedStore
	if u.HasStore(StoreMemory) {
		t.Error("nil HasStore should be false")
	}
}

func TestNewUnifiedStore_AllNil(t *testing.T) {
	u := NewUnifiedStore(nil, nil, nil, nil, nil)
	if u == nil {
		t.Fatal("expected non-nil UnifiedStore")
	}
	for _, label := range []StoreLabel{StoreMemory, StoreLessons, StoreSessions, StoreLedger, StoreEpisodes} {
		if u.HasStore(label) {
			t.Errorf("HasStore(%v) should be false with all nil stores", label)
		}
	}
}

// ---------------------------------------------------------------------------
// vector_index.go coverage — assign, removeIDFromClustersLocked, nil safety
// ---------------------------------------------------------------------------

func TestVectorIndex_Assign(t *testing.T) {
	vi := NewVectorIndex(3, 2)
	vi.Add("a", []float32{1, 0, 0})
	vi.Add("b", []float32{0, 1, 0})
	vi.Build()
	c := vi.assign([]float32{1, 0, 0})
	if c < 0 {
		t.Errorf("assign returned negative cluster: %d", c)
	}
}

func TestVectorIndex_Size_Nil(t *testing.T) {
	var vi *VectorIndex
	if got := vi.Size(); got != 0 {
		t.Errorf("nil Size: got %d, want 0", got)
	}
}

func TestVectorIndex_Build_Nil(t *testing.T) {
	var vi *VectorIndex
	vi.Build()
}

func TestVectorIndex_Add_Nil(t *testing.T) {
	var vi *VectorIndex
	vi.Add("x", []float32{1, 2, 3})
}

func TestVectorIndex_Remove_Nil(t *testing.T) {
	var vi *VectorIndex
	if vi.Remove("x") {
		t.Error("nil Remove should return false")
	}
}

func TestVectorIndex_RemoveIDFromClustersLocked_MultipleClusters(t *testing.T) {
	vi := NewVectorIndex(2, 4)
	vi.Add("a", []float32{1, 0})
	vi.Add("b", []float32{0, 1})
	vi.Add("c", []float32{1, 0})
	vi.Build()
	vi.mu.Lock()
	vi.removeIDFromClustersLocked("a")
	vi.mu.Unlock()
	totalEntries := 0
	vi.mu.RLock()
	for _, list := range vi.entries {
		for _, e := range list {
			if e.id == "a" {
				t.Error("a should have been removed from clusters")
			}
			totalEntries++
		}
	}
	vi.mu.RUnlock()
	if totalEntries < 2 {
		t.Errorf("expected at least 2 entries remaining, got %d", totalEntries)
	}
}

func TestVectorIndex_Add_AfterBuild(t *testing.T) {
	vi := NewVectorIndex(2, 2)
	vi.Add("a", []float32{1, 0})
	vi.Build()
	vi.Add("b", []float32{0, 1})
	if vi.Size() != 2 {
		t.Errorf("size after post-build add: got %d, want 2", vi.Size())
	}
}

func TestVectorIndex_Build_Empty(t *testing.T) {
	vi := NewVectorIndex(2, 4)
	vi.Build()
	if vi.Size() != 0 {
		t.Errorf("size after empty build: got %d, want 0", vi.Size())
	}
}

func TestMinLen(t *testing.T) {
	if got := minLen([]float32{1, 2, 3}, []float32{4, 5}); got != 2 {
		t.Errorf("minLen(3, 2) = %d, want 2", got)
	}
	if got := minLen([]float32{1}, []float32{2, 3, 4}); got != 1 {
		t.Errorf("minLen(1, 3) = %d, want 1", got)
	}
	if got := minLen(nil, []float32{1, 2}); got != 0 {
		t.Errorf("minLen(nil, 2) = %d, want 0", got)
	}
}

func TestDotProduct(t *testing.T) {
	got := DotProduct([]float32{1, 2, 3}, []float32{4, 5, 6})
	want := float32(1*4 + 2*5 + 3*6)
	if got != want {
		t.Errorf("DotProduct = %.2f, want %.2f", got, want)
	}
}

func TestDotProduct_LengthMismatch(t *testing.T) {
	got := DotProduct([]float32{1, 2, 3}, []float32{4, 5})
	want := float32(1*4 + 2*5)
	if got != want {
		t.Errorf("DotProduct mismatched = %.2f, want %.2f", got, want)
	}
}

func TestVectorIndex_Search_NotBuilt(t *testing.T) {
	vi := NewVectorIndex(2, 2)
	vi.Add("a", []float32{1, 0})
	results := vi.Search([]float32{1, 0}, 1)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result even without Build")
	}
	if results[0].ID != "a" {
		t.Errorf("expected 'a', got %q", results[0].ID)
	}
}

// ---------------------------------------------------------------------------
// versioning.go coverage — lineDiff, Diff error paths
// ---------------------------------------------------------------------------

func TestLineDiff(t *testing.T) {
	diff := lineDiff("line1\nline2\nline3", "line1\nmodified\nline3")
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
}

func TestLineDiff_Identical(t *testing.T) {
	diff := lineDiff("same\ncontent", "same\ncontent")
	// Identical content produces only common lines (prefixed with "  "), no - or + lines
	if strings.Contains(diff, "- ") || strings.Contains(diff, "+ ") {
		t.Errorf("expected no diff lines for identical content, got %q", diff)
	}
}

func TestLineDiff_EmptyOld(t *testing.T) {
	diff := lineDiff("", "new\ncontent")
	if diff == "" {
		t.Fatal("expected non-empty diff when old is empty")
	}
}

func TestLineDiff_EmptyNew(t *testing.T) {
	diff := lineDiff("old\ncontent", "")
	if diff == "" {
		t.Fatal("expected non-empty diff when new is empty")
	}
}

func TestVersioningStore_Diff_VersionNotFound(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()
	_ = vs.SaveVersion(ctx, "mem1", "content v1", "content v1 edited", "first edit")
	_, err := vs.Diff(ctx, "mem1", 1, 99)
	if err == nil {
		t.Fatal("expected error for non-existent version")
	}
}

func TestVersioningStore_Diff_MemoryNotFound(t *testing.T) {
	vs := newVersioningStore(t)
	ctx := context.Background()
	_, err := vs.Diff(ctx, "nonexistent", 1, 2)
	if err == nil {
		t.Fatal("expected error for non-existent memory")
	}
}

// ---------------------------------------------------------------------------
// instinct.go coverage — error paths
// ---------------------------------------------------------------------------

func TestInstinctStore_Demote_NotFound(t *testing.T) {
	is := instinctStore(t)
	err := is.Demote(context.Background(), "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for demoting non-existent instinct")
	}
}

func TestInstinctStore_Promote_NotFound(t *testing.T) {
	is := instinctStore(t)
	err := is.Promote(context.Background(), "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for promoting non-existent instinct")
	}
}

func TestInstinctStore_Get_NotFound(t *testing.T) {
	is := instinctStore(t)
	_, err := is.Get(context.Background(), "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for getting non-existent instinct")
	}
}

func TestInstinctStore_List_Empty(t *testing.T) {
	is := instinctStore(t)
	list, err := is.List(context.Background(), "global")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// evidence_graph.go coverage — edge cases
// ---------------------------------------------------------------------------

func TestEvidenceGraph_GetNode_NotFound(t *testing.T) {
	g := NewEvidenceGraph()
	_, ok := g.GetNode("nonexistent")
	if ok {
		t.Error("expected false for non-existent node")
	}
}

func TestEvidenceGraph_NodeCount_Empty(t *testing.T) {
	g := NewEvidenceGraph()
	if g.NodeCount() != 0 {
		t.Errorf("expected 0 nodes, got %d", g.NodeCount())
	}
}

func TestEvidenceGraph_LinkCount_Empty(t *testing.T) {
	g := NewEvidenceGraph()
	if g.LinkCount() != 0 {
		t.Errorf("expected 0 links, got %d", g.LinkCount())
	}
}

func TestEvidenceGraph_RenderDOT_Empty(t *testing.T) {
	g := NewEvidenceGraph()
	dot := g.RenderDOT()
	if dot == "" {
		t.Fatal("expected non-empty DOT for empty graph")
	}
}

// ---------------------------------------------------------------------------
// honcho_native.go coverage — nil and error paths
// ---------------------------------------------------------------------------

func TestHonchoIntegration_NilSafe(t *testing.T) {
	var h *HonchoIntegration
	prefs, err := h.GetUserPreferences(context.Background())
	if err != nil || prefs != nil {
		t.Errorf("nil GetUserPreferences: prefs=%v err=%v", prefs, err)
	}
	model, err := h.GetPeerModel(context.Background(), "user")
	if err != nil || model != nil {
		t.Errorf("nil GetPeerModel: model=%v err=%v", model, err)
	}
	if err := h.SavePreference(context.Background(), "key", "value"); err != nil {
		t.Errorf("nil SavePreference: err=%v", err)
	}
	if err := h.SavePeerModel(context.Background(), "user", nil); err != nil {
		t.Errorf("nil SavePeerModel: err=%v", err)
	}
}

func TestHonchoIntegration_NilStore(t *testing.T) {
	h := NewHonchoIntegration(nil)
	prefs, err := h.GetUserPreferences(context.Background())
	if err != nil || prefs != nil {
		t.Errorf("nil store GetUserPreferences: prefs=%v err=%v", prefs, err)
	}
	model, err := h.GetPeerModel(context.Background(), "user")
	if err != nil || model != nil {
		t.Errorf("nil store GetPeerModel: model=%v err=%v", model, err)
	}
	if err := h.SavePreference(context.Background(), "key", "value"); err != nil {
		t.Errorf("nil store SavePreference: err=%v", err)
	}
}

func TestHonchoIntegration_SaveAndGetPreference(t *testing.T) {
	store := tempStore(t)
	h := NewHonchoIntegration(store)
	ctx := context.Background()
	if err := h.SavePreference(ctx, "prefers-terse", "true"); err != nil {
		t.Fatalf("SavePreference: %v", err)
	}
	prefs, err := h.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	if len(prefs) != 1 {
		t.Fatalf("expected 1 preference, got %d", len(prefs))
	}
}

func TestHonchoIntegration_SaveAndGetPeerModel(t *testing.T) {
	store := tempStore(t)
	h := NewHonchoIntegration(store)
	ctx := context.Background()
	model := map[string]any{"name": "test-model", "score": 0.85}
	if err := h.SavePeerModel(ctx, "user-1", model); err != nil {
		t.Fatalf("SavePeerModel: %v", err)
	}
	got, err := h.GetPeerModel(ctx, "user-1")
	if err != nil {
		t.Fatalf("GetPeerModel: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil peer model")
	}
	if got["name"] != "test-model" {
		t.Errorf("expected name 'test-model', got %v", got["name"])
	}
}

func TestHonchoIntegration_GetPeerModel_NotFound(t *testing.T) {
	store := tempStore(t)
	h := NewHonchoIntegration(store)
	got, err := h.GetPeerModel(context.Background(), "nonexistent-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent peer model, got %v", got)
	}
}

func TestHonchoIntegration_SavePeerModel_InvalidJSON(t *testing.T) {
	store := tempStore(t)
	h := NewHonchoIntegration(store)
	ctx := context.Background()
	// Save a peer model with a non-JSON insight (simulates corruption)
	m := &Memory{
		Insight: "not valid json",
		Tags:    []string{tagPeerModel},
		Actor:   "bad-user",
	}
	if err := store.Add(m); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, err := h.GetPeerModel(ctx, "bad-user")
	if err == nil {
		t.Fatal("expected error for invalid JSON peer model")
	}
}

// ---------------------------------------------------------------------------
// import_export.go coverage — edge cases
// ---------------------------------------------------------------------------

func TestExportToInstinct_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instincts.json")
	if err := ExportToInstinct(nil, path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExportToInstinct_WithInstincts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instincts.json")
	instincts := []Instinct{
		{Content: "test instinct", Confidence: 0.9, Scope: "global", Source: "test"},
	}
	if err := ExportToInstinct(instincts, path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestImportFromInstinct_NonexistentFile(t *testing.T) {
	_, err := ImportFromInstinct("/nonexistent/file.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestImportFromInstinct_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	instincts, err := ImportFromInstinct(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instincts) != 0 {
		t.Errorf("expected 0 instincts from empty JSON array, got %d", len(instincts))
	}
}

func TestImportFromInstinct_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ImportFromInstinct(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseMEMORYMD_Empty(t *testing.T) {
	memories := parseMEMORYMD("")
	if len(memories) != 0 {
		t.Errorf("expected 0 memories from empty string, got %d", len(memories))
	}
}

func TestParseMEMORYMD_NoSections(t *testing.T) {
	memories := parseMEMORYMD("just some text\nwithout any sections")
	if len(memories) != 0 {
		t.Errorf("expected 0 memories without sections, got %d", len(memories))
	}
}
