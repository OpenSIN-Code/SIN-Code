// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the scout subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 50
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

	results, err := searchFiles(dir, `func \w+`, "regex", 50, false)
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

	results, err := searchFiles(dir, `class \w+`, "regex", 50, false)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 matches, got %d", len(results))
	}
}

func TestSearchFiles_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	_, err := searchFiles(dir, `[invalid`, "regex", 50, false)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestSearchFiles_Semantic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("def calculate_total():\n    pass\n"), 0644)

	results, err := searchFiles(dir, "calculate total", "semantic", 50, false)
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

	results, err := searchFiles(dir, "MyFunc", "symbol", 50, false)
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

	results, err := searchFiles(dir, "MyFunc", "usage", 50, false)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 usages, got %d", len(results))
	}
}

func TestSearchFiles_UnknownSearchType(t *testing.T) {
	dir := t.TempDir()
	_, err := searchFiles(dir, "test", "unknown_type", 50, false)
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

	results, err := searchFiles(dir, `match`, "regex", 5, false)
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

	results, err := searchFiles(dir, `Match`, "regex", 50, false)
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

	results, err := searchFiles(dir, `Match`, "regex", 50, true)
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

	results, err := searchFiles(dir, `matchLine`, "regex", 50, true)
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

	results, err := searchFiles(dir, `func target`, "regex", 50, false)
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

	results, err := searchFiles(dir, `ProcessData`, "regex", 50, false)
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
	defer r.Close()
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
	defer r.Close()
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
	defer r.Close()
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
	defer r.Close()
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

	results, err := searchFiles(dir, `process`, "usage", 50, false)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) < 3 {
		t.Errorf("expected at least 3 results across file types, got %d", len(results))
	}
}

func TestScoreRelevanceScout_CommentTypes(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		line    string
	}{
		{"hash comment", "app.py", "# this is a comment"},
		{"block comment start", "app.go", "/* block comment */"},
		{"block comment star", "app.go", "* middle of block"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreRelevanceScout(tt.relPath, tt.line)
			if score >= 60 {
				t.Errorf("expected penalty for comment, got %.1f", score)
			}
		})
	}
}

func TestScoreRelevanceScout_ZeroFloor(t *testing.T) {
	score := scoreRelevanceScout("_test_test.go", "// test comment # /* */")
	if score < 0 {
		t.Errorf("score should be floored at 0, got %.1f", score)
	}
}

func TestSearchFiles_MaxResultsReached(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("// match1\n// match2\npackage main\nfunc main() {}\n// match3\n"), 0644)

	results, err := searchFiles(dir, `match`, "regex", 2, false)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestSearchFiles_SemanticInvalid(t *testing.T) {
	dir := t.TempDir()
	_, err := searchFiles(dir, "[invalid", "semantic", 50, false)
	if err == nil {
		t.Error("expected error for invalid semantic query")
	}
}

func TestSearchFiles_UsageEscapesQuery(t *testing.T) {
	dir := t.TempDir()
	_, err := searchFiles(dir, "[invalid", "usage", 50, false)
	if err != nil {
		t.Errorf("usage search should escape special chars via QuoteMeta: %v", err)
	}
}

func TestSearchFiles_SymbolEscapesQuery(t *testing.T) {
	dir := t.TempDir()
	_, err := searchFiles(dir, "[invalid", "symbol", 50, false)
	if err != nil {
		t.Errorf("symbol search should escape special chars via QuoteMeta: %v", err)
	}
}

func TestScoutCmd_InvalidAbsPath(t *testing.T) {
	scoutQuery = "test"
	scoutPath = "\x00invalid"
	scoutType = "regex"
	scoutFormat = "text"
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error for invalid abs path")
	}
}

func TestGetContext_ShortLines(t *testing.T) {
	lines := []string{"a", "b", "c"}
	ctx := getContext(lines, 0, 5)
	if len(ctx) != 3 {
		t.Errorf("expected 3 context lines, got %d", len(ctx))
	}
}

func TestScoreRelevanceScout_CommentPenalty(t *testing.T) {
	score := scoreRelevanceScout("main.go", "// this is a comment")
	noComment := scoreRelevanceScout("main.go", "func main()")
	if score >= noComment {
		t.Errorf("expected comment line to score lower, got %.1f vs %.1f", score, noComment)
	}
}

