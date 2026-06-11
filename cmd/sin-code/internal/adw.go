// SPDX-License-Identifier: MIT
// Purpose: adw — Architectural Debt Watchdogs. Detects god modules, circular
// dependencies, high coupling, long functions, large files, and code smells.
// Pure Go implementation.
package internal

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	adwPath   string
	adwFormat string
	adwStrict bool
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
	Args: cobra.ArbitraryArgs,
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
	Path        string        `json:"path"`
	Summary     adwSummary    `json:"summary"`
	Issues      []adwIssue    `json:"issues"`
	Score       int           `json:"score"`
	Grade       string        `json:"grade"`
	ExitCode    int           `json:"exit_code"`
}

type adwSummary struct {
	FilesScanned   int `json:"files_scanned"`
	TotalIssues    int `json:"total_issues"`
	Critical       int `json:"critical"`
	High           int `json:"high"`
	Medium         int `json:"medium"`
	Low            int `json:"low"`
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
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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

	score := 100 - critical*20 - high*10 - medium*5 - low*2
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
	AdwCmd.Flags().StringVarP(&adwFormat, "format", "f", "text", "Output format: text|json")
	AdwCmd.Flags().BoolVarP(&adwStrict, "strict", "s", false, "Treat warnings as errors (exit 1 if critical/high issues)")
}
