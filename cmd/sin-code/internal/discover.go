// SPDX-License-Identifier: MIT
// Purpose: discover — file discovery with relevance scoring, pattern matching,
// and dependency analysis. Built-in Go implementation (no external binary
// dependency).
package internal

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	discoverPattern string
	discoverSort    string
	discoverFormat  string
	discoverLimit   int
)

var DiscoverCmd = &cobra.Command{
	Use:   "discover [path]",
	Short: "Discover files with relevance scoring and pattern matching",
	Long: `Discover files in a directory with relevance scoring, pattern matching,
and dependency analysis. Pure Go implementation — no external binary needed.

Example:
  sin-code discover . --pattern "**/*.go" --sort_by relevance --format json`,
	Args: cobra.ArbitraryArgs,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			if err != nil {
				return fmt.Errorf("path not found: %w", err)
			}
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		results, err := discoverFiles(absPath, discoverPattern, discoverLimit)
		if err != nil {
			return err
		}

		sortResults(results, discoverSort)
		if len(results) > discoverLimit {
			results = results[:discoverLimit]
		}

		if discoverFormat == "json" {
			return outputJSON(results)
		}
		return outputText(results)
	},
}

type fileResult struct {
	Path         string   `json:"path"`
	RelPath      string   `json:"rel_path"`
	Size         int64    `json:"size"`
	ModTime      string   `json:"mod_time"`
	Relevance    float64  `json:"relevance"`
	Dependencies []string `json:"dependencies,omitempty"`
}

