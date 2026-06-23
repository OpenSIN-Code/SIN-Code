// SPDX-License-Identifier: MIT
// Purpose: result cache for deterministic read-only MCP tool calls
// (issue #366). Deterministic, read-only tools like sin_read, sin_scout,
// sin_map, sin_grasp return the same result for the same (server, tool,
// args) tuple within a short time window. Caching the result saves
// tokens, wall-clock, and downstream LLM cost.
//
// Mandate M3 (verification gate) is preserved: cache HITS do NOT report
// success on their own — the caller still routes through the verification
// gate when a task uses cached results, so a stale cache cannot leak past
// the gate.
//
// Mandate M7 (race-free concurrency): every entry mutation goes through a
// sync.RWMutex.
package mcpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

// CacheEntry is one stored result. Value is opaque (any) — callers can
// stash the verbatim MCP result struct, a string, or a richer Go value.
// ExpiresAt is wall-clock; Get purges expired entries on read.
type CacheEntry struct {
	Value     any
	ExpiresAt time.Time
}

// CacheStats is a point-in-time snapshot of cache effectiveness.
// Hits / Misses / Entries are monotonically increasing (Entries is the
// current size, not the lifetime count).
type CacheStats struct {
	Hits    int
	Misses  int
	Entries int
}

// ResultCache is an in-memory TTL cache keyed by a string. It is safe for
// concurrent use. The zero value is unusable — always construct via
// NewResultCache.
type ResultCache struct {
	ttl     time.Duration
	now     func() time.Time
	mu      sync.RWMutex
	entries map[string]CacheEntry
	hits    int
	misses  int
}

// NewResultCache returns a cache with the given TTL. Pass a non-positive
// duration to cache forever (entries never expire). now is overridable so
// tests can drive the clock deterministically.
func NewResultCache(ttl time.Duration) *ResultCache {
	return &ResultCache{
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[string]CacheEntry),
	}
}

// CacheKey returns the canonical cache key for one tool invocation.
// serverName + toolName disciples the MCP server prefix convention from
// mcpclient.Tool.Qualified; the args portion is a stable SHA-256 of the
// normalised argument map so key-equality is robust against map iteration
// order and nil/empty equivalence.
//
// args may be nil — an empty-args call hashes to a fixed sentinel that
// callers can predict.
func CacheKey(serverName, toolName string, args map[string]any) string {
	h := sha256.New()
	h.Write([]byte(serverName))
	h.Write([]byte{0x1f}) // unit separator: never appears in a server or tool name
	h.Write([]byte(toolName))
	if len(args) > 0 {
		keys := make([]string, 0, len(args))
		for k := range args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte{0x1f})
			h.Write([]byte(k))
			h.Write([]byte{0x1f})
			h.Write([]byte(argsCanonical(args[k])))
		}
	}
	return serverName + "\x1f" + toolName + "\x1f" + hex.EncodeToString(h.Sum(nil))
}

// Get returns the cached value for key if present and not expired.
// Returns (nil, false) on miss OR on hit-but-expired (the entry is
// purged as a side-effect). Counters update atomically with the read.
func (c *ResultCache) Get(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return nil, false
	}
	if !entry.ExpiresAt.IsZero() && c.now().After(entry.ExpiresAt) {
		delete(c.entries, key)
		c.misses++
		return nil, false
	}
	c.hits++
	return entry.Value, true
}

// Put inserts value under key with the cache's TTL. If the key is
// already present, it is overwritten. A zero-value ExpiresAt means
// "never expires" (only relevant when the cache was constructed with
// a non-positive TTL).
func (c *ResultCache) Put(key string, value any) {
	if c == nil {
		return
	}
	var expiresAt time.Time
	if c.ttl > 0 {
		expiresAt = c.now().Add(c.ttl)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = CacheEntry{Value: value, ExpiresAt: expiresAt}
}

// Delete removes key (no-op if missing). Intended for invalidation on
// sin_write / sin_edit — callers compute the affected keys and pass
// them in. Failure to find the key is not an error.
func (c *ResultCache) Delete(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Stats returns a snapshot of the cache counters. Reads are taken
// under the write lock so the snapshot is internally consistent
// (Hits + Misses reflects activity up to the moment of the call).
func (c *ResultCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Entries: len(c.entries),
	}
}

// PurgeExpired walks the cache and drops every entry whose ExpiresAt is
// in the past. Cheap to call periodically; not required because Get
// purges lazily on read.
func (c *ResultCache) PurgeExpired() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	purged := 0
	for k, e := range c.entries {
		if !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt) {
			delete(c.entries, k)
			purged++
		}
	}
	return purged
}

// argsCanonical renders any value into a deterministic string for hashing.
// Maps and slices recurse so structurally-equal args produce the same
// bytes; non-supported types fall through to fmt.Sprintf which is good
// enough for keying purposes.
func argsCanonical(v any) string {
	switch t := v.(type) {
	case nil:
		return "<nil>"
	case string:
		return "s:" + t
	case bool:
		if t {
			return "b:t"
		}
		return "b:f"
	case int:
		return "i:" + strconv.FormatInt(int64(t), 10)
	case int32:
		return "i:" + strconv.FormatInt(int64(t), 10)
	case int64:
		return "i:" + strconv.FormatInt(t, 10)
	case uint:
		return "u:" + strconv.FormatUint(uint64(t), 10)
	case uint32:
		return "u:" + strconv.FormatUint(uint64(t), 10)
	case uint64:
		return "u:" + strconv.FormatUint(t, 10)
	case float32:
		return "f:" + fmt.Sprintf("%g", float64(t))
	case float64:
		return "f:" + fmt.Sprintf("%g", t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := "{"
		for i, k := range keys {
			if i > 0 {
				out += ","
			}
			out += k + "=" + argsCanonical(t[k])
		}
		out += "}"
		return out
	case []any:
		out := "["
		for i, e := range t {
			if i > 0 {
				out += ","
			}
			out += argsCanonical(e)
		}
		out += "]"
		return out
	default:
		// Fall back to the value's own String() representation. This is
		// not perfectly stable across implementations (e.g. a struct with
		// non-deterministic map fields) but it is good enough for the
		// common cases: time.Time implements Stringer with canonical
		// nanosecond precision, and ints/floats above already have
		// dedicated branches.
		return "v:" + fmt.Sprintf("%v", v)
	}
}
