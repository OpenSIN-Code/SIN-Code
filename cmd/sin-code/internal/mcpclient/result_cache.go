// SPDX-License-Identifier: MIT
// Purpose: in-memory TTL cache for MCP tool results (issue #366).
// Caches tool-call responses keyed by (server, tool, args) so repeat
// read-only invocations within the TTL window skip the network/subprocess
// hop entirely. Race-safe (mandate M7): every access is guarded by a
// sync.RWMutex; the concurrency test runs 16 goroutines under -race.
package mcpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// CacheEntry holds a cached value and its absolute expiry timestamp.
type CacheEntry struct {
	Value     any
	ExpiresAt time.Time
}

// CacheStats reports cache hit/miss counters and the live entry count.
type CacheStats struct {
	Hits    int
	Misses  int
	Entries int
}

// ResultCache is an in-memory TTL cache for MCP tool results.
// The zero value is NOT usable — construct with NewResultCache.
type ResultCache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
	ttl     time.Duration
	hits    int
	misses  int
}

// NewResultCache returns a cache that expires entries after ttl.
// A ttl <= 0 means entries never expire.
func NewResultCache(ttl time.Duration) *ResultCache {
	return &ResultCache{
		entries: make(map[string]CacheEntry),
		ttl:     ttl,
	}
}

// Get returns the cached value for key if present and unexpired.
// The second return is false on miss or expiry.
func (c *ResultCache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}
	if c.ttl > 0 && time.Now().After(e.ExpiresAt) {
		// expired — remove lazily and count as miss
		c.mu.Lock()
		delete(c.entries, key)
		c.misses++
		c.mu.Unlock()
		return nil, false
	}
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	return e.Value, true
}

// Put stores value under key with the cache's TTL.
func (c *ResultCache) Put(key string, value any) {
	var exp time.Time
	if c.ttl > 0 {
		exp = time.Now().Add(c.ttl)
	}
	c.mu.Lock()
	c.entries[key] = CacheEntry{Value: value, ExpiresAt: exp}
	c.mu.Unlock()
}

// Delete removes the entry for key if present.
func (c *ResultCache) Delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Stats returns a snapshot of hit/miss counters and live entry count.
// Expired-but-not-yet-evicted entries are included in Entries; call
// PurgeExpired first for an exact live count.
func (c *ResultCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Entries: len(c.entries),
	}
}

// PurgeExpired removes all entries whose TTL has elapsed and returns the
// number purged. A cache with ttl <= 0 purges nothing.
func (c *ResultCache) PurgeExpired() int {
	now := time.Now()
	n := 0
	c.mu.Lock()
	for k, e := range c.entries {
		if c.ttl > 0 && now.After(e.ExpiresAt) {
			delete(c.entries, k)
			n++
		}
	}
	c.mu.Unlock()
	return n
}

// CacheKey builds a deterministic cache key from the server name, tool
// name, and call arguments. The args map is serialised with sorted keys
// so the same logical call always maps to the same key regardless of
// map iteration order. The key is SHA-256(server + "\x00" + tool + "\x00"
// + sortedArgs).
func CacheKey(server, tool string, args map[string]any) string {
	var b strings.Builder
	b.WriteString(server)
	b.WriteByte(0)
	b.WriteString(tool)
	b.WriteByte(0)
	if len(args) > 0 {
		keys := make([]string, 0, len(args))
		for k := range args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(k)
			b.WriteByte(1)
			b.WriteString(toKeyString(args[k]))
			b.WriteByte(0)
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// toKeyString renders an arg value into a stable string for keying.
func toKeyString(v any) string {
	if v == nil {
		return "\x00nil"
	}
	return fmt.Sprint(v)
}
