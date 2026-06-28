// SPDX-License-Identifier: MIT
// Purpose: analysis tools — scout, adw, sckg, grasp.
// Merged from the single-export scout.go, adw.go, sckg.go, and grasp.go files
// to satisfy the shrink recommendation in issue #426.
// Also contains: harvest — URL fetching with caching, structure extraction,
// and change detection (merged from harvest.go).
package internal

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/circuitbreaker"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/egress"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

var (
	scoutQuery  string
	scoutPath   string
	scoutType   string
	scoutFormat string
	scoutMax    int
	scoutNoRG   bool
	scoutFile   string

	// numCPU is a test hook for the worker-pool sizing used by goSearch.
	numCPU = runtime.NumCPU

	// openFileFn is a test hook for isBinaryFile error paths.
	openFileFn = os.Open

	// scoreScoutModifier is a test hook for adjusting scout relevance scores.
	scoreScoutModifier func(score float64) float64

	scoutAbsPath = filepath.Abs
)

// searchFileFn is the searchFile implementation used by searchWithIndex.
// It is a variable so tests can inject per-candidate errors.
var searchFileFn = searchFile

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
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		if scoutQuery == "" {
			return fmt.Errorf("--query is required")
		}
		absPath, err := scoutAbsPath(scoutPath)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			if err != nil {
				return fmt.Errorf("path not found: %w", err)
			}
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		// If --file is set, search a single file (bypasses directory check).
		if scoutFile != "" {
			return searchSingleFile(scoutFile, scoutQuery, scoutType, scoutMax, scoutFormat)
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

	var cmd *exec.Cmd
	if rgCommandFn != nil {
		cmd = rgCommandFn("rg", args...)
	} else {
		cmd = exec.Command("rg", args...)
	}
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
			Path           struct{ Text string } `json:"path"`
			Lines          struct{ Text string } `json:"lines"`
			LineNumber     int                   `json:"line_number"`
			AbsoluteOffset int                   `json:"absolute_offset"`
			Submatches     []struct {
				Match struct{ Text string } `json:"match"`
				Start int                   `json:"start"`
				End   int                   `json:"end"`
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
	// Keep parity with goSearch: callers rely on descending-relevance order
	// regardless of which backend (ripgrep or pure Go) produced the results.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})
	return results, scanner.Err()
}

func goSearch(root, query, searchType string, maxResults int) ([]scoutResult, error) {
	re, err := compileQuery(query, searchType)
	if err != nil {
		return nil, err
	}

	ignorePatterns := loadGitignore(root)
	numWorkers := numCPU()
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
				m, err := searchFileFn(j.path, j.rel, root, re, searchType)
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
	f, err := openFileFn(path)
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
	rgOnce    sync.Once
	rgChecked bool
	rgOnPath  bool

	// rgCommandFn is a test hook so rgSearch's error paths are reachable.
	rgCommandFn func(name string, args ...string) *exec.Cmd
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
	if scoreScoutModifier != nil {
		score = scoreScoutModifier(score)
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
	RegisterVersionCmd(ScoutCmd)
	ScoutCmd.Flags().StringVarP(&scoutQuery, "query", "q", "", "Search query (regex or semantic)")
	_ = ScoutCmd.MarkFlagRequired("query")
	ScoutCmd.Flags().StringVarP(&scoutPath, "path", "p", ".", "Path to search")
	ScoutCmd.Flags().StringVarP(&scoutType, "search_type", "t", "regex", "Search type: regex|semantic|symbol|usage")
	ScoutCmd.Flags().StringVarP(&scoutFormat, "format", "f", "text", "Output format: text|json")
	ScoutCmd.Flags().IntVarP(&scoutMax, "max_results", "m", 50, "Max results")
	ScoutCmd.Flags().BoolVar(&scoutNoRG, "no-rg", false, "Skip ripgrep bridge even if rg is on PATH")
	ScoutCmd.Flags().StringVar(&scoutFile, "file", "", "Search a single file (bypasses --path; use for scoped searches)")
}

func searchSingleFile(file, query, searchType string, maxResults int, format string) error {
	absFile, err := filepathAbsFn(file)
	if err != nil {
		return fmt.Errorf("invalid file: %w", err)
	}
	if info, err := os.Stat(absFile); err != nil || info.IsDir() {
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
		return fmt.Errorf("--file must be a file, not a directory: %s", absFile)
	}
	re, err := compileQuery(query, searchType)
	if err != nil {
		return err
	}
	results, err := searchFileFn(absFile, relOf(absFile), filepath.Dir(absFile), re, searchType)
	if err != nil {
		return err
	}
	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	return outputTextScout(results)
}

// relOfFunc is a test hook for filepath.Rel so the error fallback can be
// exercised on platforms where Rel never fails (e.g. Unix).
var relOfFunc = filepath.Rel

func relOf(abs string) string {
	wd, _ := os.Getwd()
	if rel, err := relOfFunc(wd, abs); err == nil {
		return rel
	}
	return abs
}

var (
	adwPath         string
	adwFormat       string
	adwStrict       bool
	adwAbs          = filepath.Abs  // test hook for filepath.Abs errors
	adwWalk         = filepath.Walk // test hook for filepath.Walk errors
	adwInitialScore = 100           // test hook for the score > 100 cap
)

var AdwCmd = &cobra.Command{
	Use:   "adw",
	Short: "Architectural Debt Watchdogs — detect god modules, circular deps, etc.",
	Long: `Detect and report architectural debt in a codebase. Pure Go implementation.

Detects:
  - God modules (files with >15 imports or >500 lines)
  - Circular dependencies (import cycles)
  - High coupling (files imported by >10 others)
  - Long functions (>100 lines)
  - Large files (>500 lines)
  - TODO/FIXME comments
  - Missing tests (source files without corresponding test files)

Examples:
  sin-code adw .
  sin-code adw ./src --strict
  sin-code adw . --format json`,
	Args:    cobra.ArbitraryArgs,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := adwAbs(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			if err != nil {
				return fmt.Errorf("path not found: %w", err)
			}
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		result := scanDebt(absPath, adwStrict)

		if adwFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return outputTextADW(result)
	},
}

type adwResult struct {
	Path     string     `json:"path"`
	Summary  adwSummary `json:"summary"`
	Issues   []adwIssue `json:"issues"`
	Score    int        `json:"score"`
	Grade    string     `json:"grade"`
	ExitCode int        `json:"exit_code"`
}

type adwSummary struct {
	FilesScanned int `json:"files_scanned"`
	TotalIssues  int `json:"total_issues"`
	Critical     int `json:"critical"`
	High         int `json:"high"`
	Medium       int `json:"medium"`
	Low          int `json:"low"`
}

type adwIssue struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Metric   string `json:"metric,omitempty"`
}

func scanDebt(root string, strict bool) *adwResult {
	var issues []adwIssue
	filesScanned := 0
	imports := make(map[string][]string)     // file -> list of imports
	reverseDeps := make(map[string][]string) // import -> list of files importing it

	// First pass: collect all files and their imports
	err := adwWalk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "dist" || base == "build" || base == "target" || base == ".venv" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		lang := detectLanguage(path)
		if lang == "unknown" || lang == "markdown" || lang == "json" || lang == "yaml" || lang == "text" {
			return nil
		}

		filesScanned++
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 2_000_000 {
			return nil
		}
		content := string(data)
		lines := strings.Count(content, "\n") + 1

		// Check file size
		if lines > 500 {
			issues = append(issues, adwIssue{
				Type:     "large_file",
				Severity: "medium",
				File:     rel,
				Message:  fmt.Sprintf("File has %d lines (>500)", lines),
				Metric:   fmt.Sprintf("%d", lines),
			})
		}

		// Extract imports and detect god modules
		fileDeps := extractDependencies(path)
		if len(fileDeps) > 15 {
			issues = append(issues, adwIssue{
				Type:     "god_module",
				Severity: "high",
				File:     rel,
				Message:  fmt.Sprintf("File imports %d modules (>15)", len(fileDeps)),
				Metric:   fmt.Sprintf("%d imports", len(fileDeps)),
			})
		}
		imports[rel] = fileDeps
		for _, dep := range fileDeps {
			reverseDeps[dep] = append(reverseDeps[dep], rel)
		}

		// Check long functions
		if lang == "go" {
			issues = append(issues, checkLongFunctionsGo(path, rel, content)...)
		} else if lang == "python" {
			issues = append(issues, checkLongFunctionsPython(path, rel, content)...)
		} else if lang == "javascript" || lang == "typescript" || lang == "tsx" || lang == "jsx" {
			issues = append(issues, checkLongFunctionsJS(path, rel, content)...)
		}

		// Check for TODO/FIXME
		issues = append(issues, checkTODOs(rel, content)...)

		// Check missing tests
		if !isTestFile(rel) && !isConfigFile(rel) && !isDocFile(rel) {
			testExists := findTestFile(root, rel, lang)
			if !testExists {
				issues = append(issues, adwIssue{
					Type:     "missing_test",
					Severity: "low",
					File:     rel,
					Message:  "No corresponding test file found",
				})
			}
		}

		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: walk error: %v\n", err)
	}

	// Check circular dependencies
	issues = append(issues, findCircularDeps(imports)...)

	// Check high coupling (files imported by many others)
	for file, importers := range reverseDeps {
		if len(importers) > 10 {
			issues = append(issues, adwIssue{
				Type:     "high_coupling",
				Severity: "medium",
				File:     file,
				Message:  fmt.Sprintf("File is imported by %d other files (>10)", len(importers)),
				Metric:   fmt.Sprintf("%d importers", len(importers)),
			})
		}
	}

	// Calculate score and grade
	critical := 0
	high := 0
	medium := 0
	low := 0
	for _, issue := range issues {
		switch issue.Severity {
		case "critical":
			critical++
		case "high":
			high++
		case "medium":
			medium++
		case "low":
			low++
		}
	}

	score := adwInitialScore - critical*20 - high*10 - medium*5 - low*2
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	grade := "A"
	if score < 90 {
		grade = "B"
	}
	if score < 80 {
		grade = "C"
	}
	if score < 60 {
		grade = "D"
	}
	if score < 40 {
		grade = "F"
	}

	exitCode := 0
	if strict && (critical > 0 || high > 0) {
		exitCode = 1
	} else if critical > 0 {
		exitCode = 2
	}

	return &adwResult{
		Path: root,
		Summary: adwSummary{
			FilesScanned: filesScanned,
			TotalIssues:  len(issues),
			Critical:     critical,
			High:         high,
			Medium:       medium,
			Low:          low,
		},
		Issues:   issues,
		Score:    score,
		Grade:    grade,
		ExitCode: exitCode,
	}
}

