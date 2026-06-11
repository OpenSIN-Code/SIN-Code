// SPDX-License-Identifier: MIT

package internal

import (
	"strings"
	"testing"
)

func TestFindSymbol_Direct(t *testing.T) {
	outline := &FileOutline{
		Language: "go",
		Engine:   "go/ast",
		Symbols: []SymbolInfo{
			{Name: "main", Kind: "func", StartLine: 5, EndLine: 10},
			{Name: "helper", Kind: "func", StartLine: 12, EndLine: 15},
			{Name: "Server", Kind: "struct", StartLine: 17, EndLine: 25,
				Children: []SymbolInfo{
					{Name: "Server.handle", Kind: "method", StartLine: 20, EndLine: 24},
				}},
		},
	}
	hits := findSymbol(outline, "main")
	if len(hits) != 1 || hits[0].Name != "main" {
		t.Errorf("expected 1 hit for 'main', got %+v", hits)
	}
}

func TestFindSymbol_QualifiedName(t *testing.T) {
	outline := &FileOutline{
		Language: "go",
		Engine:   "go/ast",
		Symbols: []SymbolInfo{
			{Name: "Server", Kind: "struct", StartLine: 1, EndLine: 10,
				Children: []SymbolInfo{
					{Name: "Server.handle", Kind: "method", StartLine: 3, EndLine: 8},
				}},
		},
	}
	hits := findSymbol(outline, "handle")
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit for qualified 'handle', got %d", len(hits))
	}
	if hits[0].Name != "Server.handle" {
		t.Errorf("expected 'Server.handle', got %q", hits[0].Name)
	}
}

func TestFindSymbol_NotFound(t *testing.T) {
	outline := &FileOutline{
		Language: "go",
		Engine:   "go/ast",
		Symbols: []SymbolInfo{
			{Name: "main", Kind: "func", StartLine: 1, EndLine: 5},
		},
	}
	hits := findSymbol(outline, "nonexistent")
	if len(hits) != 0 {
		t.Errorf("expected 0 hits for 'nonexistent', got %d", len(hits))
	}
}

func TestFindSymbol_EmptyOutline(t *testing.T) {
	outline := &FileOutline{Language: "go", Engine: "go/ast"}
	hits := findSymbol(outline, "main")
	if len(hits) != 0 {
		t.Errorf("expected 0 hits for empty outline, got %d", len(hits))
	}
}

func TestOutlineEngineFor(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"test.go", "go/ast"},
		{"test.py", "structural"},
		{"test.js", "structural"},
		{"test.unknown", "none"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := outlineEngineFor(tt.path)
			if got != tt.want {
				t.Errorf("outlineEngineFor(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestFormatHashlines(t *testing.T) {
	lines := []string{"package main", "func hello() {}", "}"}
	out := FormatHashlines(lines, 1)
	if !strings.HasPrefix(out, "1:") {
		t.Errorf("expected output to start with line number '1:', got %q", out[:5])
	}
	if !strings.Contains(out, "package main") {
		t.Errorf("expected output to contain 'package main', got %q", out)
	}
	if !strings.Contains(out, "|") {
		t.Errorf("expected hash separator '|' in output, got %q", out)
	}
}

func TestFormatHashlines_StartOffset(t *testing.T) {
	lines := []string{"line1", "line2"}
	out := FormatHashlines(lines, 100)
	if !strings.HasPrefix(out, "100:") {
		t.Errorf("expected output to start with line number '100:', got %q", out[:5])
	}
	if !strings.Contains(out, "101:") {
		t.Errorf("expected line 101 reference, got %q", out)
	}
}

func TestFormatHashlines_Empty(t *testing.T) {
	out := FormatHashlines([]string{}, 1)
	if out != "" {
		t.Errorf("expected empty string for empty input, got %q", out)
	}
}

func TestApplySymbolEdit_Replace(t *testing.T) {
	original := "package main\n\nfunc main() {\n\tfmt.Println(\"old\")\n}\n"
	req := editRequest{
		Symbol:  "main",
		NewText: "func main() { fmt.Println(\"new\") }",
	}
	res := &editResult{}
	out, err := applySymbolEdit([]string{
		"package main",
		"",
		"func main() {",
		"\tfmt.Println(\"old\")",
		"}",
	}, "test.go", original, req, res)
	if err != nil {
		t.Fatalf("applySymbolEdit: %v", err)
	}
	if !strings.Contains(strings.Join(out, "\n"), "fmt.Println(\"new\")") {
		t.Errorf("expected replaced content to contain new text, got: %s", out)
	}
}

func TestApplySymbolEdit_Delete(t *testing.T) {
	original := "package main\n\nfunc unused() {}\n\nfunc main() {}\n"
	req := editRequest{
		Symbol: "unused",
		Delete: true,
	}
	res := &editResult{}
	out, err := applySymbolEdit([]string{
		"package main",
		"",
		"func unused() {}",
		"",
		"func main() {}",
	}, "test.go", original, req, res)
	if err != nil {
		t.Fatalf("applySymbolEdit delete: %v", err)
	}
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "func unused") {
		t.Errorf("expected 'unused' function to be deleted, got: %s", joined)
	}
	if !strings.Contains(joined, "func main") {
		t.Errorf("expected 'main' to be preserved, got: %s", joined)
	}
}

func TestApplySymbolEdit_InsertBefore(t *testing.T) {
	original := "package main\n\nfunc main() {}\n"
	req := editRequest{
		Symbol:  "main",
		NewText: "// new comment\n",
		Insert:  "before",
	}
	res := &editResult{}
	out, err := applySymbolEdit([]string{
		"package main",
		"",
		"func main() {}",
	}, "test.go", original, req, res)
	if err != nil {
		t.Fatalf("applySymbolEdit insert before: %v", err)
	}
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "// new comment") {
		t.Errorf("expected comment to be inserted, got: %s", joined)
	}
}

