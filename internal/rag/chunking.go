package rag

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// QASCChunker implements QASC (Quantile-Adaptive Sentence Chunking)
type QASCChunker struct{}

// NewQASCChunker creates a new QASC chunker
func NewQASCChunker() *QASCChunker {
	return &QASCChunker{}
}

// Chunk chunks a document using QASC strategy
func (q *QASCChunker) Chunk(ctx context.Context, doc *Document, strategy *ChunkingStrategy) ([]*Chunk, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}

	if strategy == nil {
		strategy = &ChunkingStrategy{
			Name:    "default",
			MinSize: 256,
			MaxSize: 2048,
			Overlap: 100,
			SplitOn: "sentence",
		}
	}

	sentences := q.splitIntoSentences(doc.Content)
	chunks := []*Chunk{}
	currentChunk := ""
	chunkIndex := 0

	for _, sentence := range sentences {
		testChunk := currentChunk + " " + sentence
		tokenCount := q.countTokens(testChunk)

		if tokenCount > strategy.MaxSize {
			if currentChunk != "" {
				chunk := q.createChunk(doc.ID, currentChunk, chunkIndex, strategy)
				chunks = append(chunks, chunk)
				chunkIndex++
				currentChunk = sentence
			} else {
				chunk := q.createChunk(doc.ID, sentence, chunkIndex, strategy)
				chunks = append(chunks, chunk)
				chunkIndex++
			}
		} else {
			currentChunk = testChunk
		}
	}

	if currentChunk != "" {
		chunk := q.createChunk(doc.ID, currentChunk, chunkIndex, strategy)
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

func (q *QASCChunker) splitIntoSentences(text string) []string {
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	return sentences
}

func (q *QASCChunker) countTokens(text string) int {
	words := strings.Fields(text)
	return len(words)
}

func (q *QASCChunker) createChunk(docID, content string, index int, strategy *ChunkingStrategy) *Chunk {
	chunkID := fmt.Sprintf("%s_chunk_%d", docID, index)
	tokenCount := q.countTokens(content)

	return &Chunk{
		ID:         chunkID,
		DocumentID: docID,
		Content:    strings.TrimSpace(content),
		Tokens:     tokenCount,
		ChunkType:  strategy.SplitOn,
	}
}

// SentenceChunker implements basic sentence-level chunking
type SentenceChunker struct{}

// NewSentenceChunker creates a new sentence chunker
func NewSentenceChunker() *SentenceChunker {
	return &SentenceChunker{}
}

// Chunk chunks a document by sentences
func (s *SentenceChunker) Chunk(ctx context.Context, doc *Document, strategy *ChunkingStrategy) ([]*Chunk, error) {
	sentences := strings.Split(doc.Content, ".")
	chunks := []*Chunk{}

	for i, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		chunk := &Chunk{
			ID:         fmt.Sprintf("%s_chunk_%d", doc.ID, i),
			DocumentID: doc.ID,
			Content:    sentence,
			Tokens:     len(strings.Fields(sentence)),
			ChunkType:  "sentence",
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// SemanticChunker implements semantic chunking
type SemanticChunker struct {
	embedder Embedder
}

// NewSemanticChunker creates a new semantic chunker
func NewSemanticChunker(embedder Embedder) *SemanticChunker {
	return &SemanticChunker{embedder: embedder}
}

// Chunk chunks a document semantically
func (s *SemanticChunker) Chunk(ctx context.Context, doc *Document, strategy *ChunkingStrategy) ([]*Chunk, error) {
	sentences := strings.Split(doc.Content, ".")
	chunks := []*Chunk{}
	currentGroup := []string{}

	for i, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		currentGroup = append(currentGroup, sentence)
		groupText := strings.Join(currentGroup, ". ")
		tokenCount := len(strings.Fields(groupText))

		if tokenCount >= strategy.MaxSize {
			chunk := &Chunk{
				ID:         fmt.Sprintf("%s_chunk_%d", doc.ID, len(chunks)),
				DocumentID: doc.ID,
				Content:    strings.TrimSpace(groupText),
				Tokens:     tokenCount,
				ChunkType:  "semantic",
			}
			chunks = append(chunks, chunk)
			currentGroup = []string{}
		} else if i == len(sentences)-1 {
			chunk := &Chunk{
				ID:         fmt.Sprintf("%s_chunk_%d", doc.ID, len(chunks)),
				DocumentID: doc.ID,
				Content:    strings.TrimSpace(groupText),
				Tokens:     tokenCount,
				ChunkType:  "semantic",
			}
			chunks = append(chunks, chunk)
		}
	}

	return chunks, nil
}