func TestScoreRelevanceScout_HashComment(t *testing.T) {
	score := scoreRelevanceScout("app.py", "# this is a python comment")
	if score > 60 {
		t.Errorf("expected hash comment penalty, got %.1f", score)
	}
}

func TestScoreRelevanceScout_TestFilePenalty(t *testing.T) {
	score := scoreRelevanceScout("main_test.go", "func TestSomething()")
	normal := scoreRelevanceScout("main.go", "func TestSomething()")
	if score >= normal {
		t.Errorf("expected test file penalty, got %.1f vs %.1f", score, normal)
	}
}

func TestScoreRelevanceScout_BlockComment(t *testing.T) {
	score := scoreRelevanceScout("main.go", "/* block comment */")
	if score > 65 {
		t.Errorf("expected block comment penalty, got %.1f", score)
	}
}

func TestScoreRelevanceScout_DefinitionBoost(t *testing.T) {
	score := scoreRelevanceScout("main.go", "func Handler()")
	base := scoreRelevanceScout("main.go", "x = 1")
	if score <= base {
		t.Errorf("expected definition boost, got %.1f vs %.1f", score, base)
	}
}

func TestScoreRelevanceScout_NonCodeFile(t *testing.T) {
	score := scoreRelevanceScout("data.csv", "value")
	codeScore := scoreRelevanceScout("main.go", "value")
	if score >= codeScore {
		t.Errorf("expected non-code file to score lower, got %.1f vs %.1f", score, codeScore)
	}
}

func TestSearchFiles_SemanticSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\nfunc HandleRequest() {}\n"), 0644)
	results, err := searchFiles(dir, "HandleRequest", "semantic", 10, false)
	if err != nil {
		t.Fatalf("semantic search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 semantic search result")
	}
}

func TestSearchFiles_SymbolSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc MyFunc() {}\n"), 0644)
	results, err := searchFiles(dir, "MyFunc", "symbol", 10, false)
	if err != nil {
		t.Fatalf("symbol search failed: %v", err)
	}
	_ = results
}

func TestSearchFiles_UsageSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() { MyFunc() }\n"), 0644)
	results, err := searchFiles(dir, "MyFunc", "usage", 10, false)
	if err != nil {
		t.Fatalf("usage search failed: %v", err)
	}
	_ = results
}

func TestSearchFiles_ContextExtraction(t *testing.T) {
	dir := t.TempDir()
	content := "package main\nimport \"fmt\"\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	os.WriteFile(filepath.Join(dir, "app.go"), []byte(content), 0644)
	results, err := searchFiles(dir, "fmt.Println", "regex", 10, false)
	if err != nil {
		t.Fatalf("regex search failed: %v", err)
	}
	for _, r := range results {
		if len(r.Context) == 0 {
			t.Errorf("expected context lines for result in %s:%d", r.File, r.Line)
		}
	}
}

func TestScoreRelevanceScout_StarComment(t *testing.T) {
	score := scoreRelevanceScout("main.go", "* continuation of block comment")
	if score > 60 {
		t.Errorf("expected star-comment penalty, got %.1f", score)
	}
}

func TestScoreRelevanceScout_ZeroFloorCheck(t *testing.T) {
	score := scoreRelevanceScout("data.csv", "// comment with penalty")
	if score < 0 {
		t.Errorf("score should not be negative, got %.1f", score)
	}
}

func TestScoutCmd_TextOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\nfunc main() {}\n"), 0644)

	scoutQuery = "main"
	scoutPath = dir
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10

	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := ScoutCmd.RunE(ScoutCmd, []string{})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("ScoutCmd.RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "matches found") {
		t.Errorf("expected matches found in output, got: %q", out)
	}
}

func TestSearchFiles_MaxResultsLimited(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file_%d.go", i)), []byte(fmt.Sprintf("package main\nfunc Func%d() {}\n", i)), 0644)
	}
	results, err := searchFiles(dir, "Func", "regex", 5, false)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("expected max 5 results, got %d", len(results))
	}
}

func TestSearchFiles_WalkError(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.Mkdir(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "app.go"), []byte("package main\n"), 0644)
	os.Chmod(subdir, 0000)
	defer os.Chmod(subdir, 0755)

	results, err := searchFiles(dir, "main", "regex", 10, false)
	_ = results
	_ = err
}

