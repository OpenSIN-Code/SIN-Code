// SPDX-License-Identifier: MIT
// Purpose: MetaSpec for token optimization and intelligent spec selection.
// Builds searchable indexes, allocates token budgets, and optimizes context window usage (Phase 4).
// Docs: internal/spec/metaspec.go.doc.md
package spec

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// MetaSpec is a compressed, searchable index of specs for efficient retrieval.
// Optimizes context window usage by selecting only relevant specs.
type MetaSpec struct {
	ID               string                     `json:"id"`
	CollectionID     string                     `json:"collection_id"`
	IndexedSpecs     map[string]*SpecIndex      `json:"indexed_specs"`     // SpecID -> compressed index
	Keywords         map[string][]string       `json:"keywords"`          // Keyword -> list of SpecIDs
	TokenBudget      int                        `json:"token_budget"`
	UsedTokens       int                        `json:"used_tokens"`
	SearchIndex      *SearchIndex               `json:"search_index"`
	IndexedAt        int64                      `json:"indexed_at"`
	Version          int                        `json:"version"`
}

// SpecIndex is a compressed representation of a spec for fast retrieval.
type SpecIndex struct {
	SpecID          string           `json:"spec_id"`
	Title           string           `json:"title"`
	Kind            SpecKind         `json:"kind"`
	Namespace       string           `json:"namespace"`
	Status          SpecStatus       `json:"status"`
	TokenEstimate   int              `json:"token_estimate"`
	TokenActual     int              `json:"token_actual"`
	Hash            string           `json:"hash"`
	Priority        int              `json:"priority"`
	Keywords        []string         `json:"keywords"`
	DependencyCount int              `json:"dependency_count"`
	DependentCount  int              `json:"dependent_count"`
	Score           float64          `json:"relevance_score"` // Computed relevance score
	Summary         string           `json:"summary"`         // First 200 chars of description
}

// SearchIndex enables full-text search over indexed specs.
type SearchIndex struct {
	// Inverted index: normalized_term -> list of SpecIDs
	Terms      map[string][]string `json:"terms"`
	// N-grams for fuzzy matching: ngram -> list of SpecIDs
	Ngrams     map[string][]string `json:"ngrams"`
}

// SpecIndexer builds and maintains the MetaSpec index.
type SpecIndexer struct {
	Collection *SpecCollection
	MetaSpec   *MetaSpec
	// Configuration
	MaxTokenBudget int
	MinPriority    int
	Keywords       map[string][]string // Predefined keywords per spec
}

// NewSpecIndexer creates a new indexer for a collection.
func NewSpecIndexer(collection *SpecCollection, maxTokens int) *SpecIndexer {
	return &SpecIndexer{
		Collection:     collection,
		MaxTokenBudget: maxTokens,
		Keywords:       make(map[string][]string),
		MetaSpec: &MetaSpec{
			ID:           fmt.Sprintf("metaspec_%d", time.Now().UnixNano()),
			CollectionID: collection.ID,
			IndexedSpecs: make(map[string]*SpecIndex),
			Keywords:     make(map[string][]string),
			TokenBudget:  maxTokens,
			SearchIndex:  &SearchIndex{Terms: make(map[string][]string), Ngrams: make(map[string][]string)},
		},
	}
}

// BuildIndex constructs the full metaspec index.
func (si *SpecIndexer) BuildIndex() error {
	si.MetaSpec.IndexedSpecs = make(map[string]*SpecIndex)
	si.MetaSpec.Keywords = make(map[string][]string)
	si.MetaSpec.SearchIndex = &SearchIndex{Terms: make(map[string][]string), Ngrams: make(map[string][]string)}

	for id, spec := range si.Collection.Specs {
		if spec == nil {
			continue
		}

		// Create compressed index
		index := si.indexSpec(id, spec)
		si.MetaSpec.IndexedSpecs[id] = index

		// Add to search index
		si.addToSearchIndex(index)

		// Add keywords
		if keywords, ok := si.Keywords[id]; ok {
			si.MetaSpec.Keywords[id] = keywords
			for _, kw := range keywords {
				si.MetaSpec.Keywords[kw] = append(si.MetaSpec.Keywords[kw], id)
			}
		}
	}

	si.MetaSpec.IndexedAt = int64(time.Now().Unix())
	si.MetaSpec.Version++

	return nil
}

