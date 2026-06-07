// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the scout subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScoutCmd_Flags(t *testing.T) {
	cmd := ScoutCmd
	if cmd.Use != "scout" {
		t.Errorf("expected Use 'scout', got %q", cmd.Use)
	}
	flags := []string{"query", "path", "search_type", "format", "max_results"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestScoutCmd_DefaultValues(t *testing.T) {
	scoutPath = "."
	if scoutPath != "." {
		t.Errorf("default scoutPath should be ., got %q", scoutPath)
	}
	v, _ := ScoutCmd.Flags().GetString("search_type")
	if v != "regex" {
		t.Errorf("default search_type should be regex, got %q", v)
	}
	v2, _ := ScoutCmd.Flags().GetString("format")
	if v2 != "text" {
		t.Errorf("default format should be text, got %q", v2)
	}
	n, _ := ScoutCmd.Flags().GetInt("max_results")
	if n != 50 {
		t.Errorf("default max_results should be 50, got %d", n)
	}
}

func TestScoutCmd_RequiresQueryDetailed(t *testing.T) {
	scoutQuery = ""
	scoutPath = "."
	scoutType = "regex"
	scoutFormat = "text"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error when --query is empty")
	}
}

func TestScoutCmd_InvalidPath(t *testing.T) {
	scoutQuery = "test"
	scoutPath = "/nonexistent/path"
	scoutType = "regex"
	scoutFormat = "text"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestScoutCmd_PathIsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	os.WriteFile(f, []byte("package main\n"), 0644)
	scoutQuery = "main"
	scoutPath = f
	scoutType = "regex"
	scoutFormat = "text"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error when path is a file, not a directory")
	}
}

func TestSearchFiles_Regex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "util.go"), []byte("package util\nfunc Helper() {}\n"), 0644)

	results, err := searchFiles(dir, `func \w+`, "regex", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 matches, got %d", len(results))
	}
}

func TestSearchFiles_RegexNoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)

	results, err := searchFiles(dir, `class \w+`, "regex", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 matches, got %d", len(results))
	}
}

func TestSearchFiles_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	_, err := searchFiles(dir, `[invalid`, "regex", 50)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestSearchFiles_Semantic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("def calculate_total():\n    pass\n"), 0644)

	results, err := searchFiles(dir, "calculate total", "semantic", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 match, got %d", len(results))
	}
}

func TestSearchFiles_Symbol(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc MyFunc() {}\n"), 0644)

	results, err := searchFiles(dir, "MyFunc", "symbol", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 match for symbol search, got %d", len(results))
	}
}

func TestSearchFiles_Usage(t *testing.T) {
	dir := t.TempDir()
	content := `package main
func main() {
	MyFunc()
	MyFunc()
}
func MyFunc() {}
`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0644)

	results, err := searchFiles(dir, "MyFunc", "usage", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 usages, got %d", len(results))
	}
}

func TestSearchFiles_UnknownSearchType(t *testing.T) {
	dir := t.TempDir()
	_, err := searchFiles(dir, "test", "unknown_type", 50)
	if err == nil {
		t.Error("expected error for unknown search type")
	}
	if !strings.Contains(err.Error(), "unknown search_type") {
		t.Errorf("error should mention unknown search_type, got %v", err)
	}
}

func TestSearchFiles_MaxResults(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("// match line %d", i))
	}
	os.WriteFile(filepath.Join(dir, "big.go"), []byte(strings.Join(lines, "\n")), 0644)

	results, err := searchFiles(dir, `match`, "regex", 5)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("expected at most 5 results, got %d", len(results))
	}
}

func TestSearchFiles_SkipsDotDirs(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "hooks.go"), []byte("func hookMatch() {}\n"), 0644)
	os.Mkdir(filepath.Join(dir, ".hidden"), 0755)
	os.WriteFile(filepath.Join(dir, ".hidden", "secret.go"), []byte("func secretMatch() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "visible.go"), []byte("func visibleMatch() {}\n"), 0644)

	results, err := searchFiles(dir, `Match`, "regex", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.File, ".git") || strings.Contains(r.File, ".hidden") {
			t.Errorf("should skip dot directories, found %q", r.File)
		}
	}
}

func TestSearchFiles_SkipsCommonDirs(t *testing.T) {
	dir := t.TempDir()
	for _, skip := range []string{"node_modules", "vendor", "__pycache__", "dist", "build", "target"} {
		os.Mkdir(filepath.Join(dir, skip), 0755)
		os.WriteFile(filepath.Join(dir, skip, "skip.go"), []byte("func skipMatch() {}\n"), 0644)
	}
	os.WriteFile(filepath.Join(dir, "keep.go"), []byte("func keepMatch() {}\n"), 0644)

	results, err := searchFiles(dir, `Match`, "regex", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (keep.go), got %d", len(results))
	}
}

func TestSearchFiles_SkipsLargeFiles(t *testing.T) {
	dir := t.TempDir()
	bigContent := strings.Repeat("func matchLine() {}\n", 500000)
	os.WriteFile(filepath.Join(dir, "huge.go"), []byte(bigContent), 0644)
	os.WriteFile(filepath.Join(dir, "small.go"), []byte("func matchLine() {}\n"), 0644)

	results, err := searchFiles(dir, `matchLine`, "regex", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.File, "huge.go") {
			t.Error("should skip files > 5MB")
		}
	}
}

func TestSearchFiles_ContextIncluded(t *testing.T) {
	dir := t.TempDir()
	content := `package main
func before() {}
func target() {}
func after() {}
`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0644)

	results, err := searchFiles(dir, `func target`, "regex", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if len(results[0].Context) == 0 {
		t.Error("expected context lines to be included")
	}
}