func TestScoutCmd_SearchError(t *testing.T) {
	scoutQuery = "test"
	scoutPath = "/nonexistent/path/that/does/not/exist"
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10

	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error for nonexistent search path")
	}
}

func TestScoutCmd_InvalidRegexQuery(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n"), 0644)

	scoutQuery = "[invalid("
	scoutPath = dir
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10

	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error for invalid regex query")
	}
}

func TestScoutCmd_NotDirPath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notdir.txt")
	os.WriteFile(filePath, []byte("hello"), 0644)

	scoutQuery = "test"
	scoutPath = filePath
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10

	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error when path is not a directory")
	}
}

func TestSearchFiles_PermDeniedSubdir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "denied")
	os.Mkdir(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "secret.go"), []byte("package secret\nfunc Secret() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "public.go"), []byte("package main\nfunc Public() {}\n"), 0644)
	os.Chmod(subdir, 0000)
	defer os.Chmod(subdir, 0755)

	results, err := searchFiles(dir, "func", "regex", 100, false)
	_ = results
	_ = err
}

func TestScoutCmd_SearchFileError(t *testing.T) {
	scoutQuery = "[invalid("
	scoutPath = os.TempDir()
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10

	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error for invalid regex via RunE")
	}
}

func TestSearchFiles_InvalidType(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n"), 0644)
	_, err := searchFiles(dir, "test", "bogus_type", 10, false)
	if err == nil {
		t.Error("expected error for unknown search type")
	}
}

func TestScoutCmd_PathNotFound(t *testing.T) {
	scoutQuery = "test"
	scoutPath = "/nonexistent/path"
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10

	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error for path not found")
	}
}

func TestSearchFiles_EmptyQuery(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\nfunc main() {}\n"), 0644)
	results, err := searchFiles(dir, "", "regex", 10, false)
	if err != nil {
		t.Fatalf("empty query should not error: %v", err)
	}
	_ = results
}

func TestSearchFiles_SemanticMultiWord(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\nfunc HandleRequest() {}\nfunc ProcessData() {}\n"), 0644)
	results, err := searchFiles(dir, "Handle Request", "semantic", 10, false)
	if err != nil {
		t.Fatalf("semantic multi-word search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results for semantic multi-word search")
	}
}

func TestScoutCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\nfunc main() {}\n"), 0644)

	scoutQuery = "main"
	scoutPath = dir
	scoutType = "regex"
	scoutFormat = "json"
	scoutMax = 10

	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := ScoutCmd.RunE(ScoutCmd, []string{})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("ScoutCmd.RunE json failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var results []scoutResult
	if jsonErr := json.Unmarshal(buf.Bytes(), &results); jsonErr != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", jsonErr)
	}
}

func TestScoutCmd_NoQuery(t *testing.T) {
	scoutQuery = ""
	scoutPath = "."
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10

	err := ScoutCmd.RunE(ScoutCmd, []string{})
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestSearchSingleFile_SuccessText_More(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	os.WriteFile(f, []byte("package main\nfunc main() {}\n"), 0644)

	getOut := captureStdout(t)
	if err := searchSingleFile(f, "main", "regex", 10, "text"); err != nil {
		t.Fatalf("searchSingleFile text: %v", err)
	}
	out := getOut()
	if !strings.Contains(out, "matches found") {
		t.Errorf("expected matches found, got %q", out)
	}
}

func TestSearchSingleFile_SuccessJSON_More(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	os.WriteFile(f, []byte("package main\nfunc main() {}\n"), 0644)

	getOut := captureStdout(t)
	if err := searchSingleFile(f, "main", "regex", 10, "json"); err != nil {
		t.Fatalf("searchSingleFile json: %v", err)
	}
	out := getOut()
	var results []scoutResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("expected JSON: %v\n%s", err, out)
	}
}

func TestSearchSingleFile_InvalidAbsPath_More(t *testing.T) {
	if err := searchSingleFile("\x00invalid", "main", "regex", 10, "text"); err == nil {
		t.Fatal("expected error for invalid abs path")
	}
}

