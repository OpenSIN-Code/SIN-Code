// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the oracle (Verification Oracle) subcommand.
package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCoverage_GoFiles(t *testing.T) {
	dir := t.TempDir()
	claim := filepath.Join(dir, "main.go")
	evidence := filepath.Join(dir, "main_test.go")

	claimContent := `package main
func Add(a, b int) int { return a + b }
func Subtract(a, b int) int { return a - b }
func Multiply(a, b int) int { return a * b }
`
	evidenceContent := `package main
import "testing"
func TestAdd(t *testing.T) {}
func TestSubtract(t *testing.T) {}
func TestMultiply(t *testing.T) {}
`
	os.WriteFile(claim, []byte(claimContent), 0644)
	os.WriteFile(evidence, []byte(evidenceContent), 0644)

	result, err := verifyCoverage(claim, evidence)
	if err != nil {
		t.Fatalf("verifyCoverage failed: %v", err)
	}
	if result.Coverage != 100.0 {
		t.Errorf("expected 100%% coverage, got %.1f%%", result.Coverage)
	}
	if len(result.Covered) != 3 {
		t.Errorf("expected 3 covered functions, got %d", len(result.Covered))
	}
	if len(result.Uncovered) != 0 {
		t.Errorf("expected 0 uncovered functions, got %d", len(result.Uncovered))
	}
}

func TestVerifyCoverage_PartialCoverage(t *testing.T) {
	dir := t.TempDir()
	claim := filepath.Join(dir, "calc.go")
	evidence := filepath.Join(dir, "calc_test.go")

	claimContent := `package main
func Add(a, b int) int { return a + b }
func Subtract(a, b int) int { return a - b }
func Multiply(a, b int) int { return a * b }
`
	evidenceContent := `package main
import "testing"
func TestAdd(t *testing.T) {}
func TestMultiply(t *testing.T) {}
`
	os.WriteFile(claim, []byte(claimContent), 0644)
	os.WriteFile(evidence, []byte(evidenceContent), 0644)

	result, err := verifyCoverage(claim, evidence)
	if err != nil {
		t.Fatalf("verifyCoverage failed: %v", err)
	}
	if result.Coverage < 66.6 || result.Coverage > 66.8 {
		t.Errorf("expected ~66.7%% coverage, got %.1f%%", result.Coverage)
	}
	if len(result.Covered) != 2 {
		t.Errorf("expected 2 covered functions, got %d", len(result.Covered))
	}
	if len(result.Uncovered) != 1 {
		t.Errorf("expected 1 uncovered function, got %d", len(result.Uncovered))
	}
	if result.Uncovered[0].Name != "Subtract" {
		t.Errorf("expected uncovered function to be Subtract, got %q", result.Uncovered[0].Name)
	}
}

func TestVerifyCoverage_NoTests(t *testing.T) {
	dir := t.TempDir()
	claim := filepath.Join(dir, "empty.go")
	evidence := filepath.Join(dir, "empty_test.go")

	claimContent := `package main
func DoNothing() {}
`
	evidenceContent := `package main
import "testing"
func TestSomethingElse(t *testing.T) {}
`
	os.WriteFile(claim, []byte(claimContent), 0644)
	os.WriteFile(evidence, []byte(evidenceContent), 0644)

	result, err := verifyCoverage(claim, evidence)
	if err != nil {
		t.Fatalf("verifyCoverage failed: %v", err)
	}
	if result.Coverage != 0.0 {
		t.Errorf("expected 0%% coverage, got %.1f%%", result.Coverage)
	}
	if len(result.Uncovered) != 1 {
		t.Errorf("expected 1 uncovered function, got %d", len(result.Uncovered))
	}
	if len(result.TestsWithoutSource) != 1 {
		t.Errorf("expected 1 test without source, got %d", len(result.TestsWithoutSource))
	}
}

