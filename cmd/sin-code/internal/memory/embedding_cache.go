// SPDX-License-Identifier: MIT
// Purpose: Embedding cache with TTL + LRU eviction (issue #351).
package memory

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

type CacheStats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Expired   int64
	Size      int
}

type cacheEntry struct {
	key       string
	vec       []float32
	createdAt time.Time
	lruElem   *list.Element
}

type EmbeddingCache struct {
	maxEntries int
	ttl        time.Duration
	mu         sync.Mutex
	entries    map[string]*cacheEntry
	lru        *list.List
	stats      CacheStats
}

func NewEmbeddingCache(maxEntries int, ttl time.Duration) *EmbeddingCache {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &EmbeddingCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		entries:    make(map[string]*cacheEntry),
		lru:        list.New(),
	}
}

func cacheKey(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:8])
}

func (c *EmbeddingCache) Get(key string) ([]float32, bool) {
	if c == nil {
		return nil, false
	}
	h := cacheKey(key)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[h]
	if !ok {
		c.stats.Misses++
		return nil, false
	}
	if time.Since(e.createdAt) > c.ttl {
		c.removeElementLocked(e)
		c.stats.Expired++
		c.stats.Misses++
		return nil, false
	}
	c.lru.MoveToFront(e.lruElem)
	c.stats.Hits++
	return e.vec, true
}

func (c *EmbeddingCache) Set(key string, vec []float32) {
	if c == nil || len(vec) == 0 {
		return
	}
	h := cacheKey(key)
	cp := make([]float32, len(vec))
	copy(cp, vec)
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[h]; ok {
		e.vec = cp
		e.createdAt = time.Now()
		c.lru.MoveToFront(e.lruElem)
		return
	}
	for len(c.entries) >= c.maxEntries {
		back := c.lru.Back()
		if back == nil {
			break
		}
		old := back.Value.(*cacheEntry)
		c.removeElementLocked(old)
		c.stats.Evictions++
	}
	elem := c.lru.PushFront(nil)
	entry := &cacheEntry{key: h, vec: cp, createdAt: time.Now(), lruElem: elem}
	elem.Value = entry
	c.entries[h] = entry
}

func (c *EmbeddingCache) removeElementLocked(e *cacheEntry) {
	c.lru.Remove(e.lruElem)
	delete(c.entries, e.key)
}

func (c *EmbeddingCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stats
	s.Size = len(c.entries)
	return s
}

func (c *EmbeddingCache) Clear() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(c.entries)
	c.entries = make(map[string]*cacheEntry)
	c.lru = list.New()
	c.stats = CacheStats{}
	return n
}

func (c *EmbeddingCache) PurgeExpired() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	removed := 0
	for h, e := range c.entries {
		if now.Sub(e.createdAt) > c.ttl {
			c.lru.Remove(e.lruElem)
			delete(c.entries, h)
			c.stats.Expired++
			removed++
		}
	}
	return removed
}

func (c *EmbeddingCache) MaxEntries() int {
	if c == nil {
		return 0
	}
	return c.maxEntries
}

func (c *EmbeddingCache) TTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.ttl
}
