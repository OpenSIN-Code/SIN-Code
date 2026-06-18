// SPDX-License-Identifier: MIT
// Purpose: Semantic tool retrieval with offline, deterministic embedding
// features for lazy tool loading (issue #364). No external embedding service
// is required; vectors are built from TF-IDF-like weights over normalized
// word tokens so the binary remains self-contained and offline-capable (M2).
package mcpclient

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// SemanticIndex builds deterministic, normalized feature vectors for a set of
// tool specs and returns the top-k most similar specs for a query.
//
// Thread-safe: reads use RLock, writes use Lock (M7).
type SemanticIndex struct {
	mu      sync.RWMutex
	specs   []ToolSpec
	df      map[string]int
	vectors []map[string]float64
}

// NewSemanticIndex creates a fresh index and indexes the supplied specs.
func NewSemanticIndex(specs []ToolSpec) *SemanticIndex {
	idx := &SemanticIndex{}
	idx.Index(specs)
	return idx
}

// Index rebuilds the index from the supplied specs. The input slice is copied
// and never mutated.
func (idx *SemanticIndex) Index(specs []ToolSpec) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	copied := make([]ToolSpec, len(specs))
	copy(copied, specs)
	idx.specs = copied

	n := len(copied)
	idx.df = make(map[string]int)
	idx.vectors = make([]map[string]float64, n)
	if n == 0 {
		return
	}

	raw := make([]map[string]float64, n)
	for i, spec := range copied {
		counts := make(map[string]float64)
		present := make(map[string]bool)

		for _, t := range tokenize(spec.Name) {
			counts[t] += 2.0
			present[t] = true
		}
		for _, t := range tokenize(spec.Description) {
			counts[t] += 1.0
			present[t] = true
		}
		for t := range present {
			idx.df[t]++
		}
		raw[i] = counts
	}

	// Build normalized TF-IDF vectors.
	floatN := float64(n)
	for i, counts := range raw {
		vec := make(map[string]float64, len(counts))
		var norm float64
		for term, tf := range counts {
			idf := math.Log(floatN/float64(idx.df[term])) + 1.0
			w := tf * idf
			vec[term] = w
			norm += w * w
		}
		if norm > 0 {
			inv := 1.0 / math.Sqrt(norm)
			for term := range vec {
				vec[term] *= inv
			}
		}
		idx.vectors[i] = vec
	}
}

// Search returns up to k tools ranked by cosine similarity to the query.
// Returns nil for an empty query, k <= 0, or when no tools score positively.
func (idx *SemanticIndex) Search(query string, k int) []ToolSpec {
	query = strings.TrimSpace(query)
	if query == "" || k <= 0 {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.specs) == 0 {
		return nil
	}

	queryVec := idx.queryVector(query)
	if len(queryVec) == 0 {
		return nil
	}

	type scored struct {
		spec  ToolSpec
		score float64
	}
	results := make([]scored, 0, len(idx.specs))
	for i, spec := range idx.specs {
		score := cosine(queryVec, idx.vectors[i])
		if score > 0 {
			results = append(results, scored{spec: spec, score: score})
		}
	}
	if len(results) == 0 {
		return nil
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].spec.Name < results[j].spec.Name
		}
		return results[i].score > results[j].score
	})

	if k > len(results) {
		k = len(results)
	}
	out := make([]ToolSpec, k)
	for i := 0; i < k; i++ {
		out[i] = results[i].spec
	}
	return out
}

// All returns a defensive copy of every indexed spec.
func (idx *SemanticIndex) All() []ToolSpec {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return append([]ToolSpec(nil), idx.specs...)
}

// Count returns the number of indexed specs.
func (idx *SemanticIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.specs)
}

// queryVector builds a normalized TF-IDF vector for the query using the
// corpus statistics already stored in the index.
func (idx *SemanticIndex) queryVector(query string) map[string]float64 {
	counts := make(map[string]float64)
	for _, t := range tokenize(query) {
		counts[t]++
	}
	if len(counts) == 0 {
		return nil
	}

	vec := make(map[string]float64, len(counts))
	var norm float64
	floatN := float64(len(idx.specs))
	for term, tf := range counts {
		df, ok := idx.df[term]
		var idf float64
		if ok {
			idf = math.Log(floatN/float64(df)) + 1.0
		} else {
			idf = 1.0
		}
		w := tf * idf
		vec[term] = w
		norm += w * w
	}
	if norm > 0 {
		inv := 1.0 / math.Sqrt(norm)
		for term := range vec {
			vec[term] *= inv
		}
	}
	return vec
}

