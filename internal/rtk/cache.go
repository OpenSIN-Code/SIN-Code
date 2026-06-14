package rtk

import (
	"sync"
	"time"
)

// CacheEntry represents a cached RTK result
type CacheEntry struct {
	Result    *RTKResult
	ExpiresAt time.Time
}

// ResultCache is a simple in-memory cache for RTK results
type ResultCache struct {
	mu       sync.RWMutex
	entries  map[string]*CacheEntry
	ttl      time.Duration
	maxSize  int
	cleanupInterval time.Duration
	stopCleanup chan struct{}
}

// NewResultCache creates a new result cache
func NewResultCache(ttl time.Duration) *ResultCache {
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}

	cache := &ResultCache{
		entries: make(map[string]*CacheEntry),
		ttl:     ttl,
		maxSize: 1000, // Default max entries
		cleanupInterval: 5 * time.Minute,
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// Set adds an entry to the cache
func (c *ResultCache) Set(key string, result *RTKResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple eviction: remove oldest entry if at max size
	if len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, entry := range c.entries {
			if oldestTime.IsZero() || entry.ExpiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = entry.ExpiresAt
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}

	c.entries[key] = &CacheEntry{
		Result:    result,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Get retrieves an entry from cache
func (c *ResultCache) Get(key string) (*RTKResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry.Result, true
}

// Delete removes an entry from cache
func (c *ResultCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear removes all entries from cache
func (c *ResultCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CacheEntry)
}

// Size returns the number of entries in cache
func (c *ResultCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// cleanupExpired periodically removes expired entries
func (c *ResultCache) cleanupExpired() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCleanup:
			return
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for key, entry := range c.entries {
				if now.After(entry.ExpiresAt) {
					delete(c.entries, key)
				}
			}
			c.mu.Unlock()
		}
	}
}

// Stop stops the cleanup goroutine
func (c *ResultCache) Stop() {
	close(c.stopCleanup)
}

// FilePersistentCache persists cache to disk (optional enhancement)
type FilePersistentCache struct {
	cache    *ResultCache
	dir      string
}

// NewFilePersistentCache creates a file-backed cache
func NewFilePersistentCache(dir string, ttl time.Duration) *FilePersistentCache {
	return &FilePersistentCache{
		cache: NewResultCache(ttl),
		dir:   dir,
	}
}

// Set adds an entry and optionally persists it
func (f *FilePersistentCache) Set(key string, result *RTKResult) {
	f.cache.Set(key, result)
	// TODO: Implement file persistence if needed
}

// Get retrieves an entry from cache
func (f *FilePersistentCache) Get(key string) (*RTKResult, bool) {
	return f.cache.Get(key)
}

// Delete removes an entry
func (f *FilePersistentCache) Delete(key string) {
	f.cache.Delete(key)
}

// Clear removes all entries
func (f *FilePersistentCache) Clear() {
	f.cache.Clear()
}

// Stop stops the cache
func (f *FilePersistentCache) Stop() {
	f.cache.Stop()
}
