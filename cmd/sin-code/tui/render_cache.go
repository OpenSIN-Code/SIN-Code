package tui

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
)

type cacheEntry struct { key string; value string }

type RenderCache struct {
	mu sync.RWMutex; maxEntries int; entries *list.List; index map[string]*list.Element
	hits int; misses int; evictions int
}

func NewRenderCache(maxEntries int) *RenderCache {
	if maxEntries < 0 { maxEntries = 0 }
	return &RenderCache{maxEntries: maxEntries, entries: list.New(), index: make(map[string]*list.Element)}
}

func (c *RenderCache) Get(key string) (string, bool) {
	c.mu.Lock(); defer c.mu.Unlock()
	if el, ok := c.index[key]; ok { c.entries.MoveToFront(el); c.hits++; return el.Value.(*cacheEntry).value, true }
	c.misses++; return "", false
}

func (c *RenderCache) Set(key, value string) {
	c.mu.Lock(); defer c.mu.Unlock()
	if c.maxEntries == 0 { return }
	if el, ok := c.index[key]; ok { el.Value.(*cacheEntry).value = value; c.entries.MoveToFront(el); return }
	entry := &cacheEntry{key: key, value: value}
	el := c.entries.PushFront(entry); c.index[key] = el
	if c.entries.Len() > c.maxEntries { oldest := c.entries.Back(); if oldest != nil { c.entries.Remove(oldest); delete(c.index, oldest.Value.(*cacheEntry).key); c.evictions++ } }
}

func (c *RenderCache) Invalidate(msgIdx int) {
	c.mu.Lock(); defer c.mu.Unlock()
	prefix := fmt.Sprintf("msg-%d:", msgIdx)
	var toRemove []*list.Element
	for el := c.entries.Front(); el != nil; el = el.Next() { if strings.HasPrefix(el.Value.(*cacheEntry).key, prefix) { toRemove = append(toRemove, el) } }
	for _, el := range toRemove { c.entries.Remove(el); delete(c.index, el.Value.(*cacheEntry).key) }
}

func (c *RenderCache) Clear() { c.mu.Lock(); defer c.mu.Unlock(); c.entries.Init(); c.index = make(map[string]*list.Element) }
func (c *RenderCache) Stats() (hits, misses, evictions int) { c.mu.RLock(); defer c.mu.RUnlock(); return c.hits, c.misses, c.evictions }

func renderCacheKey(msgIdx int, content string, width int, themeName string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%s", msgIdx, content, width, themeName)))
	return fmt.Sprintf("msg-%d:%x", msgIdx, h[:8])
}
