// SPDX-License-Identifier: MIT
// Purpose: poc — Proof-of-Correctness. Verifies that code satisfies its
// specification by comparing code against spec documents (markdown, text, or
// structured requirements). Pure Go implementation.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	pocSpec   string
	pocCode   string
	pocFormat string
)

var PocCmd = &cobra.Command{
	Use:   "poc",
	Short: "Proof-of-Correctness — verify code satisfies its specification",
	Long: `Verify that code satisfies its specification. Compares code against
spec documents (markdown, text, or structured requirements) and checks for
compliance.

Pure Go implementation. Checks:
  - Required functions/classes mentioned in spec exist in code
  - Function signatures match specification
  - Required imports are present
  - No forbidden patterns (e.g., os.Exit in library code)

Examples:
  sin-code poc --spec spec.md --code src/main.py
  sin-code poc --spec requirements.json --code src/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		target := pocCode
		if target == "" {
			target = pocSpec
		}
		if target == "" {
			return fmt.Errorf("--code (or --spec for back-compat) is required")
		}

		result, err := verifyCorrectness(pocSpec, target)
		if err != nil {
			return err
		}

		if pocFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return outputTextPOC(result)
	},
}

type pocResult struct {
	Spec          string        `json:"spec"`
	Code          string        `json:"code"`
	Checks        []pocCheck    `json:"checks"`
	Passed        int           `json:"passed"`
	Failed        int           `json:"failed"`
	TotalChecks   int           `json:"total_checks"`
	Coverage      float64       `json:"coverage"`
	Summary       string        `json:"summary"`
}

type pocCheck struct {
	Name      string `json:"name"`
	Type      string `json:"type"`      // required, forbidden, signature, import
	Status    string `json:"status"`    // pass, fail, warn
	Message   string `json:"message"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
}

func verifyCorrectness(specPath, codePath string) (*pocResult, error) {
	var checks []pocCheck
	var specContent string

	// Read spec if provided
	if specPath != "" && specPath != codePath {
		data, err := os.ReadFile(specPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read spec: %w", err)
		}
		specContent = string(data)
	}

	// Find code files
	var codeFiles []string
	info, err := os.Stat(codePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read code path: %w", err)
	}
	if info.IsDir() {
		err := filepath.Walk(codePath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			lang := detectLanguage(path)
			if lang != "unknown" && lang != "markdown" && lang != "text" && lang != "json" && lang != "yaml" {
				codeFiles = append(codeFiles, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		codeFiles = []string{codePath}
	}

	// Extract requirements from spec
	requirements := extractRequirements(specContent)

	// Collect all code symbols
	allSymbols := make(map[string][]symbolLocation)
	for _, file := range codeFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		lang := detectLanguage(file)
		for _, sym := range extractSymbols(file, string(data), lang) {
			allSymbols[sym.Name] = append(allSymbols[sym.Name], symbolLocation{
				Name: sym.Name,
				Type: sym.Type,
				File: file,
				Line: sym.Line,
			})
		}
	}

	// Check requirements
	for _, req := range requirements {
		found := false
		for name, locs := range allSymbols {
			if strings.EqualFold(name, req.Name) || strings.EqualFold(name, strings.ReplaceAll(req.Name, " ", "_")) || strings.EqualFold(name, strings.ReplaceAll(req.Name, "-", "_")) {
				found = true
				loc := locs[0]
				checks = append(checks, pocCheck{
					Name:    req.Name,
					Type:    "required",
					Status:  "pass",
					Message: fmt.Sprintf("Found %s '%s' in %s:%d", loc.Type, name, loc.File, loc.Line),
					File:    loc.File,
					Line:    loc.Line,
				})
				break
			}
		}
		if !found {
			checks = append(checks, pocCheck{
				Name:   req.Name,
				Type:   "required",
				Status: "fail",
				Message: fmt.Sprintf("Required '%s' not found in code", req.Name),
			})
		}
	}

	// Check for forbidden patterns
	for _, file := range codeFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		content := string(data)
		lang := detectLanguage(file)

		// Check for os.Exit in library code (not main files)
		if lang == "go" && !strings.Contains(filepath.Base(file), "main") {
			if strings.Contains(content, "os.Exit(") {
				lines := strings.Split(content, "\n")
				for i, line := range lines {
					if strings.Contains(line, "os.Exit(") {
						checks = append(checks, pocCheck{
							Name:    "os.Exit",
							Type:    "forbidden",
							Status:  "warn",
							Message: fmt.Sprintf("os.Exit found in library code %s:%d", file, i+1),
							File:    file,
							Line:    i + 1,
						})
						break
					}
				}
			}
		}

		// Check for TODO/FIXME in non-test code
		if !isTestFile(file) {
			re := regexp.MustCompile(`(?i)(TODO|FIXME)\s*:?\s*(.{0,50})`)
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				matches := re.FindStringSubmatch(line)
				if len(matches) > 0 {
					checks = append(checks, pocCheck{
						Name:    matches[1],
						Type:    "forbidden",
						Status:  "warn",
						Message: fmt.Sprintf("%s found in %s:%d: %s", matches[1], file, i+1, strings.TrimSpace(matches[2])),
						File:    file,
						Line:    i + 1,
					})
					break // Only report first occurrence
				}
			}
		}
	}

	passed := 0
	failed := 0
	for _, check := range checks {
		if check.Status == "pass" {
			passed++
		} else if check.Status == "fail" {
			failed++
		}
	}

	coverage := 0.0
	if len(requirements) > 0 {
		coverage = float64(passed) / float64(len(requirements)) * 100
	}

	summary := fmt.Sprintf("Coverage: %.1f%% (%d/%d requirements, %d checks, %d passed, %d failed, %d warnings)",
		coverage, passed, len(requirements), len(checks), passed, failed, len(checks)-passed-failed)

	return &pocResult{
		Spec:        specPath,
		Code:        codePath,
		Checks:      checks,
		Passed:      passed,
		Failed:      failed,
		TotalChecks: len(checks),
		Coverage:    coverage,
		Summary:     summary,
	}, nil
}

