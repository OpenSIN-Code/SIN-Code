// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the adw (Architectural Debt Watchdogs) subcommand.
package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckLongFunctionsGo(t *testing.T) {
	content := `package main
func short() { println("hi") }
func veryLongFunction() {
	// line 1
	// line 2
` + strings.Repeat(`	println("filler")
`, 100) + `}
`
	issues := checkLongFunctionsGo("test.go", "test.go", content)
	if len(issues) != 1 {
		t.Fatalf("expected 1 long function issue, got %d", len(issues))
	}
	if issues[0].Type != "long_function" {
		t.Errorf("expected type 'long_function', got %q", issues[0].Type)
	}
	if !strings.Contains(issues[0].Message, "veryLongFunction") {
		t.Errorf("expected message to mention function name, got %q", issues[0].Message)
	}
}

func TestCheckLongFunctionsGo_InvalidSyntax(t *testing.T) {
	issues := checkLongFunctionsGo("bad.go", "bad.go", "not valid go")
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for invalid syntax, got %d", len(issues))
	}
}

func TestCheckLongFunctionsPython(t *testing.T) {
	content := `def short(): pass
def long_func():
` + strings.Repeat(`    pass
`, 101) + `
`
	issues := checkLongFunctionsPython("test.py", "test.py", content)
	if len(issues) != 1 {
		t.Fatalf("expected 1 long function issue, got %d", len(issues))
	}
	if issues[0].Type != "long_function" {
		t.Errorf("expected type 'long_function', got %q", issues[0].Type)
	}
	if !strings.Contains(issues[0].Message, "long_func") {
		t.Errorf("expected message to mention function name, got %q", issues[0].Message)
	}
}

func TestCheckLongFunctionsPython_Class(t *testing.T) {
	content := `class LongClass:
` + strings.Repeat(`    pass
`, 101) + `
`
	issues := checkLongFunctionsPython("test.py", "test.py", content)
	if len(issues) != 1 {
		t.Fatalf("expected 1 long class issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "LongClass") {
		t.Errorf("expected message to mention class name, got %q", issues[0].Message)
	}
}

func TestCheckLongFunctionsJS(t *testing.T) {
	content := `function short() { return 1; }
function longFunc() {
` + strings.Repeat(`  console.log(1);
`, 101) + `}
`
	issues := checkLongFunctionsJS("test.js", "test.js", content)
	if len(issues) != 1 {
		t.Fatalf("expected 1 long function issue, got %d", len(issues))
	}
	if issues[0].Type != "long_function" {
		t.Errorf("expected type 'long_function', got %q", issues[0].Type)
	}
	if !strings.Contains(issues[0].Message, "longFunc") {
		t.Errorf("expected message to mention function name, got %q", issues[0].Message)
	}
}

func TestCheckLongFunctionsJS_ClassNotDetected(t *testing.T) {
	// The JS regex requires a '(' after the name, so classes without parens won't match
	content := `class LongClass {
` + strings.Repeat(`  method() {}
`, 101) + `}
`
	issues := checkLongFunctionsJS("test.js", "test.js", content)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for class without parens, got %d", len(issues))
	}
}

func TestCheckTODOs(t *testing.T) {
	content := `package main
// TODO: fix this
// FIXME: urgent bug
// HACK: workaround
// normal comment
// XXX: review
// OPTIMIZE: slow
// REFACTOR: cleanup
`
	issues := checkTODOs("main.go", content)
	if len(issues) < 3 {
		t.Fatalf("expected at least 3 TODO issues, got %d", len(issues))
	}

	// Check FIXME and BUG are medium severity
	for _, issue := range issues {
		if strings.Contains(issue.Message, "FIXME") && issue.Severity != "medium" {
			t.Errorf("expected FIXME to be medium severity, got %q", issue.Severity)
		}
	}
}

func TestFindCircularDeps(t *testing.T) {
	imports := map[string][]string{
		"a.go": {"b.go"},
		"b.go": {"a.go"},
		"c.go": {"d.go"},
		"d.go": {},
	}
	issues := findCircularDeps(imports)
	// Code does not deduplicate reverse pairs, so a->b and b->a both produce issues
	if len(issues) != 2 {
		t.Fatalf("expected 2 circular dependency issues (reverse pairs), got %d", len(issues))
	}
	if issues[0].Type != "circular_dependency" {
		t.Errorf("expected type 'circular_dependency', got %q", issues[0].Type)
	}
	if issues[0].Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", issues[0].Severity)
	}
}