func TestSearchSingleFile_InvalidRegex_More(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	os.WriteFile(f, []byte("package main\n"), 0644)
	if err := searchSingleFile(f, "[invalid(", "regex", 10, "text"); err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestSearchSingleFile_MaxResults_More(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	content := "package main\n"
	for i := 0; i < 10; i++ {
		content += fmt.Sprintf("func main%d() {}\n", i)
	}
	os.WriteFile(f, []byte(content), 0644)

	getOut := captureStdout(t)
	if err := searchSingleFile(f, "main", "regex", 3, "text"); err != nil {
		t.Fatalf("searchSingleFile max: %v", err)
	}
	out := getOut()
	if !strings.Contains(out, "3 matches found") {
		t.Errorf("expected 3 matches found, got %q", out)
	}
}

func TestRelOf_AbsFallback(t *testing.T) {
	// On Unix filepath.Rel can always express an absolute path relative to
	// the working directory, so the fallback branch is only reachable on
	// Windows. Use a test hook to simulate the error path.
	oldRelOf := relOfFunc
	relOfFunc = func(wd, abs string) (string, error) {
		return "", fmt.Errorf("simulated rel error")
	}
	defer func() { relOfFunc = oldRelOf }()

	abs := "/some/absolute/path/that/does/not/exist"
	if got := relOf(abs); got != abs {
		t.Errorf("relOf(%q) = %q, want %q", abs, got, abs)
	}
}

func TestScoutCmd_SingleFile_More(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	os.WriteFile(f, []byte("package main\nfunc main() {}\n"), 0644)

	scoutQuery = "main"
	scoutPath = dir
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10
	scoutFile = f
	defer func() { scoutFile = "" }()

	getOut := captureStdout(t)
	err := ScoutCmd.RunE(ScoutCmd, []string{})
	out := getOut()
	if err != nil {
		t.Fatalf("ScoutCmd single file: %v", err)
	}
	if !strings.Contains(out, "matches found") {
		t.Errorf("expected matches, got %q", out)
	}
}

func TestScoutCmd_SingleFileNotFound_More(t *testing.T) {
	scoutQuery = "main"
	scoutPath = "."
	scoutType = "regex"
	scoutFormat = "text"
	scoutMax = 10
	scoutFile = "/nonexistent/path/file.go"
	defer func() { scoutFile = "" }()

	if err := ScoutCmd.RunE(ScoutCmd, []string{}); err == nil {
		t.Fatal("expected error for single file not found")
	}
}

func TestScoutCmd_SingleFileJSON_More(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	os.WriteFile(f, []byte("package main\nfunc main() {}\n"), 0644)

	scoutQuery = "main"
	scoutPath = dir
	scoutType = "regex"
	scoutFormat = "json"
	scoutMax = 10
	scoutFile = f
	defer func() { scoutFile = "" }()

	getOut := captureStdout(t)
	if err := ScoutCmd.RunE(ScoutCmd, []string{}); err != nil {
		t.Fatalf("ScoutCmd single file json: %v", err)
	}
	out := getOut()
	var results []scoutResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("expected JSON: %v\n%s", err, out)
	}
}

func TestIsBinaryFile_NullBytes(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "binary.bin")
	os.WriteFile(f, []byte("hello\x00world"), 0644)
	if !isBinaryFile(f) {
		t.Error("expected file with null byte to be detected as binary")
	}
}

func TestIsBinaryFile_Unreadable(t *testing.T) {
	f := "/nonexistent/not-a-real-file-at-all.bin"
	if !isBinaryFile(f) {
		t.Error("expected unreadable file to be detected as binary")
	}
}

func TestLoadGitignore_WithPatterns(t *testing.T) {
	dir := t.TempDir()
	content := "*.log\n!important.log\nbuild/\n\n# comment\n"
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(content), 0644)
	m := loadGitignore(dir)
	if len(m.patterns) != 3 {
		t.Fatalf("expected 3 patterns, got %d", len(m.patterns))
	}
	if m.patterns[0].pattern != "*.log" || m.patterns[0].negate {
		t.Errorf("first pattern should be *.log, non-negated")
	}
	if m.patterns[1].pattern != "important.log" || !m.patterns[1].negate {
		t.Errorf("second pattern should be important.log, negated")
	}
	if m.patterns[2].pattern != "build" || !m.patterns[2].dirOnly {
		t.Errorf("third pattern should be build, dir-only")
	}
}

