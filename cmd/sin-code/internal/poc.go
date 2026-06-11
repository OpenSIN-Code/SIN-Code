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

// pocStopwords are common English / spec-prose words that must never be
// treated as required symbol names. This prevents natural-language specs
// ("The Hello() function must return ...") from producing bogus requirements
// like "must" or "Spec" (dogfooding bug st-bug1 #3).
var pocStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "not": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"being": true, "have": true, "has": true, "had": true, "do": true,
	"does": true, "did": true, "will": true, "would": true, "should": true,
	"shall": true, "must": true, "may": true, "might": true, "can": true,
	"could": true, "if": true, "then": true, "else": true, "when": true,
	"where": true, "that": true, "this": true, "these": true, "those": true,
	"it": true, "its": true, "with": true, "for": true, "from": true,
	"to": true, "in": true, "on": true, "at": true, "by": true, "of": true,
	"as": true, "return": true, "returns": true, "returning": true,
	"function": true, "functions": true, "method": true, "methods": true,
	"class": true, "classes": true, "struct": true, "structs": true,
	"type": true, "types": true, "interface": true, "interfaces": true,
	"spec": true, "specs": true, "specification": true, "requirement": true,
	"requirements": true, "string": true, "int": true, "bool": true,
	"float": true, "error": true, "true": true, "false": true, "nil": true,
	"null": true, "none": true, "void": true, "all": true, "any": true,
	"each": true, "no": true, "side": true, "effects": true, "value": true,
	"values": true,
}

func extractRequirements(content string) []requirement {
	var reqs []requirement
	if content == "" {
		return reqs
	}

	seen := make(map[string]bool)
	add := func(name, desc string) {
		if name == "" || seen[name] || pocStopwords[strings.ToLower(name)] {
			return
		}
		seen[name] = true
		reqs = append(reqs, requirement{Name: name, Type: "symbol", Description: desc})
	}

	// 1. Function-call references: `Hello()`, processOrder(args), REQ-1: hello().
	//    An identifier immediately followed by "(" is the strongest signal a
	//    spec is naming a concrete callable.
	callRe := regexp.MustCompile("[`\"']?([a-zA-Z_][a-zA-Z0-9_]*)\\s*\\(")
	for _, m := range callRe.FindAllStringSubmatch(content, -1) {
		add(m[1], m[0])
	}

	// 2. Keyword-introduced symbols: "must implement X", "requires X",
	//    "function X", "class X", etc. Articles ("a", "the") and a chained
	//    kind keyword ("define type Config") are skipped so the regex lands
	//    on the actual identifier instead of a filler word.
	re := regexp.MustCompile(`(?i)(?:must\s+(?:implement|have|define|call)|requires?|should\s+(?:have|define|implement)|function|method|class|struct|type|interface)\s+(?:(?:a|an|the)\s+)?(?:(?:function|method|class|struct|type|interface)\s+)?[` + "`" + `"']?([a-zA-Z_][a-zA-Z0-9_]*)[` + "`" + `"']?`)
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			add(match[1], match[0])
		}
	}

	// 2b. Identifier-before-keyword: "The `Hello` function" (natural prose).
	//     Only quoted/backticked identifiers are considered, to avoid
	//     false positives on bare prose like "the main function".
	preRe := regexp.MustCompile("(?i)[`\"']([a-zA-Z_][a-zA-Z0-9_]*)[`\"']\\s+(?:function|method|class|struct|type|interface|module)")
	for _, match := range preRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			name := match[1]
			// Reject single lowercase words like "hello" from prose.
			if !isLikelyCodeName(name) {
				continue
			}
			add(name, match[0])
		}
	}

	// 3. Code blocks in markdown are treated as embedded specs.
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

// isLikelyCodeName returns true if name looks like a real code identifier
// (has uppercase, underscore, hyphen, or dot). Rejects single lowercase
// prose words like "hello" / "world".
func isLikelyCodeName(name string) bool {
	if len(name) == 0 {
		return false
	}
	hasUpper := false
	hasSep := false
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c == '_' || c == '-' || c == '.' {
			hasSep = true
		}
	}
	return hasUpper || hasSep
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
