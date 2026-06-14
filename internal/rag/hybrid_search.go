package rag

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// HybridSearcher combines vector and keyword search
type HybridSearcher struct {
	vectorStore VectorStore
	embedder    Embedder
	documents   map[string]*Document
}

// NewHybridSearcher creates a new hybrid searcher
func NewHybridSearcher(vectorStore VectorStore, embedder Embedder) *HybridSearcher {
	return &HybridSearcher{
		vectorStore: vectorStore,
		embedder:    embedder,
		documents:   make(map[string]*Document),
	}
}

// Search performs hybrid search with RRF (Reciprocal Rank Fusion)
func (h *HybridSearcher) Search(ctx context.Context, query string, topK int) ([]*SearchResult, error) {
	vectorEmbedding, err := h.embedder.EmbedSingle(ctx, query)
	if err != nil {
		return nil, err
	}

	vectorResults, err := h.vectorStore.Search(ctx, vectorEmbedding, topK*2)
	if err != nil {
		return nil, err
	}

	keywordResults := h.keywordSearch(query, topK*2)

	mergedResults := h.rrfMerge(vectorResults, keywordResults, topK)
	return mergedResults, nil
}

// keywordSearch performs BM25-style keyword search
func (h *HybridSearcher) keywordSearch(query string, topK int) []*SearchResult {
	queryTerms := strings.Fields(strings.ToLower(query))
	results := []*SearchResult{}

	for chunkID, chunk := range h.getChunks() {
		score := h.calculateBM25(queryTerms, chunk.Content)
		results = append(results, &SearchResult{
			ChunkID:    chunkID,
			DocumentID: chunk.DocumentID,
			Content:    chunk.Content,
			Score:      score,
			SearchType: "keyword",
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

func (h *HybridSearcher) calculateBM25(terms []string, document string) float32 {
	docTerms := strings.Fields(strings.ToLower(document))
	var score float32
	k1 := float32(1.5)
	b := float32(0.75)

	for _, term := range terms {
		freq := 0
		for _, docTerm := range docTerms {
			if term == docTerm {
				freq++
			}
		}

		if freq > 0 {
			idf := float32(math.Log(float64(len(h.getChunks()))))
			score += idf * (k1 + 1) * float32(freq) / (k1*(1-b+b) + float32(freq))
		}
	}

	return score
}

// rrfMerge merges results using Reciprocal Rank Fusion
func (h *HybridSearcher) rrfMerge(vectorResults, keywordResults []*SearchResult, topK int) []*SearchResult {
	scores := make(map[string]float32)
	docs := make(map[string]*SearchResult)

	k := float32(60)

	for i, result := range vectorResults {
		scores[result.ChunkID] += 1 / (k + float32(i+1))
		docs[result.ChunkID] = result
	}

	for i, result := range keywordResults {
		scores[result.ChunkID] += 1 / (k + float32(i+1))
		if _, exists := docs[result.ChunkID]; !exists {
			docs[result.ChunkID] = result
		}
	}

	mergedResults := []*SearchResult{}
	for chunkID, score := range scores {
		doc := docs[chunkID]
		doc.Score = score
		doc.SearchType = "hybrid"
		mergedResults = append(mergedResults, doc)
	}

	sort.Slice(mergedResults, func(i, j int) bool {
		return mergedResults[i].Score > mergedResults[j].Score
	})

	if len(mergedResults) > topK {
		mergedResults = mergedResults[:topK]
	}

	return mergedResults
}

func (h *HybridSearcher) getChunks() map[string]*Chunk {
	chunks := make(map[string]*Chunk)
	return chunks
}
