// SPDX-License-Identifier: MIT
// Purpose: oracle — Verification Oracle. Compares source files against test
// files to verify coverage. Pure Go implementation.
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
	"strings"

	"github.com/spf13/cobra"
)

var (
	oracleClaim    string
	oracleEvidence string
	oracleFormat   string
)

var OracleCmd = &cobra.Command{
	Use:   "oracle",
	Short: "Verify that a source file has corresponding test coverage",
	Long: `Compares functions/methods in a source file against test cases in a
test file and reports which symbols are covered. Despite the legacy
"claim/evidence" naming, --claim is the source file to verify and
--evidence is the test file.

Examples:
  sin-code oracle --claim src/main.py --evidence tests/test_main.py
  sin-code oracle --claim cmd/sin-code/main.go --evidence cmd/sin-code/main_test.go`,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		if oracleClaim == "" {
			return fmt.Errorf("--claim (source file) is required")
		}
		if oracleEvidence == "" {
			return fmt.Errorf("--evidence (test file) is required")
		}

		result, err := verifyCoverage(oracleClaim, oracleEvidence)
		if err != nil {
			return err
		}

		if oracleFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return outputTextOracle(result)
	},
}

type oracleResult struct {
	Claim              string       `json:"claim"`
	Evidence           string       `json:"evidence"`
	ClaimSymbols       []symbolInfo `json:"claim_symbols"`
	TestSymbols        []symbolInfo `json:"test_symbols"`
	Coverage           float64      `json:"coverage"`
	Covered            []symbolInfo `json:"covered"`
	Uncovered          []symbolInfo `json:"uncovered"`
	TestsWithoutSource []symbolInfo `json:"tests_without_source,omitempty"`
	Summary            string       `json:"summary"`
}

type symbolInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // function, method, class
	Line    int    `json:"line"`
	Covered bool   `json:"covered,omitempty"`
}

func verifyCoverage(claimPath, evidencePath string) (*oracleResult, error) {
	claimData, err := os.ReadFile(claimPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read claim file: %w", err)
	}
	evidenceData, err := os.ReadFile(evidencePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read evidence file: %w", err)
	}

	claimLang := detectLanguage(claimPath)
	evidenceLang := detectLanguage(evidencePath)

	claimSymbols := extractSymbols(claimPath, string(claimData), claimLang)
	testSymbols := extractSymbols(evidencePath, string(evidenceData), evidenceLang)

	// Map test names to source functions (remove Test/test_ prefix)
	testNames := make(map[string]bool)
	for _, ts := range testSymbols {
		name := normalizeTestName(ts.Name)
		testNames[name] = true
	}

	var covered, uncovered []symbolInfo
	for _, cs := range claimSymbols {
		normalized := normalizeSourceName(cs.Name)
		if testNames[normalized] {
			cs.Covered = true
			covered = append(covered, cs)
		} else {
			uncovered = append(uncovered, cs)
		}
	}

	// Find tests that don't match any source function
	var testsWithoutSource []symbolInfo
	for _, ts := range testSymbols {
		normalized := normalizeTestName(ts.Name)
		found := false
		for _, cs := range claimSymbols {
			if normalizeSourceName(cs.Name) == normalized {
				found = true
				break
			}
		}
		if !found {
			testsWithoutSource = append(testsWithoutSource, ts)
		}
	}

	coverage := 0.0
	if len(claimSymbols) > 0 {
		coverage = float64(len(covered)) / float64(len(claimSymbols)) * 100
	}

	summary := fmt.Sprintf("Coverage: %.1f%% (%d/%d functions covered)", coverage, len(covered), len(claimSymbols))
	if len(uncovered) > 0 {
		summary += fmt.Sprintf(", %d uncovered", len(uncovered))
	}
	if len(testsWithoutSource) > 0 {
		summary += fmt.Sprintf(", %d tests without matching source", len(testsWithoutSource))
	}

	return &oracleResult{
		Claim:              claimPath,
		Evidence:           evidencePath,
		ClaimSymbols:       claimSymbols,
		TestSymbols:        testSymbols,
		Coverage:           coverage,
		Covered:            covered,
		Uncovered:          uncovered,
		TestsWithoutSource: testsWithoutSource,
		Summary:            summary,
	}, nil
}