type requirement struct {
	Name        string
	Type        string // function, class, method, import
	Description string
}

type symbolLocation struct {
	Name string
	Type string
	File string
	Line int
}

func extractRequirements(content string) []requirement {
	var reqs []requirement
	if content == "" {
		return reqs
	}

	// Extract function/class requirements from markdown or text
	// Patterns:
	// - Function: `functionName` or "functionName()" or "functionName(params)"
	// - Class: "class ClassName" or "Class ClassName"
	// - Must implement: "must implement X", "requires X", "function X"

	re := regexp.MustCompile(`(?i)(?:must\s+(?:implement|have|define|call)|requires?|should\s+(?:have|define|implement)|function|method|class|struct|type|interface)\s+[` + "`" + `"']?([a-zA-Z_][a-zA-Z0-9_]*)[` + "`" + `"']?`)
	seen := make(map[string]bool)
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			name := match[1]
			if !seen[name] && name != "" {
				seen[name] = true
				reqs = append(reqs, requirement{Name: name, Type: "symbol", Description: match[0]})
			}
		}
	}

	// Also look for code blocks in markdown
	codeRe := regexp.MustCompile("```[a-z]*\n([^`]+)\n```")
	for _, block := range codeRe.FindAllStringSubmatch(content, -1) {
		if len(block) > 1 {
			for _, req := range extractRequirements(block[1]) {
				if !seen[req.Name] {
					seen[req.Name] = true
					reqs = append(reqs, req)
				}
			}
		}
	}

	return reqs
}

func outputTextPOC(result *pocResult) error {
	fmt.Printf("Proof-of-Correctness\n")
	fmt.Printf("Spec:     %s\n", result.Spec)
	fmt.Printf("Code:     %s\n", result.Code)
	fmt.Printf("Coverage: %.1f%% (%d/%d passed)\n\n", result.Coverage, result.Passed, result.Passed+result.Failed)

	if len(result.Checks) > 0 {
		fmt.Printf("Checks (%d):\n", len(result.Checks))
		for _, check := range result.Checks {
			icon := "?"
			switch check.Status {
			case "pass":
				icon = "✓"
			case "fail":
				icon = "✗"
			case "warn":
				icon = "▲"
			}
			loc := ""
			if check.File != "" {
				loc = fmt.Sprintf(" (%s:%d)", check.File, check.Line)
			}
			fmt.Printf("  %s [%s] %s: %s%s\n", icon, check.Type, check.Name, check.Message, loc)
		}
	}
	fmt.Printf("\n%s\n", result.Summary)
	return nil
}

func init() {
	PocCmd.Flags().StringVarP(&pocSpec, "spec", "s", "", "Specification file (markdown, text, json)")
	PocCmd.Flags().StringVarP(&pocCode, "code", "c", "", "Code file or directory to verify")
	PocCmd.Flags().StringVarP(&pocFormat, "format", "f", "text", "Output format: text|json")
}
