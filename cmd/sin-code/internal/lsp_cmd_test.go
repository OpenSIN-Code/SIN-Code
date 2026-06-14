// SPDX-License-Identifier: MIT

package internal

import (
	"errors"
	"fmt"

	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lsp"
)

// fakeGoplsScript is a minimal LSP server that returns canned responses.
const fakeGoplsScript = `#!/usr/bin/env python3
import sys, json

def send(obj):
    s = json.dumps(obj, separators=(',', ':'))
    sys.stdout.write("Content-Length: " + str(len(s)) + "\r\n\r\n" + s)
    sys.stdout.flush()

while True:
    headers = {}
    while True:
        line = sys.stdin.readline()
        if not line:
            sys.exit(0)
        line = line.strip()
        if not line:
            break
        if ':' in line:
            k, v = line.split(':', 1)
            headers[k.strip()] = v.strip()
    n = int(headers.get('Content-Length', 0))
    if n == 0:
        continue
    body = sys.stdin.read(n)
    try:
        msg = json.loads(body)
    except Exception:
        continue
    if msg.get('id') is None:
        continue
    method = msg.get('method', '')
    rid = msg['id']
    if method == 'initialize':
        send({"jsonrpc":"2.0","id":rid,"result":{"capabilities":{
            "documentSymbolProvider":True,
            "definitionProvider":True,
            "referencesProvider":True,
            "hoverProvider":True,
            "renameProvider":True,
            "documentFormattingProvider":True
        }}})
    elif method == 'textDocument/documentSymbol':
        send({"jsonrpc":"2.0","id":rid,"result":[
            {"name":"Hello","kind":12,"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}},"selectionRange":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}}}
        ]})
    elif method == 'textDocument/definition':
        send({"jsonrpc":"2.0","id":rid,"result":[
            {"uri":"file:///tmp/f.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":5}}}
        ]})
    elif method == 'textDocument/references':
        send({"jsonrpc":"2.0","id":rid,"result":[
            {"uri":"file:///tmp/f.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}}}
        ]})
    elif method == 'textDocument/hover':
        send({"jsonrpc":"2.0","id":rid,"result":{"contents":"hello"}})
    elif method == 'textDocument/rename':
        send({"jsonrpc":"2.0","id":rid,"result":{"changes":{"file:///tmp/f.go":[
            {"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}},"newText":"World"}
        ]}}})
    elif method == 'textDocument/formatting':
        send({"jsonrpc":"2.0","id":rid,"result":[
            {"range":{"start":{"line":0,"character":0},"end":{"line":1,"character":0}},"newText":"formatted\n"}
        ]})
    elif method == 'shutdown':
        send({"jsonrpc":"2.0","id":rid,"result":None})
    else:
        send({"jsonrpc":"2.0","id":rid,"result":None})
`