// indexSpec creates a compressed index for a single spec.
func (si *SpecIndexer) indexSpec(id string, spec *Spec) *SpecIndex {
	// Extract summary (first 200 chars of description)
	summary := spec.Description
	if len(summary) > 200 {
		summary = summary[:200] + "..."
	}

	// Extract keywords from spec content
	keywords := si.extractKeywords(spec)

	// Calculate relevance score (0.0-1.0)
	score := si.calculateRelevance(spec)

	return &SpecIndex{
		SpecID:          id,
		Title:           spec.Title,
		Kind:            spec.Kind,
		Namespace:       spec.Namespace,
		Status:          spec.Status,
		TokenEstimate:   spec.TokenEstimate,
		TokenActual:     spec.TokenActual,
		Hash:            spec.Hash,
		Priority:        spec.Priority,
		Keywords:        keywords,
		DependencyCount: len(spec.Dependencies),
		DependentCount:  len(spec.Dependents),
		Score:           score,
		Summary:         summary,
	}
}

// extractKeywords extracts keywords from a spec.
func (si *SpecIndexer) extractKeywords(spec *Spec) []string {
	var keywords []string

	// Add title words
	titleWords := strings.Fields(strings.ToLower(spec.Title))
	keywords = append(keywords, titleWords...)

	// Add namespace
	if spec.Namespace != "" {
		keywords = append(keywords, strings.ToLower(spec.Namespace))
	}

	// Add kind
	keywords = append(keywords, string(spec.Kind))

	// Add selected words from goals and constraints
	goalWords := extractImportantWords(spec.Goals)
	keywords = append(keywords, goalWords...)

	constraintWords := extractImportantWords(spec.Constraints)
	keywords = append(keywords, constraintWords...)

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, kw := range keywords {
		if !seen[kw] && len(kw) > 2 {
			unique = append(unique, kw)
			seen[kw] = true
		}
	}

	return unique
}

// extractImportantWords extracts important words from text (simple heuristic).
func extractImportantWords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var important []string

	// Simple heuristic: words after dashes, in uppercase, or uncommon
	for _, w := range words {
		if len(w) > 4 || strings.Contains(w, "-") {
			important = append(important, strings.ToLower(w))
		}
	}

	return important
}

// addToSearchIndex adds a spec to the search index.
func (si *SpecIndexer) addToSearchIndex(index *SpecIndex) {
	// Index title
	terms := strings.Fields(strings.ToLower(index.Title))
	for _, term := range terms {
		term = strings.TrimPunctuation(term)
		if len(term) > 2 {
			si.MetaSpec.SearchIndex.Terms[term] = append(si.MetaSpec.SearchIndex.Terms[term], index.SpecID)
		}
	}

	// Index keywords
	for _, kw := range index.Keywords {
		si.MetaSpec.SearchIndex.Terms[strings.ToLower(kw)] = append(si.MetaSpec.SearchIndex.Terms[strings.ToLower(kw)], index.SpecID)
	}

	// Build 3-grams for fuzzy matching
	fullText := index.Title + " " + index.Summary
	for i := 0; i < len(fullText)-2; i++ {
		ngram := fullText[i : i+3]
		si.MetaSpec.SearchIndex.Ngrams[ngram] = append(si.MetaSpec.SearchIndex.Ngrams[ngram], index.SpecID)
	}
}

// calculateRelevance computes a relevance score for a spec.
func (si *SpecIndexer) calculateRelevance(spec *Spec) float64 {
	score := 0.0

	// Active specs get higher score
	if spec.Status == SpecStatusActive {
		score += 0.5
	}

	// Higher priority = higher relevance
	score += float64(spec.Priority) / 10.0 // Assume priority 0-10

	// Specs with fewer tokens get higher relevance (cheaper)
	if spec.TokenEstimate > 0 {
		score += 0.3 / math.Log(float64(spec.TokenEstimate)+1)
	}

	// Clamp to 0-1
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}

