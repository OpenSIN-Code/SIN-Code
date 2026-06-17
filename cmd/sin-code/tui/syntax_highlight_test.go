// SPDX-License-Identifier: MIT
package tui

import (
	"regexp"
	"strings"
	"sync"
	"testing"
)

func cleanANSIForTest(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

func containsPlain(s, substr string) bool {
	return strings.Contains(cleanANSIForTest(s), substr)
}

func TestHighlightGoCode(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}`
	result := h.Highlight(code, "go")
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !containsPlain(result, "package") {
		t.Error("expected 'package' keyword in output")
	}
	if !containsPlain(result, "func") {
		t.Error("expected 'func' keyword in output")
	}
	if !containsPlain(result, "import") {
		t.Error("expected 'import' keyword in output")
	}
	if !containsPlain(result, "hello") {
		t.Error("expected string content 'hello' in output")
	}
}

func TestHighlightGoKeywords(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `func test() {
	var x int = 42
	const y = "value"
	type MyStruct struct{}
	return x
}`
	result := h.Highlight(code, "go")
	for _, kw := range []string{"func", "var", "const", "type", "return"} {
		if !containsPlain(result, kw) {
			t.Errorf("expected keyword %q in output", kw)
		}
	}
}

func TestHighlightGoComments(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `// this is a line comment
x := 42 /* inline comment */`
	result := h.Highlight(code, "go")
	if !containsPlain(result, "this is a line comment") {
		t.Error("expected line comment content in output")
	}
	if !containsPlain(result, "inline comment") {
		t.Error("expected block comment content in output")
	}
}

func TestHighlightGoStrings(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `s := "hello world"
	r := 'x'
	b := ` + "`backtick`"
	result := h.Highlight(code, "go")
	if !containsPlain(result, "hello world") {
		t.Error("expected double-quoted string content")
	}
	if !containsPlain(result, "backtick") {
		t.Error("expected backtick string content")
	}
}

func TestHighlightPythonCode(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `def hello(name):
    # print greeting
    print(f"Hello, {name}!")
    return True`
	result := h.Highlight(code, "python")
	if !containsPlain(result, "def") {
		t.Error("expected 'def' keyword in output")
	}
	if !containsPlain(result, "print") {
		t.Error("expected 'print' keyword in output")
	}
	if !containsPlain(result, "return") {
		t.Error("expected 'return' keyword in output")
	}
	if !containsPlain(result, "print greeting") {
		t.Error("expected comment content in output")
	}
}

func TestHighlightJavaScriptCode(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `const x = 42;
function greet(name) {
    // say hello
    return ` + "`hello " + "`" + ` + name;
}`
	result := h.Highlight(code, "javascript")
	if !containsPlain(result, "const") {
		t.Error("expected 'const' keyword in output")
	}
	if !containsPlain(result, "function") {
		t.Error("expected 'function' keyword in output")
	}
	if !containsPlain(result, "return") {
		t.Error("expected 'return' keyword in output")
	}
}

func TestHighlightJSON(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `{
    "name": "test",
    "value": 42,
    "active": true,
    "data": null
}`
	result := h.Highlight(code, "json")
	if !containsPlain(result, "name") {
		t.Error("expected 'name' key in output")
	}
	if !containsPlain(result, "test") {
		t.Error("expected string value 'test' in output")
	}
	if !containsPlain(result, "42") {
		t.Error("expected number '42' in output")
	}
	if !containsPlain(result, "true") {
		t.Error("expected boolean 'true' in output")
	}
	if !containsPlain(result, "null") {
		t.Error("expected 'null' in output")
	}
}

func TestHighlightBashCode(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `#!/bin/bash
# This is a comment
echo "hello world"
if [ "$x" = "1" ]; then
    exit 0
fi`
	result := h.Highlight(code, "bash")
	if !containsPlain(result, "echo") {
		t.Error("expected 'echo' keyword in output")
	}
	if !containsPlain(result, "hello world") {
		t.Error("expected string content in output")
	}
	if !containsPlain(result, "This is a comment") {
		t.Error("expected comment content in output")
	}
}

func TestHighlightUnknownLanguageFallback(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `some random text
that is not any known language`
	result := h.Highlight(code, "cobol")
	if !containsPlain(result, "some random text") {
		t.Error("expected plain text fallback for unknown language")
	}
}

func TestHighlightEmptyCode(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	result := h.Highlight("", "go")
	if result != "" {
		t.Errorf("expected empty string for empty code, got %q", result)
	}
}

func TestHighlightEmptyLanguage(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `plain text without language`
	result := h.Highlight(code, "")
	if !containsPlain(result, "plain text without language") {
		t.Error("expected plain text fallback for empty language")
	}
}

func TestHighlightMultiLineCode(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `package main

import "fmt"

// Comment line
func main() {
    x := 42
    fmt.Println(x)
}`
	result := h.Highlight(code, "go")
	cleaned := cleanANSIForTest(result)
	lines := strings.Split(cleaned, "\n")
	if len(lines) < 7 {
		t.Errorf("expected multi-line output (%d+ lines), got %d", 7, len(lines))
	}
	if !containsPlain(result, "package main") {
		t.Error("expected first line content")
	}
	if !containsPlain(result, "fmt.Println") {
		t.Error("expected last function call")
	}
}

func TestHighlightNestedStrings(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `s := "hello \"world\""`
	result := h.Highlight(code, "go")
	if !containsPlain(result, "hello") {
		t.Error("expected escaped string content")
	}
}

func TestHighlightConcurrent(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])

	code := `package main
import "fmt"
func main() {
    fmt.Println("hello")
}`

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := h.Highlight(code, "go")
			if !containsPlain(result, "package") {
				t.Error("expected 'package' in concurrent highlight")
			}
		}()
	}
	wg.Wait()
}

func TestHighlightThemeColorsApplied(t *testing.T) {
	for _, theme := range Themes {
		h := NewSyntaxHighlighter(theme)
		code := `func test() { return 42 }`
		result := h.Highlight(code, "go")
		if !strings.Contains(result, "\x1b[") {
			t.Errorf("theme %s: expected ANSI escape codes in output", theme.Name)
		}
		if !containsPlain(result, "func") {
			t.Errorf("theme %s: expected 'func' in output", theme.Name)
		}
	}
}

func TestHighlightSupportedLanguages(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	langs := h.SupportedLanguages()

	required := []string{"go", "python", "javascript", "json", "bash", "yaml", "rust"}
	langSet := make(map[string]bool)
	for _, l := range langs {
		langSet[l] = true
	}
	for _, req := range required {
		if !langSet[req] {
			t.Errorf("expected language %q in supported languages", req)
		}
	}
}

func TestHighlightSupportsLanguage(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	if !h.SupportsLanguage("go") {
		t.Error("expected to support 'go'")
	}
	if !h.SupportsLanguage("GO") {
		t.Error("expected case-insensitive language support")
	}
	if !h.SupportsLanguage("python") {
		t.Error("expected to support 'python'")
	}
	if !h.SupportsLanguage("py") {
		t.Error("expected to support 'py' alias")
	}
	if h.SupportsLanguage("cobol") {
		t.Error("expected to not support 'cobol'")
	}
}

func TestHighlightNumbers(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `x := 42
y := 3.14
z := 1000000`
	result := h.Highlight(code, "go")
	if !containsPlain(result, "42") {
		t.Error("expected number '42' in output")
	}
	if !containsPlain(result, "3.14") {
		t.Error("expected float '3.14' in output")
	}
}

func TestHighlightYAMLCode(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `name: test
value: 42
active: true
# comment here`
	result := h.Highlight(code, "yaml")
	if !containsPlain(result, "test") {
		t.Error("expected string value in YAML output")
	}
	if !containsPlain(result, "42") {
		t.Error("expected number in YAML output")
	}
	if !containsPlain(result, "true") {
		t.Error("expected boolean in YAML output")
	}
	if !containsPlain(result, "comment here") {
		t.Error("expected comment in YAML output")
	}
}