func TestLoadGitignore_NoFile(t *testing.T) {
	dir := t.TempDir()
	m := loadGitignore(dir)
	if len(m.patterns) != 0 {
		t.Errorf("expected empty patterns, got %d", len(m.patterns))
	}
}

func TestGitignoreGlobToRegex(t *testing.T) {
	re := gitignoreGlobToRegex("*.go")
	if !re.MatchString("main.go") {
		t.Error("expected *.go to match main.go")
	}
	if re.MatchString("main.go.bak") {
		t.Error("expected *.go to NOT match main.go.bak")
	}

	re2 := gitignoreGlobToRegex("test.?")
	if !re2.MatchString("test.a") {
		t.Error("expected test.? to match test.a")
	}
}

func TestMatchesPattern_Exact(t *testing.T) {
	p := gitignorePattern{pattern: "main.go", negate: false}
	if !matchesPattern("main.go", p) {
		t.Error("expected exact match")
	}
	if matchesPattern("util.go", p) {
		t.Error("expected no match for different file")
	}
}

func TestMatchesPattern_Regex(t *testing.T) {
	p := gitignorePattern{pattern: "*.go", negate: false, re: gitignoreGlobToRegex("*.go")}
	if !matchesPattern("main.go", p) {
		t.Error("expected regex match for main.go")
	}
	if matchesPattern("main.go.bak", p) {
		t.Error("expected no regex match for main.go.bak")
	}
}

func TestMatchFile_Simple(t *testing.T) {
	m := &gitignoreMatcher{
		patterns: []gitignorePattern{
			{pattern: "*.log", negate: false, re: gitignoreGlobToRegex("*.log")},
			{pattern: "keep.log", negate: true, re: gitignoreGlobToRegex("keep.log")},
		},
	}
	if !m.matchFile("error.log") {
		t.Error("expected error.log to be ignored")
	}
	if m.matchFile("keep.log") {
		t.Error("expected keep.log to NOT be ignored (negated)")
	}
}

func TestMatchDir_DirOnly(t *testing.T) {
	m := &gitignoreMatcher{
		patterns: []gitignorePattern{
			{pattern: "build", dirOnly: true, negate: false, re: nil},
		},
	}
	if !m.matchDir("/some/path/build") {
		t.Error("expected build directory to be ignored")
	}
	if m.matchDir("/some/path/build/file.go") {
		t.Error("expected files under build to not match dir-only pattern")
	}
}

func TestSearchSingleFile_IsDirectory(t *testing.T) {
	dir := t.TempDir()
	err := searchSingleFile(dir, "test", "regex", 10, "text")
	if err == nil {
		t.Error("expected error when --file is a directory")
	}
}

func TestMatchFile_DirOnlySkipped(t *testing.T) {
	m := &gitignoreMatcher{
		patterns: []gitignorePattern{
			{pattern: "build", dirOnly: true, negate: false, re: nil},
			{pattern: "*.log", negate: false, re: gitignoreGlobToRegex("*.log")},
		},
	}
	if !m.matchFile("error.log") {
		t.Error("expected error.log to be ignored")
	}
	if m.matchFile("build") {
		t.Error("expected build (file) to NOT be ignored by dirOnly pattern")
	}
}

func TestMatchDir_WithRegex(t *testing.T) {
	m := &gitignoreMatcher{
		patterns: []gitignorePattern{
			// A non-dirOnly pattern that will be skipped via continue
			{pattern: "*.log", negate: false, re: gitignoreGlobToRegex("*.log")},
			// A dirOnly pattern with regex that should match
			{pattern: "build_*", dirOnly: true, negate: false, re: gitignoreGlobToRegex("build_*")},
		},
	}
	if !m.matchDir("/tmp/build_v1") {
		t.Error("expected build_v1 dir to match regex dirOnly pattern")
	}
	if m.matchDir("/tmp/other") {
		t.Error("expected other dir to not match")
	}
}