func installFakeGopls(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gopls")
	if err := os.WriteFile(path, []byte(fakeGoplsScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return dir
}

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

// TestLspStripURI verifies that stripURI removes the file:// prefix
// and unescapes path components. (st-cov1)
func TestLspStripURI(t *testing.T) {
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

// TestLspLangForPath verifies that langForPath delegates to lsp.languageForFile. (st-cov1)
func TestLspLangForPath(t *testing.T) {
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

// TestLspPrintLSPResult_Locations verifies that printLSPResult handles
// []lsp.Location correctly. (st-cov1)
func TestLspPrintLSPResult_Locations(t *testing.T) {
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

// TestLspPrintLSPResult_EmptyLocations verifies that printLSPResult handles
// empty slices. (st-cov1)
func TestLspPrintLSPResult_EmptyLocations(t *testing.T) {
	get := captureStdout(t)

	printLSPResult("test", []lsp.Location{})
	out := get()

	if !strings.Contains(out, "(no results)") {
		t.Errorf("expected output to contain '(no results)', got %q", out)
	}
}

// TestLspPrintLSPResult_NilHover verifies that printLSPResult handles
// nil *lsp.Hover. (st-cov1)
func TestLspPrintLSPResult_NilHover(t *testing.T) {
	get := captureStdout(t)

	printLSPResult("hover", (*lsp.Hover)(nil))
	out := get()

	if !strings.Contains(out, "(no hover info)") {
		t.Errorf("expected output to contain '(no hover info)', got %q", out)
	}
}

// TestLspPrintLSPResult_HoverWithContent verifies that printLSPResult handles
// a non-nil *lsp.Hover. (st-cov1)
func TestLspPrintLSPResult_HoverWithContent(t *testing.T) {
	get := captureStdout(t)

	h := &lsp.Hover{Contents: "func hello() string"}
	printLSPResult("hover", h)
	out := get()

	if !strings.Contains(out, "func hello()") {
		t.Errorf("expected output to contain hover content, got %q", out)
	}
}

// TestLspPrintLSPResult_DocumentSymbols verifies that printLSPResult handles
// []lsp.DocumentSymbol. (st-cov1)
func TestLspPrintLSPResult_DocumentSymbols(t *testing.T) {
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

// TestLspPrintLSPResult_EmptyDocumentSymbols verifies that printLSPResult
// handles empty symbol list. (st-cov1)
func TestLspPrintLSPResult_EmptyDocumentSymbols(t *testing.T) {
	get := captureStdout(t)

	printLSPResult("symbols", []lsp.DocumentSymbol{})
	out := get()

	if !strings.Contains(out, "(no symbols)") {
		t.Errorf("expected output to contain '(no symbols)', got %q", out)
	}
}

// TestLspPrintLSPResult_TextEdits verifies that printLSPResult handles
// []lsp.TextEdit. (st-cov1)
func TestLspPrintLSPResult_TextEdits(t *testing.T) {
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

// TestLspPrintLSPResult_WorkspaceEdit verifies that printLSPResult handles
// *lsp.WorkspaceEdit. (st-cov1)
func TestLspPrintLSPResult_WorkspaceEdit(t *testing.T) {
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

// TestLspPrintLSPResult_MapFallback verifies that printLSPResult handles
// map[string]any output. (st-cov1)
func TestLspPrintLSPResult_MapFallback(t *testing.T) {
	get := captureStdout(t)

	m := map[string]any{"key": "value"}
	printLSPResult("diagnostics", m)
	out := get()

	if !strings.Contains(out, `"key"`) {
		t.Errorf("expected output to contain map JSON, got %q", out)
	}
}

// TestLspRun_MissingLineCol verifies that lspRun returns an error when
// positional line/col are missing. (st-cov1)
func TestLspRun_MissingLineCol(t *testing.T) {
	oldFile, oldLine, oldCol := lspFile, lspLine, lspCol
	oldRoot := lspRoot
	defer func() { lspFile, lspLine, lspCol, lspRoot = oldFile, oldLine, oldCol, oldRoot }()

	lspFile, lspLine, lspCol = "", 0, 0
	lspRoot = t.TempDir()

	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRun(cmd, []string{"test.go"}, func(*lsp.Client, string, int, int) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error for missing line/col")
	}
}

// TestLspRunSimple_UnknownLanguage verifies that lspRunSimple returns an error
// when the language cannot be determined. (st-cov1)
func TestLspRunSimple_UnknownLanguage(t *testing.T) {
	oldFile, oldLang := lspFile, lspLang
	oldRoot := lspRoot
	defer func() { lspFile, lspLang, lspRoot = oldFile, oldLang, oldRoot }()

	lspFile = "test.unknown_ext"
	lspLang = ""
	lspRoot = t.TempDir()

	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRunSimple(cmd, []string{"test.unknown_ext"}, func(*lsp.Client, string) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error for unknown language")
	}
}

func TestLspServersCmd_None(t *testing.T) {
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", oldPath)

	oldFormat := orch2Format
	orch2Format = "text"
	defer func() { orch2Format = oldFormat }()

	get := captureStdout(t)
	if err := lspServersCmd.RunE(lspServersCmd, []string{}); err != nil {
		t.Fatalf("lspServersCmd failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "no LSP servers detected") {
		t.Errorf("expected no servers message, got %q", out)
	}
}

func TestLspServersCmd_JSON(t *testing.T) {
	binDir := t.TempDir()
	fakeGopls := filepath.Join(binDir, "gopls")
	os.WriteFile(fakeGopls, []byte("#!/bin/sh\necho ok"), 0o755)

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir)
	defer os.Setenv("PATH", oldPath)

	oldFormat := orch2Format
	orch2Format = "json"
	defer func() { orch2Format = oldFormat }()

	get := captureStdout(t)
	if err := lspServersCmd.RunE(lspServersCmd, []string{}); err != nil {
		t.Fatalf("lspServersCmd failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "go") {
		t.Errorf("expected JSON output with go server, got %q", out)
	}
}

func TestLspSetup(t *testing.T) {
	oldRoot := lspRoot
	oldFile := lspFile
	oldLine := lspLine
	oldCol := lspCol
	defer func() { lspRoot, lspFile, lspLine, lspCol = oldRoot, oldFile, oldLine, oldCol }()

	root := t.TempDir()
	lspRoot = root
	lspFile = "main.go"
	lspLine = 1
	lspCol = 1

	_, rootURI, fileURI, err := lspSetup(&cobra.Command{Use: "definition"}, lspFile, true)
	if err != nil {
		t.Fatalf("lspSetup failed: %v", err)
	}
	if !strings.HasPrefix(rootURI, "file://") {
		t.Errorf("expected rootURI to start with file://, got %q", rootURI)
	}
	if !strings.HasSuffix(fileURI, "main.go") {
		t.Errorf("expected fileURI to end with main.go, got %q", fileURI)
	}
}

func TestLspSetup_MissingLineCol(t *testing.T) {
	oldRoot := lspRoot
	oldFile := lspFile
	oldLine := lspLine
	oldCol := lspCol
	defer func() { lspRoot, lspFile, lspLine, lspCol = oldRoot, oldFile, oldLine, oldCol }()

	root := t.TempDir()
	lspRoot = root
	lspFile = "main.go"
	lspLine = 0
	lspCol = 0

	if _, _, _, err := lspSetup(&cobra.Command{Use: "definition"}, lspFile, true); err == nil {
		t.Fatal("expected error for missing line/col")
	}
}

func TestLspRunSimple_SymbolsWithFakeGopls(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot := lspFile, lspLang, lspRoot
	oldFormat := orch2Format
	defer func() { lspFile, lspLang, lspRoot, orch2Format = oldFile, oldLang, oldRoot, oldFormat }()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	orch2Format = "text"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	get := captureStdout(t)
	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRunSimple(cmd, []string{"main.go"}, func(c *lsp.Client, uri string) (any, error) {
		return c.Symbols(uri)
	}); err != nil {
		t.Fatalf("lspRunSimple symbols failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected symbols output, got %q", out)
	}
}

func TestLspRun_DefinitionWithFakeGopls(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot, oldLine, oldCol := lspFile, lspLang, lspRoot, lspLine, lspCol
	oldFormat := orch2Format
	defer func() {
		lspFile, lspLang, lspRoot, lspLine, lspCol, orch2Format = oldFile, oldLang, oldRoot, oldLine, oldCol, oldFormat
	}()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	lspLine = 1
	lspCol = 1
	orch2Format = "text"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	get := captureStdout(t)
	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(c *lsp.Client, uri string, line, col int) (any, error) {
		return c.Definition(uri, lsp.Position{Line: line, Character: col})
	}); err != nil {
		t.Fatalf("lspRun definition failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "f.go:2:1") {
		t.Errorf("expected definition output, got %q", out)
	}
}

func TestLspRun_FormatWithFakeGopls(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot := lspFile, lspLang, lspRoot
	oldFormat := orch2Format
	defer func() { lspFile, lspLang, lspRoot, orch2Format = oldFile, oldLang, oldRoot, oldFormat }()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	orch2Format = "text"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	get := captureStdout(t)
	cmd := &cobra.Command{Use: "format"}
	if err := lspRunSimple(cmd, []string{"main.go"}, func(c *lsp.Client, uri string) (any, error) {
		return c.Format(uri)
	}); err != nil {
		t.Fatalf("lspRunSimple format failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "formatted") {
		t.Errorf("expected format output, got %q", out)
	}
}

func TestLspRun_RenameWithFakeGopls(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot, oldLine, oldCol, oldNewName := lspFile, lspLang, lspRoot, lspLine, lspCol, lspNewName
	oldFormat := orch2Format
	defer func() {
		lspFile, lspLang, lspRoot, lspLine, lspCol, lspNewName, orch2Format = oldFile, oldLang, oldRoot, oldLine, oldCol, oldNewName, oldFormat
	}()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	lspLine = 1
	lspCol = 1
	lspNewName = "World"
	orch2Format = "text"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	get := captureStdout(t)
	cmd := &cobra.Command{Use: "rename"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(c *lsp.Client, uri string, line, col int) (any, error) {
		return c.Rename(uri, lsp.Position{Line: line, Character: col}, "World")
	}); err != nil {
		t.Fatalf("lspRun rename failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "World") {
		t.Errorf("expected rename output, got %q", out)
	}
}

func TestLspRun_ReferencesWithFakeGopls(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot, oldLine, oldCol := lspFile, lspLang, lspRoot, lspLine, lspCol
	oldFormat := orch2Format
	defer func() {
		lspFile, lspLang, lspRoot, lspLine, lspCol, orch2Format = oldFile, oldLang, oldRoot, oldLine, oldCol, oldFormat
	}()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	lspLine = 1
	lspCol = 1
	orch2Format = "text"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	get := captureStdout(t)
	cmd := &cobra.Command{Use: "references"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(c *lsp.Client, uri string, line, col int) (any, error) {
		return c.References(uri, lsp.Position{Line: line, Character: col}, true)
	}); err != nil {
		t.Fatalf("lspRun references failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "f.go:1:1") {
		t.Errorf("expected references output, got %q", out)
	}
}

func TestLspRun_HoverWithFakeGopls(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot, oldLine, oldCol := lspFile, lspLang, lspRoot, lspLine, lspCol
	oldFormat := orch2Format
	defer func() {
		lspFile, lspLang, lspRoot, lspLine, lspCol, orch2Format = oldFile, oldLang, oldRoot, oldLine, oldCol, oldFormat
	}()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	lspLine = 1
	lspCol = 1
	orch2Format = "text"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	get := captureStdout(t)
	cmd := &cobra.Command{Use: "hover"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(c *lsp.Client, uri string, line, col int) (any, error) {
		return c.Hover(uri, lsp.Position{Line: line, Character: col})
	}); err != nil {
		t.Fatalf("lspRun hover failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "hello") {
		t.Errorf("expected hover output, got %q", out)
	}
}

func TestLspSetup_FileFlag(t *testing.T) {
	oldRoot := lspRoot
	oldFile := lspFile
	oldLine := lspLine
	oldCol := lspCol
	defer func() { lspRoot, lspFile, lspLine, lspCol = oldRoot, oldFile, oldLine, oldCol }()

	root := t.TempDir()
	lspRoot = root
	lspFile = ""
	lspLine = 1
	lspCol = 1

	_, _, fileURI, err := lspSetup(&cobra.Command{Use: "definition"}, "main.go", true)
	if err != nil {
		t.Fatalf("lspSetup failed: %v", err)
	}
	if !strings.HasSuffix(fileURI, "main.go") {
		t.Errorf("expected fileURI to end with main.go, got %q", fileURI)
	}
}

func TestLspSetup_AbsoluteFile(t *testing.T) {
	oldRoot := lspRoot
	oldFile := lspFile
	oldLine := lspLine
	oldCol := lspCol
	defer func() { lspRoot, lspFile, lspLine, lspCol = oldRoot, oldFile, oldLine, oldCol }()

	root := t.TempDir()
	lspRoot = root
	lspFile = "/tmp/abs.go"
	lspLine = 1
	lspCol = 1

	_, _, fileURI, err := lspSetup(&cobra.Command{Use: "definition"}, "", true)
	if err != nil {
		t.Fatalf("lspSetup failed: %v", err)
	}
	if !strings.HasSuffix(fileURI, "abs.go") {
		t.Errorf("expected fileURI to end with abs.go, got %q", fileURI)
	}
}

func TestLspRun_NewNameOnNonRename(t *testing.T) {
	oldFile, oldLine, oldCol, oldNewName := lspFile, lspLine, lspCol, lspNewName
	defer func() { lspFile, lspLine, lspCol, lspNewName = oldFile, oldLine, oldCol, oldNewName }()

	lspFile = "main.go"
	lspLine = 1
	lspCol = 1
	lspNewName = "X"

	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(*lsp.Client, string, int, int) (any, error) { return nil, nil }); err != nil {
		t.Fatalf("expected nil when new-name set on non-rename command, got %v", err)
	}
}

func TestLspRun_UnknownLanguage(t *testing.T) {
	oldFile, oldLang, oldRoot, oldLine, oldCol := lspFile, lspLang, lspRoot, lspLine, lspCol
	defer func() { lspFile, lspLang, lspRoot, lspLine, lspCol = oldFile, oldLang, oldRoot, oldLine, oldCol }()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "test.unknown_ext")
	lspLang = ""
	lspLine = 1
	lspCol = 1

	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"test.unknown_ext", "1", "1"}, func(*lsp.Client, string, int, int) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error for unknown language")
	}
}

func TestLspRunSimple_JSONOutput(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot := lspFile, lspLang, lspRoot
	oldFormat := orch2Format
	defer func() { lspFile, lspLang, lspRoot, orch2Format = oldFile, oldLang, oldRoot, oldFormat }()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	orch2Format = "json"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	get := captureStdout(t)
	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRunSimple(cmd, []string{"main.go"}, func(c *lsp.Client, uri string) (any, error) {
		return c.Symbols(uri)
	}); err != nil {
		t.Fatalf("lspRunSimple json failed: %v", err)
	}
	out := get()
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("expected JSON array output, got %q", out)
	}
}

func TestLspRunSimple_FunctionError(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot := lspFile, lspLang, lspRoot
	defer func() { lspFile, lspLang, lspRoot = oldFile, oldLang, oldRoot }()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRunSimple(cmd, []string{"main.go"}, func(*lsp.Client, string) (any, error) {
		return nil, fmt.Errorf("symbol error")
	}); err == nil {
		t.Fatal("expected error from lspRunSimple function")
	}
}

func TestLspRun_JSONOutput(t *testing.T) {
	installFakeGopls(t)

	oldFile, oldLang, oldRoot, oldLine, oldCol := lspFile, lspLang, lspRoot, lspLine, lspCol
	oldFormat := orch2Format
	defer func() {
		lspFile, lspLang, lspRoot, lspLine, lspCol, orch2Format = oldFile, oldLang, oldRoot, oldLine, oldCol, oldFormat
	}()

	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	lspLine = 1
	lspCol = 1
	orch2Format = "json"
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)

	get := captureStdout(t)
	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(c *lsp.Client, uri string, line, col int) (any, error) {
		return c.Definition(uri, lsp.Position{Line: line, Character: col})
	}); err != nil {
		t.Fatalf("lspRun json failed: %v", err)
	}
	out := get()
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("expected JSON array output, got %q", out)
	}
}

func TestLspPrintLSPResult_NilWorkspaceEdit(t *testing.T) {
	get := captureStdout(t)
	printLSPResult("rename", (*lsp.WorkspaceEdit)(nil))
	out := get()
	if !strings.Contains(out, "(no edit)") {
		t.Errorf("expected '(no edit)' output, got %q", out)
	}
}

func TestLspPrintLSPResult_Default(t *testing.T) {
	get := captureStdout(t)
	printLSPResult("diagnostics", []string{"a", "b"})
	out := get()
	if !strings.Contains(out, "a") {
		t.Errorf("expected default JSON output, got %q", out)
	}
}

// runLspCmd runs a cobra LSP subcommand with a fake gopls server and returns
// captured stdout.
func runLspCmd(t *testing.T, cmd *cobra.Command, args []string, newName string, format string) (string, error) {
	t.Helper()
	installFakeGopls(t)
	root := t.TempDir()

	oldFile, oldLang, oldRoot, oldLine, oldCol, oldNewName, oldFormat :=
		lspFile, lspLang, lspRoot, lspLine, lspCol, lspNewName, orch2Format
	defer func() {
		lspFile, lspLang, lspRoot, lspLine, lspCol, lspNewName, orch2Format =
			oldFile, oldLang, oldRoot, oldLine, oldCol, oldNewName, oldFormat
	}()

	lspRoot = root
	lspLang = "go"
	lspNewName = newName
	orch2Format = format

	if len(args) > 0 && !strings.HasPrefix(args[0], "/") {
		os.WriteFile(filepath.Join(root, args[0]), []byte("package main\n"), 0o644)
	}

	get := captureStdout(t)
	err := cmd.RunE(cmd, args)
	return get(), err
}

func TestLspServersCmd_Text(t *testing.T) {
	out, err := runLspCmd(t, lspServersCmd, []string{}, "", "text")
	if err != nil {
		t.Fatalf("servers text failed: %v", err)
	}
	if !strings.Contains(out, "Detected") || !strings.Contains(out, "go") {
		t.Errorf("expected text server list, got %q", out)
	}
}

func TestLspCmd_Definition(t *testing.T) {
	out, err := runLspCmd(t, lspDefinitionCmd, []string{"main.go", "1", "1"}, "", "text")
	if err != nil {
		t.Fatalf("definition command failed: %v", err)
	}
	if !strings.Contains(out, "f.go:2:1") {
		t.Errorf("expected definition output, got %q", out)
	}
}

func TestLspCmd_References(t *testing.T) {
	out, err := runLspCmd(t, lspReferencesCmd, []string{"main.go", "1", "1"}, "", "text")
	if err != nil {
		t.Fatalf("references command failed: %v", err)
	}
	if !strings.Contains(out, "f.go:1:1") {
		t.Errorf("expected references output, got %q", out)
	}
}

func TestLspCmd_Hover(t *testing.T) {
	out, err := runLspCmd(t, lspHoverCmd, []string{"main.go", "1", "1"}, "", "text")
	if err != nil {
		t.Fatalf("hover command failed: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected hover output, got %q", out)
	}
}

func TestLspCmd_Rename(t *testing.T) {
	out, err := runLspCmd(t, lspRenameCmd, []string{"main.go", "1", "1"}, "World", "text")
	if err != nil {
		t.Fatalf("rename command failed: %v", err)
	}
	if !strings.Contains(out, "World") {
		t.Errorf("expected rename output, got %q", out)
	}
}

func TestLspCmd_RenameMissingNewName(t *testing.T) {
	_, err := runLspCmd(t, lspRenameCmd, []string{"main.go", "1", "1"}, "", "text")
	if err == nil {
		t.Fatal("expected error for missing --new-name")
	}
	if !strings.Contains(err.Error(), "new-name required") {
		t.Errorf("expected new-name error, got %v", err)
	}
}

func TestLspCmd_Symbols(t *testing.T) {
	out, err := runLspCmd(t, lspSymbolsCmd, []string{"main.go"}, "", "text")
	if err != nil {
		t.Fatalf("symbols command failed: %v", err)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected symbols output, got %q", out)
	}
}

func TestLspCmd_Format(t *testing.T) {
	out, err := runLspCmd(t, lspFormatCmd, []string{"main.go"}, "", "text")
	if err != nil {
		t.Fatalf("format command failed: %v", err)
	}
	if !strings.Contains(out, "formatted") {
		t.Errorf("expected format output, got %q", out)
	}
}

func TestLspCmd_Diagnostics(t *testing.T) {
	out, err := runLspCmd(t, lspDiagnosticsCmd, []string{"main.go"}, "", "text")
	if err != nil {
		t.Fatalf("diagnostics command failed: %v", err)
	}
	if !strings.Contains(out, "\"file\"") || !strings.Contains(out, "hint") {
		t.Errorf("expected diagnostics map output, got %q", out)
	}
}

func TestLspCmd_DiagnosticsReadError(t *testing.T) {
	_, err := runLspCmd(t, lspDiagnosticsCmd, []string{"/nonexistent/missing.go"}, "", "text")
	if err == nil {
		t.Fatal("expected error for missing diagnostics file")
	}
}

func TestLspRun_MgrGetError(t *testing.T) {
	oldFile, oldLang, oldRoot, oldLine, oldCol := lspFile, lspLang, lspRoot, lspLine, lspCol
	defer func() { lspFile, lspLang, lspRoot, lspLine, lspCol = oldFile, oldLang, oldRoot, oldLine, oldCol }()
	t.Setenv("PATH", "")
	root := t.TempDir()
	lspRoot = root
	lspFile = ""
	lspLang = ""
	lspLine = 1
	lspCol = 1
	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"test.rs", "1", "1"}, func(*lsp.Client, string, int, int) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error when mgr.Get fails")
	}
}

func TestLspRun_FunctionError(t *testing.T) {
	installFakeGopls(t)
	oldFile, oldLang, oldRoot, oldLine, oldCol := lspFile, lspLang, lspRoot, lspLine, lspCol
	defer func() { lspFile, lspLang, lspRoot, lspLine, lspCol = oldFile, oldLang, oldRoot, oldLine, oldCol }()
	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "main.go")
	lspLang = "go"
	lspLine = 1
	lspCol = 1
	os.WriteFile(lspFile, []byte("package main\n"), 0o644)
	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(*lsp.Client, string, int, int) (any, error) { return nil, errors.New("fn error") }); err == nil {
		t.Fatal("expected error from fn")
	}
}

func TestLspRun_FallbackLanguage(t *testing.T) {
	installFakeGopls(t)
	oldFile, oldLang, oldRoot, oldLine, oldCol := lspFile, lspLang, lspRoot, lspLine, lspCol
	oldFormat := orch2Format
	defer func() {
		lspFile, lspLang, lspRoot, lspLine, lspCol, orch2Format = oldFile, oldLang, oldRoot, oldLine, oldCol, oldFormat
	}()
	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "test.unknown_ext")
	lspLang = "go"
	lspLine = 1
	lspCol = 1
	orch2Format = "text"
	get := captureStdout(t)
	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"test.unknown_ext", "1", "1"}, func(c *lsp.Client, uri string, line, col int) (any, error) {
		return []lsp.Location{{URI: uri, Range: lsp.Range{Start: lsp.Position{Line: line, Character: col}}}}, nil
	}); err != nil {
		t.Fatalf("lspRun fallback failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "test.unknown_ext") {
		t.Errorf("expected fallback language output, got %q", out)
	}
}