func checkLongFunctionsGo(path, rel, content string) []adwIssue {
	var issues []adwIssue
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.AllErrors)
	if err != nil {
		return nil
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			start := fset.Position(fn.Pos()).Line
			end := fset.Position(fn.End()).Line
			length := end - start + 1
			if length > 100 {
				issues = append(issues, adwIssue{
					Type:     "long_function",
					Severity: "medium",
					File:     rel,
					Line:     start,
					Message:  fmt.Sprintf("Function '%s' is %d lines long (>100)", fn.Name.Name, length),
					Metric:   fmt.Sprintf("%d lines", length),
				})
			}
		}
	}
	return issues
}

func checkLongFunctionsPython(path, rel, content string) []adwIssue {
	var issues []adwIssue
	re := regexp.MustCompile(`^(\s*)(def|class)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) < 4 {
			continue
		}
		indent := len(matches[1])
		name := matches[3]
		start := i
		end := i
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" {
				end = j
				continue
			}
			lineIndent := len(lines[j]) - len(strings.TrimLeft(lines[j], " \t"))
			if lineIndent <= indent && strings.TrimSpace(lines[j]) != "" {
				break
			}
			end = j
		}
		length := end - start + 1
		if length > 100 {
			issues = append(issues, adwIssue{
				Type:     "long_function",
				Severity: "medium",
				File:     rel,
				Line:     start + 1,
				Message:  fmt.Sprintf("Function/class '%s' is %d lines long (>100)", name, length),
				Metric:   fmt.Sprintf("%d lines", length),
			})
		}
	}
	return issues
}

func checkLongFunctionsJS(path, rel, content string) []adwIssue {
	var issues []adwIssue
	re := regexp.MustCompile(`(?:export\s+)?(?:async\s+)?(?:function|class)\s+([a-zA-Z_$][a-zA-Z0-9_$]*)\s*\(`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		name := matches[1]
		start := i
		braceCount := 0
		foundOpen := false
		end := i
		for j := i; j < len(lines); j++ {
			for _, ch := range lines[j] {
				if ch == '{' {
					braceCount++
					foundOpen = true
				} else if ch == '}' {
					braceCount--
				}
			}
			end = j
			if foundOpen && braceCount == 0 {
				break
			}
		}
		length := end - start + 1
		if length > 100 {
			issues = append(issues, adwIssue{
				Type:     "long_function",
				Severity: "medium",
				File:     rel,
				Line:     start + 1,
				Message:  fmt.Sprintf("Function/class '%s' is %d lines long (>100)", name, length),
				Metric:   fmt.Sprintf("%d lines", length),
			})
		}
	}
	return issues
}

func checkTODOs(rel, content string) []adwIssue {
	var issues []adwIssue
	// Skip ADW's own source file — the regex patterns, help-text bullets, and
	// "Check for TODO/FIXME" comments legitimately mention these keywords
	// but are not actual TODO debt. Same for any file with "adw" in the path
	// (e.g. adw_test.go which has the same patterns).
	lower := strings.ToLower(rel)
	if strings.HasSuffix(lower, "adw.go") || strings.HasSuffix(lower, "adw_test.go") {
		return nil
	}
	re := regexp.MustCompile(`(?i)(TODO|FIXME|XXX|HACK|BUG|OPTIMIZE|REFACTOR)[\s:]*(.{0,100})`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// Skip lines where the keyword appears inside a Go raw-string literal
		// (backticks) or a regexp.MustCompile pattern — these are tool patterns,
		// not real TODOs. Heuristic: count backticks; if odd, the line contains
		// a raw string. Also skip if the line contains a string-literal
		// assignment to a variable named like a regex pattern.
		if strings.Count(line, "`")%2 == 1 {
			continue
		}
		if strings.Contains(line, "regexp.MustCompile") || strings.Contains(line, "regexp.Compile") {
			continue
		}
		// Skip help-text bullet lines (e.g. "  - TODO/FIXME comments")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			continue
		}
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				// Skip if the match is inside a quoted string on the same line
				// (e.g. a test assertion message). Heuristic: count quotes; if
				// the keyword is between two double-quotes, it's a string.
				if strings.Count(line, "\"") >= 2 {
					idx := strings.Index(strings.ToUpper(line), m[1])
					if idx >= 0 {
						before := line[:idx]
						after := line[idx+len(m[1]):]
						opens := strings.Count(before, "\"")
						if opens%2 == 1 && strings.Contains(after, "\"") {
							continue
						}
					}
				}
				severity := "low"
				if strings.Contains(strings.ToUpper(m[1]), "FIXME") || strings.Contains(strings.ToUpper(m[1]), "BUG") {
					severity = "medium"
				}
				issues = append(issues, adwIssue{
					Type:     "todo",
					Severity: severity,
					File:     rel,
					Line:     i + 1,
					Message:  fmt.Sprintf("%s: %s", m[1], strings.TrimSpace(m[2])),
				})
			}
		}
	}
	return issues
}