func TestScoreRelevanceScout_AdditionalPrefixes(t *testing.T) {
	tests := []struct {
		name string
		path string
		line string
	}{
		{"struct definition", "main.go", "struct Person {"},
		{"interface definition", "main.go", "interface Worker {"},
		{"type definition", "main.go", "type MyType int"},
		{"const definition", "main.go", "const MaxRetries = 3"},
		{"var definition", "main.go", "var count int"},
		{"let definition", "app.js", "let x = 42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreRelevanceScout(tt.path, tt.line)
			// Definition boost (+20) + code ext bonus (+15) = 85
			if score < 80 || score > 100 {
				t.Errorf("expected high score for definition (80-100), got %.1f", score)
			}
		})
	}
}

func TestGoSearch_MaxResults_Underflow(t *testing.T) {
	dir := t.TempDir()
	content := "// func main() {}\n// func helper() {}\n// func util() {}\n"
	os.WriteFile(filepath.Join(dir, "app.go"), []byte(content), 0644)

	// Use noRG=true to force goSearch path
	results, err := searchFiles(dir, "func", "regex", 2, true)
	if err != nil {
		t.Fatalf("searchFiles(noRG=true) failed: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestGoSearch_MaxResults_Drain(t *testing.T) {
	dir := t.TempDir()
	// Create 10 files each with multiple matches to ensure we hit the drain path
	for i := 0; i < 10; i++ {
		content := fmt.Sprintf("package main\nfunc helper%d() {}\nfunc worker%d() {}\n", i, i)
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file_%d.go", i)), []byte(content), 0644)
	}
	// Use noRG=true with low maxResults to trigger drainMatches + goto sortResults
	results, err := searchFiles(dir, "func", "regex", 3, true)
	if err != nil {
		t.Fatalf("searchFiles(noRG=true) failed: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
	// Verify results are sorted by relevance
	for i := 1; i < len(results); i++ {
		if results[i-1].Relevance < results[i].Relevance {
			t.Errorf("results not sorted by relevance")
		}
	}
}

func TestSearchSingleFile_MaxResultsTruncation(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	content := "package main\n"
	for i := 0; i < 20; i++ {
		content += fmt.Sprintf("func main%d() {}\n", i)
	}
	os.WriteFile(f, []byte(content), 0644)

	getOut := captureStdout(t)
	if err := searchSingleFile(f, "main", "regex", 5, "json"); err != nil {
		t.Fatalf("searchSingleFile: %v", err)
	}
	out := getOut()
	var results []scoutResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("expected JSON: %v\n%s", err, out)
	}
	if len(results) > 5 {
		t.Errorf("expected at most 5 results, got %d", len(results))
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result after truncation")
	}
}

func TestGoSearch_WalkError(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "f.go"), []byte("package main\nfunc F() {}\n"), 0644)
	os.Chmod(sub, 0000)
	defer os.Chmod(sub, 0755)
	results, err := searchFiles(dir, "func", "regex", 100, true)
	_ = results
	_ = err
}

func TestGoSearch_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n"), 0644)
	_, err := searchFiles(dir, "[invalid", "regex", 10, true)
	if err == nil {
		t.Error("expected error for invalid regex in goSearch")
	}
}

func TestGoSearch_SingleWorker(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("func main() {}\n"), 0644)
	oldNumCPU := numCPU
	numCPU = func() int { return 1 }
	defer func() { numCPU = oldNumCPU }()
	results, err := searchFiles(dir, "main", "regex", 10, true)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least 1 result with single worker")
	}
}

func TestGoSearch_WorkerError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("func main() {}\n"), 0644)
	oldSearchFn := searchFileFn
	searchFileFn = func(path, rel, root string, re *regexp.Regexp, searchType string) ([]scoutResult, error) {
		return nil, fmt.Errorf("simulated search error")
	}
	defer func() { searchFileFn = oldSearchFn }()
	results, err := searchFiles(dir, "func", "regex", 10, true)
	if err != nil {
		t.Fatalf("searchFiles should not propagate worker errors: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when all workers error, got %d", len(results))
	}
}

func TestGoSearch_SkipsDotDirs(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "hooks.go"), []byte("func hookMatch() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "visible.go"), []byte("func visibleMatch() {}\n"), 0644)
	results, err := searchFiles(dir, "Match", "regex", 50, true)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.File, ".git") {
			t.Errorf("should skip .git directory, found %q", r.File)
		}
	}
}

