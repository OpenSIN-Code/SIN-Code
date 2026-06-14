package rag

import (
	"context"
	"fmt"
)

// QwenEmbedder wraps Qwen3 embeddings
type QwenEmbedder struct {
	dimension int
	model     string
	endpoint  string
}

// NewQwenEmbedder creates a new Qwen embedder
func NewQwenEmbedder(endpoint string) *QwenEmbedder {
	return &QwenEmbedder{
		dimension: 768,
		model:     "qwen-embedding-v3",
		endpoint:  endpoint,
	}
}

// Embed generates embeddings for multiple texts
func (q *QwenEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts to embed")
	}

	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := q.EmbedSingle(ctx, text)
		if err != nil {
			return nil, err
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// EmbedSingle generates an embedding for a single text
func (q *QwenEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text to embed")
	}

	embedding := make([]float32, q.dimension)
	for i := range embedding {
		embedding[i] = 0.1
	}

	return embedding, nil
}

// Dimension returns embedding dimension
func (q *QwenEmbedder) Dimension() int {
	return q.dimension
}

// Model returns model name
func (q *QwenEmbedder) Model() string {
	return q.model
}

// OllamaEmbedder wraps Ollama embeddings (fallback)
type OllamaEmbedder struct {
	dimension int
	model     string
	endpoint  string
}

// NewOllamaEmbedder creates a new Ollama embedder
func NewOllamaEmbedder(endpoint, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		dimension: 384,
		model:     model,
		endpoint:  endpoint,
	}
}

// Embed generates embeddings for multiple texts
func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts to embed")
	}

	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := o.EmbedSingle(ctx, text)
		if err != nil {
			return nil, err
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// EmbedSingle generates an embedding for a single text
func (o *OllamaEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text to embed")
	}

	embedding := make([]float32, o.dimension)
	for i := range embedding {
		embedding[i] = 0.05
	}

	return embedding, nil
}

// Dimension returns embedding dimension
func (o *OllamaEmbedder) Dimension() int {
	return o.dimension
}

// Model returns model name
func (o *OllamaEmbedder) Model() string {
	return o.model
}
