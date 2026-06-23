// SPDX-License-Identifier: MIT
// Purpose: in-memory LRU+TTL cache for native websearch query results
// (issue #381). Race-safe, stdlib-only (container/list + sync.RWMutex),
// no external deps. Bounded size + 15-minute default TTL honour mandate
// M2 (single static binary, no runtime dependencies).
package native_websearch

import (
	"container/list"
	"sync"
	"time"
)

// Cache is a thread-safe LRU cache with per-entry TTL. Zero value
// is unusable; callers should construct via NewCache. All Cache methods
// are safe for concurrent use (mandate M7).
type Cache struct {
	mu       sync.Mutex
	items    map[string]*list.Element
	order    *list.List
	maxSize  int
	ttl      time.Duration
	hits     uint64
	misses   uint64
	evicted  uint64
}

// cacheEntry is the value type stored in the LRU's linked list. Each
// entry carries its insertion timestamp so the Get path can decide
// whether to serve or evict without scanning a separate index.
type cacheEntry struct {
	key       string
	value     any
	createdAt time.Time
}

// CacheStats is a snapshot of the cache footprint. All counters are
// monotonic since construction; the surface is purposely narrow so the
// `internal/ledger` consumer can pipe it through byte-stable telemetry
// without schema churn.
type CacheStats struct {
	Size     int           `json:"size"`
	MaxSize  int           `json:"max_size"`
	Hits     uint64        `json:"hits"`
	Misses   uint64        `json:"misses"`
	Evicted  uint64        `json:"evicted"`
	TTL      time.Duration `json:"ttl"`
}

// NewCache returns a Cache pre-sized to maxSize with the given TTL.
// maxSize <= 0 collapses to 1 so an empty cache cannot panic on insert;
// ttl <= 0 collapses to a 15-minute default — the canonical value
// named in the cache contract.
func NewCache(maxSize int, ttl time.Duration) *Cache {
	if maxSize <= 0 {
		maxSize = 1
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &Cache{
		items:   make(map[string]*list.Element, maxSize),
		order:   list.New(),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get returns the cached value for key and a bool indicating whether it
// is still fresh. Expired or missing keys return (nil, false) so the
// caller can fall through to the network path.
func (c *Cache) Get(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		c.misses++
		return nil, false
	}
	entry := el.Value.(*cacheEntry)
	if time.Since(entry.createdAt) > c.ttl {
		c.order.Remove(el)
		delete(c.items, key)
		c.evicted++
		c.misses++
		return nil, false
	}
	c.order.MoveToFront(el)
	c.hits++
	return entry.value, true
}

// Put inserts or replaces the value for key. Insertions beyond maxSize
// evict the least-recently-used entry; the insertion timestamp is the
// moment of Put, not Get, so a hot key that survives many Gets and then
// ages out gets a fresh TTL window on its next Put.
func (c *Cache) Put(key string, value any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		entry := el.Value.(*cacheEntry)
		entry.value = value
		entry.createdAt = time.Now()
		c.order.MoveToFront(el)
		return
	}
	entry := &cacheEntry{key: key, value: value, createdAt: time.Now()}
	el := c.order.PushFront(entry)
	c.items[key] = el
	for c.order.Len() > c.maxSize {
		back := c.order.Back()
		if back == nil {
			break
		}
		old := back.Value.(*cacheEntry)
		c.order.Remove(back)
		delete(c.items, old.key)
		c.evicted++
	}
}

// Delete removes key if present. No-op for unknown keys.
func (c *Cache) Delete(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
}

// Len returns the current item count. Useful for tests asserting that
// the eviction loop actually fires.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Stats returns a snapshot of the cache's counters. The output is taken
// under a single lock so hits + misses + size cannot disagree mid-flight.
func (c *Cache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{
		Size:    c.order.Len(),
		MaxSize: c.maxSize,
		Hits:    c.hits,
		Misses:  c.misses,
		Evicted: c.evicted,
		TTL:     c.ttl,
	}
}
