// SPDX-License-Identifier: MIT
// Purpose: tests for the IVF-flat vector index (issue #347).
package memory

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
)

func TestVectorIndexNewPanicsOnZeroDim(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on dim <= 0")
		}
	}()
	_ = NewVectorIndex(0, 8)
}

func TestVectorIndexNewDefaultsClusters(t *testing.T) {
	vi := NewVectorIndex(4, 0)
	if vi.nClusters != 16 {
		t.Errorf("expected default 16 clusters, got %d", vi.nClusters)
	}
}

func TestVectorIndexAddAndSize(t *testing.T) {
	vi := NewVectorIndex(3, 4)
	vi.Add("a", []float32{1, 0, 0})
	vi.Add("b", []float32{0, 1, 0})
	vi.Add("c", []float32{0, 0, 1})
	if vi.Size() != 3 {
		t.Errorf("size: got %d, want 3", vi.Size())
	}
}

func TestVectorIndexAddDimMismatch(t *testing.T) {
	vi := NewVectorIndex(3, 4)
	vi.Add("bad", []float32{1, 0})
	if vi.Size() != 0 {
		t.Errorf("mismatched-dim add should be ignored, size=%d", vi.Size())
	}
}

func TestVectorIndexAddOverwrites(t *testing.T) {
	vi := NewVectorIndex(2, 2)
	vi.Add("x", []float32{1, 0})
	vi.Add("x", []float32{0, 1})
	if vi.Size() != 1 {
		t.Errorf("overwrite should keep size 1, got %d", vi.Size())
	}
}

func TestVectorIndexRemove(t *testing.T) {
	vi := NewVectorIndex(3, 4)
	vi.Add("a", []float32{1, 0, 0})
	vi.Add("b", []float32{0, 1, 0})
	if !vi.Remove("a") {
		t.Error("Remove should return true for existing id")
	}
	if vi.Size() != 1 {
		t.Errorf("size after remove: got %d, want 1", vi.Size())
	}
	if vi.Remove("nonexistent") {
		t.Error("Remove should return false for missing id")
	}
}

func TestVectorIndexSearchExact(t *testing.T) {
	vi := NewVectorIndex(3, 2)
	vi.Add("a", []float32{1, 0, 0})
	vi.Add("b", []float32{0, 1, 0})
	vi.Add("c", []float32{0, 0, 1})
	vi.Build()
	res := vi.Search([]float32{1, 0, 0}, 1)
	if len(res) != 1 || res[0].ID != "a" {
		t.Fatalf("expected a, got %+v", res)
	}
	if res[0].Distance != 0 {
		t.Errorf("exact match distance should be 0, got %f", res[0].Distance)
	}
}

func TestVectorIndexSearchFallbackBruteForce(t *testing.T) {
	vi := NewVectorIndex(3, 4)
	vi.Add("a", []float32{1, 0, 0})
	vi.Add("b", []float32{0.9, 0.1, 0})
	vi.Add("c", []float32{0, 0, 1})
	res := vi.Search([]float32{1, 0, 0}, 2)
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].ID != "a" {
		t.Errorf("nearest should be a, got %s (d=%f)", res[0].ID, res[0].Distance)
	}
}

func TestVectorIndexSearchReturnsSortedByDistance(t *testing.T) {
	vi := NewVectorIndex(2, 2)
	vi.Add("far", []float32{10, 10})
	vi.Add("near", []float32{1, 1})
	vi.Add("mid", []float32{5, 5})
	vi.Build()
	res := vi.Search([]float32{0, 0}, 3)
	if len(res) != 3 {
		t.Fatalf("expected 3 results, got %d", len(res))
	}
	for i := 1; i < len(res); i++ {
		if res[i].Distance < res[i-1].Distance {
			t.Errorf("results not sorted ascending at index %d: %f < %f",
				i, res[i].Distance, res[i-1].Distance)
		}
	}
}

func TestVectorIndexSearchLargeCorpus(t *testing.T) {
	vi := NewVectorIndex(16, 16)
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 1000; i++ {
		vec := make([]float32, 16)
		for j := range vec {
			vec[j] = float32(rng.NormFloat64())
		}
		vi.Add(fmt.Sprintf("v%d", i), vec)
	}
	vi.Build()
	query := make([]float32, 16)
	for j := range query {
		query[j] = float32(rng.NormFloat64())
	}
	res := vi.Search(query, 10)
	if len(res) != 10 {
		t.Fatalf("expected 10 results, got %d", len(res))
	}
	for i := 1; i < len(res); i++ {
		if res[i].Distance < res[i-1].Distance {
			t.Errorf("unsorted at %d", i)
		}
	}
}

func TestVectorIndexDotProduct(t *testing.T) {
	got := DotProduct([]float32{1, 2, 3}, []float32{4, 5, 6})
	want := float32(1*4 + 2*5 + 3*6)
	if got != want {
		t.Errorf("dot product: got %f, want %f", got, want)
	}
}

func TestVectorIndexDotProductLengthMismatch(t *testing.T) {
	got := DotProduct([]float32{1, 2, 3}, []float32{4, 5})
	if got != 14 {
		t.Errorf("dot product mismatch: got %f, want 14", got)
	}
}

func TestVectorIndexNormalize(t *testing.T) {
	v := []float32{3, 4}
	Normalize(v)
	norm := float32(0)
	for _, x := range v {
		norm += x * x
	}
	if math.Abs(float64(norm)-1.0) > 1e-5 {
		t.Errorf("normalized vector should have unit norm, got %f", norm)
	}
}

func TestVectorIndexNormalizeZeroVector(t *testing.T) {
	v := []float32{0, 0, 0}
	Normalize(v)
	for _, x := range v {
		if x != 0 {
			t.Errorf("zero vector should remain zero, got %v", v)
		}
	}
}

func TestVectorIndexConcurrentAddSearch(t *testing.T) {
	vi := NewVectorIndex(4, 4)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			vec := []float32{float32(n), float32(n), 0, 0}
			vi.Add(fmt.Sprintf("g%d", n), vec)
		}(i)
	}
	wg.Wait()
	vi.Build()
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			res := vi.Search([]float32{float32(n), float32(n), 0, 0}, 1)
			if len(res) != 1 {
				t.Errorf("concurrent search: expected 1 result, got %d", len(res))
			}
		}(i)
	}
	wg.Wait()
}