func TestVerifyCoverage_EmptyFiles(t *testing.T) {
	dir := t.TempDir()
	claim := filepath.Join(dir, "empty.go")
	evidence := filepath.Join(dir, "empty_test.go")

	os.WriteFile(claim, []byte("package main\n"), 0644)
	os.WriteFile(evidence, []byte("package main\nimport \"testing\"\n"), 0644)

	result, err := verifyCoverage(claim, evidence)
	if err != nil {
		t.Fatalf("verifyCoverage failed: %v", err)
	}
	if result.Coverage != 0.0 {
		t.Errorf("expected 0%% coverage for empty files, got %.1f%%", result.Coverage)
	}
	if len(result.ClaimSymbols) != 0 {
		t.Errorf("expected 0 claim symbols, got %d", len(result.ClaimSymbols))
	}
}

func TestVerifyCoverage_MissingClaim(t *testing.T) {
	_, err := verifyCoverage("/nonexistent/file.go", "/nonexistent/file_test.go")
	if err == nil {
		t.Error("expected error for missing claim file")
	}
}

func TestVerifyCoverage_MissingEvidence(t *testing.T) {
	dir := t.TempDir()
	claim := filepath.Join(dir, "main.go")
	os.WriteFile(claim, []byte("package main\n"), 0644)
	_, err := verifyCoverage(claim, "/nonexistent/file_test.go")
	if err == nil {
		t.Error("expected error for missing evidence file")
	}
}

func TestExtractGoSymbols(t *testing.T) {
	content := `package main
type Point struct { X, Y float64 }
func Add(a, b int) int { return a + b }
func (p *Point) Move(dx, dy float64) {}
func main() {}
`
	syms := extractGoSymbols("main.go", content)
	expected := map[string]bool{
		"Add":         true,
		"main":        true,
		"(Point).Move": true,
		"Point":       true,
	}
	if len(syms) < 4 {
		t.Fatalf("expected at least 4 symbols, got %d: %v", len(syms), syms)
	}
	for _, sym := range syms {
		if !expected[sym.Name] {
			t.Errorf("unexpected symbol %q", sym.Name)
		}
	}
}

func TestExtractGoSymbols_InvalidSyntax(t *testing.T) {
	syms := extractGoSymbols("bad.go", "not valid go")
	if syms != nil {
		t.Errorf("expected nil for invalid Go syntax, got %v", syms)
	}
}

func TestExtractPythonSymbols(t *testing.T) {
	content := `def hello():
    pass
class MyClass:
    def method(self):
        pass
`
	syms := extractPythonSymbols(content)
	if len(syms) != 3 {
		t.Fatalf("expected 3 symbols, got %d: %v", len(syms), syms)
	}
	found := make(map[string]bool)
	for _, sym := range syms {
		found[sym.Name] = true
	}
	if !found["hello"] || !found["MyClass"] || !found["method"] {
		t.Errorf("expected hello, MyClass, method, got %v", syms)
	}
}

func TestExtractJSSymbols(t *testing.T) {
	content := `function hello() { return 1; }
const x = 5;
class MyClass {}
interface MyInterface {}
type MyType = string;
`
	syms := extractJSSymbols(content)
	found := make(map[string]bool)
	for _, sym := range syms {
		found[sym.Name] = true
	}
	if !found["hello"] || !found["MyClass"] || !found["MyInterface"] || !found["MyType"] || !found["x"] {
		t.Errorf("expected hello, MyClass, MyInterface, MyType, x, got %v", syms)
	}
}

func TestExtractRustSymbols(t *testing.T) {
	content := `fn hello() {}
struct Point { x: i32, y: i32 }
enum Color { Red, Green }
trait Drawable {}
`
	syms := extractRustSymbols(content)
	found := make(map[string]bool)
	for _, sym := range syms {
		found[sym.Name] = true
	}
	if !found["hello"] || !found["Point"] || !found["Color"] || !found["Drawable"] {
		t.Errorf("expected hello, Point, Color, Drawable, got %v", syms)
	}
}