func TestLspRun_ParseArgsError(t *testing.T) {
	oldFile, oldLine, oldCol := lspFile, lspLine, lspCol
	oldParse := lspParseArgsFn
	defer func() { lspFile, lspLine, lspCol = oldFile, oldLine, oldCol; lspParseArgsFn = oldParse }()
	lspParseArgsFn = func([]string, bool) error { return errors.New("forced parse error") }
	lspFile = "main.go"
	lspLine = 1
	lspCol = 1
	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(*lsp.Client, string, int, int) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error from lspParseArgs")
	}
}

func TestLspRun_SetupError(t *testing.T) {
	oldFile, oldLine, oldCol := lspFile, lspLine, lspCol
	oldRoot := lspRoot
	oldGetwd := osGetwd
	defer func() { lspFile, lspLine, lspCol, lspRoot = oldFile, oldLine, oldCol, oldRoot; osGetwd = oldGetwd }()
	osGetwd = func() (string, error) { return "", errors.New("forced getwd error") }
	lspRoot = ""
	lspFile = "main.go"
	lspLine = 1
	lspCol = 1
	cmd := &cobra.Command{Use: "definition"}
	if err := lspRun(cmd, []string{"main.go", "1", "1"}, func(*lsp.Client, string, int, int) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error from lspSetup")
	}
}

