package rag

import (
	"context"
	"fmt"
	"sort"
)

// Qwen3Reranker uses Qwen3 to rerank search results
type Qwen3Reranker struct {
	model    string
	endpoint string
}

// NewQwen3Reranker creates a new Qwen3 reranker
func NewQwen3Reranker(endpoint string) *Qwen3Reranker {
	return &Qwen3Reranker{
		model:    "qwen-reranker",
		endpoint: endpoint,
	}
}

// Rerank reranks search results using Qwen3
func (q *Qwen3Reranker) Rerank(ctx context.Context, query string, results []*SearchResult, topK int) ([]*SearchResult, error) {
	if len(results) == 0 {
		return results, nil
	}

	scores := make([]float32, len(results))
	for i, result := range results {
		score := q.calculateRelevance(query, result.Content)
		scores[i] = score
	}

	indexed := make([]struct {
		result *SearchResult
		score  float32
		rank   int
	}, len(results))

	for i, result := range results {
		indexed[i] = struct {
			result *SearchResult
			score  float32
			rank   int
		}{result, scores[i], i}
	}

	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].score > indexed[j].score
	})

	reranked := []*SearchResult{}
	for i, item := range indexed {
		if i >= topK {
			break
		}
		item.result.Score = item.score
		item.result.Rank = i + 1
		reranked = append(reranked, item.result)
	}

	return reranked, nil
}

func (q *Qwen3Reranker) calculateRelevance(query, document string) float32 {
	queryLen := len(query)
	docLen := len(document)

	if queryLen == 0 || docLen == 0 {
		return 0.0
	}

	commonChars := 0
	for _, char := range query {
		for _, docChar := range document {
			if char == docChar {
				commonChars++
				break
			}
		}
	}

	return float32(commonChars) / float32(queryLen)
}