func findCircularDeps(imports map[string][]string) []adwIssue {
	var issues []adwIssue
	seen := make(map[string]bool)

	for file, deps := range imports {
		for _, dep := range deps {
			// Check if dep imports back to file
			if depImports, ok := imports[dep]; ok {
				for _, depDep := range depImports {
					if depDep == file {
						key := file + " <-> " + dep
						if !seen[key] {
							seen[key] = true
							issues = append(issues, adwIssue{
								Type:     "circular_dependency",
								Severity: "critical",
								File:     file,
								Message:  fmt.Sprintf("Circular dependency: %s <-> %s", file, dep),
							})
						}
					}
				}
			}
		}
	}
	return issues
}

func isTestFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "_test") || strings.Contains(lower, "test_") || strings.Contains(lower, ".spec.") || strings.Contains(lower, ".test.")
}

func isConfigFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "config") || strings.Contains(lower, "setup") || strings.Contains(lower, "dockerfile") || strings.Contains(lower, "makefile") || strings.Contains(lower, ".mod") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".toml") || strings.HasSuffix(lower, ".ini")
}

func isDocFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".rst") || strings.HasSuffix(lower, ".txt") || strings.Contains(lower, "readme") || strings.Contains(lower, "license") || strings.Contains(lower, "changelog") || strings.Contains(lower, "contributing")
}