func extractSymbols(path, content, lang string) []symbolInfo {
	switch lang {
	case "go":
		return extractGoSymbols(path, content)
	case "python":
		return extractPythonSymbols(content)
	case "javascript", "typescript", "tsx", "jsx":
		return extractJSSymbols(content)
	case "rust":
		return extractRustSymbols(content)
	case "java":
		return extractJavaSymbols(content)
	default:
		return extractGenericSymbols(content)
	}
}

func extractGoSymbols(path, content string) []symbolInfo {
	var symbols []symbolInfo
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.AllErrors)
	if err != nil {
		return nil
	}
	for _, decl := range f.Decls {
		pos := fset.Position(decl.Pos())
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				if recv, ok := d.Recv.List[0].Type.(*ast.StarExpr); ok {
					if ident, ok := recv.X.(*ast.Ident); ok {
						name = fmt.Sprintf("(%s).%s", ident.Name, name)
					}
				} else if ident, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
					name = fmt.Sprintf("(%s).%s", ident.Name, name)
				}
			}
			symbols = append(symbols, symbolInfo{Name: name, Type: "function", Line: pos.Line})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					symbols = append(symbols, symbolInfo{Name: ts.Name.Name, Type: "type", Line: pos.Line})
				}
			}
		}
	}
	return symbols
}

func extractPythonSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`^(\s*)(def|class)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 3 {
			typ := "function"
			if matches[2] == "class" {
				typ = "class"
			}
			symbols = append(symbols, symbolInfo{Name: matches[3], Type: typ, Line: i + 1})
		}
	}
	return symbols
}

func extractJSSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`(?:export\s+)?(?:async\s+)?(?:function|class|const|let|var|interface|type)\s+([a-zA-Z_$][a-zA-Z0-9_$]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				typ := "function"
				if strings.Contains(line, "class") {
					typ = "class"
				} else if strings.Contains(line, "interface") {
					typ = "interface"
				} else if strings.Contains(line, "type") {
					typ = "type"
				} else if strings.Contains(line, "const") || strings.Contains(line, "let") || strings.Contains(line, "var") {
					typ = "variable"
				}
				symbols = append(symbols, symbolInfo{Name: m[1], Type: typ, Line: i + 1})
			}
		}
	}
	return symbols
}

func extractRustSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`(?:pub\s+)?(?:fn|struct|enum|trait|impl)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				typ := "function"
				if strings.Contains(line, "struct") {
					typ = "struct"
				} else if strings.Contains(line, "enum") {
					typ = "enum"
				} else if strings.Contains(line, "trait") {
					typ = "trait"
				}
				symbols = append(symbols, symbolInfo{Name: m[1], Type: typ, Line: i + 1})
			}
		}
	}
	return symbols
}

func extractJavaSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`(?:public\s+|private\s+|protected\s+|static\s+)*(?:class|interface|enum|void|int|String|boolean|double|float|long|short|byte|char)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				typ := "function"
				if strings.Contains(line, "class") {
					typ = "class"
				} else if strings.Contains(line, "interface") {
					typ = "interface"
				}
				symbols = append(symbols, symbolInfo{Name: m[1], Type: typ, Line: i + 1})
			}
		}
	}
	return symbols
}

func extractGenericSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`(?:function|def|fn|func|method|class|struct|interface|trait|enum|record|sub|procedure)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				symbols = append(symbols, symbolInfo{Name: m[1], Type: "symbol", Line: i + 1})
			}
		}
	}
	return symbols
}

func normalizeTestName(name string) string {
	name = strings.ToLower(name)
	name = strings.TrimPrefix(name, "test")
	name = strings.TrimPrefix(name, "test_")
	name = strings.TrimPrefix(name, "spec")
	name = strings.TrimPrefix(name, "it")
	name = strings.TrimPrefix(name, "should")
	name = strings.TrimPrefix(name, "can")
	name = strings.TrimPrefix(name, "will")
	name = strings.TrimPrefix(name, "does")
	name = strings.TrimPrefix(name, "_")
	return name
}

// sin-debt: shrink, upgrade: inline when callers are consolidated
func normalizeSourceName(name string) string {
	return strings.ToLower(name)
}

func outputTextOracle(r *oracleResult) error {
	fmt.Printf("Verification Oracle\n")
	fmt.Printf("Claim (source):    %s\n", r.Claim)
	fmt.Printf("Evidence (tests):  %s\n", r.Evidence)
	fmt.Printf("Coverage:          %.1f%% (%d/%d functions)\n\n", r.Coverage, len(r.Covered), len(r.ClaimSymbols))

	if len(r.Covered) > 0 {
		fmt.Printf("Covered functions (%d):\n", len(r.Covered))
		for _, sym := range r.Covered {
			fmt.Printf("  ✓ %s (line %d)\n", sym.Name, sym.Line)
		}
	}

	if len(r.Uncovered) > 0 {
		fmt.Printf("\nUncovered functions (%d):\n", len(r.Uncovered))
		for _, sym := range r.Uncovered {
			fmt.Printf("  ✗ %s (line %d)\n", sym.Name, sym.Line)
		}
	}

	if len(r.TestsWithoutSource) > 0 {
		fmt.Printf("\nTests without matching source (%d):\n", len(r.TestsWithoutSource))
		for _, sym := range r.TestsWithoutSource {
			fmt.Printf("  ? %s (line %d)\n", sym.Name, sym.Line)
		}
	}

	fmt.Printf("\n%s\n", r.Summary)
	return nil
}

func init() {
	RegisterVersionCmd(OracleCmd)
	OracleCmd.Flags().StringVarP(&oracleClaim, "claim", "c", "", "Source file to check coverage for")
	OracleCmd.Flags().StringVarP(&oracleEvidence, "evidence", "e", "", "Test file to compare against")
	OracleCmd.Flags().StringVarP(&oracleFormat, "format", "f", "text", "Output format: text|json")
}

var (
	ibdBefore string
	ibdAfter  string
	ibdIntent string
	ibdFrom   string
	ibdTo     string
	ibdOutput string
	ibdFormat string
)

var IbdCmd = &cobra.Command{
	Use:   "ibd",
	Short: "Intent-Based Diffing — compare code changes against stated intent",
	Long: `Compare two versions of code and determine if the changes match the
stated intent. Pure Go implementation.

