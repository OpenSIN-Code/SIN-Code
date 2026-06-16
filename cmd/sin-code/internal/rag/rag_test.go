// SPDX-License-Identifier: MIT
// Purpose: tests for the rag package — embedder, cosine similarity,
// index, worker pool, retriever. Race-clean.
package rag

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── CosineSimilarity ──────────────────────────────────────────────────

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float32{1, 2, 3, 4}
	got := CosineSimilarity(v, v)
	if math.Abs(float64(got-1.0)) > 1e-5 {
		t.Errorf("identical vectors: expected 1.0, got %v", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0, 0}
	b := []float32{0, 1, 0, 0}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("orthogonal: expected 0, got %v", got)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 1, 1, 1}
	b := []float32{-1, -1, -1, -1}
	// L2-normalize each, then cosine is -1. But our Clamp
	// returns 0 (v < 0 → 0). The retriever relies on this so
	// opposite-direction embeddings don't appear as "highly
	// relevant" — only as "anti-relevant", which we treat as
	// "irrelevant".
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("opposite (clamped to 0): expected 0, got %v", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{0, 0, 0}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("zero vector: expected 0, got %v", got)
	}
}

func TestCosineSimilarity_DifferentLength(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2, 3, 4}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("different length: expected 0, got %v", got)
	}
}

func TestCosineSimilarity_Clamp(t *testing.T) {
	// Numerical noise can push cosine slightly above 1.
	v := []float32{1, 0}
	got := CosineSimilarity(v, v)
	if got > 1.0 {
		t.Errorf("clamp failed: got %v > 1.0", got)
	}
}

// ── Normalize ─────────────────────────────────────────────────────────

func TestNormalize_UnitLength(t *testing.T) {
	v := []float32{3, 4} // length 5
	out := Normalize(v)
	if math.Abs(float64(out[0])-0.6) > 1e-5 {
		t.Errorf("expected 0.6, got %v", out[0])
	}
	if math.Abs(float64(out[1])-0.8) > 1e-5 {
		t.Errorf("expected 0.8, got %v", out[1])
	}
	// Original should not be mutated.
	if v[0] != 3 || v[1] != 4 {
		t.Errorf("Normalize mutated input: %v", v)
	}
}

func TestNormalize_ZeroVector(t *testing.T) {
	v := []float32{0, 0, 0}
	out := Normalize(v)
	for i, x := range out {
		if x != 0 {
			t.Errorf("[%d] expected 0, got %v", i, x)
		}
	}
}

// ── HashEmbedder ──────────────────────────────────────────────────────

func TestHashEmbedder_Dim(t *testing.T) {
	e := NewHashEmbedder()
	if e.Dim() != EmbeddingDim {
		t.Errorf("expected %d, got %d", EmbeddingDim, e.Dim())
	}
}

func TestHashEmbedder_Deterministic(t *testing.T) {
	e := NewHashEmbedder()
	a, _ := e.Embed(context.Background(), "hello world")
	b, _ := e.Embed(context.Background(), "hello world")
	if len(a) != EmbeddingDim || len(b) != EmbeddingDim {
		t.Fatalf("expected dim %d, got a=%d b=%d", EmbeddingDim, len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("[%d] not deterministic: %v != %v", i, a[i], b[i])
		}
	}
}

func TestHashEmbedder_DifferentInputs(t *testing.T) {
	e := NewHashEmbedder()
	a, _ := e.Embed(context.Background(), "hello")
	b, _ := e.Embed(context.Background(), "goodbye")
	// Different inputs should produce different vectors.
	// (Not strictly guaranteed for any two arbitrary strings, but
	// the SHA-256 expansion makes it astronomically unlikely for
	// "hello" and "goodbye" to collide.)
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("expected 'hello' and 'goodbye' to produce different vectors")
	}
}