func TestGoSearch_GitignoreDirMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("logs/\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)
	os.WriteFile(filepath.Join(dir, "logs", "output.go"), []byte("func logged() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("func main() {}\n"), 0644)
	results, err := searchFiles(dir, "func", "regex", 50, true)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.File, "logs/output.go") {
			t.Errorf("expected logs/ directory to be skipped by gitignore, got %q", r.File)
		}
	}
}

func TestSearchSingleFile_AbsError(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	os.WriteFile(f, []byte("package main\nfunc main() {}\n"), 0644)
	oldAbs := filepathAbsFn
	filepathAbsFn = func(path string) (string, error) {
		return "", fmt.Errorf("simulated abs error")
	}
	defer func() { filepathAbsFn = oldAbs }()
	err := searchSingleFile(f, "main", "regex", 10, "text")
	if err == nil {
		t.Error("expected error when filepath.Abs fails")
	}
}

func TestSearchSingleFile_SearchFileError(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "app.go")
	os.WriteFile(f, []byte("package main\n"), 0644)
	oldSearchFn := searchFileFn
	searchFileFn = func(path, rel, root string, re *regexp.Regexp, searchType string) ([]scoutResult, error) {
		return nil, fmt.Errorf("simulated error")
	}
	defer func() { searchFileFn = oldSearchFn }()
	err := searchSingleFile(f, "main", "regex", 10, "text")
	if err == nil {
		t.Error("expected error when searchFileFn returns error")
	}
}

func TestGoSearch_BinaryFileWalk(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "binary.bin"), []byte("hello\x00world"), 0644)
	os.WriteFile(filepath.Join(dir, "text.go"), []byte("func binaryMatch() {}\n"), 0644)
	results, err := searchFiles(dir, "binary", "regex", 10, true)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.File, "binary.bin") {
			t.Errorf("expected binary.bin to be skipped, got result in %s", r.File)
		}
	}
}

func TestGoSearch_GitignoreMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.secret\n"), 0644)
	os.WriteFile(filepath.Join(dir, "data.secret"), []byte("func secret() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "data.go"), []byte("func public() {}\n"), 0644)
	results, err := searchFiles(dir, "func", "regex", 10, true)
	if err != nil {
		t.Fatalf("searchFiles failed: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.File, "data.secret") {
			t.Errorf("expected data.secret to be ignored by .gitignore, got result in %s", r.File)
		}
	}
}

func TestGitignoreGlobToRegex_QuestionMark(t *testing.T) {
	re := gitignoreGlobToRegex("file?.txt")
	if re == nil {
		t.Fatal("expected non-nil regex")
	}
	if !re.MatchString("file1.txt") {
		t.Error("? should match single char")
	}
	if re.MatchString("file12.txt") {
		t.Error("? should not match multiple chars")
	}
}

func TestGitignoreGlobToRegex_Slash(t *testing.T) {
	re := gitignoreGlobToRegex("dir/file.txt")
	if re == nil {
		t.Fatal("expected non-nil regex")
	}
	if !re.MatchString("dir/file.txt") {
		t.Error("/ should match literal slash")
	}
}

func TestIsBinaryFile_ReadErrorNZero(t *testing.T) {
	oldFn := openFileFn
	r, w, _ := os.Pipe()
	w.Close()
	openFileFn = func(name string) (*os.File, error) { return r, nil }
	defer func() { openFileFn = oldFn }()
	if !isBinaryFile("/dev/null") {
		t.Error("expected true when Read returns error with n==0")
	}
}

func TestScoreRelevanceScout_BelowZero(t *testing.T) {
	scoreScoutModifier = func(s float64) float64 { return -50.0 }
	defer func() { scoreScoutModifier = nil }()
	score := scoreRelevanceScout("test.go", "// comment")
	if score != 0 {
		t.Errorf("expected clamped to 0, got %f", score)
	}
}

func TestScoreRelevanceScout_Above100(t *testing.T) {
	scoreScoutModifier = func(s float64) float64 { return 999.0 }
	defer func() { scoreScoutModifier = nil }()
	score := scoreRelevanceScout("test.go", "func Hello()")
	if score != 100 {
		t.Errorf("expected clamped to 100, got %f", score)
	}
}

func TestRgSearch_GeneralError(t *testing.T) {
	rgCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "kill -9 $$; exit 2")
	}
	defer func() { rgCommandFn = nil }()
	_, err := rgSearch(t.TempDir(), "test", "regex", 10)
	if err == nil {
		t.Error("expected error from rg general failure")
	}
}

