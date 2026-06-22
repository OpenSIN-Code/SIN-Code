// SPDX-License-Identifier: MIT
package native_websearch

import (
	"testing"
	"time"
)

func TestNewCacheDefaults(t *testing.T) {
	c := NewCache(0, 0)
	if c.maxSize != 1 {
		t.Errorf("maxSize default = %d, want 1", c.maxSize)
	}
	if c.ttl != 15*time.Minute {
		t.Errorf("TTL default = %v, want 15m", c.ttl)
	}
}

func TestCacheEviction(t *testing.T) {
	c := NewCache(2, time.Minute)
	c.Put("a", 1)
	c.Put("b", 2)
	if c.Len() != 2 {
		t.Fatalf("len=%d, want 2", c.Len())
	}
	c.Put("c", 3)
	if c.Len() != 2 {
		t.Fatalf("len after Put(c)=%d, want 2", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Errorf("a should be evicted (LRU policy); was retrieved")
	}
	if _, ok := c.Get("c"); !ok {
		t.Errorf("c should still be present; was missing")
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	c := NewCache(4, 20*time.Millisecond)
	c.Put("k", "v")
	if _, ok := c.Get("k"); !ok {
		t.Fatal("Get before TTL expiry returned miss")
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("Get after TTL expiry returned hit; want miss")
	}
	s := c.Stats()
	if s.Evicted == 0 {
		t.Errorf("evicted count = 0 after expiry; want >=1")
	}
}

func TestCacheNilSafe(t *testing.T) {
	var c *Cache
	if _, ok := c.Get("k"); ok {
		t.Error("nil.Get returned hit")
	}
	c.Put("k", 1)
	if got := c.Len(); got != 0 {
		t.Errorf("nil.Len=%d, want 0", got)
	}
	if got := c.Stats(); got != (CacheStats{}) {
		t.Errorf("nil.Stats=%+v, want zero", got)
	}
}

func TestCacheReplace(t *testing.T) {
	c := NewCache(2, time.Minute)
	c.Put("k", "v1")
	c.Put("k", "v2")
	if c.Len() != 1 {
		t.Fatalf("len after replace = %d, want 1", c.Len())
	}
	if v, ok := c.Get("k"); !ok || v.(string) != "v2" {
		t.Errorf("Get after replace = %v, %v; want v2 true", v, ok)
	}
}