func findTestFile(root, relPath, lang string) bool {
	dir := filepath.Dir(relPath)
	base := filepath.Base(relPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	var testNames []string
	switch lang {
	case "go":
		testNames = []string{
			name + "_test" + ext,
		}
	case "python":
		testNames = []string{
			"test_" + name + ext,
			name + "_test" + ext,
		}
	case "javascript", "typescript":
		testNames = []string{
			name + ".test" + ext,
			name + ".spec" + ext,
		}
	case "rust":
		testNames = []string{
			name + "_test" + ext,
		}
	case "java":
		testNames = []string{
			name + "Test" + ext,
			"Test" + name + ext,
		}
	default:
		return true // Unknown language, don't report missing tests
	}

	for _, tn := range testNames {
		testPath := filepath.Join(root, dir, tn)
		if _, err := os.Stat(testPath); err == nil {
			return true
		}
	}
	return false
}

func outputTextADW(r *adwResult) error {
	fmt.Printf("Architectural Debt Watchdogs: %s\n", r.Path)
	fmt.Printf("Grade: %s (Score: %d/100)\n\n", r.Grade, r.Score)
	fmt.Printf("Summary:\n")
	fmt.Printf("  Files scanned: %d\n", r.Summary.FilesScanned)
	fmt.Printf("  Total issues:  %d\n", r.Summary.TotalIssues)
	fmt.Printf("  Critical:      %d\n", r.Summary.Critical)
	fmt.Printf("  High:          %d\n", r.Summary.High)
	fmt.Printf("  Medium:        %d\n", r.Summary.Medium)
	fmt.Printf("  Low:           %d\n", r.Summary.Low)

	if len(r.Issues) > 0 {
		fmt.Printf("\nIssues:\n")
		// Sort by severity
		severityOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		sort.Slice(r.Issues, func(i, j int) bool {
			return severityOrder[r.Issues[i].Severity] < severityOrder[r.Issues[j].Severity]
		})
		for _, issue := range r.Issues {
			icon := "○"
			switch issue.Severity {
			case "critical":
				icon = "✗"
			case "high":
				icon = "!"
			case "medium":
				icon = "▲"
			}
			loc := issue.File
			if issue.Line > 0 {
				loc = fmt.Sprintf("%s:%d", issue.File, issue.Line)
			}
			fmt.Printf("  %s [%s] %s: %s\n", icon, issue.Severity, loc, issue.Message)
			if issue.Metric != "" {
				fmt.Printf("     metric: %s\n", issue.Metric)
			}
		}
	} else {
		fmt.Printf("\nNo architectural debt detected.\n")
	}
	return nil
}

func init() {
	RegisterVersionCmd(AdwCmd)
	AdwCmd.Flags().StringVarP(&adwFormat, "format", "f", "text", "Output format: text|json")
	AdwCmd.Flags().BoolVarP(&adwStrict, "strict", "s", false, "Treat warnings as errors (exit 1 if critical/high issues)")
}

var (
	sckgPath   string
	sckgAction string
	sckgQuery  string
	sckgFormat string
	sckgAbs    = filepath.Abs  // test hook for filepath.Abs errors
	sckgWalk   = filepath.Walk // test hook for filepath.Walk errors
)

var SckgCmd = &cobra.Command{
	Use:   "sckg",
	Short: "Semantic Codebase Knowledge Graphs — build & query code graph",
	Long: `Build and query a semantic graph of a codebase. Pure Go implementation.

Actions:
  build  — Build the knowledge graph from source code
  query  — Query the graph for relationships (requires --query)
  stats  — Show graph statistics
  export — Export graph as JSON

Examples:
  sin-code sckg . --action build
  sin-code sckg . --action query --query "auth module dependencies"
  sin-code sckg . --action stats
  sin-code sckg . --action export --format json`,
	Args:    cobra.ArbitraryArgs,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := sckgAbs(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			if err != nil {
				return fmt.Errorf("path not found: %w", err)
			}
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		switch sckgAction {
		case "build":
			graph, err := buildGraph(absPath)
			if err != nil {
				return err
			}
			if sckgFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(graph)
			}
			return outputTextSCKGBuild(graph)
		case "query":
			if sckgQuery == "" {
				return fmt.Errorf("--query is required for action=query")
			}
			graph, err := buildGraph(absPath)
			if err != nil {
				return err
			}
			results := queryGraph(graph, sckgQuery)
			if sckgFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}
			return outputTextSCKGQuery(results)
		case "stats":
			graph, err := buildGraph(absPath)
			if err != nil {
				return err
			}
			stats := graphStats(graph)
			if sckgFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(stats)
			}
			return outputTextSCKGStats(stats)
		case "export":
			graph, err := buildGraph(absPath)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(graph)
		default:
			return fmt.Errorf("unknown action: %s (use build|query|stats|export)", sckgAction)
		}
	},
}

type sckgGraph struct {
	Nodes []sckgNode `json:"nodes"`
	Edges []sckgEdge `json:"edges"`
}

