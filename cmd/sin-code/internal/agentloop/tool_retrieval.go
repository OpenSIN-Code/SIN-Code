// SPDX-License-Identifier: MIT
// Purpose: Semantic tool retrieval with embedding-based lazy loading
// (issue #364). The ToolRetriever stores pre-computed embedding vectors for
// each registered tool and returns the top-K most relevant tools for a query
// using cosine similarity. It is the agentloop-side counterpart to
// mcpclient.SemanticIndex: callers that already have embeddings (e.g. from an
// external embedding service or the offline TF-IDF builder) register them here
// and let the loop lazy-load only the tools the current task needs.
//
// Thread-safe: all reads take RLock, all writes take Lock (mandate M7).
//
// Note on ToolSpec: this package already defines ToolSpec (loop.go) with
// Name/Description/InputSchema. We reuse that type rather than redefining it
// (a redefinition would not compile). Tool tags — which the issue spec lists
// as a ToolSpec field — are stored in a side map keyed by tool name and exposed
// via RegisterWithTags / Tags, so the functionality is preserved without
// mutating the shared ToolSpec struct.
package agentloop

import (
	"math"
	"sort"
	"strings"
	"sync"
)

// ToolRetriever is an embedding-indexed registry of tools that supports
// semantic search and lazy top-K loading. The embeddings map is keyed by tool
// name; tools preserves registration order.
type ToolRetriever struct {
	embeddings map[string][]float32
	tools      []ToolSpec
	tags       map[string][]string
	mu         sync.RWMutex
}

// NewToolRetriever creates an empty retriever.
func NewToolRetriever() *ToolRetriever {
	return &ToolRetriever{
		embeddings: make(map[string][]float32),
		tags:       make(map[string][]string),
	}
}

// Register adds (or updates, by name) a tool and its pre-computed embedding.
// The embedding slice is copied so the caller's slice is never mutated.
func (r *ToolRetriever) Register(spec ToolSpec, embedding []float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerLocked(spec, nil, embedding)
}

// RegisterWithTags is like Register but also associates searchable tags with
// the tool. A tool whose tag matches a query token receives a small similarity
// boost during Search, improving relevance beyond pure embedding distance.
func (r *ToolRetriever) RegisterWithTags(spec ToolSpec, tags []string, embedding []float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerLocked(spec, tags, embedding)
}

// registerLocked is the shared write helper; caller must hold r.mu.
func (r *ToolRetriever) registerLocked(spec ToolSpec, tags []string, embedding []float32) {
	emb := make([]float32, len(embedding))
	copy(emb, embedding)
	if _, exists := r.embeddings[spec.Name]; exists {
		for i, t := range r.tools {
			if t.Name == spec.Name {
				r.tools[i] = spec
				break
			}
		}
	} else {
		r.tools = append(r.tools, spec)
	}
	r.embeddings[spec.Name] = emb
	if tags != nil {
		cp := make([]string, len(tags))
		copy(cp, tags)
		r.tags[spec.Name] = cp
	} else {
		delete(r.tags, spec.Name)
	}
}

// Search returns up to topK tools ranked by cosine similarity between
// queryEmbedding and each registered tool's embedding. A tool whose tag
// matches a token in query receives a small additive boost. Results with a
// non-positive score are omitted. Returns nil for topK <= 0, an empty
// embedding, or an empty registry.
func (r *ToolRetriever) Search(query string, queryEmbedding []float32, topK int) []ToolSpec {
	if topK <= 0 || len(queryEmbedding) == 0 {
		return nil
	}
	qTokens := queryTokens(query)

	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.tools) == 0 {
		return nil
	}

	type scored struct {
		spec  ToolSpec
		score float32
	}
	results := make([]scored, 0, len(r.tools))
	for _, spec := range r.tools {
		emb := r.embeddings[spec.Name]
		score := CosineSimilarity(queryEmbedding, emb)
		if score > 0 && len(qTokens) > 0 {
			for _, tag := range r.tags[spec.Name] {
				if qTokens[strings.ToLower(tag)] {
					score += 0.1
					break
				}
			}
		}
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

	if topK > len(results) {
		topK = len(results)
	}
	out := make([]ToolSpec, topK)
	for i := 0; i < topK; i++ {
		out[i] = results[i].spec
	}
	return out
}

// LazyLoad returns the topK "most relevant" tools without an explicit query.
// Relevance is approximated by the L2 norm of each tool's embedding — tools
// with richer (higher-magnitude) embeddings are surfaced first, which is a
// deterministic, embedding-only heuristic. Returns nil for topK <= 0 or an
// empty registry.
func (r *ToolRetriever) LazyLoad(topK int) []ToolSpec {
	if topK <= 0 {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.tools) == 0 {
		return nil
	}

	type scored struct {
		spec  ToolSpec
		score float32
	}
	results := make([]scored, len(r.tools))
	for i, spec := range r.tools {
		emb := r.embeddings[spec.Name]
		var norm float32
		for _, v := range emb {
			norm += v * v
		}
		results[i] = scored{spec: spec, score: norm}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].spec.Name < results[j].spec.Name
		}
		return results[i].score > results[j].score
	})

	if topK > len(results) {
		topK = len(results)
	}
	out := make([]ToolSpec, topK)
	for i := 0; i < topK; i++ {
		out[i] = results[i].spec
	}
	return out
}

// Count returns the number of registered tools.
func (r *ToolRetriever) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Tags returns a copy of the tags associated with a tool (nil if none).
func (r *ToolRetriever) Tags(name string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tags := r.tags[name]
	if tags == nil {
		return nil
	}
	cp := make([]string, len(tags))
	copy(cp, tags)
	return cp
}

// All returns a defensive copy of every registered tool.
func (r *ToolRetriever) All() []ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]ToolSpec(nil), r.tools...)
}

// CosineSimilarity returns the cosine similarity between two float32 vectors.
// Vectors of differing lengths are compared over the shared prefix. A zero
// vector yields 0 (never NaN).
func CosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, na, nb float32
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(na))) * float32(math.Sqrt(float64(nb))))
}

// queryTokens splits a query into lowercase token set for tag matching.
func queryTokens(query string) map[string]bool {
	query = strings.ToLower(query)
	parts := strings.FieldsFunc(query, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	out := make(map[string]bool, len(parts))
	for _, p := range parts {
		if len(p) >= 2 {
			out[p] = true
		}
	}
	return out
}