// cosine computes the dot product of two normalized vectors. Because both the
// document and query vectors are L2-normalized, the dot product equals cosine
// similarity.
func cosine(queryVec, docVec map[string]float64) float64 {
	score := 0.0
	for term, qw := range queryVec {
		if dw, ok := docVec[term]; ok {
			score += qw * dw
		}
	}
	return score
}

// tokenize splits text into lowercase alphanumeric tokens, filters short tokens
// and a small set of English stop words. The result is deterministic.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) < 2 {
			continue
		}
		if stopWords[p] {
			continue
		}
		out = append(out, p)
	}
	return out
}

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "been": true, "being": true, "by": true, "can": true,
	"do": true, "does": true, "did": true, "for": true, "from": true,
	"has": true, "had": true, "have": true, "in": true, "is": true,
	"it": true, "its": true, "may": true, "might": true, "not": true,
	"of": true, "on": true, "or": true, "shall": true, "should": true,
	"that": true, "the": true, "these": true, "this": true, "those": true,
	"to": true, "was": true, "were": true, "will": true, "with": true,
	"would": true, "could": true,
}

// DefaultSemanticIndexCache returns the default path for the persisted
// embedding cache: ~/.config/sin-code/tool-embeddings.bin (or the platform
// equivalent under os.UserConfigDir). An empty string is returned when the
// user config directory cannot be determined.
func DefaultSemanticIndexCache() string {
	cfg, err := os.UserConfigDir()
	if err != nil || cfg == "" {
		return ""
	}
	return filepath.Join(cfg, "sin-code", "tool-embeddings.bin")
}

// semanticIndexCache is the on-disk format for the embedding cache. It stores
// only the corpus statistics and feature vectors; the caller still supplies
// the current specs and the content hash decides whether the cache is valid.
type semanticIndexCache struct {
	Version int
	Hash    string
	DF      map[string]int
	Vectors []map[string]float64
}

const semanticIndexCacheVersion = 1

// SaveCachedSemanticIndex writes the index vectors to path. The write is
// atomic (temp file + rename). Errors are returned so callers can decide
// whether to ignore them; the cache is purely a performance optimization.
func SaveCachedSemanticIndex(path string, idx *SemanticIndex) error {
	if path == "" {
		return nil
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	cache := semanticIndexCache{
		Version: semanticIndexCacheVersion,
		Hash:    semanticIndexHash(idx.specs),
		DF:      idx.df,
		Vectors: idx.vectors,
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := gob.NewEncoder(f)
	if err := enc.Encode(cache); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// LoadCachedSemanticIndex attempts to restore a SemanticIndex from path. It
// returns ok=true only when the cache version and content hash match the
// supplied specs. On any mismatch the caller should rebuild and save a new
// cache.
func LoadCachedSemanticIndex(path string, specs []ToolSpec) (*SemanticIndex, bool, error) {
	if path == "" {
		return nil, false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	var cache semanticIndexCache
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&cache); err != nil {
		return nil, false, err
	}
	if cache.Version != semanticIndexCacheVersion {
		return nil, false, nil
	}
	if cache.Hash != semanticIndexHash(specs) {
		return nil, false, nil
	}
	if len(cache.Vectors) != len(specs) {
		return nil, false, nil
	}

	copied := make([]ToolSpec, len(specs))
	copy(copied, specs)
	idx := &SemanticIndex{
		specs:   copied,
		df:      cache.DF,
		vectors: cache.Vectors,
	}
	return idx, true, nil
}

// semanticIndexHash produces a content hash for a spec slice. The hash is used
// to invalidate the cache whenever the effective tool set changes.
func semanticIndexHash(specs []ToolSpec) string {
	h := sha256.New()
	for _, s := range specs {
		schema := s.InputSchema
		if schema == nil {
			schema = map[string]any{}
		}
		b, _ := json.Marshal(struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"input_schema"`
		}{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: schema,
		})
		h.Write(b)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
