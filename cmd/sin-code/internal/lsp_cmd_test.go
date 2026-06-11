// SPDX-License-Identifier: MIT

package internal

import (
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/lsp"
)

// TestLspParseArgs_PositionArgs verifies that lspParseArgs correctly
// extracts file, line, col from positional args. (st-cov1)
func TestLspParseArgs_PositionArgs(t *testing.T) {
	// Reset globals
	oldFile, oldLine, oldCol := lspFile, lspLine, lspCol
	defer func() { lspFile, lspLine, lspCol = oldFile, oldLine, oldCol }()
	lspFile, lspLine, lspCol = "", 0, 0

	if err := lspParseArgs([]string{"main.go", "5", "9"}, true); err != nil {
		t.Fatalf("lspParseArgs: %v", err)
	}
	if lspFile != "main.go" {
		t.Errorf("expected file=main.go, got %q", lspFile)
	}
	if lspLine != 5 {
		t.Errorf("expected line=5, got %d", lspLine)
	}
	if lspCol != 9 {
		t.Errorf("expected col=9, got %d", lspCol)
	}
}

// TestLspParseArgs_InvalidLine verifies that lspParseArgs rejects non-numeric line. (st-cov1)
func TestLspParseArgs_InvalidLine(t *testing.T) {
	if err := lspParseArgs([]string{"main.go", "abc"}, true); err == nil {
		t.Error("expected error for invalid line number")
	}
	if err := lspParseArgs([]string{"main.go", "5", "xyz"}, true); err == nil {
		t.Error("expected error for invalid col number")
	}
}

// TestLspParseArgs_TooFewArgs verifies that lspParseArgs handles
// missing args gracefully. (st-cov1)
func TestLspParseArgs_TooFewArgs(t *testing.T) {
	// Just file, no line/col
	if err := lspParseArgs([]string{"main.go"}, true); err != nil {
		t.Errorf("expected no error for missing line/col, got %v", err)
	}
	// No args
	if err := lspParseArgs([]string{}, true); err != nil {
		t.Errorf("expected no error for no args, got %v", err)
	}
}