// SearchByKeyword searches for specs matching a keyword.
func (ms *MetaSpec) SearchByKeyword(keyword string) []*SpecIndex {
	keyword = strings.ToLower(keyword)
	specIDs := ms.SearchIndex.Terms[keyword]

	var results []*SpecIndex
	for _, id := range specIDs {
		if index, ok := ms.IndexedSpecs[id]; ok {
			results = append(results, index)
		}
	}

	// Sort by relevance score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// SelectByBudget selects specs to include within a token budget.
// Returns a subset of specs prioritized by relevance and priority.
func (ms *MetaSpec) SelectByBudget(tokenBudget int, maxSpecs int) []*SpecIndex {
	// Sort by priority and relevance
	var all []*SpecIndex
	for _, index := range ms.IndexedSpecs {
		all = append(all, index)
	}

	sort.Slice(all, func(i, j int) bool {
		// Primary: priority (descending)
		if all[i].Priority != all[j].Priority {
			return all[i].Priority > all[j].Priority
		}
		// Secondary: relevance score (descending)
		return all[i].Score > all[j].Score
	})

	// Select until budget exhausted
	var selected []*SpecIndex
	usedTokens := 0

	for _, index := range all {
		if len(selected) >= maxSpecs {
			break
		}
		if usedTokens+index.TokenEstimate <= tokenBudget {
			selected = append(selected, index)
			usedTokens += index.TokenEstimate
		}
	}

	return selected
}

// SelectByNamespace selects all specs in a given namespace.
func (ms *MetaSpec) SelectByNamespace(namespace string) []*SpecIndex {
	var results []*SpecIndex
	for _, index := range ms.IndexedSpecs {
		if index.Namespace == namespace {
			results = append(results, index)
		}
	}
	return results
}

// SelectByKind selects all specs of a given kind.
func (ms *MetaSpec) SelectByKind(kind SpecKind) []*SpecIndex {
	var results []*SpecIndex
	for _, index := range ms.IndexedSpecs {
		if index.Kind == kind {
			results = append(results, index)
		}
	}
	return results
}

// SelectByStatus selects specs with a specific status.
func (ms *MetaSpec) SelectByStatus(status SpecStatus) []*SpecIndex {
	var results []*SpecIndex
	for _, index := range ms.IndexedSpecs {
		if index.Status == status {
			results = append(results, index)
		}
	}
	return results
}

// ─────────────────────────────────────────────────────────────────────
// Token Budgeter
// ─────────────────────────────────────────────────────────────────────

// TokenBudgeter allocates token budgets to specs in a collection.
type TokenBudgeter struct {
	TotalBudget    int
	SpecCount      int
	PerSpecQuota   int
	ReserveBudget  int // Percent reserved for agent overhead
}

// NewTokenBudgeter creates a new token budgeter.
func NewTokenBudgeter(totalBudget int, specCount int, reservePercent int) *TokenBudgeter {
	if reservePercent < 0 {
		reservePercent = 0
	}
	if reservePercent > 100 {
		reservePercent = 100
	}

	reserveBudget := (totalBudget * reservePercent) / 100
	availableBudget := totalBudget - reserveBudget
	perSpecQuota := 0

	if specCount > 0 {
		perSpecQuota = availableBudget / specCount
	}

	return &TokenBudgeter{
		TotalBudget:   totalBudget,
		SpecCount:     specCount,
		PerSpecQuota:  perSpecQuota,
		ReserveBudget: reserveBudget,
	}
}

// AllocateProportional allocates budgets proportionally to current estimates.
func (tb *TokenBudgeter) AllocateProportional(specs []*Spec) map[string]int {
	allocation := make(map[string]int)

	if len(specs) == 0 {
		return allocation
	}

	// Calculate total estimated tokens
	totalEstimate := 0
	for _, s := range specs {
		totalEstimate += s.TokenEstimate
	}

	if totalEstimate == 0 {
		// Equal distribution
		perSpec := (tb.TotalBudget - tb.ReserveBudget) / len(specs)
		for _, s := range specs {
			allocation[s.ID] = perSpec
		}
	} else {
		// Proportional distribution
		availableBudget := tb.TotalBudget - tb.ReserveBudget
		for _, s := range specs {
			proportion := float64(s.TokenEstimate) / float64(totalEstimate)
			allocation[s.ID] = int(float64(availableBudget) * proportion)
		}
	}

	return allocation
}

// AllocatePriority allocates budgets with higher priorities getting more tokens.
func (tb *TokenBudgeter) AllocatePriority(specs []*Spec) map[string]int {
	allocation := make(map[string]int)

	if len(specs) == 0 {
		return allocation
	}

	// Calculate total priority
	totalPriority := 0
	for _, s := range specs {
		totalPriority += s.Priority
	}

	if totalPriority == 0 {
		// Fallback to equal distribution
		perSpec := (tb.TotalBudget - tb.ReserveBudget) / len(specs)
		for _, s := range specs {
			allocation[s.ID] = perSpec
		}
	} else {
		// Priority-weighted distribution
		availableBudget := tb.TotalBudget - tb.ReserveBudget
		for _, s := range specs {
			proportion := float64(s.Priority) / float64(totalPriority)
			allocation[s.ID] = int(float64(availableBudget) * proportion)
		}
	}

	return allocation
}

// Summary returns a text summary of token allocation.
func (tb *TokenBudgeter) Summary() string {
	return fmt.Sprintf("Token Budget: %d total | %d reserve | %d per-spec quota | %d specs",
		tb.TotalBudget, tb.ReserveBudget, tb.PerSpecQuota, tb.SpecCount)
}

// Helper function to trim punctuation from strings
// (Not in stdlib, so we define it locally)
func (s string) TrimPunctuation(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_')
	})
}