func discoverFiles(root, pattern string, limit int) ([]fileResult, error) {
	matcher, err := buildGlobMatcher(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	var results []fileResult
	walked := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == ".venv" || base == "__pycache__" || base == ".pytest_cache" || base == "dist" || base == "build" || base == "target" || base == ".idea" || base == ".vscode" || strings.HasPrefix(base, ".") && (base == ".DS_Store" || base == ".gitignore") {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		if !matcher(rel) {
			return nil
		}

		fr := fileResult{
			Path:    path,
			RelPath: rel,
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		}
		fr.Relevance = scoreRelevance(rel, info.Size())
		fr.Dependencies = extractDependencies(path)
		results = append(results, fr)

		walked++
		if walked > limit*10 && len(results) > limit {
			// Early stop if we have enough results
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func buildGlobMatcher(pattern string) (func(string) bool, error) {
	// Convert glob pattern to regex
	// ** matches any number of directories
	// * matches any characters except /
	// ? matches single character
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "**/*" {
		return func(s string) bool { return true }, nil
	}

	// Replace ** with placeholder
	pattern = strings.ReplaceAll(pattern, "**", "{{DOUBLESTAR}}")
	var reParts []string
	parts := strings.Split(pattern, "/")
	for _, part := range parts {
		if part == "{{DOUBLESTAR}}" {
			reParts = append(reParts, ".*")
		} else {
			reParts = append(reParts, globToRegex(part))
		}
	}
	rePattern := "^" + strings.Join(reParts, "/") + "$"
	re, err := regexp.Compile(rePattern)
	if err != nil {
		return nil, err
	}
	return func(s string) bool {
		return re.MatchString(s)
	}, nil
}

func globToRegex(glob string) string {
	var b strings.Builder
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.':
			b.WriteString("\\.")
		case '+', '(', ')', '[', ']', '{', '}', '^', '$', '|':
			b.WriteString("\\")
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func scoreRelevance(relPath string, size int64) float64 {
	score := 50.0

	// Proximity to root (shorter paths = more relevant)
	depth := strings.Count(relPath, string(filepath.Separator))
	score -= float64(depth) * 5.0

	// File extension bonus
	ext := strings.ToLower(filepath.Ext(relPath))
	bonus := map[string]float64{
		".go":   15, ".py": 15, ".js": 12, ".ts": 14, ".tsx": 12,
		".rs":   14, ".java": 10, ".c": 8, ".cpp": 10, ".h": 8,
		".md":   10, ".json": 5, ".yaml": 8, ".yml": 8, ".toml": 8,
		".sh":   8, ".dockerfile": 5, ".mod": 10, ".sum": 3,
	}
	if b, ok := bonus[ext]; ok {
		score += b
	}

	// Important filename keywords
	name := strings.ToLower(filepath.Base(relPath))
	keywords := map[string]float64{
		"main": 20, "index": 15, "app": 15, "config": 15, "server": 12,
		"router": 12, "handler": 10, "middleware": 10, "model": 10,
		"service": 10, "controller": 10, "test": 8, "spec": 8,
		"readme": 15, "license": 5, "makefile": 10, "dockerfile": 8,
		"compose": 8, "go.mod": 15, "package.json": 12, "requirements": 10,
	}
	for kw, b := range keywords {
		if strings.Contains(name, kw) {
			score += b
		}
	}

	// Penalty for very large files (likely generated or data)
	if size > 1_000_000 {
		score -= 20
	} else if size > 100_000 {
		score -= 10
	}

	// Penalty for certain paths
	lowerPath := strings.ToLower(relPath)
	penalties := []string{"vendor/", "node_modules/", "dist/", "build/", "__pycache__/", ".git/", "target/", ".next/", "coverage/"}
	for _, p := range penalties {
		if strings.Contains(lowerPath, p) {
			score -= 30
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func extractDependencies(path string) []string {
	ext := strings.ToLower(filepath.Ext(path))
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 500_000 {
		return nil
	}
	content := string(data)

	var deps []string
	switch ext {
	case ".go":
		deps = extractGoImports(content, path)
	case ".py":
		deps = extractPythonImports(content)
	case ".js", ".ts", ".tsx", ".jsx":
		deps = extractJSImports(content)
	}

	if len(deps) > 20 {
		deps = deps[:20]
	}
	return deps
}

func extractGoImports(content, path string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	var deps []string
	for _, imp := range f.Imports {
		deps = append(deps, strings.Trim(imp.Path.Value, `"`))
	}
	return deps
}

func extractPythonImports(content string) []string {
	var deps []string
	seen := make(map[string]bool)
	re := regexp.MustCompile(`^(?:import|from)\s+([a-zA-Z_][a-zA-Z0-9_.]*)`)
	for _, line := range strings.Split(content, "\n") {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			pkg := matches[1]
			if strings.Contains(pkg, ".") {
				pkg = strings.Split(pkg, ".")[0]
			}
			if !seen[pkg] && pkg != "" {
				seen[pkg] = true
				deps = append(deps, pkg)
			}
		}
	}
	return deps
}

func extractJSImports(content string) []string {
	var deps []string
	seen := make(map[string]bool)
	re := regexp.MustCompile(`(?:import\s+.*?\s+from\s+['"]([^'"]+)['"]|require\s*\(\s*['"]([^'"]+)['"]\s*\))`)
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		for i := 1; i < len(match); i++ {
			if match[i] != "" && !seen[match[i]] {
				seen[match[i]] = true
				deps = append(deps, match[i])
			}
		}
	}
	return deps
}

func sortResults(results []fileResult, sortBy string) {
	switch sortBy {
	case "name":
		sort.Slice(results, func(i, j int) bool {
			return results[i].RelPath < results[j].RelPath
		})
	case "size":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Size > results[j].Size
		})
	case "mtime":
		sort.Slice(results, func(i, j int) bool {
			return results[i].ModTime > results[j].ModTime
		})
	default: // relevance
		sort.Slice(results, func(i, j int) bool {
			return results[i].Relevance > results[j].Relevance
		})
	}
}

func outputJSON(results []fileResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func outputText(results []fileResult) error {
	for _, r := range results {
		fmt.Printf("%s  (score: %.1f, size: %d, deps: %d)\n",
			r.RelPath, r.Relevance, r.Size, len(r.Dependencies))
	}
	return nil
}

func init() {
	RegisterVersionCmd(DiscoverCmd)
	DiscoverCmd.Flags().StringVarP(&discoverPattern, "pattern", "p", "**/*", "File pattern (glob)")
	DiscoverCmd.Flags().StringVarP(&discoverSort, "sort_by", "s", "relevance", "Sort by: relevance|name|size|mtime")
	DiscoverCmd.Flags().StringVarP(&discoverFormat, "format", "f", "text", "Output format: text|json")
	DiscoverCmd.Flags().IntVarP(&discoverLimit, "limit", "l", 100, "Max results")
}
