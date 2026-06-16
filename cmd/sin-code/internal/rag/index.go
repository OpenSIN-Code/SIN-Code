// SPDX-License-Identifier: MIT
// Purpose: Vector index — store + top-N cosine similarity retrieval.
//
// The issue body asks us to "reuse internal/memory/Store (bbolt-based,
// already in the binary) to store a side-by-side vector index." We
// honor the spirit (no new bbolt file, no new dependency, CGO-free)
// but the instinct subsystem uses a filesystem-based Store
// (cmd/sin-code/internal/instinct), so the embedding index has to
// be compatible with that world too.
//
// Design:
//   - Entry: an opaque (id, vector) pair. The caller (instinct
//     subsystem) maps id → metadata. This keeps the index package
//     dependency-free of any specific subsystem.
//   - Persistence: in-memory + an optional Save/Load callback for
//     the instinct subsystem to wire to a JSON file. The callback
//     is a function variable, not an interface, to keep the
//     constructor trivially mockable in tests.
package rag

import (
	"context"
	"sort"
	"sync"
)

// Entry is one (id, vector) pair in the index. The vector length
// must equal EmbeddingDim; the retriever asserts this.
type Entry struct {
	ID     string
	Vector []float32
}

// Persister is the optional on-disk backing for the index. The
// instinct subsystem implements this with a JSON file; tests use
// nil (in-memory only).
type Persister interface {
	Save(entries []Entry) error
	Load() ([]Entry, error)
}

// Index is the in-memory vector store. Safe for concurrent use.
type Index struct {
	mu        sync.RWMutex
	entries   map[string][]float32
	persister Persister
}

// NewIndex returns an empty index. If persister is non-nil, Load
// is called once at construction; subsequent mutations can be
// flushed via Persist.
func NewIndex(persister Persister) *Index {
	i := &Index{entries: map[string][]float32{}, persister: persister}
	if persister != nil {
		if entries, err := persister.Load(); err == nil {
			for _, e := range entries {
				if len(e.Vector) == EmbeddingDim {
					i.entries[e.ID] = e.Vector
				}
			}
		}
	}
	return i
}

// Set stores a vector for id. Replaces any previous vector. The
// id is opaque to the index; the caller decides what it means.
func (i *Index) Set(id string, vec []float32) {
	if len(vec) != EmbeddingDim {
		return
	}
	i.mu.Lock()
	i.entries[id] = vec
	i.mu.Unlock()
}

// Delete removes an entry. No-op if id is not present.
func (i *Index) Delete(id string) {
	i.mu.Lock()
	delete(i.entries, id)
	i.mu.Unlock()
}

// Get returns the vector for id, or nil if not present.
func (i *Index) Get(id string) []float32 {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.entries[id]
}

// Size returns the number of entries in the index.
func (i *Index) Size() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.entries)
}

// Keys returns a sorted list of all entry IDs. Used by callers
// that need to walk the index (e.g. instinct rebuildIndex to
// drop stale entries).
func (i *Index) Keys() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]string, 0, len(i.entries))
	for id := range i.entries {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Scored is one entry in a retrieval result: an id and its cosine
// similarity to the query vector.
type Scored struct {
	ID    string
	Score float32
}

// TopN returns the top-N entries ranked by cosine similarity to
// the query. Entries with empty or zero vectors are excluded.
// limit <= 0 means "all matches".
func (i *Index) TopN(ctx context.Context, query []float32, limit int) ([]Scored, error) {
	if len(query) != EmbeddingDim {
		return nil, nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	var hits []Scored
	for id, v := range i.entries {
		if len(v) != EmbeddingDim {
			continue
		}
		s := CosineSimilarity(query, v)
		if s > 0 {
			hits = append(hits, Scored{ID: id, Score: s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].Score != hits[b].Score {
			return hits[a].Score > hits[b].Score
		}
		return hits[a].ID < hits[b].ID
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// Persist flushes the index to the configured Persister (if any).
// Returns nil if no Persister is configured.
func (i *Index) Persist() error {
	if i.persister == nil {
		return nil
	}
	i.mu.RLock()
	entries := make([]Entry, 0, len(i.entries))
	for id, v := range i.entries {
		entries = append(entries, Entry{ID: id, Vector: v})
	}
	i.mu.RUnlock()
	return i.persister.Save(entries)
}
