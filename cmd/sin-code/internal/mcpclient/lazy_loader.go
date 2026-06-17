// SPDX-License-Identifier: MIT
// Purpose: Lazy tool loading (issue #270). Instead of sending all 47+ tool
// definitions in every API call (~134K tokens), the LazyToolLoader maintains
// a searchable index. The LLM calls tool_search to discover relevant tools
// on demand, reducing tool-prompt tokens to ~5K.
package mcpclient

import (
	"sort"
	"strings"
	"sync"
)

// ToolSpec is the lazy-loader's internal tool representation. It mirrors
// agentloop.ToolSpec so callers can convert with a simple field copy.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// LazyToolLoader maintains a searchable keyword index of tool specs.
// Thread-safe (mandate M7).
type LazyToolLoader struct {
	mu    sync.RWMutex
	specs []ToolSpec
}

// NewLazyToolLoader creates a loader from the given specs. The input slice
// is copied so the caller's slice is never mutated.
func NewLazyToolLoader(specs []ToolSpec) *LazyToolLoader {
	copied := make([]ToolSpec, len(specs))
	copy(copied, specs)
	return &LazyToolLoader{specs: copied}
}

// Search returns up to k tools whose name or description match the query.
// Scoring: exact name match (10), name substring (5), description
// substring (1). Results are sorted by score descending. Returns nil for
// an empty query or k <= 0.
func (l *LazyToolLoader) Search(query string, k int) []ToolSpec {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || k <= 0 {
		return nil
	}
	tokens := strings.Fields(query)

	l.mu.RLock()
	defer l.mu.RUnlock()

	type scored struct {
		spec  ToolSpec
		score int
	}
	var results []scored

	for _, spec := range l.specs {
		name := strings.ToLower(spec.Name)
		desc := strings.ToLower(spec.Description)
		score := 0

		for _, token := range tokens {
			if name == token {
				score += 10
			}
			if strings.Contains(name, token) {
				score += 5
			}
			if strings.Contains(desc, token) {
				score += 1
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
func (l *LazyToolLoader) All() []ToolSpec {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]ToolSpec(nil), l.specs...)
}

// Count returns the number of indexed specs.
func (l *LazyToolLoader) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.specs)
}

// ToolSearchSpec returns the tool_search meta-tool spec. When lazy loading
// is enabled, this is the only spec sent to the LLM initially. The LLM
// calls tool_search to discover and load real tools on demand.
func ToolSearchSpec() ToolSpec {
	return ToolSpec{
		Name:        "tool_search",
		Description: "Search available tools by keyword. Returns matching tool definitions.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search keywords to find relevant tools",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of tools to return (default 10)",
					"default":     10,
				},
			},
			"required": []string{"query"},
		},
	}
}
