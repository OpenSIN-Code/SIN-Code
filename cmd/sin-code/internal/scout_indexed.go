// SPDX-License-Identifier: MIT

package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// scoutSearchAuto is the production search entry point.
// It checks for a cached index, auto-refreshes if stale, and delegates.
func scoutSearchAuto(root, query, searchType string, maxResults int, noRG bool) ([]scoutResult, error) {
	// For regex queries with metacharacters, the trigram index cannot prune
	// candidates. Fall back to the full-scan path to preserve accuracy.
	if searchType == "regex" && regexp.QuoteMeta(query) != query {
		return searchFiles(root, query, searchType, maxResults, noRG)
	}

	idx, existed, err := getFileIndex(root)
	if err != nil {
		return nil, fmt.Errorf("index load: %w", err)
	}

	if !existed {
		idx, err = buildIndex(root)
		if err != nil {
			return nil, fmt.Errorf("index build: %w", err)
		}
		if err := saveIndex(idx); err != nil {
			return nil, fmt.Errorf("index save: %w", err)
		}
		setFileIndex(idx)
	} else {
		idx, _, _, err = refreshIndex(idx)
		if err != nil {
			return nil, fmt.Errorf("index refresh: %w", err)
		}
		if err := saveIndex(idx); err != nil {
			return nil, fmt.Errorf("index save: %w", err)
		}
		setFileIndex(idx)
	}

	return searchWithIndex(idx, root, query, searchType, maxResults, noRG)
}

func searchWithIndex(idx *inMemoryIndex, root, query, searchType string, maxResults int, noRG bool) ([]scoutResult, error) {
	var candidates []string

	switch searchType {
	case "regex":
		candidates = idx.searchTrigram(query)
	case "semantic":
		candidates = idx.searchTrigram(query)
	case "symbol":
		parts := strings.Fields(query)
		var name, stype string
		if len(parts) >= 2 {
			name = parts[len(parts)-1]
			stype = parts[0]
		} else {
			name = query
		}
		candidates = idx.searchSymbols(name, stype)
	case "usage":
		candidates = idx.searchTrigram(query)
	default:
		return nil, fmt.Errorf("unknown search_type: %s", searchType)
	}

	re, err := compileQuery(query, searchType)
	if err != nil {
		return nil, err
	}

	var results []scoutResult
	targetMax := maxResults
	if targetMax == 0 {
		targetMax = 50
	}
	overScan := targetMax * 4

	for _, relPath := range candidates {
		absPath := filepath.Join(root, relPath)
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		if isBinaryFile(absPath) {
			continue
		}
		fileResults, err := searchFile(absPath, relPath, root, re, searchType)
		if err != nil {
			continue
		}
		results = append(results, fileResults...)
		if len(results) >= overScan {
			break
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})
	if len(results) > targetMax {
		results = results[:targetMax]
	}
	return results, nil
}
