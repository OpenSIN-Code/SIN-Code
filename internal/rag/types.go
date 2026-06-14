package rag

import (
	"context"
	"time"
)

// Document represents a source document for RAG
type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}

// Chunk represents a chunked piece of a document
type Chunk struct {
	ID          string    `json:"id"`
	DocumentID  string    `json:"document_id"`
	Content     string    `json:"content"`
	StartPos    int       `json:"start_pos"`
	EndPos      int       `json:"end_pos"`
	Tokens      int       `json:"tokens"`
	Embedding   []float32 `json:"embedding,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	ChunkType   string    `json:"chunk_type"` // semantic, sentence, overlap
}

// Embedding represents a vector embedding
type Embedding struct {
	Vector    []float32 `json:"vector"`
	Dimension int       `json:"dimension"`
	Model     string    `json:"model"`
	Timestamp time.Time `json:"timestamp"`
}

// SearchResult represents a search query result
type SearchResult struct {
	ChunkID     string    `json:"chunk_id"`
	DocumentID  string    `json:"document_id"`
	Content     string    `json:"content"`
	Score       float32   `json:"score"`
	SearchType  string    `json:"search_type"` // vector, keyword, hybrid
	Rank        int       `json:"rank"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// QueryRequest represents a search query
type QueryRequest struct {
	Query    string   `json:"query"`
	TopK     int      `json:"top_k"`
	Filters  map[string]interface{} `json:"filters"`
	SearchType string `json:"search_type"` // vector, hybrid, graphrag
}

// ChunkingStrategy defines how to chunk documents
type ChunkingStrategy struct {
	Name       string
	MinSize    int
	MaxSize    int
	Overlap    int
	SplitOn    string // newline, sentence, semantic
}

// Embedder interface for embedding text
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedSingle(ctx context.Context, text string) ([]float32, error)
	Dimension() int
	Model() string
}

// Chunker interface for chunking documents
type Chunker interface {
	Chunk(ctx context.Context, doc *Document, strategy *ChunkingStrategy) ([]*Chunk, error)
}

// VectorStore interface for storing and retrieving embeddings
type VectorStore interface {
	Add(ctx context.Context, chunks []*Chunk) error
	Search(ctx context.Context, embedding []float32, topK int) ([]*SearchResult, error)
	Delete(ctx context.Context, chunkIDs []string) error
	Size() int
}

// Reranker interface for reranking search results
type Reranker interface {
	Rerank(ctx context.Context, query string, results []*SearchResult, topK int) ([]*SearchResult, error)
}

// GraphRAG represents graph-based retrieval
type GraphRAG struct {
	Entities   map[string]map[string]interface{}
	Relations  []map[string]interface{}
	Community  map[string][]string
}

// RAGConfig contains RAG system configuration
type RAGConfig struct {
	EmbedderModel    string
	ChunkingStrategy *ChunkingStrategy
	VectorDBType     string // faiss, annoy, pgvector
	RerankingModel   string
	TopK             int
	Timeout          time.Duration
}

// RAGSystem represents the complete RAG system
type RAGSystem struct {
	embedder     Embedder
	chunker      Chunker
	vectorStore  VectorStore
	reranker     Reranker
	config       *RAGConfig
}

// NewRAGSystem creates a new RAG system
func NewRAGSystem(embedder Embedder, chunker Chunker, vectorStore VectorStore, config *RAGConfig) *RAGSystem {
	return &RAGSystem{
		embedder:    embedder,
		chunker:     chunker,
		vectorStore: vectorStore,
		config:      config,
	}
}

// Index indexes documents in the RAG system
func (rag *RAGSystem) Index(ctx context.Context, docs []*Document) error {
	chunks := []*Chunk{}
	for _, doc := range docs {
		docChunks, err := rag.chunker.Chunk(ctx, doc, rag.config.ChunkingStrategy)
		if err != nil {
			return err
		}
		chunks = append(chunks, docChunks...)
	}

	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Content
	}

	embeddings, err := rag.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}

	for i, embedding := range embeddings {
		chunks[i].Embedding = embedding
	}

	return rag.vectorStore.Add(ctx, chunks)
}

// Search searches the RAG system
func (rag *RAGSystem) Search(ctx context.Context, query *QueryRequest) ([]*SearchResult, error) {
	embedding, err := rag.embedder.EmbedSingle(ctx, query.Query)
	if err != nil {
		return nil, err
	}

	results, err := rag.vectorStore.Search(ctx, embedding, query.TopK)
	if err != nil {
		return nil, err
	}

	if rag.reranker != nil {
		results, err = rag.reranker.Rerank(ctx, query.Query, results, query.TopK)
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}
