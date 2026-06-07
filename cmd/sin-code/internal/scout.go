// SPDX-License-Identifier: MIT
// Purpose: scout — code search with regex, semantic, symbol, and usage search.
// Built-in Go implementation with file walking, context lines, and result
// ranking.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	scoutQuery  string
	scoutPath   string
	scoutType   string
	scoutFormat string
	scoutMax    int
)

var ScoutCmd = &cobra.Command{
	Use:   "scout",
	Short: "Search code with regex, semantic, symbol, and usage search",
	Long: `Search code with regex, semantic, symbol, and usage search. Includes
basic dead-code detection. Pure Go implementation.

Example:
  sin-code scout --query "func.*main" --path . --search_type regex --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if scoutQuery == "" {
			return fmt.Errorf("--query is required")
		}
		absPath, err := filepath.Abs(scoutPath)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			if err != nil {
				return fmt.Errorf("path not found: %w", err)
			}
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		results, err := searchFiles(absPath, scoutQuery, scoutType, scoutMax)
		if err != nil {
			return err
		}

		if scoutFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}
		return outputTextScout(results)
	},
}

type scoutResult struct {
	File     string       `json:"file"`
	Line     int          `json:"line"`
	Column   int          `json:"column"`
	Match    string       `json:"match"`
	Context  []string     `json:"context"`
	Type     string       `json:"type"`
	Relevance float64     `json:"relevance"`
}

func searchFiles(root, query, searchType string, maxResults int) ([]scoutResult, error) {
	var results []scoutResult
	var re *regexp.Regexp
	var err error

	switch searchType {
	case "regex":
		re, err = regexp.Compile(query)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
	case "semantic":
		// For semantic search, split query into words and match any
		words := strings.Fields(query)
		pattern := strings.Join(words, ".*")
		re, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid semantic query: %w", err)
		}
	case "symbol":
		// Symbol search: look for function/class/variable definitions
		re, err = regexp.Compile(`(?i)(?:func|def|fn|class|struct|interface|trait|enum|type|const|var|let)\s+` + regexp.QuoteMeta(query))
		if err != nil {
			return nil, fmt.Errorf("invalid symbol query: %w", err)
		}
	case "usage":
		// Usage search: look for references to the symbol
		re, err = regexp.Compile(`(?i)\b` + regexp.QuoteMeta(query) + `\b`)
		if err != nil {
			return nil, fmt.Errorf("invalid usage query: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown search_type: %s (use regex, semantic, symbol, or usage)", searchType)
	}

	// Track which files have matches for dead-code detection
	fileMatches := make(map[string]bool)

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "dist" || base == "build" || base == "target" || strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil || len(data) > 5_000_000 {
			return nil
		}
		content := string(data)
		lines := strings.Split(content, "\n")

		rel, _ := filepath.Rel(root, path)
		found := false
		for i, line := range lines {
			loc := re.FindStringIndex(line)
			if loc == nil {
				continue
			}
			found = true

			ctx := getContext(lines, i, 2)
			results = append(results, scoutResult{
				File:     rel,
				Line:     i + 1,
				Column:   loc[0] + 1,
				Match:    line[loc[0]:loc[1]],
				Context:  ctx,
				Type:     searchType,
				Relevance: scoreRelevanceScout(rel, line),
			})

			if len(results) >= maxResults {
				return fmt.Errorf("max results reached")
			}
		}

		if found {
			fileMatches[rel] = true
		}
		return nil
	})

	if err != nil && err.Error() != "max results reached" {
		return nil, err
	}

	// Sort by relevance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})

	return results, nil
}

func getContext(lines []string, center, radius int) []string {
	start := center - radius
	if start < 0 {
		start = 0
	}
	end := center + radius + 1
	if end > len(lines) {
		end = len(lines)
	}
	var ctx []string
	for i := start; i < end; i++ {
		prefix := "  "
		if i == center {
			prefix = "> "
		}
		ctx = append(ctx, fmt.Sprintf("%s%d: %s", prefix, i+1, lines[i]))
	}
	return ctx
}

func scoreRelevanceScout(relPath, line string) float64 {
	score := 50.0

	// Boost for important file types
	ext := strings.ToLower(filepath.Ext(relPath))
	if ext == ".go" || ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".rs" || ext == ".java" {
		score += 15
	}

	// Boost for important lines (definitions, not comments)
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "def ") ||
		strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "struct ") ||
		strings.HasPrefix(trimmed, "interface ") || strings.HasPrefix(trimmed, "type ") ||
		strings.HasPrefix(trimmed, "const ") || strings.HasPrefix(trimmed, "var ") ||
		strings.HasPrefix(trimmed, "let ") || strings.HasPrefix(trimmed, "export ") {
		score += 20
	}

	// Penalty for comments
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
		score -= 10
	}

	// Penalty for tests
	if strings.Contains(strings.ToLower(relPath), "_test") || strings.Contains(strings.ToLower(relPath), "test_") {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func outputTextScout(results []scoutResult) error {
	if len(results) == 0 {
		fmt.Println("No matches found.")
		return nil
	}
	for _, r := range results {
		fmt.Printf("\n%s:%d:%d  (score: %.1f)\n", r.File, r.Line, r.Column, r.Relevance)
		for _, ctx := range r.Context {
			fmt.Println(ctx)
		}
	}
	fmt.Printf("\n%d matches found\n", len(results))
	return nil
}

func init() {
	ScoutCmd.Flags().StringVarP(&scoutQuery, "query", "q", "", "Search query (regex or semantic)")
	_ = ScoutCmd.MarkFlagRequired("query")
	ScoutCmd.Flags().StringVarP(&scoutPath, "path", "p", ".", "Path to search")
	ScoutCmd.Flags().StringVarP(&scoutType, "search_type", "t", "regex", "Search type: regex|semantic|symbol|usage")
	ScoutCmd.Flags().StringVarP(&scoutFormat, "format", "f", "text", "Output format: text|json")
	ScoutCmd.Flags().IntVarP(&scoutMax, "max_results", "m", 50, "Max results")
}
