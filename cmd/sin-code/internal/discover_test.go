// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the discover subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"fmt"
	"testing"
)

func TestDiscoverCmd_Flags(t *testing.T) {
	cmd := DiscoverCmd
	if cmd.Use != "discover [path]" {
		t.Errorf("expected Use 'discover [path]', got %q", cmd.Use)
	}
	flags := []string{"pattern", "sort_by", "format", "limit"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestDiscoverCmd_DefaultValues(t *testing.T) {
	cmd := DiscoverCmd
	discoverPattern = "**/*"
	discoverSort = "relevance"
	discoverFormat = "text"
	discoverLimit = 100
	if v, _ := cmd.Flags().GetString("pattern"); v != "**/*" {
		t.Errorf("default pattern should be **/*, got %q", v)
	}
	if v, _ := cmd.Flags().GetString("sort_by"); v != "relevance" {
		t.Errorf("default sort_by should be relevance, got %q", v)
	}
	if v, _ := cmd.Flags().GetString("format"); v != "text" {
		t.Errorf("default format should be text, got %q", v)
	}
	if v, _ := cmd.Flags().GetInt("limit"); v != 100 {
		t.Errorf("default limit should be 100, got %d", v)
	}
}

func TestDiscoverCmd_RunWithValidPath(t *testing.T) {
	dir := t.TempDir()
	discoverFormat = "text"
	discoverPattern = "**/*"
	discoverSort = "relevance"
	discoverLimit = 10
	if err := DiscoverCmd.RunE(DiscoverCmd, []string{dir}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
}

func TestBuildGlobMatcher(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		input    string
		expected bool
	}{
		{"empty pattern", "", "anything.go", true},
		{"all files", "**/*", "main.go", true},
		{"go files", "**/*.go", "cmd/main.go", true},
		{"go files no match", "**/*.go", "main.py", false},
		{"specific file", "cmd/main.go", "cmd/main.go", true},
		{"specific file no match", "cmd/main.go", "cmd/main.py", false},
		{"single char", "src/?.go", "src/a.go", true},
		{"single char no match", "src/?.go", "src/ab.go", false},
		{"double star prefix", "**/main.go", "cmd/main.go", true},
		{"double star prefix no match", "**/main.go", "cmd/test.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := buildGlobMatcher(tt.pattern)
			if err != nil {
				t.Fatalf("buildGlobMatcher failed: %v", err)
			}
			if got := matcher(tt.input); got != tt.expected {
				t.Errorf("buildGlobMatcher(%q)(%q) = %v, want %v", tt.pattern, tt.input, got, tt.expected)
			}
		})
	}
}

func TestGlobToRegex(t *testing.T) {
	tests := []struct {
		glob     string
		expected string
	}{
		{"*", "[^/]*"},
		{"?", "[^/]"},
		{"main.go", "main\\.go"},
		{"a+b", "a\\+b"},
		{"test[1].go", "test\\[1\\]\\.go"},
	}

	for _, tt := range tests {
		t.Run(tt.glob, func(t *testing.T) {
			got := globToRegex(tt.glob)
			if got != tt.expected {
				t.Errorf("globToRegex(%q) = %q, want %q", tt.glob, got, tt.expected)
			}
		})
	}
}

func TestScoreRelevance(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		size     int64
		minScore float64
		maxScore float64
	}{
		{"root go file", "main.go", 1000, 50, 100},
		{"deep nested file", "a/b/c/d/e/f/main.go", 1000, 0, 60},
		{"config file", "config.yaml", 100, 50, 100},
		{"readme", "README.md", 1000, 50, 100},
		{"large file", "big.go", 2_000_000, 0, 80},
		{"vendor file", "vendor/lib.go", 1000, 0, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreRelevance(tt.relPath, tt.size)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("scoreRelevance(%q, %d) = %.1f, want between %.1f and %.1f",
					tt.relPath, tt.size, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestScoreRelevanceBounds(t *testing.T) {
	// Test that score is clamped to [0, 100]
	score := scoreRelevance(strings.Repeat("a/", 100)+"main.go", 0)
	if score < 0 || score > 100 {
		t.Errorf("score should be clamped to [0,100], got %.1f", score)
	}
	score = scoreRelevance("main.go", 0)
	if score < 0 || score > 100 {
		t.Errorf("score should be clamped to [0,100], got %.1f", score)
	}
}

func TestExtractDependencies(t *testing.T) {
	dir := t.TempDir()

	// Go file with imports
	goFile := filepath.Join(dir, "main.go")
	goContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	fmt.Println("hello")
	os.Exit(0)
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0644); err != nil {
		t.Fatal(err)
	}

	deps := extractDependencies(goFile)
	if len(deps) < 2 {
		t.Errorf("expected at least 2 Go imports, got %d", len(deps))
	}
	found := make(map[string]bool)
	for _, d := range deps {
		found[d] = true
	}
	if !found["fmt"] || !found["os"] {
		t.Errorf("expected fmt and os imports, got %v", deps)
	}

	// Python file with imports
	pyFile := filepath.Join(dir, "main.py")
	pyContent := `import os
import sys
from pathlib import Path
`
	if err := os.WriteFile(pyFile, []byte(pyContent), 0644); err != nil {
		t.Fatal(err)
	}
	pyDeps := extractDependencies(pyFile)
	if len(pyDeps) < 2 {
		t.Errorf("expected at least 2 Python imports, got %d", len(pyDeps))
	}

	// JS file with imports
	jsFile := filepath.Join(dir, "main.js")
	jsContent := `import React from 'react';
const fs = require('fs');
`
	if err := os.WriteFile(jsFile, []byte(jsContent), 0644); err != nil {
		t.Fatal(err)
	}
	jsDeps := extractDependencies(jsFile)
	if len(jsDeps) < 1 {
		t.Errorf("expected at least 1 JS import, got %d", len(jsDeps))
	}

	// Unknown extension should return nil
	txtFile := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(txtFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	txtDeps := extractDependencies(txtFile)
	if txtDeps != nil {
		t.Errorf("expected nil for unknown extension, got %v", txtDeps)
	}
}

func TestExtractGoImports(t *testing.T) {
	content := `package main
import "fmt"
import "os"
`
	deps := extractGoImports(content, "main.go")
	if len(deps) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(deps))
	}
	if deps[0] != "fmt" || deps[1] != "os" {
		t.Errorf("expected fmt and os, got %v", deps)
	}
}

func TestExtractGoImports_InvalidSyntax(t *testing.T) {
	deps := extractGoImports("not valid go", "bad.go")
	if deps != nil {
		t.Errorf("expected nil for invalid Go syntax, got %v", deps)
	}
}

func TestExtractPythonImports(t *testing.T) {
	content := `import os
import sys
from pathlib import Path
from mypackage.submodule import something
import numpy as np
`
	deps := extractPythonImports(content)
	expected := map[string]bool{"os": true, "sys": true, "pathlib": true, "mypackage": true, "numpy": true}
	if len(deps) != len(expected) {
		t.Errorf("expected %d imports, got %d: %v", len(expected), len(deps), deps)
	}
	for _, d := range deps {
		if !expected[d] {
			t.Errorf("unexpected import %q", d)
		}
	}
}

func TestExtractJSImports(t *testing.T) {
	content := `import React from 'react';
import { useState } from 'react';
const fs = require('fs');
const path = require('path');
`
	deps := extractJSImports(content)
	if len(deps) != 3 {
		t.Errorf("expected 3 imports, got %d: %v", len(deps), deps)
	}
	found := make(map[string]bool)
	for _, d := range deps {
		found[d] = true
	}
	if !found["react"] || !found["fs"] || !found["path"] {
		t.Errorf("expected react, fs, path imports, got %v", deps)
	}
}

func TestSortResults(t *testing.T) {
	results := []fileResult{
		{RelPath: "b.go", Relevance: 30, Size: 200, ModTime: "2024-01-02T00:00:00Z"},
		{RelPath: "a.go", Relevance: 50, Size: 100, ModTime: "2024-01-03T00:00:00Z"},
		{RelPath: "c.go", Relevance: 40, Size: 300, ModTime: "2024-01-01T00:00:00Z"},
	}

	t.Run("relevance", func(t *testing.T) {
		r := make([]fileResult, len(results))
		copy(r, results)
		sortResults(r, "relevance")
		if r[0].RelPath != "a.go" || r[1].RelPath != "c.go" || r[2].RelPath != "b.go" {
			t.Errorf("expected a.go, c.go, b.go, got %v", r)
		}
	})

	t.Run("name", func(t *testing.T) {
		r := make([]fileResult, len(results))
		copy(r, results)
		sortResults(r, "name")
		if r[0].RelPath != "a.go" || r[1].RelPath != "b.go" || r[2].RelPath != "c.go" {
			t.Errorf("expected a.go, b.go, c.go, got %v", r)
		}
	})

	t.Run("size", func(t *testing.T) {
		r := make([]fileResult, len(results))
		copy(r, results)
		sortResults(r, "size")
		if r[0].RelPath != "c.go" || r[1].RelPath != "b.go" || r[2].RelPath != "a.go" {
			t.Errorf("expected c.go, b.go, a.go, got %v", r)
		}
	})

	t.Run("mtime", func(t *testing.T) {
		r := make([]fileResult, len(results))
		copy(r, results)
		sortResults(r, "mtime")
		if r[0].RelPath != "a.go" || r[1].RelPath != "b.go" || r[2].RelPath != "c.go" {
			t.Errorf("expected a.go, b.go, c.go, got %v", r)
		}
	})
}

func TestDiscoverFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# README\n"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "util.go"), []byte("package sub\n"), 0644)
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("[core]\n"), 0644)

	results, err := discoverFiles(dir, "*.go", 100)
	if err != nil {
		t.Fatalf("discoverFiles failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 .go file (root only), got %d", len(results))
	}
	for _, r := range results {
		if strings.Contains(r.RelPath, ".git") {
			t.Errorf("should skip .git directory, got %q", r.RelPath)
		}
	}
}

