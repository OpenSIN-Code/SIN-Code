// SPDX-License-Identifier: MIT
// Purpose: TTL-based prompt prefix cache for LLM API calls (issue #277).
// Anthropic/Claude models support prompt caching with a 5-minute TTL via
// prefix matching. This cache tracks the prefix ID returned by the API so
// subsequent requests within the TTL window can skip re-sending the full
// system prompt and instead reference the cached prefix.
//
// Thread-safe (mandate M7): all reads/writes are guarded by sync.RWMutex.
package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const DefaultCacheTTL = 5 * time.Minute

type CacheStats struct {
	Hits       int64
	Misses     int64
	Evictions  int64
	Entries    int64
	TTLSeconds int64
}

type cacheEntry struct {
	prefixID  string
	expiresAt time.Time
}

type PromptCache struct {
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]cacheEntry

	hits      int64
	misses    int64
	evictions int64
}

func NewPromptCache(ttl time.Duration) *PromptCache {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &PromptCache{
		ttl:     ttl,
		entries: make(map[string]cacheEntry),
	}
}

func (c *PromptCache) Get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		c.evictions++
		c.misses++
		return "", false
	}
	c.hits++
	return entry.prefixID, true
}

func (c *PromptCache) Set(key, prefixID string) {
	if c == nil || key == "" || prefixID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = make(map[string]cacheEntry)
	}
	c.entries[key] = cacheEntry{
		prefixID:  prefixID,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *PromptCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	var expired int64
	now := time.Now()
	for _, e := range c.entries {
		if now.After(e.expiresAt) {
			expired++
		}
	}
	return CacheStats{
		Hits:       c.hits,
		Misses:     c.misses,
		Evictions:  c.evictions,
		Entries:    int64(len(c.entries)) - expired,
		TTLSeconds: int64(c.ttl.Seconds()),
	}
}

func (c *PromptCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}

func (c *PromptCache) TTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.ttl
}

func CacheKey(systemPrompt, firstUserMessage string) string {
	h := sha256.Sum256([]byte(systemPrompt + "\x00" + firstUserMessage))
	return hex.EncodeToString(h[:])
}

func SupportsCaching(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "anthropic") ||
		strings.Contains(m, "claude")
}

func (c *PromptCache) EvictExpired() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	count := 0
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
			c.evictions++
			count++
		}
	}
	return count
}
