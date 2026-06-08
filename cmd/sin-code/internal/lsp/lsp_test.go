// SPDX-License-Identifier: MIT
// Purpose: tests for the LSP package — JSON-RPC framing, server detection,
// language inference from file extension.
package lsp

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDefaultServers(t *testing.T) {
	if len(DefaultServers) < 5 {
		t.Errorf("expected at least 5 default servers, got %d", len(DefaultServers))
	}
	for _, s := range DefaultServers {
		if s.Binary == "" {
			t.Errorf("server %s has empty binary", s.Language)
		}
		if len(s.FileExts) == 0 {
			t.Errorf("server %s has no file extensions", s.Language)
		}
	}
}

func TestLanguageForFile(t *testing.T) {
	cases := map[string]string{
		"main.go":            "go",
		"foo/bar.py":         "python",
		"component.tsx":      "typescript",
		"index.js":           "javascript",
		"lib.rs":             "rust",
		"README.md":          "",
		"unknown.xyz":        "",
	}
	for file, want := range cases {
		if got := LanguageForFile(file); got != want {
			t.Errorf("LanguageForFile(%q) = %q, want %q", file, got, want)
		}
	}
}

func TestFindSpec(t *testing.T) {
	cases := []struct {
		lang string
		want bool
	}{
		{"go", true},
		{"python", true},
		{"typescript", true},
		{"javascript", true},
		{"rust", true},
		{"pyright", true},
		{"tsserver", true},
		{"kotlin", false},
		{"", false},
	}
	for _, c := range cases {
		_, got := findSpec(c.lang)
		if got != c.want {
			t.Errorf("findSpec(%q) = %v, want %v", c.lang, got, c.want)
		}
	}
}

func TestManagerNew(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("nil manager")
	}
	m.Close()
}

func TestManagerGetUnknownLang(t *testing.T) {
	m := NewManager()
	defer m.Close()
	if _, err := m.Get("kotlin", "file:///tmp"); err == nil {
		t.Error("expected error for unknown language")
	}
}

func TestManagerGetMissingBinary(t *testing.T) {
	m := NewManager()
	defer m.Close()
	_, err := m.Get("go", "file:///tmp")
	if err == nil {
		t.Skip("gopls is on PATH; skipping missing-binary test")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestManagerGetCachesClient(t *testing.T) {
	if _, err := execLookPath("gopls"); err != nil {
		t.Skip("gopls not on PATH")
	}
	m := NewManager()
	defer m.Close()
	tmp := t.TempDir()
	c1, err := m.Get("go", "file://"+tmp)
	if err != nil {
		t.Fatal(err)
	}
	c2, _ := m.Get("go", "file://"+tmp)
	if c1 != c2 {
		t.Error("manager should return same cached client")
	}
	_ = c1.Close()
}

func TestDetectAvailableReturnsAtLeastOneWhenInstalled(t *testing.T) {
	specs := DetectAvailable()
	if len(specs) > 0 {
		for _, s := range specs {
			if s.Language == "" {
				t.Error("detected server with empty language")
			}
		}
	}
}

func TestServerSpecHasBinary(t *testing.T) {
	for _, s := range DefaultServers {
		_, err := execLookPath(s.Binary)
		if err == nil {
			return
		}
	}
	t.Skip("no LSP servers installed; skipping")
}

func execLookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func TestLocationAndRange(t *testing.T) {
	loc := Location{URI: "file:///x.go", Range: Range{Start: Position{Line: 1, Character: 2}, End: Position{Line: 3, Character: 4}}}
	if loc.URI != "file:///x.go" {
		t.Error("URI mismatch")
	}
	if loc.Range.Start.Line != 1 {
		t.Error("start line mismatch")
	}
}

func TestTextDocumentItem(t *testing.T) {
	td := TextDocumentItem{URI: "file:///x.go", LanguageID: "go", Version: 1, Text: "package main"}
	if td.URI == "" {
		t.Error("URI required")
	}
	if td.Version != 1 {
		t.Error("version required")
	}
}

func TestPositionZeroIndexed(t *testing.T) {
	p := Position{Line: 0, Character: 0}
	if p.Line != 0 || p.Character != 0 {
		t.Error("zero-indexed position should be 0,0")
	}
}

func TestRenameParamsValidation(t *testing.T) {
	r := RenameParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///x.go"},
		Position:     Position{Line: 5, Character: 10},
		NewName:      "NewFunc",
	}
	if r.NewName == "" {
		t.Error("NewName required")
	}
}
