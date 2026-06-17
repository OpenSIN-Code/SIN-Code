// SPDX-License-Identifier: MIT
// Purpose: tests for the TTL-based prompt prefix cache (issue #277, M7).
package llm

import (
	"sync"
	"testing"
	"time"
)

func TestPromptCache_SetGet(t *testing.T) {
	c := NewPromptCache(DefaultCacheTTL)
	c.Set("key1", "prefix-abc")

	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "prefix-abc" {
		t.Errorf("got %q, want %q", got, "prefix-abc")
	}
}

func TestPromptCache_MissOnUnknownKey(t *testing.T) {
	c := NewPromptCache(DefaultCacheTTL)
	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected miss for unknown key")
	}
}

func TestPromptCache_TTLExpiry(t *testing.T) {
	c := NewPromptCache(50 * time.Millisecond)
	c.Set("key1", "prefix-xyz")

	_, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected hit before TTL expiry")
	}

	time.Sleep(60 * time.Millisecond)

	_, ok = c.Get("key1")
	if ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestPromptCache_StatsCounting(t *testing.T) {
	c := NewPromptCache(DefaultCacheTTL)
	c.Set("k1", "p1")
	c.Set("k2", "p2")

	c.Get("k1")
	c.Get("k1")
	c.Get("missing")

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Errorf("hits: got %d, want 2", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("misses: got %d, want 1", stats.Misses)
	}
	if stats.Entries != 2 {
		t.Errorf("entries: got %d, want 2", stats.Entries)
	}
}

func TestPromptCache_EvictionOnExpiry(t *testing.T) {
	c := NewPromptCache(30 * time.Millisecond)
	c.Set("key1", "prefix-1")
	c.Set("key2", "prefix-2")

	time.Sleep(40 * time.Millisecond)

	c.Get("key1")
	c.Get("key2")

	stats := c.Stats()
	if stats.Evictions != 2 {
		t.Errorf("evictions: got %d, want 2", stats.Evictions)
	}
	if stats.Entries != 0 {
		t.Errorf("entries after eviction: got %d, want 0", stats.Entries)
	}
}

func TestPromptCache_ConcurrentAccess(t *testing.T) {
	c := NewPromptCache(DefaultCacheTTL)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key-" + string(rune('A'+n%5))
			c.Set(key, "prefix")
			c.Get(key)
			_ = c.Stats()
		}(i)
	}
	wg.Wait()

	stats := c.Stats()
	if stats.Entries == 0 {
		t.Fatal("expected some entries after concurrent writes")
	}
}

func TestPromptCache_KeyNormalization(t *testing.T) {
	key1 := CacheKey("You are a coding agent.", "Write a function")
	key2 := CacheKey("You are a coding agent.", "Write a function")
	key3 := CacheKey("You are a coding agent.", "Write a different function")

	if key1 != key2 {
		t.Fatal("same inputs should produce same cache key")
	}
	if key1 == key3 {
		t.Fatal("different user messages should produce different keys")
	}
	if len(key1) != 64 {
		t.Errorf("cache key length: got %d, want 64 (sha256 hex)", len(key1))
	}
}

func TestPromptCache_DefaultTTL(t *testing.T) {
	c := NewPromptCache(0)
	if c.TTL() != DefaultCacheTTL {
		t.Errorf("default TTL: got %v, want %v", c.TTL(), DefaultCacheTTL)
	}
}

func TestPromptCache_SupportsCaching(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"claude-fable-5", true},
		{"claude-mythos-5", true},
		{"claude-sonnet-4-5", true},
		{"anthropic/claude-3", true},
		{"gpt-4o", false},
		{"llama-3.3-70b", false},
		{"meta/llama-3.1-8b", false},
	}
	for _, tc := range cases {
		if got := SupportsCaching(tc.model); got != tc.want {
			t.Errorf("SupportsCaching(%q): got %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestPromptCache_NilSafe(t *testing.T) {
	var c *PromptCache
	_, ok := c.Get("key")
	if ok {
		t.Fatal("nil cache Get should return false")
	}
	c.Set("key", "val")
	stats := c.Stats()
	if stats.Hits != 0 {
		t.Fatal("nil cache Stats should be zero")
	}
}

func TestPromptCache_EmptyKeyIgnored(t *testing.T) {
	c := NewPromptCache(DefaultCacheTTL)
	c.Set("", "prefix")
	c.Set("key", "")

	_, ok := c.Get("")
	if ok {
		t.Fatal("empty key should not be stored")
	}
	_, ok = c.Get("key")
	if ok {
		t.Fatal("empty prefixID should not be stored")
	}
}

func TestPromptCache_EvictExpired(t *testing.T) {
	c := NewPromptCache(20 * time.Millisecond)
	c.Set("k1", "p1")
	c.Set("k2", "p2")
	c.Set("k3", "p3")

	time.Sleep(25 * time.Millisecond)

	evicted := c.EvictExpired()
	if evicted != 3 {
		t.Errorf("evicted: got %d, want 3", evicted)
	}
	stats := c.Stats()
	if stats.Evictions != 3 {
		t.Errorf("stats evictions: got %d, want 3", stats.Evictions)
	}
	if stats.Entries != 0 {
		t.Errorf("entries after evict: got %d, want 0", stats.Entries)
	}
}

func TestPromptCache_Clear(t *testing.T) {
	c := NewPromptCache(DefaultCacheTTL)
	c.Set("k1", "p1")
	c.Set("k2", "p2")
	c.Clear()

	_, ok := c.Get("k1")
	if ok {
		t.Fatal("expected miss after Clear")
	}
	stats := c.Stats()
	if stats.Entries != 0 {
		t.Errorf("entries after Clear: got %d, want 0", stats.Entries)
	}
}