type sckgNode struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // file, function, class, module
	Name     string `json:"name"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Language string `json:"language,omitempty"`
}

type sckgEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // imports, calls, defines, contains
}

type sckgStats struct {
	TotalNodes  int            `json:"total_nodes"`
	TotalEdges  int            `json:"total_edges"`
	NodeTypes   map[string]int `json:"node_types"`
	EdgeTypes   map[string]int `json:"edge_types"`
	TopImports  []importCount  `json:"top_imports"`
	OrphanNodes []string       `json:"orphan_nodes,omitempty"`
}

type importCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type queryResult struct {
	Query   string     `json:"query"`
	Matches []sckgNode `json:"matches"`
	Related []sckgNode `json:"related"`
	Total   int        `json:"total"`
}

func buildGraph(root string) (*sckgGraph, error) {
	var nodes []sckgNode
	var edges []sckgEdge
	nodeIndex := make(map[string]int) // id -> index in nodes

	addNode := func(id, typ, name, path string, line int, lang string) int {
		if idx, ok := nodeIndex[id]; ok {
			return idx
		}
		idx := len(nodes)
		nodes = append(nodes, sckgNode{
			ID:       id,
			Type:     typ,
			Name:     name,
			Path:     path,
			Line:     line,
			Language: lang,
		})
		nodeIndex[id] = idx
		return idx
	}

	addEdge := func(source, target, typ string) {
		edges = append(edges, sckgEdge{Source: source, Target: target, Type: typ})
	}

	err := sckgWalk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "dist" || base == "build" || base == "target" || base == ".venv" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		lang := detectLanguage(path)
		if lang == "unknown" || lang == "markdown" || lang == "text" {
			return nil
		}

		fileID := "file:" + rel
		addNode(fileID, "file", filepath.Base(rel), rel, 0, lang)

		data, err := os.ReadFile(path)
		if err != nil || len(data) > 2_000_000 {
			return nil
		}

		// Extract imports and create edges
		deps := extractDependencies(path)
		for _, dep := range deps {
			depID := "dep:" + dep
			addNode(depID, "module", dep, "", 0, "")
			addEdge(fileID, depID, "imports")
		}

		// Extract symbols and create edges via parseOutline (Phase 4b).
		outline := parseOutline(path, data)
		if outline != nil && outline.Engine != "none" {
			var walk func([]SymbolInfo)
			walk = func(syms []SymbolInfo) {
				for _, sym := range syms {
					typ := sckgKind(sym.Kind)
					id := fmt.Sprintf("%s:%s:%s", typ, rel, sym.Name)
					addNode(id, typ, sym.Name, rel, sym.StartLine, lang)
					addEdge(fileID, id, "contains")
					if len(sym.Children) > 0 {
						walk(sym.Children)
					}
				}
			}
			walk(outline.Symbols)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &sckgGraph{Nodes: nodes, Edges: edges}, nil
}

func queryGraph(graph *sckgGraph, query string) *queryResult {
	query = strings.ToLower(query)
	var matches []sckgNode
	var related []sckgNode
	matchSet := make(map[string]bool)
	relatedSet := make(map[string]bool)

	// Find matching nodes
	for _, node := range graph.Nodes {
		if strings.Contains(strings.ToLower(node.Name), query) ||
			strings.Contains(strings.ToLower(node.Path), query) ||
			strings.Contains(strings.ToLower(node.Type), query) {
			matches = append(matches, node)
			matchSet[node.ID] = true
		}
	}

	// Find related nodes (connected by edges)
	for _, edge := range graph.Edges {
		if matchSet[edge.Source] && !matchSet[edge.Target] && !relatedSet[edge.Target] {
			for _, node := range graph.Nodes {
				if node.ID == edge.Target {
					related = append(related, node)
					relatedSet[node.ID] = true
					break
				}
			}
		}
		if matchSet[edge.Target] && !matchSet[edge.Source] && !relatedSet[edge.Source] {
			for _, node := range graph.Nodes {
				if node.ID == edge.Source {
					related = append(related, node)
					relatedSet[node.ID] = true
					break
				}
			}
		}
	}

	return &queryResult{
		Query:   query,
		Matches: matches,
		Related: related,
		Total:   len(matches) + len(related),
	}
}

func graphStats(graph *sckgGraph) *sckgStats {
	nodeTypes := make(map[string]int)
	edgeTypes := make(map[string]int)
	importCounts := make(map[string]int)
	nodeConnections := make(map[string]int)

	for _, node := range graph.Nodes {
		nodeTypes[node.Type]++
	}
	for _, edge := range graph.Edges {
		edgeTypes[edge.Type]++
		nodeConnections[edge.Source]++
		nodeConnections[edge.Target]++
		if edge.Type == "imports" {
			importCounts[edge.Target]++
		}
	}

	var topImports []importCount
	for name, count := range importCounts {
		topImports = append(topImports, importCount{Name: name, Count: count})
	}
	sort.Slice(topImports, func(i, j int) bool {
		return topImports[i].Count > topImports[j].Count
	})
	if len(topImports) > 10 {
		topImports = topImports[:10]
	}

	var orphans []string
	for _, node := range graph.Nodes {
		if nodeConnections[node.ID] == 0 && node.Type != "file" {
			orphans = append(orphans, node.Name)
		}
	}

	return &sckgStats{
		TotalNodes:  len(graph.Nodes),
		TotalEdges:  len(graph.Edges),
		NodeTypes:   nodeTypes,
		EdgeTypes:   edgeTypes,
		TopImports:  topImports,
		OrphanNodes: orphans,
	}
}

func sckgKind(kind string) string {
	switch kind {
	case "func", "method":
		return "function"
	case "struct", "type":
		return "type"
	case "var", "const":
		return "variable"
	default:
		return kind
	}
}

func outputTextSCKGBuild(graph *sckgGraph) error {
	fmt.Printf("SCKG Knowledge Graph Built\n")
	fmt.Printf("Nodes: %d\n", len(graph.Nodes))
	fmt.Printf("Edges: %d\n", len(graph.Edges))
	fmt.Printf("\nNode Types:\n")
	types := make(map[string]int)
	for _, node := range graph.Nodes {
		types[node.Type]++
	}
	var typeList []struct {
		name  string
		count int
	}
	for k, v := range types {
		typeList = append(typeList, struct {
			name  string
			count int
		}{k, v})
	}
	sort.Slice(typeList, func(i, j int) bool { return typeList[i].count > typeList[j].count })
	for _, t := range typeList {
		fmt.Printf("  %-12s %d\n", t.name, t.count)
	}
	return nil
}

func outputTextSCKGQuery(result *queryResult) error {
	fmt.Printf("Query: %s\n", result.Query)
	fmt.Printf("Total matches: %d\n\n", result.Total)

	if len(result.Matches) > 0 {
		fmt.Printf("Direct matches (%d):\n", len(result.Matches))
		for _, node := range result.Matches {
			fmt.Printf("  %-12s %-20s %s:%d\n", node.Type, node.Name, node.Path, node.Line)
		}
	}

	if len(result.Related) > 0 {
		fmt.Printf("\nRelated (%d):\n", len(result.Related))
		for _, node := range result.Related {
			fmt.Printf("  %-12s %-20s %s:%d\n", node.Type, node.Name, node.Path, node.Line)
		}
	}
	return nil
}

func outputTextSCKGStats(stats *sckgStats) error {
	fmt.Printf("SCKG Graph Statistics\n")
	fmt.Printf("Total nodes: %d\n", stats.TotalNodes)
	fmt.Printf("Total edges: %d\n", stats.TotalEdges)

	fmt.Printf("\nNode Types:\n")
	for typ, count := range stats.NodeTypes {
		fmt.Printf("  %-12s %d\n", typ, count)
	}

	fmt.Printf("\nEdge Types:\n")
	for typ, count := range stats.EdgeTypes {
		fmt.Printf("  %-12s %d\n", typ, count)
	}

	if len(stats.TopImports) > 0 {
		fmt.Printf("\nTop Imports:\n")
		for _, imp := range stats.TopImports {
			fmt.Printf("  %-40s %d\n", imp.Name, imp.Count)
		}
	}

	if len(stats.OrphanNodes) > 0 {
		fmt.Printf("\nOrphan nodes (%d):\n", len(stats.OrphanNodes))
		for _, orphan := range stats.OrphanNodes {
			fmt.Printf("  %s\n", orphan)
		}
	}
	return nil
}

func init() {
	RegisterVersionCmd(SckgCmd)
	SckgCmd.Flags().StringVarP(&sckgAction, "action", "a", "build", "Action: build|query|stats|export")
	SckgCmd.Flags().StringVarP(&sckgQuery, "query", "q", "", "Query (for action=query)")
	SckgCmd.Flags().StringVarP(&sckgFormat, "format", "f", "text", "Output format: text|json")
}

// ---------------------------------------------------------------------------
// grasp — deep code understanding for a single file
// ---------------------------------------------------------------------------

var (
	graspPath   string
	graspFormat string
)

var (
	graspAbsPath       = filepath.Abs
	graspAnalyzeFileFn = analyzeFile
)

var GraspCmd = &cobra.Command{
	Use:   "grasp [path]",
	Short: "Deep code understanding for a single file",
	Long: `Deep code understanding for individual files — structure, dependencies,
