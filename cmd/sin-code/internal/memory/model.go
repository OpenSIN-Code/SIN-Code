// SPDX-License-Identifier: MIT
// Purpose: long-term project memory — bbolt-backed knowledge store with
// semantic search via NIM embeddings. Inspired by beads (gastownhall/beads).
package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Memory struct {
	ID        string    `json:"id"`
	Insight   string    `json:"insight"`
	Project   string    `json:"project,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	Embedding []float32 `json:"-"`
}

type Link struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	Rel      string    `json:"rel"`
	Created  time.Time `json:"created"`
}

type LinkType string

const (
	LinkReferences LinkType = "references"
	LinkSupports   LinkType = "supports"
	LinkContradicts LinkType = "contradicts"
	LinkExtends    LinkType = "extends"
	LinkCauses     LinkType = "causes"
)

var ValidLinkTypes = []LinkType{LinkReferences, LinkSupports, LinkContradicts, LinkExtends, LinkCauses}

func (l LinkType) Valid() bool {
	for _, v := range ValidLinkTypes {
		if v == l {
			return true
		}
	}
	return false
}

func GenerateID(insight string) string {
	h := sha256.Sum256([]byte(insight + fmt.Sprintf("%d", time.Now().UnixNano())))
	return "mem-" + hex.EncodeToString(h[:6])
}

func NormalizeTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

type EmbeddingFunc func(text string) ([]float32, error)

// NoopEmbedding returns an empty vector — used when no embedder is configured.
func NoopEmbedding(text string) ([]float32, error) {
	return nil, nil
}

var (
	mu          sync.RWMutex
	embedder    EmbeddingFunc
	embedDim    int
	embeddingMu sync.RWMutex
)

func SetEmbedder(fn EmbeddingFunc, dim int) {
	mu.Lock()
	defer mu.Unlock()
	embedder = fn
	embedDim = dim
}

func GetEmbedder() (EmbeddingFunc, int) {
	mu.RLock()
	defer mu.RUnlock()
	return embedder, embedDim
}
