// SPDX-License-Identifier: MIT
// Purpose: tests for the MCP tool-result cache (issue #366).
// Includes a 16-goroutine concurrency test run under -race (mandate M7).
package mcpclient

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestResultCache_PutAndGet(t *testing.T) {
	c := NewResultCache(time.Minute)
	c.Put("a", "alpha")
	v, ok := c.Get("a")
	if !ok || v != "alpha" {
		t.Fatalf("Get(a) = %v, %v; want alpha, true", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("Get(missing) hit; want miss")
	}
}

func TestResultCache_TTLExpiry(t *testing.T) {
	c := NewResultCache(20 * time.Millisecond)
	c.Put("k", 42)
	if v, ok := c.Get("k"); !ok || v != 42 {
		t.Fatalf("immediate Get = %v, %v; want 42, true", v, ok)
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expired entry hit; want miss")
	}
}

func TestResultCache_Delete(t *testing.T) {
	c := NewResultCache(time.Minute)
	c.Put("x", 1)
	c.Delete("x")
	if _, ok := c.Get("x"); ok {
		t.Fatal("Delete did not remove entry")
	}
	c.Delete("nonexistent") // no panic
}

func TestResultCache_Stats(t *testing.T) {
	c := NewResultCache(time.Minute)
	c.Put("1", "v")
	c.Put("2", "w")
	c.Get("1")
	c.Get("1")
	c.Get("nope")
	st := c.Stats()
	if st.Hits != 2 {
		t.Fatalf("Hits = %d; want 2", st.Hits)
	}
	if st.Misses != 1 {
		t.Fatalf("Misses = %d; want 1", st.Misses)
	}
	if st.Entries != 2 {
		t.Fatalf("Entries = %d; want 2", st.Entries)
	}
}

func TestResultCache_PurgeExpired(t *testing.T) {
	c := NewResultCache(15 * time.Millisecond)
	c.Put("a", 1)
	c.Put("b", 2)
	time.Sleep(30 * time.Millisecond)
	n := c.PurgeExpired()
	if n != 2 {
		t.Fatalf("PurgeExpired = %d; want 2", n)
	}
	if c.Stats().Entries != 0 {
		t.Fatalf("Entries after purge = %d; want 0", c.Stats().Entries)
	}
}

func TestResultCache_NeverExpiresWhenTTLZero(t *testing.T) {
	c := NewResultCache(0)
	c.Put("k", "v")
	time.Sleep(10 * time.Millisecond)
	if _, ok := c.Get("k"); !ok {
		t.Fatal("ttl=0 entry expired; should never expire")
	}
	if n := c.PurgeExpired(); n != 0 {
		t.Fatalf("PurgeExpired with ttl=0 = %d; want 0", n)
	}
}

func TestCacheKey_Deterministic(t *testing.T) {
	a := map[string]any{"q": "hello", "limit": 10, "flag": true}
	b := map[string]any{"limit": 10, "flag": true, "q": "hello"} // different order
	k1 := CacheKey("srv", "search", a)
	k2 := CacheKey("srv", "search", b)
	if k1 != k2 {
		t.Fatal("CacheKey not deterministic across map order")
	}
	if CacheKey("srv", "search", nil) == "" {
		t.Fatal("CacheKey returned empty")
	}
	if CacheKey("srv", "search", a) == CacheKey("srv", "other", a) {
		t.Fatal("different tools produced same key")
	}
	if CacheKey("srv", "search", a) == CacheKey("other", "search", a) {
		t.Fatal("different servers produced same key")
	}
}

func TestResultCache_Concurrency16Goroutines(t *testing.T) {
	c := NewResultCache(10 * time.Millisecond)
	const goroutines = 16
	const ops = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := CacheKey("srv", "tool", map[string]any{
					"g":   id,
					"key": fmt.Sprintf("k%d", i%8),
				})
				if _, ok := c.Get(key); !ok {
					c.Put(key, i)
				}
				if i%50 == 0 {
					c.PurgeExpired()
				}
				_ = c.Stats()
			}
		}(g)
	}
	wg.Wait()
	st := c.Stats()
	if st.Hits+st.Misses == 0 {
		t.Fatal("no Get operations recorded under concurrency")
	}
}