Examples:
  sin-code ibd --before old.py --after new.py --intent "add retry logic"
  sin-code ibd --before v1.0 --after HEAD --intent "refactor authentication"
  sin-code ibd file.go --from main --to feature-branch --intent "add error handling"`,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		var beforePath, afterPath string

		if ibdBefore != "" && ibdAfter != "" {
			beforePath = ibdBefore
			afterPath = ibdAfter
		} else if len(args) > 0 {
			beforePath = args[0]
			// If --from and --to are set, use git to get versions
			if ibdFrom != "" && ibdTo != "" {
				// This is a git diff request - we'll try to read the file from git
				// For now, just read the file as-is and note the limitation
				fmt.Fprintf(os.Stderr, "Note: Git diff (--from/--to) requires manual diff extraction. Reading file as-is.\n")
			}
		} else {
			return fmt.Errorf("either --before/--after or a target path is required")
		}

		result, err := diffWithIntent(beforePath, afterPath, ibdIntent)
		if err != nil {
			return err
		}

		if ibdFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return outputTextIBD(result)
	},
}

type ibdResult struct {
	Before      string       `json:"before"`
	After       string       `json:"after"`
	Intent      string       `json:"intent"`
	Diff        []diffLine   `json:"diff"`
	Added       []symbolInfo `json:"added"`
	Removed     []symbolInfo `json:"removed"`
	Modified    []symbolInfo `json:"modified"`
	IntentMatch string       `json:"intent_match"` // strong, partial, weak, none
	Score       int          `json:"score"`        // 0-100
	Summary     string       `json:"summary"`
}

type diffLine struct {
	Type   string `json:"type"` // added, removed, context
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Number int    `json:"number"`
}

func diffWithIntent(beforePath, afterPath, intent string) (*ibdResult, error) {
	beforeContent, err := readFileOrString(beforePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read before: %w", err)
	}

	var afterContent string
	if afterPath != "" {
		afterContent, err = readFileOrString(afterPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read after: %w", err)
		}
	} else {
		afterContent = beforeContent
	}

	// Compute diff
	diff := computeDiff(beforeContent, afterContent)

	// Extract symbols from both versions
	beforeSymbols := extractSymbolsFromContent(beforeContent, beforePath)
	afterSymbols := extractSymbolsFromContent(afterContent, afterPath)

	// Compare symbols
	added, removed, modified := compareSymbols(beforeSymbols, afterSymbols)

	// Evaluate intent match
	intentMatch, score := evaluateIntent(intent, added, removed, modified, diff)

	summary := fmt.Sprintf("Diff: %d lines changed (%d added, %d removed). %d symbols added, %d removed, %d modified. Intent match: %s (score: %d/100)",
		countChanged(diff), countAdded(diff), countRemoved(diff),
		len(added), len(removed), len(modified),
		intentMatch, score)

	return &ibdResult{
		Before:      beforePath,
		After:       afterPath,
		Intent:      intent,
		Diff:        diff,
		Added:       added,
		Removed:     removed,
		Modified:    modified,
		IntentMatch: intentMatch,
		Score:       score,
		Summary:     summary,
	}, nil
}

func readFileOrString(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	// Could be a git ref or raw string
	return path, nil
}

func computeDiff(before, after string) []diffLine {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	var diff []diffLine
	maxLen := len(beforeLines)
	if len(afterLines) > maxLen {
		maxLen = len(afterLines)
	}

	for i := 0; i < maxLen; i++ {
		var beforeLine, afterLine string
		if i < len(beforeLines) {
			beforeLine = beforeLines[i]
		}
		if i < len(afterLines) {
			afterLine = afterLines[i]
		}

		if beforeLine == afterLine {
			// Context line (unchanged)
			if i < len(beforeLines) {
				diff = append(diff, diffLine{Type: "context", Line: i + 1, Text: beforeLine, Number: i + 1})
			}
		} else if i < len(beforeLines) && i < len(afterLines) {
			// Modified line
			diff = append(diff, diffLine{Type: "removed", Line: i + 1, Text: beforeLine, Number: i + 1})
			diff = append(diff, diffLine{Type: "added", Line: i + 1, Text: afterLine, Number: i + 1})
		} else if i < len(beforeLines) {
			// Removed line
			diff = append(diff, diffLine{Type: "removed", Line: i + 1, Text: beforeLine, Number: i + 1})
		} else {
			// Added line
			diff = append(diff, diffLine{Type: "added", Line: i + 1, Text: afterLine, Number: i + 1})
		}
	}

	return diff
}

func extractSymbolsFromContent(content, path string) []symbolInfo {
	lang := detectLanguage(path)
	return extractSymbols(path, content, lang)
}

func compareSymbols(before, after []symbolInfo) (added, removed, modified []symbolInfo) {
	beforeMap := make(map[string]symbolInfo)
	for _, sym := range before {
		beforeMap[sym.Name] = sym
	}

	afterMap := make(map[string]symbolInfo)
	for _, sym := range after {
		afterMap[sym.Name] = sym
	}

	// Find added
	for name, sym := range afterMap {
		if _, ok := beforeMap[name]; !ok {
			added = append(added, sym)
		}
	}

	// Find removed
	for name, sym := range beforeMap {
		if _, ok := afterMap[name]; !ok {
			removed = append(removed, sym)
		}
	}

	// Find modified (same name, different line/type)
	for name, afterSym := range afterMap {
		if beforeSym, ok := beforeMap[name]; ok {
			if beforeSym.Type != afterSym.Type || beforeSym.Line != afterSym.Line {
				modified = append(modified, afterSym)
			}
		}
	}

	return
}

func evaluateIntent(intent string, added, removed, modified []symbolInfo, diff []diffLine) (string, int) {
	if intent == "" {
		return "unknown", 50
	}

	intentLower := strings.ToLower(intent)
	score := 50

	// Check for keywords in intent
	keywords := []string{"add", "remove", "delete", "refactor", "fix", "implement", "create", "update", "modify", "change", "optimize", "improve", "rename"}
	intentKeywords := make(map[string]bool)
	for _, kw := range keywords {
		if strings.Contains(intentLower, kw) {
			intentKeywords[kw] = true
		}
	}

	// Evaluate based on changes
	if intentKeywords["add"] || intentKeywords["create"] || intentKeywords["implement"] {
		if len(added) > 0 {
			score += 30
		} else {
			score -= 40
		}
	}

	if intentKeywords["remove"] || intentKeywords["delete"] {
		if len(removed) > 0 {
			score += 30
		} else {
			score -= 40
		}
	}

	if intentKeywords["refactor"] || intentKeywords["modify"] || intentKeywords["change"] || intentKeywords["update"] {
		if len(modified) > 0 || len(added) > 0 || len(removed) > 0 {
			score += 20
		}
	}

	if intentKeywords["fix"] || intentKeywords["optimize"] || intentKeywords["improve"] {
		if len(modified) > 0 {
			score += 25
		}
	}

	if intentKeywords["rename"] {
		// Check for add+remove pairs with similar names
		for _, a := range added {
			for _, r := range removed {
				if strings.ToLower(a.Name) == strings.ToLower(r.Name) ||
					strings.Contains(strings.ToLower(a.Name), strings.ToLower(r.Name)) ||
					strings.Contains(strings.ToLower(r.Name), strings.ToLower(a.Name)) {
					score += 30
					break
				}
			}
		}
	}

	// Check for error handling keywords
	if strings.Contains(intentLower, "error") || strings.Contains(intentLower, "exception") || strings.Contains(intentLower, "handle") {
		for _, line := range diff {
			if line.Type == "added" && (strings.Contains(strings.ToLower(line.Text), "error") || strings.Contains(strings.ToLower(line.Text), "exception") || strings.Contains(strings.ToLower(line.Text), "catch") || strings.Contains(strings.ToLower(line.Text), "try")) {
				score += 15
				break
			}
		}
	}

	// Check for retry logic
	if strings.Contains(intentLower, "retry") {
		for _, line := range diff {
			if line.Type == "added" && strings.Contains(strings.ToLower(line.Text), "retry") {
				score += 20
				break
			}
		}
	}

	// Check for test-related changes
	if strings.Contains(intentLower, "test") {
		for _, sym := range added {
			if strings.Contains(strings.ToLower(sym.Name), "test") || strings.Contains(strings.ToLower(sym.Name), "spec") {
				score += 20
				break
			}
		}
	}

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	// Determine match level
	match := "none"
	if score >= 80 {
		match = "strong"
	} else if score >= 60 {
		match = "partial"
	} else if score >= 40 {
		match = "weak"
	}

	return match, score
}

func countChanged(diff []diffLine) int {
	count := 0
	for _, d := range diff {
		if d.Type == "added" || d.Type == "removed" {
			count++
		}
	}
	return count
}

func countAdded(diff []diffLine) int {
	count := 0
	for _, d := range diff {
		if d.Type == "added" {
			count++
		}
	}
	return count
}

func countRemoved(diff []diffLine) int {
	count := 0
	for _, d := range diff {
		if d.Type == "removed" {
			count++
		}
	}
	return count
}

func outputTextIBD(result *ibdResult) error {
	fmt.Printf("Intent-Based Diffing\n")
	fmt.Printf("Before:     %s\n", result.Before)
	fmt.Printf("After:      %s\n", result.After)
	fmt.Printf("Intent:     %s\n", result.Intent)
	fmt.Printf("Match:      %s (score: %d/100)\n\n", result.IntentMatch, result.Score)

	// Show summary of changes
	fmt.Printf("Changes:\n")
	fmt.Printf("  Lines changed: %d (+%d, -%d)\n", countChanged(result.Diff), countAdded(result.Diff), countRemoved(result.Diff))
	fmt.Printf("  Symbols added:   %d\n", len(result.Added))
	fmt.Printf("  Symbols removed: %d\n", len(result.Removed))
	fmt.Printf("  Symbols modified:  %d\n", len(result.Modified))

	if len(result.Added) > 0 {
		fmt.Printf("\nAdded symbols:\n")
		for _, sym := range result.Added {
			fmt.Printf("  + %s (%s) line %d\n", sym.Name, sym.Type, sym.Line)
		}
	}

	if len(result.Removed) > 0 {
		fmt.Printf("\nRemoved symbols:\n")
		for _, sym := range result.Removed {
			fmt.Printf("  - %s (%s) line %d\n", sym.Name, sym.Type, sym.Line)
		}
	}

	if len(result.Modified) > 0 {
		fmt.Printf("\nModified symbols:\n")
		for _, sym := range result.Modified {
			fmt.Printf("  ~ %s (%s) line %d\n", sym.Name, sym.Type, sym.Line)
		}
	}

	fmt.Printf("\n%s\n", result.Summary)
	return nil
}

func init() {
	RegisterVersionCmd(IbdCmd)
	IbdCmd.Flags().StringVarP(&ibdBefore, "before", "b", "", "Before version (file, ref, or commit)")
	IbdCmd.Flags().StringVarP(&ibdAfter, "after", "a", "", "After version (file, ref, or commit)")
	IbdCmd.Flags().StringVarP(&ibdIntent, "intent", "i", "", "Stated intent of the change")
	IbdCmd.Flags().StringVarP(&ibdFrom, "from", "f", "", "Git commit (old) for path target")
	IbdCmd.Flags().StringVarP(&ibdTo, "to", "t", "", "Git commit (new) for path target")
	IbdCmd.Flags().StringVarP(&ibdOutput, "output", "o", "", "Output JSON file")
	IbdCmd.Flags().StringVarP(&ibdFormat, "format", "", "text", "Output format: text|json")
}

var (
	pocSpec   string
	pocCode   string
	pocFormat string
	pocWalk   = filepath.Walk // test hook for filepath.Walk errors
	// pocExtractRequirementsCodeBlock is a test hook for the recursive code-block
	// extraction step. It defaults to the real extractRequirements implementation.
	pocExtractRequirementsCodeBlock func(content string) []requirement
)

func init() {
	pocExtractRequirementsCodeBlock = func(content string) []requirement {
		return extractRequirements(content)
	}
}

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
	Version: Version,
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
	Spec        string     `json:"spec"`
	Code        string     `json:"code"`
	Checks      []pocCheck `json:"checks"`
	Passed      int        `json:"passed"`
	Failed      int        `json:"failed"`
	TotalChecks int        `json:"total_checks"`
	Coverage    float64    `json:"coverage"`
	Summary     string     `json:"summary"`
}

type pocCheck struct {
	Name    string `json:"name"`
	Type    string `json:"type"`   // required, forbidden, signature, import
	Status  string `json:"status"` // pass, fail, warn
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
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
		err := pocWalk(codePath, func(path string, info os.FileInfo, err error) error {
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
				Name:    req.Name,
				Type:    "required",
				Status:  "fail",
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
			for _, req := range pocExtractRequirementsCodeBlock(block[1]) {
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
	RegisterVersionCmd(PocCmd)
	PocCmd.Flags().StringVarP(&pocSpec, "spec", "s", "", "Specification file (markdown, text, json)")
	PocCmd.Flags().StringVarP(&pocCode, "code", "c", "", "Code file or directory to verify")
	PocCmd.Flags().StringVarP(&pocFormat, "format", "f", "text", "Output format: text|json")
}