func TestApplySymbolEdit_InsertAfter(t *testing.T) {
	original := "package main\n\nfunc main() {}\n"
	req := editRequest{
		Symbol:  "main",
		NewText: "func helper() {}\n",
		Insert:  "after",
	}
	res := &editResult{}
	out, err := applySymbolEdit([]string{
		"package main",
		"",
		"func main() {}",
	}, "test.go", original, req, res)
	if err != nil {
		t.Fatalf("applySymbolEdit insert after: %v", err)
	}
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "func helper") {
		t.Errorf("expected helper to be inserted after main, got: %s", joined)
	}
}

func TestApplySymbolEdit_NotFound(t *testing.T) {
	original := "package main\n\nfunc main() {}\n"
	req := editRequest{
		Symbol:  "nonexistent",
		NewText: "replacement",
	}
	res := &editResult{}
	_, err := applySymbolEdit([]string{
		"package main",
		"",
		"func main() {}",
	}, "test.go", original, req, res)
	if err == nil {
		t.Fatal("expected error for nonexistent symbol")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestApplySymbolEdit_Ambiguous(t *testing.T) {
	original := "package main\n\nfunc main() {}\n\nfunc main() {}\n"
	req := editRequest{
		Symbol:  "main",
		NewText: "replacement",
	}
	res := &editResult{}
	_, err := applySymbolEdit([]string{
		"package main",
		"",
		"func main() {}",
		"",
		"func main() {}",
	}, "test.go", original, req, res)
	if err == nil {
		t.Fatal("expected error for ambiguous symbol")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected 'ambiguous' in error, got: %v", err)
	}
}

func TestApplySymbolEdit_UnsupportedLanguage(t *testing.T) {
	original := "package main\n"
	req := editRequest{
		Symbol:  "main",
		NewText: "replacement",
	}
	res := &editResult{}
	_, err := applySymbolEdit([]string{"package main"}, "test.unknown", original, req, res)
	if err == nil {
		t.Fatal("expected error for unknown language")
	}
	if !strings.Contains(err.Error(), "no AST engine") {
		t.Errorf("expected 'no AST engine' in error, got: %v", err)
	}
}

func TestApplySymbolEdit_InvalidInsert(t *testing.T) {
	original := "package main\n\nfunc main() {}\n"
	req := editRequest{
		Symbol:  "main",
		NewText: "foo",
		Insert:  "middle",
	}
	res := &editResult{}
	_, err := applySymbolEdit([]string{
		"package main",
		"",
		"func main() {}",
	}, "test.go", original, req, res)
	if err == nil {
		t.Fatal("expected error for invalid insert value")
	}
	if !strings.Contains(err.Error(), "invalid --insert") {
		t.Errorf("expected 'invalid --insert' in error, got: %v", err)
	}
}
