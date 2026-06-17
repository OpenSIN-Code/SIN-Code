// SPDX-License-Identifier: MIT
package tui

import (
	"sort"
	"strings"
	"unicode"
)

type fuzzyResult struct {
	Item    string
	Score   int
	Indices []int
}

// fuzzySubsequenceMatch returns true if query is a subsequence of target (case-insensitive).
// Also returns match indices for highlighting.
func fuzzySubsequenceMatch(target, query string) (bool, []int) {
	if query == "" {
		return true, nil
	}

	tRunes := []rune(strings.ToLower(target))
	qRunes := []rune(strings.ToLower(query))

	var indices []int
	ti := 0
	for _, q := range qRunes {
		found := false
		for ti < len(tRunes) {
			if tRunes[ti] == q {
				indices = append(indices, ti)
				ti++
				found = true
				break
			}
			ti++
		}
		if !found {
			return false, nil
		}
	}
	return true, indices
}

// fuzzyScore scores a match based on:
// - Consecutive matches (+5 each)
// - Match at word boundary (+8)
// - Match at string start (+10)
// - Match after separator (+3)
// - Earlier matches score slightly higher
func fuzzyScore(target string, indices []int) int {
	if len(indices) == 0 {
		return 0
	}

	score := 0
	tRunes := []rune(target)

	for i, idx := range indices {
		if idx == 0 {
			score += 10
		} else if i > 0 && idx == indices[i-1]+1 {
			score += 5
		} else {
			prev := tRunes[idx-1]
			if prev == ' ' || prev == '_' || prev == '-' || prev == ':' || prev == '/' {
				score += 8
			} else if unicode.IsUpper(prev) && unicode.IsLower(tRunes[idx]) {
				score += 3
			}
		}
		score += max(0, 3-(idx/10))
	}

	return score
}

// fuzzyFilter returns items matching query, sorted by score (best first).
func fuzzyFilter(items []string, query string) []fuzzyResult {
	var matches []fuzzyResult
	for _, item := range items {
		ok, indices := fuzzySubsequenceMatch(item, query)
		if !ok {
			continue
		}
		matches = append(matches, fuzzyResult{
			Item:    item,
			Score:   fuzzyScore(item, indices),
			Indices: indices,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})
	return matches
}

// fuzzyHighlight wraps matched characters with highlight styling.
// Returns the item string with matched positions marked.
func fuzzyHighlight(item string, indices []int, normal, highlight string) string {
	if len(indices) == 0 {
		return item
	}
	idxSet := make(map[int]bool, len(indices))
	for _, idx := range indices {
		idxSet[idx] = true
	}

	var b strings.Builder
	runes := []rune(item)
	for i, r := range runes {
		if idxSet[i] {
			b.WriteString(highlight)
			b.WriteRune(r)
			b.WriteString("\x1b[0m")
		} else {
			b.WriteString(normal)
			b.WriteRune(r)
		}
	}
	return b.String()
}