func TestLspRunSimple_MgrGetError(t *testing.T) {
	oldFile, oldLang, oldRoot := lspFile, lspLang, lspRoot
	defer func() { lspFile, lspLang, lspRoot = oldFile, oldLang, oldRoot }()
	t.Setenv("PATH", "")
	root := t.TempDir()
	lspRoot = root
	lspFile = ""
	lspLang = ""
	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRunSimple(cmd, []string{"test.rs"}, func(*lsp.Client, string) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error when mgr.Get fails")
	}
}

func TestLspRunSimple_FallbackLanguage(t *testing.T) {
	installFakeGopls(t)
	oldFile, oldLang, oldRoot := lspFile, lspLang, lspRoot
	oldFormat := orch2Format
	defer func() { lspFile, lspLang, lspRoot, orch2Format = oldFile, oldLang, oldRoot, oldFormat }()
	root := t.TempDir()
	lspRoot = root
	lspFile = filepath.Join(root, "test.unknown_ext")
	lspLang = "go"
	orch2Format = "text"
	get := captureStdout(t)
	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRunSimple(cmd, []string{"test.unknown_ext"}, func(c *lsp.Client, uri string) (any, error) {
		return []lsp.DocumentSymbol{{Name: "Fallback", Kind: 12}}, nil
	}); err != nil {
		t.Fatalf("lspRunSimple fallback failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "Fallback") {
		t.Errorf("expected fallback language output, got %q", out)
	}
}

func TestLspRunSimple_ParseArgsError(t *testing.T) {
	oldFile := lspFile
	oldParse := lspParseArgsFn
	defer func() { lspFile = oldFile; lspParseArgsFn = oldParse }()
	lspParseArgsFn = func([]string, bool) error { return errors.New("forced parse error") }
	lspFile = "main.go"
	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRunSimple(cmd, []string{"main.go"}, func(*lsp.Client, string) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error from lspParseArgs")
	}
}

func TestLspRunSimple_SetupError(t *testing.T) {
	oldFile := lspFile
	oldRoot := lspRoot
	oldGetwd := osGetwd
	defer func() { lspFile = oldFile; lspRoot = oldRoot; osGetwd = oldGetwd }()
	osGetwd = func() (string, error) { return "", errors.New("forced getwd error") }
	lspRoot = ""
	lspFile = "main.go"
	cmd := &cobra.Command{Use: "symbols"}
	if err := lspRunSimple(cmd, []string{"main.go"}, func(*lsp.Client, string) (any, error) { return nil, nil }); err == nil {
		t.Fatal("expected error from lspSetup")
	}
}

func TestLspSetup_RootFromCWD(t *testing.T) {
	oldRoot, oldFile, oldLine, oldCol := lspRoot, lspFile, lspLine, lspCol
	defer func() { lspRoot, lspFile, lspLine, lspCol = oldRoot, oldFile, oldLine, oldCol }()
	lspRoot = ""
	lspFile = "main.go"
	lspLine = 1
	lspCol = 1
	_, rootURI, fileURI, err := lspSetup(&cobra.Command{Use: "definition"}, lspFile, true)
	if err != nil {
		t.Fatalf("lspSetup failed: %v", err)
	}
	cwd, _ := os.Getwd()
	if !strings.Contains(rootURI, cwd) {
		t.Errorf("expected rootURI to contain cwd %q, got %q", cwd, rootURI)
	}
	if !strings.HasSuffix(fileURI, "main.go") {
		t.Errorf("expected fileURI to end with main.go, got %q", fileURI)
	}
}

func TestLspSetup_GetwdError(t *testing.T) {
	oldRoot := lspRoot
	oldGetwd := osGetwd
	defer func() { lspRoot = oldRoot; osGetwd = oldGetwd }()
	osGetwd = func() (string, error) { return "", errors.New("forced getwd error") }
	lspRoot = ""
	if _, _, _, err := lspSetup(&cobra.Command{Use: "definition"}, "main.go", true); err == nil {
		t.Fatal("expected error from os.Getwd")
	}
}

func TestLspSetup_AbsError(t *testing.T) {
	oldRoot := lspRoot
	oldAbs := filepathAbs
	defer func() { lspRoot = oldRoot; filepathAbs = oldAbs }()
	filepathAbs = func(string) (string, error) { return "", errors.New("forced abs error") }
	lspRoot = t.TempDir()
	if _, _, _, err := lspSetup(&cobra.Command{Use: "definition"}, "main.go", true); err == nil {
		t.Fatal("expected error from filepath.Abs")
	}
}

func TestLspSetup_MissingCol(t *testing.T) {
	oldRoot, oldFile, oldLine, oldCol := lspRoot, lspFile, lspLine, lspCol
	defer func() { lspRoot, lspFile, lspLine, lspCol = oldRoot, oldFile, oldLine, oldCol }()
	root := t.TempDir()
	lspRoot = root
	lspFile = "main.go"
	lspLine = 1
	lspCol = 0
	if _, _, _, err := lspSetup(&cobra.Command{Use: "definition"}, lspFile, true); err == nil {
		t.Fatal("expected error for missing col")
	}
}

func TestLspStripURI_UnescapeError(t *testing.T) {
	in := "file:///tmp/bad%ZZ.go"
	got := stripURI(in)
	if got != in {
		t.Errorf("stripURI(%q) = %q, want original %q", in, got, in)
	}
}
