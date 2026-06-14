package rag

import (
	"context"
	"fmt"
	"math"
)

// SimpleVectorStore is an in-memory vector store
type SimpleVectorStore struct {
	vectors map[string][]float32
	chunks  map[string]*Chunk
}

// NewSimpleVectorStore creates a new simple vector store
func NewSimpleVectorStore() *SimpleVectorStore {
	return &SimpleVectorStore{
		vectors: make(map[string][]float32),
		chunks:  make(map[string]*Chunk),
	}
}

// Add adds chunks to the vector store
func (s *SimpleVectorStore) Add(ctx context.Context, chunks []*Chunk) error {
	for _, chunk := range chunks {
		if chunk.Embedding == nil {
			return fmt.Errorf("chunk %s has no embedding", chunk.ID)
		}
		s.vectors[chunk.ID] = chunk.Embedding
		s.chunks[chunk.ID] = chunk
	}
	return nil
}

// Search searches for similar vectors
func (s *SimpleVectorStore) Search(ctx context.Context, embedding []float32, topK int) ([]*SearchResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("empty embedding")
	}

	scores := make(map[string]float32)
	for chunkID, vector := range s.vectors {
		score := cosineSimilarity(embedding, vector)
		scores[chunkID] = score
	}

	results := []*SearchResult{}
	for chunkID, score := range scores {
		chunk := s.chunks[chunkID]
		results = append(results, &SearchResult{
			ChunkID:    chunkID,
			DocumentID: chunk.DocumentID,
			Content:    chunk.Content,
			Score:      score,
			SearchType: "vector",
			Rank:       len(results) + 1,
		})
	}

	topK = min(topK, len(results))
	return results[:topK], nil
}

// Delete removes chunks from the vector store
func (s *SimpleVectorStore) Delete(ctx context.Context, chunkIDs []string) error {
	for _, chunkID := range chunkIDs {
		delete(s.vectors, chunkID)
		delete(s.chunks, chunkID)
	}
	return nil
}

// Size returns the size of the vector store
func (s *SimpleVectorStore) Size() int {
	return len(s.vectors)
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct float32
	var normA float32
	var normB float32

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / float32(math.Sqrt(float64(normA))*math.Sqrt(float64(normB)))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
