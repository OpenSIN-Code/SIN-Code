// SPDX-License-Identifier: MIT
// Purpose: scout — parallel code search with regex, semantic, symbol, usage.
// Uses ripgrep (rg) as a fast bridge when available; falls back to a parallel
// Go worker pool with gitignore-aware walking. Binary files are auto-skipped.
// Docs: cmd/sin-code/internal/scout.doc.md
package internal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

var (
	scoutQuery  string
	scoutPath   string
	scoutType   string
	scoutFormat string
	scoutMax    int
	scoutNoRG   bool
)

var ScoutCmd = &cobra.Command{
	Use:   "scout",
	Short: "Search code with regex, semantic, symbol, and usage search",
	Long: `Parallel code search with optional ripgrep bridge (auto-detected on PATH).

Search types: regex|semantic|symbol|usage
  regex     literal regex pattern
  semantic  word-order matching (case insensitive)
  symbol    function/class/struct/variable definitions
  usage     all references to a symbol name

Examples:
  sin-code scout --query "func.*main" --path . --search_type regex --format json
  sin-code scout --query "handleError" --path . --search_type usage
  sin-code scout --query "class.*Factory" --search_type symbol --no-rg`,
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

		results, err := scoutSearchAuto(absPath, scoutQuery, scoutType, scoutMax, scoutNoRG)
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
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Column    int      `json:"column"`
	Match     string   `json:"match"`
	Context   []string `json:"context"`
	Type      string   `json:"type"`
	Relevance float64  `json:"relevance"`
}

type match struct {
	results []scoutResult
	err     error
}

func rgAvailable() bool {
	_, err := exec.LookPath("rg")
	return err == nil
}

func searchFiles(root, query, searchType string, maxResults int, noRG bool) ([]scoutResult, error) {
	useRG := rgAvailable() && !noRG && (searchType == "regex" || searchType == "usage")
	if useRG {
		results, err := rgSearch(root, query, searchType, maxResults)
		if err == nil {
			return results, nil
		}
		// fallback on rg error
	}
	return goSearch(root, query, searchType, maxResults)
}

func rgSearch(root, query, searchType string, maxResults int) ([]scoutResult, error) {
	args := []string{"--json", "-g", "!.git"}
	if searchType == "usage" {
		args = append(args, "--word-regexp")
	}
	if maxResults > 0 {
		args = append(args, "--max-count", fmt.Sprintf("%d", maxResults))
	}
	args = append(args, query, ".")

	cmd := exec.Command("rg", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("rg: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		// rg exits 1 when no matches: not an error
		if cmd.ProcessState.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("rg: %w", err)
	}

	var results []scoutResult
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		var raw struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}
		if raw.Type != "match" {
			continue
		}
		var matchData struct {
			Path        struct{ Text string } `json:"path"`
			Lines       struct{ Text string } `json:"lines"`
			LineNumber  int                   `json:"line_number"`
			AbsoluteOffset int                `json:"absolute_offset"`
			Submatches  []struct {
				Match   struct{ Text string } `json:"match"`
				Start   int    `json:"start"`
				End     int    `json:"end"`
			} `json:"submatches"`
		}
		if err := json.Unmarshal(raw.Data, &matchData); err != nil {
			continue
		}
		// rg returns paths relative to cmd.Dir (root), so make absolute first
		absPath := filepath.Join(root, matchData.Path.Text)
		rel, _ := filepath.Rel(root, absPath)
		matchLine := strings.TrimRight(matchData.Lines.Text, "\n")
		col := 1
		matchText := ""
		if len(matchData.Submatches) > 0 {
			col = matchData.Submatches[0].Start + 1
			matchText = matchData.Submatches[0].Match.Text
		}
		results = append(results, scoutResult{
			File:      rel,
			Line:      matchData.LineNumber,
			Column:    col,
			Match:     matchText,
			Context:   []string{"> " + matchLine},
			Type:      searchType,
			Relevance: scoreRelevanceScout(rel, matchLine),
		})
		if maxResults > 0 && len(results) >= maxResults {
			break
		}
	}
	return results, scanner.Err()
}

func goSearch(root, query, searchType string, maxResults int) ([]scoutResult, error) {
	re, err := compileQuery(query, searchType)
	if err != nil {
		return nil, err
	}

	ignorePatterns := loadGitignore(root)
	numWorkers := runtime.NumCPU()
	if numWorkers < 2 {
		numWorkers = 2
	}

	type job struct{ path, rel string }

	jobs := make(chan job, 256)
	matches := make(chan match, numWorkers)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localResults := make([]scoutResult, 0, 256)
			for j := range jobs {
				m, err := searchFile(j.path, j.rel, root, re, searchType)
				if err != nil {
					matches <- match{err: err}
					continue
				}
				localResults = append(localResults, m...)
			}
			if len(localResults) > 0 {
				matches <- match{results: localResults}
			}
		}()
	}

	var walkWg sync.WaitGroup
	walkWg.Add(1)
	go func() {
		defer walkWg.Done()
		defer close(jobs)
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "dist" || base == "build" || base == "target" || strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
				if ignorePatterns.matchDir(path) {
					return filepath.SkipDir
				}
				return nil
			}
			if isBinaryFile(path) {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if ignorePatterns.matchFile(rel) {
				return nil
			}
			jobs <- job{path: path, rel: rel}
			return nil
		})
	}()

	go func() {
		wg.Wait()
		close(matches)
	}()

	walkWg.Wait()

	var results []scoutResult
	seen := 0
	for m := range matches {
		if m.err != nil {
			continue
		}
		for _, r := range m.results {
			results = append(results, r)
			seen++
			if maxResults > 0 && seen >= maxResults {
				drainMatches(matches)
				goto sortResults
			}
		}
	}