usage, and related context. Pure Go implementation.

Example:
  sin-code grasp cmd/sin-code/main.go --format json`,
	Args:    cobra.ExactArgs(1),
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := graspAbsPath(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("path is a directory, not a file: %s", absPath)
		}

		result, err := graspAnalyzeFileFn(absPath, info)
		if err != nil {
			return err
		}

		if graspFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return outputTextGrasp(result)
	},
}

type graspResult struct {
	Path         string       `json:"path"`
	Language     string       `json:"language"`
	Size         int64        `json:"size"`
	Lines        int          `json:"lines"`
	BlankLines   int          `json:"blank_lines"`
	CommentLines int          `json:"comment_lines"`
	CodeLines    int          `json:"code_lines"`
	ModTime      string       `json:"mod_time"`
	Structure    []structItem `json:"structure"`
	Dependencies []string     `json:"dependencies"`
	Summary      string       `json:"summary"`
	Exports      []string     `json:"exports"`
}

type structItem struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Line int    `json:"line"`
}

func analyzeFile(path string, info os.FileInfo) (*graspResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)

	lang := detectLanguage(path)
	lines, blank, comments, code := countLines(content, lang)
	structure := extractStructure(path, content)
	deps := extractDependencies(path) // reuses discover.go function
	exports := extractExports(content, lang)

	summary := fmt.Sprintf("%d lines (%d code, %d comments, %d blank) in %s",
		lines, code, comments, blank, lang)

	return &graspResult{
		Path:         path,
		Language:     lang,
		Size:         info.Size(),
		Lines:        lines,
		BlankLines:   blank,
		CommentLines: comments,
		CodeLines:    code,
		ModTime:      info.ModTime().Format("2006-01-02 15:04:05"),
		Structure:    structure,
		Dependencies: deps,
		Summary:      summary,
		Exports:      exports,
	}, nil
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	langMap := map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript",
		".tsx": "tsx", ".jsx": "jsx", ".rs": "rust", ".java": "java",
		".c": "c", ".cpp": "cpp", ".h": "c-header", ".hpp": "cpp-header",
		".sh": "bash", ".md": "markdown", ".json": "json", ".yaml": "yaml",
		".yml": "yaml", ".toml": "toml", ".html": "html", ".css": "css",
		".sql": "sql", ".rb": "ruby", ".php": "php", ".swift": "swift",
		".kt": "kotlin", ".scala": "scala", ".r": "r", ".lua": "lua",
		".dockerfile": "dockerfile", ".makefile": "makefile", ".mod": "go",
	}
	if lang, ok := langMap[ext]; ok {
		return lang
	}
	name := strings.ToLower(filepath.Base(path))
	if name == "dockerfile" || strings.HasPrefix(name, "dockerfile") {
		return "dockerfile"
	}
	if name == "makefile" || name == "gnumakefile" {
		return "makefile"
	}
	return "unknown"
}

func countLines(content, lang string) (total, blank, comments, code int) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	inBlockComment := false
	blockStart := ""
	blockEnd := ""

	switch lang {
	case "go", "c", "cpp", "c-header", "cpp-header", "java", "rust", "kotlin", "swift", "scala":
		blockStart = "/*"
		blockEnd = "*/"
	case "python":
		blockStart = `"""`
		blockEnd = `"""`
		if !strings.Contains(content, `"""`) {
			blockStart = "'''"
			blockEnd = "'''"
		}
	}

	lineComment := ""
	switch lang {
	case "go", "c", "cpp", "c-header", "cpp-header", "java", "rust", "kotlin", "swift", "scala", "bash", "makefile":
		lineComment = "//"
	case "python", "ruby", "php", "r", "perl", "yaml", "dockerfile":
		lineComment = "#"
	case "javascript", "typescript", "tsx", "jsx", "css", "sql":
		lineComment = "//"
	}

	for scanner.Scan() {
		line := scanner.Text()
		total++
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blank++
			continue
		}

		if inBlockComment {
			comments++
			if blockEnd != "" && strings.Contains(line, blockEnd) {
				inBlockComment = false
			}
			continue
		}

		if blockStart != "" && strings.Contains(line, blockStart) {
			comments++
			if !strings.Contains(line, blockEnd) || strings.Index(line, blockStart) > strings.Index(line, blockEnd) {
				inBlockComment = true
			}
			continue
		}

		if lineComment != "" && strings.HasPrefix(trimmed, lineComment) {
			comments++
			continue
		}

		code++
	}
	return
}

func extractStructure(path, content string) []structItem {
	// Phase 4b: unified AST-based extraction via parseOutline.
	outline := parseOutline(path, []byte(content))
	if outline == nil || outline.Engine == "none" {
		// Fallback to generic regex for unknown languages.
		return extractGenericStructure(strings.Split(content, "\n"), detectLanguage(path))
	}
	var items []structItem
	var walk func([]SymbolInfo)
	walk = func(syms []SymbolInfo) {
		for _, sym := range syms {
			items = append(items, structItem{Type: normalizeGraspKind(sym.Kind), Name: sym.Name, Line: sym.StartLine})
			if len(sym.Children) > 0 {
				walk(sym.Children)
			}
		}
	}
	walk(outline.Symbols)
	return items
}

func normalizeGraspKind(kind string) string {
	switch kind {
	case "func":
		return "function"
	case "method":
		return "function"
	case "var":
		return "variable"
	case "const":
		return "variable"
	default:
		return kind
	}
}

func extractGenericStructure(lines []string, lang string) []structItem {
	var items []structItem
	re := regexp.MustCompile(`(?:function|def|fn|func|method|class|struct|interface|trait|enum|record|sub|procedure)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			items = append(items, structItem{Type: "symbol", Name: matches[1], Line: i + 1})
		}
	}
	return items
}

func extractExports(content, lang string) []string {
	var exports []string
	switch lang {
	case "go":
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "", content, parser.AllErrors)
		if err != nil {
			return nil
		}
		for name := range f.Scope.Objects {
			if ast.IsExported(name) {
				exports = append(exports, name)
			}
		}
	case "python":
		re := regexp.MustCompile(`^\s*__(all__)\s*=\s*\[(.*?)\]`)
		if m := re.FindStringSubmatch(content); len(m) > 2 {
			all := strings.Split(m[2], ",")
			for _, e := range all {
				e = strings.Trim(strings.TrimSpace(e), `"' `)
				if e != "" {
					exports = append(exports, e)
				}
			}
		}
	case "javascript", "typescript", "tsx", "jsx":
		re := regexp.MustCompile(`(?:export\s+(?:default\s+)?(?:class|function|const|let|var|interface|type)\s+)([a-zA-Z_$][a-zA-Z0-9_$]*)`)
		seen := make(map[string]bool)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) > 1 && !seen[m[1]] {
				seen[m[1]] = true
				exports = append(exports, m[1])
			}
		}
	case "rust":
		re := regexp.MustCompile(`^pub\s+(?:fn|struct|enum|trait|type|use|const|static|mod)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) > 1 {
				exports = append(exports, m[1])
			}
		}
	}
	return exports
}

func outputTextGrasp(r *graspResult) error {
	fmt.Printf("File:     %s\n", r.Path)
	fmt.Printf("Language: %s\n", r.Language)
	fmt.Printf("Size:     %d bytes\n", r.Size)
	fmt.Printf("Lines:    %d total (%d code, %d comments, %d blank)\n",
		r.Lines, r.CodeLines, r.CommentLines, r.BlankLines)
	fmt.Printf("Modified: %s\n", r.ModTime)
	fmt.Printf("Summary:  %s\n", r.Summary)

	if len(r.Structure) > 0 {
		fmt.Printf("\nStructure (%d symbols):\n", len(r.Structure))
		for _, s := range r.Structure {
			fmt.Printf("  %-10s %-20s (line %d)\n", s.Type, s.Name, s.Line)
		}
	}

	if len(r.Dependencies) > 0 {
		fmt.Printf("\nDependencies (%d):\n", len(r.Dependencies))
		for _, d := range r.Dependencies {
			fmt.Printf("  %s\n", d)
		}
	}

	if len(r.Exports) > 0 {
		fmt.Printf("\nExports (%d):\n", len(r.Exports))
		for _, e := range r.Exports {
			fmt.Printf("  %s\n", e)
		}
	}
	return nil
}

func init() {
	RegisterVersionCmd(GraspCmd)
	GraspCmd.Flags().StringVarP(&graspFormat, "format", "f", "text", "Output format: text|json")
}

// ============================================================================
// harvest — URL fetching with caching, structure extraction, and change
// detection. Built-in Go implementation using net/http with local disk
// cache wrapped in a circuitbreaker transport so a slow/unresponsive
// upstream cannot pin the agent loop indefinitely (#72).
// ============================================================================

// harvestBreaker rate-limits outbound plan HTTP traffic: 5 consecutive
// 5xx / transport errors trip the breaker for 30s; the next call
// after that gets a HalfOpen probe that re-trips if the upstream is
// still down. Shared across all `sin-code harvest` invocations in the
// same process — a slow upstream should affect plan-phase holistically,
// not just one call site.
var harvestBreaker = circuitbreaker.New(&circuitbreaker.Config{
	Name:             "harvest",
	FailureThreshold: 5,
	OpenDuration:     30 * time.Second,
	HalfOpenProbes:   1,
	SuccessThreshold: 1,
})

var (
	harvestURL     string
	harvestFormat  string
	harvestMethod  string
	harvestTimeout int
)

var harvestHTTPClient = func(timeout int) *http.Client {
	return &http.Client{
		Timeout:   time.Duration(timeout) * time.Second,
		Transport: circuitbreaker.RoundTripper(http.DefaultTransport, harvestBreaker),
	}
}

// harvestEgressCheck is the SSRF allowlist gate applied before every
// http.NewRequest. The default calls into the egress package with a deny
// policy (Finding 4, security audit). Tests swap this for a permissive
// stub when using httptest.NewServer (which always binds 127.0.0.1).
// Sin-debt: scope=narrow, upgrade=migrate to per-verify-gate dial-on-resolved-ip transport hardening
var harvestEgressCheck = func(ctx context.Context, u string) error {
	return egress.Check(ctx, u, egress.Policy{})
}

var HarvestCmd = &cobra.Command{
	Use:   "harvest",
	Short: "Fetch URLs with caching, structure extraction, and change detection",
	Long: `Fetch URLs with caching, structure extraction, change detection, and
auth management. Pure Go implementation with local disk cache.

Example:
  sin-code harvest --url https://api.example.com/data --format json`,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		if harvestURL == "" {
			return fmt.Errorf("--url is required")
		}
		return harvestURLFetch(harvestURL, harvestMethod, harvestTimeout, harvestFormat)
	},
}

type harvestResult struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Status     int               `json:"status"`
	StatusText string            `json:"status_text"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Duration   string            `json:"duration"`
	Cached     bool              `json:"cached"`
	CacheAge   string            `json:"cache_age,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func harvestURLFetch(url, method string, timeout int, format string) error {
	start := time.Now()
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "sin-code", "harvest")
	_ = os.MkdirAll(cacheDir, 0755)
	cacheKey := sha256.Sum256([]byte(method + " " + url))
	cacheFile := filepath.Join(cacheDir, hex.EncodeToString(cacheKey[:])+".json")

	// Check cache (TTL: 5 minutes)
	if info, err := os.Stat(cacheFile); err == nil && time.Since(info.ModTime()) < 5*time.Minute {
		data, err := os.ReadFile(cacheFile)
		if err == nil {
			var cached harvestResult
			if err := json.Unmarshal(data, &cached); err == nil {
				cached.Cached = true
				cached.CacheAge = time.Since(info.ModTime()).String()
				if format == "json" {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(cached)
				}
				fmt.Printf("[CACHED %s] %s %s\n\n", cached.CacheAge, cached.Method, cached.URL)
				fmt.Println(cached.Body)
				return nil
			}
		}
	}

	client := harvestHTTPClient(timeout)
	if err := harvestEgressCheck(context.Background(), url); err != nil {
		return fmt.Errorf("harvest: %w", err)
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}
	req.Header.Set("User-Agent", "sin-code/1.0 (https://github.com/OpenSIN-Code/SIN-Code)")
	req.Header.Set("Accept", "text/html,application/json,text/plain,*/*")

	resp, err := client.Do(req)
	duration := time.Since(start)

	result := harvestResult{
		URL:      url,
		Method:   method,
		Duration: duration.String(),
		Cached:   false,
	}

	if err != nil {
		result.Error = err.Error()
		if format == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("ERROR: %s\n", result.Error)
		return nil
	}
	defer resp.Body.Close()

	result.Status = resp.StatusCode
	result.StatusText = resp.Status
	result.Headers = make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			result.Headers[k] = v[0]
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("read body: %s", err)
	} else {
		result.Body = string(body)
	}

	// Save to cache
	if cacheData, err := json.MarshalIndent(result, "", "  "); err == nil {
		_ = os.WriteFile(cacheFile, cacheData, filemode.Default())
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Printf("Status: %s\nDuration: %s\n\n", result.StatusText, result.Duration)
	fmt.Println(result.Body)
	return nil
}

func init() {
	RegisterVersionCmd(HarvestCmd)
	HarvestCmd.Flags().StringVarP(&harvestURL, "url", "u", "", "URL to fetch")
	_ = HarvestCmd.MarkFlagRequired("url")
	HarvestCmd.Flags().StringVarP(&harvestMethod, "method", "m", "GET", "HTTP method")
	HarvestCmd.Flags().IntVarP(&harvestTimeout, "timeout", "t", 30, "Timeout in seconds")
	HarvestCmd.Flags().StringVarP(&harvestFormat, "format", "f", "text", "Output format: text|json")
}