func TestSearchFiles_RelevanceSorted(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("func ProcessData() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "util.go"), []byte("// ProcessData helper\n"), 0644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ProcessData: true\n"), 0644)

	results, err := searchFiles(dir, `ProcessData`, "regex", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	for i := 1; i < len(results); i++ {
		if results[i-1].Relevance < results[i].Relevance {
			t.Errorf("results not sorted by relevance: %.1f < %.1f", results[i-1].Relevance, results[i].Relevance)
		}
	}
}

func TestGetContext(t *testing.T) {
	lines := []string{"line0", "line1", "line2", "line3", "line4", "line5"}

	tests := []struct {
		name   string
		center int
		radius int
		want   int
	}{
		{"middle", 3, 2, 5},
		{"start", 0, 2, 3},
		{"end", 5, 2, 3},
		{"zero radius", 2, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := getContext(lines, tt.center, tt.radius)
			if len(ctx) != tt.want {
				t.Errorf("getContext(lines, %d, %d) returned %d lines, want %d", tt.center, tt.radius, len(ctx), tt.want)
			}
			for _, line := range ctx {
				if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "> ") {
					t.Errorf("context line missing prefix: %q", line)
				}
			}
		})
	}
}

func TestGetContext_EmptyLines(t *testing.T) {
	ctx := getContext([]string{}, 0, 2)
	if len(ctx) != 0 {
		t.Errorf("expected 0 context lines for empty input, got %d", len(ctx))
	}
}

func TestGetContext_SingleLine(t *testing.T) {
	ctx := getContext([]string{"only"}, 0, 2)
	if len(ctx) != 1 {
		t.Errorf("expected 1 context line, got %d", len(ctx))
	}
	if !strings.HasPrefix(ctx[0], "> ") {
		t.Errorf("center line should have > prefix, got %q", ctx[0])
	}
}

func TestScoreRelevanceScout(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		line     string
		minScore float64
		maxScore float64
	}{
		{"go function definition", "main.go", "func MyFunc() {}", 65, 100},
		{"python definition", "app.py", "def my_func():", 65, 100},
		{"js definition", "app.js", "function myFunc() {}", 65, 100},
		{"rust definition", "main.rs", "fn main() {}", 65, 100},
		{"java definition", "App.java", "public static void main()", 65, 100},
		{"comment line", "main.go", "// just a comment", 40, 65},
		{"test file", "main_test.go", "func TestMain() {}", 60, 85},
		{"non-code file", "config.yaml", "key: value", 50, 65},
		{"export line", "index.ts", "export function run() {}", 65, 100},
		{"class definition", "app.py", "class MyClass:", 65, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreRelevanceScout(tt.relPath, tt.line)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("scoreRelevanceScout(%q, %q) = %.1f, want between %.1f and %.1f",
					tt.relPath, tt.line, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestScoreRelevanceScout_Bounds(t *testing.T) {
	score := scoreRelevanceScout("test.go", "func X() {}")
	if score < 0 || score > 100 {
		t.Errorf("score should be clamped to [0,100], got %.1f", score)
	}
}

func TestOutputTextScout_EmptyResults(t *testing.T) {
	old := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextScout([]scoutResult{})
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextScout failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(pr)
	out := buf.String()
	if !strings.Contains(out, "No matches found") {
		t.Errorf("expected 'No matches found', got %q", out)
	}
}

func TestOutputTextScout_WithResults(t *testing.T) {
	results := []scoutResult{
		{
			File:      "main.go",
			Line:      5,
			Column:    1,
			Match:     "func main()",
			Context:   []string{"  4: ", "> 5: func main()", "  6: "},
			Type:      "regex",
			Relevance: 85.0,
		},
	}

	old := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextScout(results)
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextScout failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(pr)
	out := buf.String()
	if !strings.Contains(out, "main.go:5") {
		t.Errorf("expected file:line in output, got %q", out)
	}
	if !strings.Contains(out, "1 matches found") {
		t.Errorf("expected match count, got %q", out)
	}
}

func TestScoutCmd_RegexJSONFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc hello() {}\n"), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	scoutQuery = `func \w+`
	scoutPath = dir
	scoutType = "regex"
	scoutFormat = "json"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var results []scoutResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Type != "regex" {
		t.Errorf("expected type regex, got %q", results[0].Type)
	}
}

func TestScoutCmd_SemanticJSONFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("def calculate_total():\n    pass\n"), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	scoutQuery = "calculate"
	scoutPath = dir
	scoutType = "semantic"
	scoutFormat = "json"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var results []scoutResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
}

func TestScoutCmd_SymbolJSONFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc MyFunc() {}\n"), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	scoutQuery = "MyFunc"
	scoutPath = dir
	scoutType = "symbol"
	scoutFormat = "json"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var results []scoutResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
}

func TestScoutCmd_UsageJSONFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() { Helper() }\nfunc Helper() {}\n"), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	scoutQuery = "Helper"
	scoutPath = dir
	scoutType = "usage"
	scoutFormat = "json"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var results []scoutResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(results) < 1 {
		t.Errorf("expected at least 1 usage result, got %d", len(results))
	}
}

func TestSearchFiles_MultipleFileTypes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("func process() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("def process():\n    pass\n"), 0644)
	os.WriteFile(filepath.Join(dir, "index.js"), []byte("function process() {}\n"), 0644)

	results, err := searchFiles(dir, `process`, "usage", 50)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) < 3 {
		t.Errorf("expected at least 3 results across file types, got %d", len(results))
	}
}