func TestHighlightRustCode(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	code := `fn main() {
    let x: i32 = 42;
    println!("hello");
}`
	result := h.Highlight(code, "rust")
	if !containsPlain(result, "fn") {
		t.Error("expected 'fn' keyword in output")
	}
	if !containsPlain(result, "let") {
		t.Error("expected 'let' keyword in output")
	}
	if !containsPlain(result, "hello") {
		t.Error("expected string content in output")
	}
}

func TestExtractCodeBlocks(t *testing.T) {
	text := "Here is some code:\n\n```go\nfmt.Println(\"hi\")\n```\n\nMore text."
	blocks := extractCodeBlocks(text)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 code block, got %d", len(blocks))
	}
	if blocks[0].language != "go" {
		t.Errorf("expected language 'go', got %q", blocks[0].language)
	}
	if !containsPlain(blocks[0].code, "Println") {
		t.Errorf("expected code content, got %q", blocks[0].code)
	}
}

func TestExtractCodeBlocksMultiple(t *testing.T) {
	text := "```python\nx = 1\n```\nText\n```go\ny := 2\n```"
	blocks := extractCodeBlocks(text)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 code blocks, got %d", len(blocks))
	}
	if blocks[0].language != "python" {
		t.Errorf("expected first block 'python', got %q", blocks[0].language)
	}
	if blocks[1].language != "go" {
		t.Errorf("expected second block 'go', got %q", blocks[1].language)
	}
}