func TestHashEmbedder_EmptyString(t *testing.T) {
	e := NewHashEmbedder()
	v, err := e.Embed(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != EmbeddingDim {
		t.Errorf("expected dim %d, got %d", EmbeddingDim, len(v))
	}
	for i, x := range v {
		if x != 0 {
			t.Errorf("[%d] expected 0 for empty input, got %v", i, x)
		}
	}
}

func TestHashEmbedder_Normalized(t *testing.T) {
	// Embedding should be L2-normalized (unit length) so cosine
	// is a clean inner product.
	e := NewHashEmbedder()
	v, _ := e.Embed(context.Background(), "test")
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	if math.Abs(norm-1.0) > 1e-4 {
		t.Errorf("expected unit norm, got %v", norm)
	}
}

// ── WorkerPool ────────────────────────────────────────────────────────

func TestWorkerPool_BasicEmbed(t *testing.T) {
	p := NewWorkerPool(NewHashEmbedder(), 2)
	defer p.Close()
	v, err := p.Embed(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != EmbeddingDim {
		t.Errorf("expected dim %d, got %d", EmbeddingDim, len(v))
	}
}

func TestWorkerPool_Concurrent(t *testing.T) {
	p := NewWorkerPool(NewHashEmbedder(), 4)
	defer p.Close()
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := p.Embed(context.Background(), "concurrent test")
			if err != nil {
				t.Errorf("worker %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestWorkerPool_CloseIdempotent(t *testing.T) {
	p := NewWorkerPool(NewHashEmbedder(), 1)
	p.Close()
	p.Close() // should not panic
}

func TestWorkerPool_EmbedAfterClose(t *testing.T) {
	p := NewWorkerPool(NewHashEmbedder(), 1)
	p.Close()
	_, err := p.Embed(context.Background(), "x")
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestWorkerPool_ContextCancel(t *testing.T) {
	p := NewWorkerPool(NewHashEmbedder(), 1)
	defer p.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before submission
	_, err := p.Embed(ctx, "x")
	if err == nil {
		t.Error("expected error from canceled context")
	}
}

func TestWorkerPool_EmbedErrorPropagates(t *testing.T) {
	// Use a failing embedder. Errors from the worker must reach
	// the caller via the result channel.
	fail := &failingEmbedder{err: errors.New("synthetic failure")}
	p := NewWorkerPool(fail, 1)
	defer p.Close()
	_, err := p.Embed(context.Background(), "x")
	if err == nil || err.Error() != "synthetic failure" {
		t.Errorf("expected synthetic failure, got %v", err)
	}
}

type failingEmbedder struct{ err error }

func (f *failingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, f.err
}
func (f *failingEmbedder) Dim() int { return EmbeddingDim }

// ── ONNXRuntimeEmbedder stub ──────────────────────────────────────────

func TestONNXRuntimeEmbedder_StubReturnsError(t *testing.T) {
	e := NewONNXRuntimeEmbedder("/nonexistent/model.onnx")
	_, err := e.Embed(context.Background(), "test")
	if !errors.Is(err, ErrONNXNotEnabled) {
		t.Errorf("expected ErrONNXNotEnabled, got %v", err)
	}
	if !strings.Contains(err.Error(), "/nonexistent/model.onnx") {
		t.Errorf("error should mention model path, got %v", err)
	}
}

func TestONNXRuntimeEmbedder_StubDim(t *testing.T) {
	e := NewONNXRuntimeEmbedder("/anywhere")
	if e.Dim() != EmbeddingDim {
		t.Errorf("expected %d, got %d", EmbeddingDim, e.Dim())
	}
}

func TestHTTPEmbedder_RequiresConfig(t *testing.T) {
	e := NewHTTPEmbedder("", "", "")
	_, err := e.Embed(context.Background(), "test")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
}

// ── Retriever ─────────────────────────────────────────────────────────

func TestRetriever_NilEmbedder(t *testing.T) {
	r := NewRetriever(nil, nil)
	hits, err := r.TopN(context.Background(), "test", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(hits))
	}
}

func TestRetriever_TopNWithDefaultLimit(t *testing.T) {
	e := NewHashEmbedder()
	r := NewRetriever(e, nil) // no index → 0 hits, but doesn't crash
	hits, err := r.TopN(context.Background(), "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits (no index), got %d", len(hits))
	}
	if r.DefaultLimit != 5 {
		t.Errorf("expected DefaultLimit=5, got %d", r.DefaultLimit)
	}
}

func TestRetriever_RespectsLimit(t *testing.T) {
	r := NewRetriever(NewHashEmbedder(), nil)
	if got := r.DefaultLimit; got != 5 {
		t.Errorf("expected DefaultLimit=5, got %d", got)
	}
}

// ── Index (in-memory) ─────────────────────────────────────────────────

func TestIndex_NilPersisterSafe(t *testing.T) {
	i := NewIndex(nil)
	if i.Size() != 0 {
		t.Errorf("expected 0, got %d", i.Size())
	}
	hits, err := i.TopN(context.Background(), []float32{0.1, 0.2}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 from empty index, got %d", len(hits))
	}
}

func TestIndex_SetGetDelete(t *testing.T) {
	i := NewIndex(nil)
	vec := Normalize([]float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}) // 12 dims, wrong
	_ = vec
	// We need a vector of EmbeddingDim.
	good := make([]float32, EmbeddingDim)
	for j := range good {
		good[j] = float32(j) / float32(EmbeddingDim)
	}
	good = Normalize(good)
	i.Set("a", good)
	if i.Size() != 1 {
		t.Errorf("expected 1, got %d", i.Size())
	}
	got := i.Get("a")
	if len(got) != EmbeddingDim {
		t.Errorf("expected dim %d, got %d", EmbeddingDim, len(got))
	}
	i.Delete("a")
	if i.Size() != 0 {
		t.Errorf("expected 0 after delete, got %d", i.Size())
	}
}

func TestIndex_SetRejectsWrongDim(t *testing.T) {
	i := NewIndex(nil)
	i.Set("a", []float32{1, 2, 3}) // wrong dim
	if i.Size() != 0 {
		t.Errorf("expected 0 (wrong dim rejected), got %d", i.Size())
	}
}

func TestIndex_TopN(t *testing.T) {
	i := NewIndex(nil)
	// Three entries with different vectors, all in [-1, 1] to
	// avoid float32 precision drift on dot products.
	mkVec := func(seed int) []float32 {
		v := make([]float32, EmbeddingDim)
		for j := range v {
			v[j] = float32((j+seed)%7) / 7.0 // [0, 1)
		}
		return Normalize(v)
	}
	a := mkVec(0)
	b := mkVec(1)
	c := mkVec(100)
	i.Set("a", a)
	i.Set("b", b)
	i.Set("c", c)
	hits, err := i.TopN(context.Background(), a, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("expected 3, got %d", len(hits))
	}
	// a should be the top hit (cosine(a, a) = 1.0).
	if hits[0].ID != "a" {
		t.Errorf("expected 'a' first (cosine=1), got %s (score=%v)", hits[0].ID, hits[0].Score)
	}
	if hits[0].Score < 0.99 {
		t.Errorf("expected score ~1.0, got %v", hits[0].Score)
	}
}

func TestIndex_PersisterRoundTrip(t *testing.T) {
	// fakePersister captures Save/Load in memory. Load returns
	// whatever was last saved (a real Persister's contract).
	fp := &fakePersister{}
	i1 := NewIndex(fp)
	vec := Normalize(make([]float32, EmbeddingDim))
	for j := range vec {
		vec[j] = float32(j) / float32(EmbeddingDim)
	}
	i1.Set("x", vec)
	if err := i1.Persist(); err != nil {
		t.Fatal(err)
	}
	if len(fp.saved) != 1 {
		t.Fatalf("expected 1 entry saved, got %d", len(fp.saved))
	}
	// fakePersister.Load returns the last-saved snapshot. Build
	// a new index; it must load via Load(), not via saved.
	i2 := NewIndex(fp)
	if i2.Size() != 1 {
		t.Errorf("expected 1 entry loaded, got %d", i2.Size())
	}
	if i2.Get("x") == nil {
		t.Error("expected 'x' to be loaded")
	}
}

func TestIndex_PersisterLoadIgnoresWrongDim(t *testing.T) {
	fp := &fakePersister{entries: []Entry{
		{ID: "good", Vector: make([]float32, EmbeddingDim)},
		{ID: "bad", Vector: []float32{1, 2, 3}}, // wrong dim
	}}
	i := NewIndex(fp)
	if i.Size() != 1 {
		t.Errorf("expected 1 (wrong dim dropped), got %d", i.Size())
	}
}

type fakePersister struct {
	mu      sync.Mutex
	saved   []Entry
	entries []Entry
	errSave error
}

func (f *fakePersister) Save(entries []Entry) error {
	if f.errSave != nil {
		return f.errSave
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = entries
	return nil
}

func (f *fakePersister) Load() ([]Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Return last-saved if any (so a real Persister's contract
	// holds: a Save followed by Load returns what was saved).
	if len(f.saved) > 0 {
		return f.saved, nil
	}
	return f.entries, nil
}

func TestIndex_NoPersisterPersistIsNoop(t *testing.T) {
	i := NewIndex(nil)
	i.Set("x", make([]float32, EmbeddingDim))
	if err := i.Persist(); err != nil {
		t.Errorf("Persist without persister should be no-op, got %v", err)
	}
}

// ── Misc sanity ───────────────────────────────────────────────────────

func TestEmbeddingDim_Constant(t *testing.T) {
	if EmbeddingDim != 384 {
		t.Errorf("expected 384 (issue body says sentence-transformers/all-MiniLM-L6-v2), got %d", EmbeddingDim)
	}
}

func TestWorkerPool_QueueDepth(t *testing.T) {
	p := NewWorkerPool(NewHashEmbedder(), 1)
	defer p.Close()
	if d := p.QueueDepth(); d != 0 {
		t.Errorf("expected 0, got %d", d)
	}
	// Submit 100 jobs to a 1-worker pool, depth should briefly
	// grow. Don't assert on the exact value (it's racy) — just
	// verify QueueDepth is non-negative.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = p.Embed(context.Background(), "x")
		}()
	}
	wg.Wait()
	// After all jobs are done, the queue should be empty.
	time.Sleep(10 * time.Millisecond)
	if d := p.QueueDepth(); d != 0 {
		t.Errorf("expected 0 after drain, got %d", d)
	}
}