func TestExtractJavaSymbols(t *testing.T) {
	content := `public class Main {
    public void run() {}
}
interface Runnable {}
`
	syms := extractJavaSymbols(content)
	found := make(map[string]bool)
	for _, sym := range syms {
		found[sym.Name] = true
	}
	if !found["Main"] || !found["run"] || !found["Runnable"] {
		t.Errorf("expected Main, run, Runnable, got %v", syms)
	}
}

func TestExtractGenericSymbols(t *testing.T) {
	content := `function hello() {}
def foo() {}
fn bar() {}
class MyClass {}
struct Point {}
`
	syms := extractGenericSymbols(content)
	found := make(map[string]bool)
	for _, sym := range syms {
		found[sym.Name] = true
	}
	if !found["hello"] || !found["foo"] || !found["bar"] || !found["MyClass"] || !found["Point"] {
		t.Errorf("expected hello, foo, bar, MyClass, Point, got %v", syms)
	}
}

func TestNormalizeTestName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TestAdd", "add"},
		{"test_add", "add"},
		{"specAdd", "add"},
		{"itShouldAdd", "add"},
		{"canAdd", "add"},
		{"willAdd", "add"},
		{"doesAdd", "add"},
		{"Test_Some_Thing", "some_thing"},
	}
	for _, tt := range tests {
		if got := normalizeTestName(tt.input); got != tt.expected {
			t.Errorf("normalizeTestName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeSourceName(t *testing.T) {
	if got := normalizeSourceName("Add"); got != "add" {
		t.Errorf("normalizeSourceName(\"Add\") = %q, want \"add\"", got)
	}
	if got := normalizeSourceName("some_function"); got != "some_function" {
		t.Errorf("normalizeSourceName(\"some_function\") = %q, want \"some_function\"", got)
	}
}

func TestOutputTextOracle(t *testing.T) {
	result := &oracleResult{
		Claim:         "main.go",
		Evidence:      "main_test.go",
		Coverage:      50.0,
		ClaimSymbols:  []symbolInfo{{Name: "Add", Type: "function", Line: 1}, {Name: "Sub", Type: "function", Line: 2}},
		Covered:       []symbolInfo{{Name: "Add", Type: "function", Line: 1, Covered: true}},
		Uncovered:     []symbolInfo{{Name: "Sub", Type: "function", Line: 2}},
		TestSymbols:   []symbolInfo{{Name: "TestAdd", Type: "function", Line: 1}},
		Summary:       "Coverage: 50.0% (1/2 functions covered)",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextOracle(result); err != nil {
		t.Fatalf("outputTextOracle failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "50.0%") {
		t.Errorf("expected output to contain coverage percentage, got %q", out)
	}
	if !strings.Contains(out, "Add") {
		t.Errorf("expected output to contain covered function 'Add', got %q", out)
	}
	if !strings.Contains(out, "Sub") {
		t.Errorf("expected output to contain uncovered function 'Sub', got %q", out)
	}
}

func TestOutputTextOracle_FullCoverage(t *testing.T) {
	result := &oracleResult{
		Claim:         "main.go",
		Evidence:      "main_test.go",
		Coverage:      100.0,
		ClaimSymbols:  []symbolInfo{{Name: "Add", Type: "function", Line: 1}},
		Covered:       []symbolInfo{{Name: "Add", Type: "function", Line: 1, Covered: true}},
		Uncovered:     []symbolInfo{},
		TestSymbols:   []symbolInfo{{Name: "TestAdd", Type: "function", Line: 1}},
		Summary:       "Coverage: 100.0% (1/1 functions covered)",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextOracle(result); err != nil {
		t.Fatalf("outputTextOracle failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "100.0%") {
		t.Errorf("expected output to contain 100.0%%, got %q", out)
	}
}
