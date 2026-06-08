// SPDX-License-Identifier: MIT
// Purpose: semantic search, knowledge graph traversal, and prime-context
// injection for agents. All in-memory over the Store.
package memory

import (
	"math"
	"sort"
	"strings"
)

type ScoredMemory struct {
	*Memory
	Score float64
}

// Search returns memories ranked by similarity to query.
// If the embedder is configured and the query embeds, uses cosine
// similarity over stored embeddings. Falls back to substring search.
func (s *Store) Search(query string, project string, limit int) ([]ScoredMemory, error) {
	if limit <= 0 {
		limit = 10
	}
	all, err := s.List(ListFilter{Project: project, Limit: 1000})
	if err != nil {
		return nil, err
	}

	queryEmb, err := s.computeEmbedding(query)
	if err != nil || len(queryEmb) == 0 {
		return s.fallbackSearch(query, all, limit), nil
	}

	scores := make([]ScoredMemory, 0, len(all))
	for _, m := range all {
		if len(m.Embedding) == 0 {
			emb, _ := s.computeEmbedding(m.Insight)
			m.Embedding = emb
		}
		if len(m.Embedding) == 0 {
			continue
		}
		sim := CosineSimilarity(queryEmb, m.Embedding)
		scores = append(scores, ScoredMemory{Memory: m, Score: sim})
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].Score > scores[j].Score })
	if len(scores) > limit {
		scores = scores[:limit]
	}
	return scores, nil
}

func (s *Store) fallbackSearch(query string, all []*Memory, limit int) []ScoredMemory {
	needle := strings.ToLower(query)
	out := []ScoredMemory{}
	for _, m := range all {
		score := 0.0
		lower := strings.ToLower(m.Insight)
		if strings.Contains(lower, needle) {
			score = 1.0
		}
		for _, t := range m.Tags {
			if strings.Contains(needle, strings.ToLower(t)) {
				score += 0.5
			}
		}
		if score > 0 {
			out = append(out, ScoredMemory{Memory: m, Score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// Graph traverses the knowledge graph from a starting node, returning
// all reachable nodes within maxDepth hops.
func (s *Store) Graph(rootID string, maxDepth int) (map[string][]Link, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	out := map[string][]Link{}
	visited := map[string]bool{rootID: true}
	queue := []struct {
		id    string
		depth int
	}{{rootID, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth > maxDepth {
			continue
		}
		links, err := s.GetLinks(cur.id)
		if err != nil {
			return nil, err
		}
		out[cur.id] = links
		for _, l := range links {
			if !visited[l.To] {
				visited[l.To] = true
				queue = append(queue, struct {
					id    string
					depth int
				}{l.To, cur.depth + 1})
			}
		}
	}
	return out, nil
}

// Prime builds a context string with the top-K most relevant memories
// for the given query, formatted for injection into an LLM prompt.
func (s *Store) Prime(query string, project string, topK int) (string, error) {
	if topK <= 0 {
		topK = 10
	}
	results, err := s.Search(query, project, topK)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("# Relevant project memory\n\n")
	for i, r := range results {
		b.WriteString("- ")
		if r.Project != "" {
			b.WriteString("[" + r.Project + "] ")
		}
		b.WriteString(r.Insight)
		if len(r.Tags) > 0 {
			b.WriteString(" (tags: " + strings.Join(r.Tags, ", ") + ")")
		}
		b.WriteString("\n")
		_ = i
	}
	return b.String(), nil
}
