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
	Claim         string          `json:"claim"`
	Evidence      string          `json:"evidence"`
	ClaimSymbols  []symbolInfo    `json:"claim_symbols"`
	TestSymbols   []symbolInfo    `json:"test_symbols"`
	Coverage      float64         `json:"coverage"`
	Covered       []symbolInfo    `json:"covered"`
	Uncovered     []symbolInfo    `json:"uncovered"`
	TestsWithoutSource []symbolInfo `json:"tests_without_source,omitempty"`
	Summary       string          `json:"summary"`
}

type symbolInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`    // function, method, class
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