// TestStripURI verifies that stripURI removes the file:// prefix
// and unescapes path components. (st-cov1)
func TestStripURI(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"file:///tmp/test.go", "/tmp/test.go"},
		{"file:///tmp/has%20space.go", "/tmp/has space.go"},
		{"/tmp/not-a-uri.go", "/tmp/not-a-uri.go"},
		{"file:///tmp/encoded%2Fslash.go", "/tmp/encoded/slash.go"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := stripURI(tt.in)
			if got != tt.want {
				t.Errorf("stripURI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestLangForPath verifies that langForPath delegates to lsp.languageForFile. (st-cov1)
func TestLangForPath(t *testing.T) {
	tests := []struct {
		path string
		want string // empty if unknown
	}{
		{"foo.go", "go"},
		{"foo.py", "python"},
		{"foo.js", "javascript"},
		{"foo.ts", "typescript"},
		{"foo.rs", "rust"},
		{"foo.unknown_ext", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := langForPath(tt.path)
			if got != tt.want {
				t.Errorf("langForPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestPrintLSPResult_Locations verifies that printLSPResult handles
// []lsp.Location correctly. (st-cov1)
func TestPrintLSPResult_Locations(t *testing.T) {
	get := captureStdout(t)

	locs := []lsp.Location{
		{URI: "file:///tmp/test.go", Range: lsp.Range{Start: lsp.Position{Line: 5, Character: 9}, End: lsp.Position{Line: 5, Character: 15}}},
	}
	printLSPResult("test", locs)
	out := get()

	if !strings.Contains(out, "test.go:6:10") {
		t.Errorf("expected output to contain 'test.go:6:10', got %q", out)
	}
}

// TestPrintLSPResult_EmptyLocations verifies that printLSPResult handles
// empty slices. (st-cov1)
func TestPrintLSPResult_EmptyLocations(t *testing.T) {
	get := captureStdout(t)

	printLSPResult("test", []lsp.Location{})
	out := get()

	if !strings.Contains(out, "(no results)") {
		t.Errorf("expected output to contain '(no results)', got %q", out)
	}
}

// TestPrintLSPResult_NilHover verifies that printLSPResult handles
// nil *lsp.Hover. (st-cov1)
func TestPrintLSPResult_NilHover(t *testing.T) {
	get := captureStdout(t)

	printLSPResult("hover", (*lsp.Hover)(nil))
	out := get()

	if !strings.Contains(out, "(no hover info)") {
		t.Errorf("expected output to contain '(no hover info)', got %q", out)
	}
}

// TestPrintLSPResult_HoverWithContent verifies that printLSPResult handles
// a non-nil *lsp.Hover. (st-cov1)
func TestPrintLSPResult_HoverWithContent(t *testing.T) {
	get := captureStdout(t)

	h := &lsp.Hover{Contents: "func hello() string"}
	printLSPResult("hover", h)
	out := get()

	if !strings.Contains(out, "func hello()") {
		t.Errorf("expected output to contain hover content, got %q", out)
	}
}

// TestPrintLSPResult_DocumentSymbols verifies that printLSPResult handles
// []lsp.DocumentSymbol. (st-cov1)
func TestPrintLSPResult_DocumentSymbols(t *testing.T) {
	get := captureStdout(t)

	syms := []lsp.DocumentSymbol{
		{Name: "main", Kind: 12}, // 12 = Function in LSP
		{Name: "helper", Kind: 12},
	}
	printLSPResult("symbols", syms)
	out := get()

	if !strings.Contains(out, "main") || !strings.Contains(out, "helper") {
		t.Errorf("expected output to contain 'main' and 'helper', got %q", out)
	}
}

// TestPrintLSPResult_EmptyDocumentSymbols verifies that printLSPResult
// handles empty symbol list. (st-cov1)
func TestPrintLSPResult_EmptyDocumentSymbols(t *testing.T) {
	get := captureStdout(t)

	printLSPResult("symbols", []lsp.DocumentSymbol{})
	out := get()

	if !strings.Contains(out, "(no symbols)") {
		t.Errorf("expected output to contain '(no symbols)', got %q", out)
	}
}

// TestPrintLSPResult_TextEdits verifies that printLSPResult handles
// []lsp.TextEdit. (st-cov1)
func TestPrintLSPResult_TextEdits(t *testing.T) {
	get := captureStdout(t)

	edits := []lsp.TextEdit{
		{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}, NewText: "x"},
	}
	printLSPResult("format", edits)
	out := get()

	if !strings.Contains(out, `"x"`) {
		t.Errorf("expected output to contain '\"x\"', got %q", out)
	}
}

// TestPrintLSPResult_WorkspaceEdit verifies that printLSPResult handles
// *lsp.WorkspaceEdit. (st-cov1)
func TestPrintLSPResult_WorkspaceEdit(t *testing.T) {
	get := captureStdout(t)

	we := &lsp.WorkspaceEdit{
		Changes: map[string][]lsp.TextEdit{
			"file:///tmp/test.go": {{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}, NewText: "y"}},
		},
	}
	printLSPResult("rename", we)
	out := get()

	if !strings.Contains(out, `"y"`) {
		t.Errorf("expected output to contain '\"y\"', got %q", out)
	}
}

// TestPrintLSPResult_MapFallback verifies that printLSPResult handles
// map[string]any output. (st-cov1)
func TestPrintLSPResult_MapFallback(t *testing.T) {
	get := captureStdout(t)

	m := map[string]any{"key": "value"}
	printLSPResult("diagnostics", m)
	out := get()

	if !strings.Contains(out, `"key"`) {
		t.Errorf("expected output to contain map JSON, got %q", out)
	}
}
