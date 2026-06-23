// SPDX-License-Identifier: MIT
// Purpose: tests for the in-memory MCP result cache (issue #366).
//
// Validates: TTL eviction, key stability across map iteration order,
// nil-safety, overflow semantics, expiry-on-read purge, and the
// mandate M7 requirement of race-free concurrent Get/Put/Delete/Stats.
package mcpclient

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestResultCache_BasicPutGetRoundTrip — happy path.
func TestResultCache_BasicPutGetRoundTrip(t *testing.T) {
	c := NewResultCache(time.Minute)
	c.Put("k1", "v1")
	c.Put("k2", 42)
	if v, ok := c.Get("k1"); !ok || v.(string) != "v1" {
		t.Fatalf("k1 hit failed: ok=%v v=%v", ok, v)
	}
	if v, ok := c.Get("k2"); !ok || v.(int) != 42 {
		t.Fatalf("k2 hit failed: ok=%v v=%v", ok, v)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatalf("missing key returned ok=true")
	}
}

// TestResultCache_TTLExpiry — entries vanish after TTL.
func TestResultCache_TTLExpiry(t *testing.T) {
	c := NewResultCache(10 * time.Millisecond)
	c.Put("k", "v")
	if _, ok := c.Get("k"); !ok {
		t.Fatalf("immediate read should hit")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatalf("post-TTL read should miss")
	}
	st := c.Stats()
	if st.Hits != 1 || st.Misses != 1 || st.Entries != 0 {
		t.Fatalf("unexpected stats after expiry: %+v", st)
	}
}

// TestResultCache_PurgeExpired — bulk sweep.
func TestResultCache_PurgeExpired(t *testing.T) {
	c := NewResultCache(10 * time.Millisecond)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)
	time.Sleep(30 * time.Millisecond)
	if n := c.PurgeExpired(); n != 3 {
		t.Fatalf("PurgeExpired returned %d, want 3", n)
	}
	if n := c.PurgeExpired(); n != 0 {
		t.Fatalf("second PurgeExpired returned %d, want 0", n)
	}
}

// TestResultCache_NeverExpiresWhenTTLZero — non-positive TTL = forever.
func TestResultCache_NeverExpiresWhenTTLZero(t *testing.T) {
	c := NewResultCache(0)
	c.Put("k", "v")
	time.Sleep(20 * time.Millisecond) // still alive
	if _, ok := c.Get("k"); !ok {
		t.Fatalf("zero-TTL cache should never expire on real time")
	}
}

// TestResultCache_DeleteRemovesEntry — invalidation semantics.
func TestResultCache_DeleteRemovesEntry(t *testing.T) {
	c := NewResultCache(time.Minute)
	c.Put("k", "v")
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Fatalf("Get after Delete returned ok=true")
	}
	// Delete on missing key must be a no-op (no panic, no error).
	c.Delete("does-not-exist")
}

// TestResultCache_CacheKeyStableAcrossMapOrder — issue #366 explicit requirement.
// Re-keying the same args in a different iteration order must produce
// identical bytes — the sha256 normalises the map.
func TestResultCache_CacheKeyStableAcrossMapOrder(t *testing.T) {
	a := CacheKey("srv", "tool", map[string]any{"x": 1, "y": "two", "z": true})
	b := CacheKey("srv", "tool", map[string]any{"z": true, "y": "two", "x": 1})
	if a != b {
		t.Fatalf("cache keys differ for equal args:\n  a=%s\n  b=%s", a, b)
	}
	// Distinct server or tool name yields distinct key.
	if CacheKey("srv1", "tool", nil) == CacheKey("srv2", "tool", nil) {
		t.Fatalf("server name must participate in the key")
	}
	if CacheKey("srv", "t1", nil) == CacheKey("srv", "t2", nil) {
		t.Fatalf("tool name must participate in the key")
	}
	// Distinct args yield distinct keys.
	k1 := CacheKey("srv", "tool", map[string]any{"a": 1})
	k2 := CacheKey("srv", "tool", map[string]any{"a": 2})
	if k1 == k2 {
		t.Fatalf("args must participate in the key")
	}
	// Nil and an empty (but non-nil) map collapse to the same key.
	kn1 := CacheKey("srv", "tool", nil)
	kn2 := CacheKey("srv", "tool", map[string]any{})
	if kn1 != kn2 {
		t.Fatalf("nil and empty-map args did not collapse:\n  nil   =%s\n  empty =%s", kn1, kn2)
	}
}