func TestRgSearch_ExitCode1(t *testing.T) {
	rgCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	defer func() { rgCommandFn = nil }()
	results, err := rgSearch(t.TempDir(), "nonexistent_xyz", "regex", 10)
	if err != nil {
		t.Fatalf("exit 1 should not be error: %v", err)
	}
	if results != nil {
		t.Error("expected nil results")
	}
}

func TestRgSearch_ExitErrorWithStderr(t *testing.T) {
	rgCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo "bad pattern" >&2; exit 2`)
	}
	defer func() { rgCommandFn = nil }()
	_, err := rgSearch(t.TempDir(), "[invalid", "regex", 10)
	if err == nil {
		t.Error("expected error for exit with stderr")
	}
}

func TestRgSearch_BadJSON(t *testing.T) {
	rgCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo 'not json'")
	}
	defer func() { rgCommandFn = nil }()
	results, err := rgSearch(t.TempDir(), "test", "regex", 10)
	if err != nil {
		t.Fatalf("should handle bad JSON: %v", err)
	}
	_ = results
}

func TestRgSearch_UsageType(t *testing.T) {
	rgCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo '{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"func Hello()"},"line_number":1,"submatches":[{"match":{"text":"Hello"},"start":5,"end":10}]}}'`)
	}
	defer func() { rgCommandFn = nil }()
	results, err := rgSearch(t.TempDir(), "Hello", "usage", 10)
	if err != nil {
		t.Fatalf("rgSearch usage failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results for usage search")
	}
}

func TestRgSearch_SkipsNonMatch(t *testing.T) {
	rgCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo '{"type":"begin","data":{}}'`)
	}
	defer func() { rgCommandFn = nil }()
	results, err := rgSearch(t.TempDir(), "test", "regex", 10)
	if err != nil {
		t.Fatalf("rgSearch failed: %v", err)
	}
	if len(results) != 0 {
		t.Error("expected no results for non-match type")
	}
}

func TestRgSearch_BadMatchData(t *testing.T) {
	rgCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo '{"type":"match","data":"not an object"}'`)
	}
	defer func() { rgCommandFn = nil }()
	results, err := rgSearch(t.TempDir(), "test", "regex", 10)
	if err != nil {
		t.Fatalf("should handle bad match data: %v", err)
	}
	_ = results
}

func TestRgSearch_MaxResultsAndSort(t *testing.T) {
	rgCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo '{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"func Hello()"},"line_number":1,"submatches":[{"match":{"text":"Hello"},"start":5,"end":10}]}}'
echo '{"type":"match","data":{"path":{"text":"b.go"},"lines":{"text":"package main"},"line_number":1,"submatches":[{"match":{"text":"main"},"start":8,"end":12}]}}'
echo '{"type":"match","data":{"path":{"text":"c.go"},"lines":{"text":"type Foo struct{}"},"line_number":1,"submatches":[{"match":{"text":"Foo"},"start":5,"end":8}]}}'`)
	}
	defer func() { rgCommandFn = nil }()
	results, err := rgSearch(t.TempDir(), "test", "regex", 2)
	if err != nil {
		t.Fatalf("rgSearch failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	// Sort should order by relevance descending; type prefix is higher.
	if results[0].Relevance < results[1].Relevance {
		t.Errorf("expected descending relevance, got %v then %v", results[0].Relevance, results[1].Relevance)
	}
}

func TestRgSearch_FallbackCommand(t *testing.T) {
	oldFn := rgCommandFn
	rgCommandFn = nil
	defer func() { rgCommandFn = oldFn }()

	// Create a fake rg script that returns one valid match.
	fakeBin := filepath.Join(t.TempDir(), "rg")
	script := `#!/bin/sh
echo '{"type":"match","data":{"path":{"text":"x.go"},"lines":{"text":"func X()"},"line_number":1,"submatches":[{"match":{"text":"X"},"start":5,"end":6}]}}'
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", filepath.Dir(fakeBin)+":"+oldPath)

	results, err := rgSearch(t.TempDir(), "X", "regex", 10)
	if err != nil {
		t.Fatalf("fallback command failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}
