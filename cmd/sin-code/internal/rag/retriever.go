// SPDX-License-Identifier: MIT
// Purpose: Retriever — high-level RAG retrieval. Embeds the query,
// runs top-N cosine sim, and returns the matching entry IDs. The
// retriever is the public face of the rag package; the rest of
// the binary interacts with RAG through this single type.
package rag

import "context"

// Retriever wires an Embedder and an Index into a top-N search.
type Retriever struct {
	Embedder Embedder
	Index    *Index
	// DefaultLimit is used when TopN is called with limit <= 0.
	DefaultLimit int
}

// NewRetriever builds a Retriever with sensible defaults
// (DefaultLimit=5, matching the issue body's "top-5 most similar").
func NewRetriever(embedder Embedder, index *Index) *Retriever {
	return &Retriever{Embedder: embedder, Index: index, DefaultLimit: 5}
}

// TopN embeds the query, then returns the top-N matching IDs
// from the index. The limit argument overrides DefaultLimit if > 0.
func (r *Retriever) TopN(ctx context.Context, query string, limit int) ([]Scored, error) {
	if r.Embedder == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = r.DefaultLimit
	}
	vec, err := r.Embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(vec) == 0 {
		return nil, nil
	}
	if r.Index == nil {
		return nil, nil
	}
	return r.Index.TopN(ctx, vec, limit)
}