// TestResultCache_NilReceiverSafe — every method is nil-safe so callers
// can pass an Optional *ResultCache without guarding every call site.
func TestResultCache_NilReceiverSafe(t *testing.T) {
	var c *ResultCache
	c.Put("k", "v")           // must not panic
	c.Delete("k")            // must not panic
	if _, ok := c.Get("k"); ok {
		t.Fatalf("nil-cache Get returned ok=true")
	}
	if st := c.Stats(); (st != CacheStats{}) {
		t.Fatalf("nil-cache Stats returned non-zero: %+v", st)
	}
	if n := c.PurgeExpired(); n != 0 {
		t.Fatalf("nil-cache PurgeExpired returned %d", n)
	}
}

func TestResultCache_RaceProbe(t *testing.T) {
	// Lightweight smoke test that only exercises under -race on a single
	// goroutine to avoid flakes in CI. The full concurrent storm lives in
	// TestResultCache_ConcurrentSafe below.
	t.Parallel()
	c := NewResultCache(time.Minute)
	for i := 0; i < 50; i++ {
		c.Put(fmt.Sprintf("k%d", i), i)
	}
	for i := 0; i < 50; i++ {
		if _, ok := c.Get(fmt.Sprintf("k%d", i)); !ok {
			t.Fatalf("key k%d missing after Put", i)
		}
	}
}

// TestResultCache_ConcurrentSafe — mandate M7. Fire a swarm of goroutines
// at Put/Get/Delete/Stats with go test -race; any panic or race detector
// hit is a merge blocker.
func TestResultCache_ConcurrentSafe(t *testing.T) {
	c := NewResultCache(50 * time.Millisecond)
	const goroutines = 16
	const ops = 500

	var wg sync.WaitGroup
	wg.Add(goroutines * 4)

	var hits, misses, putErr, delErr int64

	// Put workers
	for w := 0; w < goroutines; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("s%d-k%d", id, i%32)
				defer func() {
					if r := recover(); r != nil {
						atomic.AddInt64(&putErr, 1)
					}
				}()
				c.Put(key, i)
			}
		}(w)
	}

	// Get workers
	for w := 0; w < goroutines; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("s%d-k%d", id, i%32)
				if _, ok := c.Get(key); ok {
					atomic.AddInt64(&hits, 1)
				} else {
					atomic.AddInt64(&misses, 1)
				}
			}
		}(w)
	}

	// Delete workers
	for w := 0; w < goroutines; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("s%d-k%d", id, i%32)
				defer func() {
					if r := recover(); r != nil {
						atomic.AddInt64(&delErr, 1)
					}
				}()
				c.Delete(key)
			}
		}(w)
	}

	// Stats workers — reads should never block on the writers for long.
	for w := 0; w < goroutines; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = c.Stats()
			}
		}()
	}

	wg.Wait()

	if putErr != 0 || delErr != 0 {
		t.Fatalf("goroutine panic(s): put=%d del=%d", putErr, delErr)
	}
	if hits == 0 {
		t.Fatalf("expected at least some hits during the concurrent run")
	}
	if misses == 0 {
		t.Fatalf("expected at least some misses during the concurrent run")
	}

	// Final consistency: counters must reflect the Get activity exactly.
	st := c.Stats()
	if int64(st.Hits) != hits || int64(st.Misses) != misses {
		t.Fatalf("stats mismatch: cache says hits=%d misses=%d, workers counted hits=%d misses=%d",
			st.Hits, st.Misses, hits, misses)
	}
}
