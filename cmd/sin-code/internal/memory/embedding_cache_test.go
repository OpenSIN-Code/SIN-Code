// SPDX-License-Identifier: MIT
// Purpose: tests for the embedding cache (issue #351).
package memory

import (
	"sync"
	"testing"
	"time"
)

func TestEmbeddingCacheGetMiss(t *testing.T) {
	c := NewEmbeddingCache(10, time.Hour)
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss on empty cache")
	}
	s := c.Stats()
	if s.Misses != 1 || s.Hits != 0 {
		t.Errorf("stats: hits=%d misses=%d, want hits=0 misses=1", s.Hits, s.Misses)
	}
}

func TestEmbeddingCacheSetGetHit(t *testing.T) {
	c := NewEmbeddingCache(10, time.Hour)
	c.Set("hello", []float32{1, 2, 3})
	vec, ok := c.Get("hello")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if len(vec) != 3 || vec[0] != 1 || vec[2] != 3 {
		t.Errorf("got %v, want [1 2 3]", vec)
	}
	if c.Stats().Hits != 1 {
		t.Errorf("hits: got %d, want 1", c.Stats().Hits)
	}
	if c.Stats().Size != 1 {
		t.Errorf("size: got %d, want 1", c.Stats().Size)
	}
}

func TestEmbeddingCacheSetDoesNotMutateStored(t *testing.T) {
	c := NewEmbeddingCache(10, time.Hour)
	v := []float32{1, 2}
	c.Set("k", v)
	v[0] = 999
	got, _ := c.Get("k")
	if got[0] == 999 {
		t.Fatal("cache should store a defensive copy")
	}
}

func TestEmbeddingCacheTTLExpiry(t *testing.T) {
	c := NewEmbeddingCache(10, 20*time.Millisecond)
	c.Set("k", []float32{1})
	if _, ok := c.Get("k"); !ok {
		t.Fatal("expected hit before TTL")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expected miss after TTL expiry")
	}
	if c.Stats().Expired == 0 {
		t.Error("expired counter should be > 0")
	}
}

func TestEmbeddingCacheLRUEviction(t *testing.T) {
	c := NewEmbeddingCache(3, time.Hour)
	c.Set("a", []float32{1})
	c.Set("b", []float32{2})
	c.Set("c", []float32{3})
	c.Get("a")
	c.Set("d", []float32{4})
	if _, ok := c.Get("b"); ok {
		t.Error("b should have been evicted (it was LRU)")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("a should still be present")
	}
	if _, ok := c.Get("d"); !ok {
		t.Error("d should be present")
	}
	if c.Stats().Evictions == 0 {
		t.Error("evictions counter should be > 0")
	}
	if c.Stats().Size != 3 {
		t.Errorf("size: got %d, want 3", c.Stats().Size)
	}
}

func TestEmbeddingCacheOverwriteDoesNotEvict(t *testing.T) {
	c := NewEmbeddingCache(2, time.Hour)
	c.Set("a", []float32{1})
	c.Set("b", []float32{2})
	c.Set("a", []float32{9})
	if c.Stats().Size != 2 {
		t.Error("overwrite should not grow size")
	}
	if c.Stats().Evictions != 0 {
		t.Error("overwrite should not evict")
	}
	got, _ := c.Get("a")
	if got[0] != 9 {
		t.Errorf("overwrite value: got %f, want 9", got[0])
	}
}

func TestEmbeddingCacheClear(t *testing.T) {
	c := NewEmbeddingCache(10, time.Hour)
	c.Set("a", []float32{1})
	c.Set("b", []float32{2})
	n := c.Clear()
	if n != 2 {
		t.Errorf("clear return: got %d, want 2", n)
	}
	if c.Stats().Size != 0 {
		t.Errorf("size after clear: got %d, want 0", c.Stats().Size)
	}
	if c.Stats().Hits != 0 || c.Stats().Misses != 0 {
		t.Error("counters should reset on Clear")
	}
}

func TestEmbeddingCachePurgeExpired(t *testing.T) {
	c := NewEmbeddingCache(10, 20*time.Millisecond)
	c.Set("fresh", []float32{1})
	c.Set("stale", []float32{2})
	time.Sleep(30 * time.Millisecond)
	c.Set("newer", []float32{3})
	removed := c.PurgeExpired()
	if removed != 2 {
		t.Errorf("purged: got %d, want 2", removed)
	}
	if _, ok := c.Get("newer"); !ok {
		t.Error("newer should survive purge")
	}
}

func TestEmbeddingCacheConcurrentAccess(t *testing.T) {
	c := NewEmbeddingCache(100, time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			c.Set(string(rune('a'+n%26)), []float32{float32(n)})
		}(i)
		go func(n int) {
			defer wg.Done()
			_, _ = c.Get(string(rune('a' + n%26)))
		}(i)
	}
	wg.Wait()
	if c.Stats().Size > 100 {
		t.Errorf("size exceeded capacity: %d", c.Stats().Size)
	}
}