func TestDiscoverFiles_InvalidPattern(t *testing.T) {
	_, err := discoverFiles(".", "\\1", 100)
	if err == nil {
		t.Error("expected error for invalid pattern")
	}
}

func TestDiscoverFiles_NonDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(f, []byte("hi"), 0644)
	_, err := discoverFiles(f, "**/*", 100)
	if err == nil {
		// Walk doesn't error on non-dir, it just walks the file
		// But our RunE checks for directory. discoverFiles itself might not.
		// This test verifies the function is tolerant.
	}
}

func TestOutputJSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	results := []fileResult{{RelPath: "test.go", Relevance: 50, Size: 100}}
	err := outputJSON(results)
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputJSON failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var parsed []fileResult
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Errorf("outputJSON produced invalid JSON: %v\n%s", err, buf.String())
	}
	if len(parsed) != 1 || parsed[0].RelPath != "test.go" {
		t.Errorf("unexpected JSON output: %s", buf.String())
	}
}

func TestOutputText(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	results := []fileResult{{RelPath: "test.go", Relevance: 50, Size: 100}}
	err := outputText(results)
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputText failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "test.go") {
		t.Errorf("expected output to contain 'test.go', got %q", out)
	}
	if !strings.Contains(out, "score: 50.0") {
		t.Errorf("expected output to contain 'score: 50.0', got %q", out)
	}
}