func TestExtractCodeBlocksNone(t *testing.T) {
	text := "Just regular text, no code blocks."
	blocks := extractCodeBlocks(text)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestExtractCodeBlocksUnclosed(t *testing.T) {
	text := "```go\nfmt.Println(\"hi\")\nNo closing fence"
	blocks := extractCodeBlocks(text)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks for unclosed fence, got %d", len(blocks))
	}
}

func TestRenderCodeBlock(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	styles := NewStyles(Themes[0])
	code := `fmt.Println("hello")`
	result := renderCodeBlock(code, "go", h, styles, 60, false)
	cleaned := cleanANSIForTest(result)
	if !strings.Contains(cleaned, "go") {
		t.Error("expected language label 'go' in code block")
	}
	if !strings.Contains(cleaned, "Println") {
		t.Error("expected code content in code block")
	}
	if !strings.Contains(cleaned, "┌") {
		t.Error("expected top border in code block")
	}
	if !strings.Contains(cleaned, "└") {
		t.Error("expected bottom border in code block")
	}
}

func TestRenderCodeBlockWithLineNumbers(t *testing.T) {
	h := NewSyntaxHighlighter(Themes[0])
	styles := NewStyles(Themes[0])
	code := "line1\nline2\nline3"
	result := renderCodeBlock(code, "go", h, styles, 60, true)
	cleaned := cleanANSIForTest(result)
	if !strings.Contains(cleaned, "1") {
		t.Error("expected line number 1")
	}
	if !strings.Contains(cleaned, "2") {
		t.Error("expected line number 2")
	}
	if !strings.Contains(cleaned, "3") {
		t.Error("expected line number 3")
	}
}

func TestRenderStreamingCursor(t *testing.T) {
	styles := NewStyles(Themes[0])
	spinner := Spinner{pulse: 0}
	visible := renderStreamingCursor(spinner, styles)
	if cleanANSIForTest(visible) != "▋" {
		t.Errorf("expected cursor '▋' when pulse is even, got %q", cleanANSIForTest(visible))
	}
	spinner.pulse = 1
	hidden := renderStreamingCursor(spinner, styles)
	if cleanANSIForTest(hidden) != " " {
		t.Errorf("expected space when pulse is odd, got %q", cleanANSIForTest(hidden))
	}
}

func TestRenderTypingDots(t *testing.T) {
	styles := NewStyles(Themes[0])
	for frame := 0; frame < 6; frame++ {
		spinner := Spinner{frame: frame}
		dots := renderTypingDots(spinner, styles)
		cleaned := cleanANSIForTest(dots)
		switch frame % 3 {
		case 0:
			if !strings.Contains(cleaned, "·") {
				t.Errorf("frame %d: expected at least one dot", frame)
			}
		case 1:
			if strings.Count(cleaned, "·") < 2 {
				t.Errorf("frame %d: expected at least two dots", frame)
			}
		case 2:
			if strings.Count(cleaned, "·") < 3 {
				t.Errorf("frame %d: expected three dots", frame)
			}
		}
	}
}