func TestFindCircularDeps_None(t *testing.T) {
	imports := map[string][]string{
		"a.go": {"b.go"},
		"b.go": {"c.go"},
		"c.go": {},
	}
	issues := findCircularDeps(imports)
	if len(issues) != 0 {
		t.Errorf("expected 0 circular dependencies, got %d", len(issues))
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main_test.go", true},
		{"test_main.py", true},
		{"main.spec.js", true},
		{"main.go", false},
		{"app.js", false},
	}
	for _, tt := range tests {
		if got := isTestFile(tt.path); got != tt.expected {
			t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestIsConfigFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"config.yaml", true},
		{"setup.py", true},
		{"Dockerfile", true},
		{"Makefile", true},
		{"go.mod", true},
		{"package.json", true},
		{"main.go", false},
	}
	for _, tt := range tests {
		if got := isConfigFile(tt.path); got != tt.expected {
			t.Errorf("isConfigFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestIsDocFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"README.md", true},
		{"LICENSE.txt", true},
		{"CHANGELOG.md", true},
		{"CONTRIBUTING.md", true},
		{"docs.rst", true},
		{"main.go", false},
	}
	for _, tt := range tests {
		if got := isDocFile(tt.path); got != tt.expected {
			t.Errorf("isDocFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestFindTestFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "utils.py"), []byte("# utils"), 0644)
	os.WriteFile(filepath.Join(dir, "test_utils.py"), []byte("# tests"), 0644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("// app"), 0644)
	os.WriteFile(filepath.Join(dir, "app.test.js"), []byte("// tests"), 0644)

	if !findTestFile(dir, "main.go", "go") {
		t.Error("expected to find test file for main.go")
	}
	if !findTestFile(dir, "utils.py", "python") {
		t.Error("expected to find test file for utils.py")
	}
	if !findTestFile(dir, "app.js", "javascript") {
		t.Error("expected to find test file for app.js")
	}
	if findTestFile(dir, "lib.rs", "rust") {
		t.Error("expected no test file for non-existent lib.rs")
	}
}

func TestFindTestFile_UnknownLang(t *testing.T) {
	if !findTestFile(".", "main.cob", "cobol") {
		t.Error("expected findTestFile to return true for unknown language")
	}
}

func TestScanDebt(t *testing.T) {
	dir := t.TempDir()
	// Create a large Go file with many imports
	goContent := `package main
import (
	"fmt"
	"os"
	"strings"
	"bytes"
	"io"
	"net/http"
	"encoding/json"
	"time"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"errors"
	"sync"
	"context"
	"math"
)
func main() {
` + strings.Repeat(`	fmt.Println("hello")
`, 510) + `}
`
	os.WriteFile(filepath.Join(dir, "big.go"), []byte(goContent), 0644)

	// Create a Python file without tests
	os.WriteFile(filepath.Join(dir, "module.py"), []byte("def hello(): pass\n"), 0644)

	// Create a file with TODO
	os.WriteFile(filepath.Join(dir, "todo.go"), []byte("package main\n// TODO: fix this\n"), 0644)

	result := scanDebt(dir, false)
	if result.Summary.FilesScanned < 2 {
		t.Errorf("expected at least 2 files scanned, got %d", result.Summary.FilesScanned)
	}
	if result.Summary.TotalIssues == 0 {
		t.Error("expected some issues to be found")
	}
	if result.Summary.Low == 0 {
		t.Error("expected at least low-severity issues (missing tests, TODOs)")
	}
	if result.Summary.Medium == 0 {
		t.Error("expected at least medium-severity issues (large file)")
	}
	if result.Summary.High == 0 {
		t.Error("expected at least high-severity issues (god module with 16+ imports)")
	}
	if result.Grade == "A" || result.Grade == "B" {
		t.Errorf("expected low grade due to many issues, got %q", result.Grade)
	}
}

func TestScanDebt_Strict(t *testing.T) {
	dir := t.TempDir()
	goContent := `package main
import (
	"fmt"
	"os"
	"strings"
	"bytes"
	"io"
	"net/http"
	"encoding/json"
	"time"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"errors"
	"sync"
	"context"
	"math"
)
func main() { fmt.Println("hi") }
`
	os.WriteFile(filepath.Join(dir, "bad.go"), []byte(goContent), 0644)

	result := scanDebt(dir, true)
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1 in strict mode with high issues, got %d", result.ExitCode)
	}
}

func TestOutputTextADW(t *testing.T) {
	result := &adwResult{
		Path: "/tmp/test",
		Summary: adwSummary{
			FilesScanned: 10,
			TotalIssues:  2,
			Critical:     0,
			High:         1,
			Medium:       1,
			Low:          0,
		},
		Score:    80,
		Grade:    "B",
		ExitCode: 0,
		Issues: []adwIssue{
			{Type: "large_file", Severity: "medium", File: "big.go", Message: "File is 600 lines"},
			{Type: "god_module", Severity: "high", File: "main.go", Message: "File imports 16 modules"},
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextADW(result); err != nil {
		t.Fatalf("outputTextADW failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Grade: B") {
		t.Errorf("expected output to contain grade B, got %q", out)
	}
	if !strings.Contains(out, "big.go") {
		t.Errorf("expected output to contain big.go, got %q", out)
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("expected output to contain main.go, got %q", out)
	}
}

func TestOutputTextADW_NoIssues(t *testing.T) {
	result := &adwResult{
		Path: "/tmp/test",
		Summary: adwSummary{
			FilesScanned: 5,
			TotalIssues:  0,
		},
		Score:    100,
		Grade:    "A",
		ExitCode: 0,
		Issues:   []adwIssue{},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextADW(result); err != nil {
		t.Fatalf("outputTextADW failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "No architectural debt detected") {
		t.Errorf("expected no-issues message, got %q", out)
	}
}