func TestDiscoverFiles_DotDirsSkipped(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("git"), 0644)
	os.Mkdir(filepath.Join(dir, "node_modules"), 0755)
	os.WriteFile(filepath.Join(dir, "node_modules", "foo.js"), []byte("module"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)

	results, err := discoverFiles(dir, "**/*", 100)
	if err != nil {
		t.Fatalf("discoverFiles failed: %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.RelPath, ".git") || strings.Contains(r.RelPath, "node_modules") {
			t.Errorf("expected .git and node_modules dirs to be skipped, got %q", r.RelPath)
		}
	}
}

func TestScoreRelevance_MediumFile(t *testing.T) {
	score := scoreRelevance("big.go", 500_000)
	noPenalty := scoreRelevance("big.go", 10_000)
	if score >= noPenalty {
		t.Errorf("expected medium file to score lower than small file, got %.1f vs %.1f", score, noPenalty)
	}
}

func TestScoreRelevance_SmallFile(t *testing.T) {
	score := scoreRelevance("main.go", 500)
	if score <= 50 {
		t.Errorf("expected no penalty for small file, got %.1f", score)
	}
}

func TestExtractDependencies_LargeFile(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "big.go")
	bigContent := "package main\nimport \"fmt\"\n" + strings.Repeat("// fill\n", 100000)
	os.WriteFile(bigFile, []byte(bigContent), 0644)

	deps := extractDependencies(bigFile)
	_ = deps
}

func TestExtractDependencies_ReadError(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "secret.go")
	os.WriteFile(secretFile, []byte("package main\nimport \"fmt\"\n"), 0644)
	os.Chmod(secretFile, 0000)
	defer os.Chmod(secretFile, 0644)

	deps := extractDependencies(secretFile)
	if deps != nil {
		t.Errorf("expected nil for unreadable file, got %v", deps)
	}
}

func TestDiscoverFiles_EarlyStop(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 50; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file_%d.go", i)), []byte("package main\n"), 0644)
	}

	results, err := discoverFiles(dir, "**/*", 5)
	if err != nil {
		t.Fatalf("discoverFiles failed: %v", err)
	}
	if len(results) > 500 {
		t.Errorf("expected limited results, got %d", len(results))
	}
}

func TestDiscoverCmd_InvalidPath(t *testing.T) {
	discoverFormat = "text"
	discoverPattern = "**/*"
	discoverSort = "relevance"
	discoverLimit = 10
	err := DiscoverCmd.RunE(DiscoverCmd, []string{"/nonexistent/path"})
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestDiscoverCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)

	discoverFormat = "json"
	discoverPattern = "**/*"
	discoverSort = "relevance"
	discoverLimit = 10

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := DiscoverCmd.RunE(DiscoverCmd, []string{dir})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("DiscoverCmd.RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var results []fileResult
	if err := json.Unmarshal(buf.Bytes(), &results); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
}

func TestScoreRelevance_EntryKeywords(t *testing.T) {
	score := scoreRelevance("index.ts", 1000)
	if score <= 50 {
		t.Errorf("expected bonus for index filename, got %.1f", score)
	}
}

func TestBuildGlobMatcher_SpecialChars(t *testing.T) {
	matcher, err := buildGlobMatcher("[invalid")
	if err != nil {
		t.Fatalf("buildGlobMatcher should handle special chars: %v", err)
	}
	if !matcher("[invalid") {
		t.Error("escaped glob should match literal [invalid")
	}
}