sortResults:
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})
	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func drainMatches(ch chan match) {
	for range ch {
	}
}

func compileQuery(query, searchType string) (*regexp.Regexp, error) {
	switch searchType {
	case "regex":
		return regexp.Compile(query)
	case "semantic":
		words := strings.Fields(query)
		return regexp.Compile("(?i)" + strings.Join(words, ".*"))
	case "symbol":
		return regexp.Compile(`(?i)(?:func|def|fn|class|struct|interface|trait|enum|type|const|var|let)\s+` + regexp.QuoteMeta(query))
	case "usage":
		return regexp.Compile(`(?i)\b` + regexp.QuoteMeta(query) + `\b`)
	default:
		return nil, fmt.Errorf("unknown search_type: %s", searchType)
	}
}

func searchFile(path, rel, root string, re *regexp.Regexp, searchType string) ([]scoutResult, error) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 5_000_000 {
		return nil, nil
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	var results []scoutResult
	for i, line := range lines {
		loc := re.FindStringIndex(line)
		if loc == nil {
			continue
		}
		results = append(results, scoutResult{
			File:      rel,
			Line:      i + 1,
			Column:    loc[0] + 1,
			Match:     line[loc[0]:loc[1]],
			Context:   getContext(lines, i, 2),
			Type:      searchType,
			Relevance: scoreRelevanceScout(rel, line),
		})
	}
	return results, nil
}

type gitignoreMatcher struct {
	patterns []gitignorePattern
}

type gitignorePattern struct {
	pattern string
	negate  bool
	dirOnly bool
	re      *regexp.Regexp
}

func loadGitignore(root string) *gitignoreMatcher {
	m := &gitignoreMatcher{}
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := strings.HasPrefix(line, "!")
		if negate {
			line = line[1:]
		}
		dirOnly := strings.HasSuffix(line, "/")
		if dirOnly {
			line = strings.TrimSuffix(line, "/")
		}
		pattern := line
		var re *regexp.Regexp
		if strings.ContainsAny(pattern, "*?[") {
			re = gitignoreGlobToRegex(pattern)
		}
		m.patterns = append(m.patterns, gitignorePattern{
			pattern: pattern,
			negate:  negate,
			dirOnly: dirOnly,
			re:      re,
		})
	}
	return m
}

func gitignoreGlobToRegex(pattern string) *regexp.Regexp {
	var buf strings.Builder
	buf.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			buf.WriteString(".*")
		case '?':
			buf.WriteString(".")
		case '.':
			buf.WriteString("\\.")
		case '/':
			buf.WriteString("/")
		default:
			buf.WriteRune(r)
		}
	}
	buf.WriteString("$")
	return regexp.MustCompile(buf.String())
}

func (m *gitignoreMatcher) matchFile(rel string) bool {
	ignored := false
	for _, p := range m.patterns {
		if p.dirOnly {
			continue
		}
		if matchesPattern(rel, p) {
			ignored = !p.negate
		}
	}
	return ignored
}

func (m *gitignoreMatcher) matchDir(path string) bool {
	base := filepath.Base(path)
	ignored := false
	for _, p := range m.patterns {
		if !p.dirOnly {
			continue
		}
		if p.pattern == base || matchesPattern(base, p) {
			ignored = !p.negate
		}
	}
	return ignored
}

func matchesPattern(name string, p gitignorePattern) bool {
	if p.re != nil {
		return p.re.MatchString(name)
	}
	return strings.TrimSuffix(name, "/") == p.pattern
}

func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return true
	}
	return bytes.IndexByte(buf[:n], 0) >= 0
}

var (
	rgOnce     sync.Once
	rgChecked  bool
	rgOnPath   bool
)

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
	ext := strings.ToLower(filepath.Ext(relPath))
	if ext == ".go" || ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".rs" || ext == ".java" {
		score += 15
	}
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "def ") ||
		strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "struct ") ||
		strings.HasPrefix(trimmed, "interface ") || strings.HasPrefix(trimmed, "type ") ||
		strings.HasPrefix(trimmed, "const ") || strings.HasPrefix(trimmed, "var ") ||
		strings.HasPrefix(trimmed, "let ") || strings.HasPrefix(trimmed, "export ") {
		score += 20
	}
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
		score -= 10
	}
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
	ScoutCmd.Flags().BoolVar(&scoutNoRG, "no-rg", false, "Skip ripgrep bridge even if rg is on PATH")
}
