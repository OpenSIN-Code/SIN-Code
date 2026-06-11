// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the grasp subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraspCmd_Flags(t *testing.T) {
	cmd := GraspCmd
	if cmd.Use != "grasp [path]" {
		t.Errorf("expected Use 'grasp [path]', got %q", cmd.Use)
	}
	if cmd.Flags().Lookup("format") == nil {
		t.Error("missing flag --format")
	}
}

func TestGraspCmd_DefaultFormat(t *testing.T) {
	v, _ := GraspCmd.Flags().GetString("format")
	if v != "text" {
		t.Errorf("default format should be text, got %q", v)
	}
}

func TestGraspCmd_RequiresExactlyOneArg(t *testing.T) {
	err := GraspCmd.Args(GraspCmd, []string{})
	if err == nil {
		t.Error("expected error for zero args")
	}
	err = GraspCmd.Args(GraspCmd, []string{"a", "b"})
	if err == nil {
		t.Error("expected error for two args")
	}
	err = GraspCmd.Args(GraspCmd, []string{"one"})
	if err != nil {
		t.Errorf("expected no error for one arg, got %v", err)
	}
}

func TestGraspCmd_NonexistentFile(t *testing.T) {
	graspFormat = "text"
	err := GraspCmd.RunE(GraspCmd, []string{"/nonexistent/path/file.go"})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestGraspCmd_DirectoryPath(t *testing.T) {
	dir := t.TempDir()
	graspFormat = "text"
	err := GraspCmd.RunE(GraspCmd, []string{dir})
	if err == nil {
		t.Error("expected error when path is a directory")
	}
}

func TestGraspCmd_ValidGoFile_TextFormat(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "sample.go")
	content := `package sample

import "fmt"

func Hello() {
	fmt.Println("hello")
}

type MyStruct struct {
	Name string
}

var GlobalVar = 42
`
	if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	graspFormat = "text"
	err := GraspCmd.RunE(GraspCmd, []string{goFile})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "Language: go") {
		t.Errorf("expected output to contain 'Language: go', got %q", out)
	}
	if !strings.Contains(out, "Structure") {
		t.Errorf("expected output to contain 'Structure', got %q", out)
	}
}

func TestGraspCmd_ValidGoFile_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "sample.go")
	content := `package sample

import "fmt"

func Hello() {
	fmt.Println("hello")
}
`
	if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	graspFormat = "json"
	err := GraspCmd.RunE(GraspCmd, []string{goFile})
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var result graspResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, buf.String())
	}
	if result.Language != "go" {
		t.Errorf("expected language go, got %q", result.Language)
	}
	if result.Lines == 0 {
		t.Error("expected non-zero line count")
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"app.ts", "typescript"},
		{"component.tsx", "tsx"},
		{"component.jsx", "jsx"},
		{"main.rs", "rust"},
		{"App.java", "java"},
		{"main.c", "c"},
		{"util.cpp", "cpp"},
		{"header.h", "c-header"},
		{"header.hpp", "cpp-header"},
		{"run.sh", "bash"},
		{"README.md", "markdown"},
		{"config.json", "json"},
		{"values.yaml", "yaml"},
		{"values.yml", "yaml"},
		{"cargo.toml", "toml"},
		{"page.html", "html"},
		{"style.css", "css"},
		{"query.sql", "sql"},
		{"app.rb", "ruby"},
		{"index.php", "php"},
		{"app.swift", "swift"},
		{"main.kt", "kotlin"},
		{"App.scala", "scala"},
		{"plot.r", "r"},
		{"script.lua", "lua"},
		{"go.mod", "go"},
		{"Dockerfile", "dockerfile"},
		{"dockerfile.dev", "dockerfile"},
		{"Makefile", "makefile"},
		{"gnumakefile", "makefile"},
		{"data.xyz", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectLanguage(tt.path)
			if got != tt.expected {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		lang         string
		wantTotal    int
		wantBlank    int
		wantComments int
		wantCode     int
	}{
		{
			name:         "empty file",
			content:      "",
			lang:         "go",
			wantTotal:    0,
			wantBlank:    0,
			wantComments: 0,
			wantCode:     0,
		},
		{
			name:         "only blank lines",
			content:      "\n\n\n",
			lang:         "go",
			wantTotal:    3,
			wantBlank:    3,
			wantComments: 0,
			wantCode:     0,
		},
		{
			name:         "go line comments",
			content:      "// comment\npackage main\n\nfunc main() {}\n",
			lang:         "go",
			wantTotal:    4,
			wantBlank:    1,
			wantComments: 1,
			wantCode:     2,
		},
		{
			name:         "go block comment",
			content:      "/* block\ncomment */\npackage main\n",
			lang:         "go",
			wantTotal:    3,
			wantBlank:    0,
			wantComments: 2,
			wantCode:     1,
		},
		{
			name:         "python comments",
			content:      "# comment\nimport os\n\ndef main():\n    pass\n",
			lang:         "python",
			wantTotal:    5,
			wantBlank:    1,
			wantComments: 1,
			wantCode:     3,
		},
		{
			name:         "javascript comments",
			content:      "// js comment\nconst x = 1;\n\nfunction foo() {}\n",
			lang:         "javascript",
			wantTotal:    4,
			wantBlank:    1,
			wantComments: 1,
			wantCode:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, blank, comments, code := countLines(tt.content, tt.lang)
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
			if blank != tt.wantBlank {
				t.Errorf("blank = %d, want %d", blank, tt.wantBlank)
			}
			if comments != tt.wantComments {
				t.Errorf("comments = %d, want %d", comments, tt.wantComments)
			}
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

func TestExtractStructure_Go(t *testing.T) {
	content := `package main

import "fmt"

func main() { fmt.Println("hi") }

func helper() string { return "" }

type Server struct{}

type Handler interface{}

var Version = "1.0"

const MaxRetries = 3
`
	items := extractStructure("test.go", content)

	found := make(map[string]string)
	for _, item := range items {
		found[item.Name] = item.Type
	}

	if found["main"] != "function" {
		t.Errorf("expected main as function, got %q", found["main"])
	}
	if found["helper"] != "function" {
		t.Errorf("expected helper as function, got %q", found["helper"])
	}
	if found["Server"] != "struct" {
		t.Errorf("expected Server as struct, got %q", found["Server"])
	}
	if found["Handler"] != "interface" {
		t.Errorf("expected Handler as interface, got %q", found["Handler"])
	}
	if found["Version"] != "variable" {
		t.Errorf("expected Version as variable, got %q", found["Version"])
	}
	if found["MaxRetries"] != "variable" {
		t.Errorf("expected MaxRetries as variable, got %q", found["MaxRetries"])
	}
}

func TestExtractStructure_InvalidSyntax(t *testing.T) {
	items := extractStructure("test.go", "not valid go code at all")
	_ = items
}

func TestExtractStructure_Python(t *testing.T) {
	content := "class MyClass:\n    def method(self):\n        pass\ndef standalone():\n    pass\n"
	items := extractStructure("test.py", content)
	if len(items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(items))
	}
	found := make(map[string]bool)
	for _, item := range items {
		found[item.Name] = true
	}
	if !found["MyClass"] {
		t.Errorf("expected MyClass in structure, got %v", items)
	}
	if !found["standalone"] {
		t.Errorf("expected standalone in structure, got %v", items)
	}
}

func TestExtractStructure_JS(t *testing.T) {
	content := "export function myFunc() {}\nclass MyClass {}\nconst MY_CONST = 42;\ninterface MyInterface {}\ntype MyType = string;\n"
	items := extractStructure("test.js", content)
	found := make(map[string]string)
	for _, item := range items {
		found[item.Name] = item.Type
	}
	if found["myFunc"] != "function" {
		t.Errorf("expected myFunc as function, got %q", found["myFunc"])
	}
	if found["MyClass"] != "class" {
		t.Errorf("expected MyClass as class, got %q", found["MyClass"])
	}
	// Structural engine doesn't detect const declarations (brace-based tracking only).
	if found["MY_CONST"] != "" {
		t.Errorf("expected MY_CONST NOT found by structural engine, got %q", found["MY_CONST"])
	}
}

func TestExtractStructure_Rust(t *testing.T) {
	content := "fn main() {}\npub fn public_func() {}\nstruct MyStruct {}\nenum MyEnum {}\ntrait MyTrait {}\n"
	items := extractStructure("test.rs", content)
	found := make(map[string]string)
	for _, item := range items {
		found[item.Name] = item.Type
	}
	if found["main"] != "function" {
		t.Errorf("expected main as function, got %q", found["main"])
	}
	if found["MyStruct"] != "struct" {
		t.Errorf("expected MyStruct as struct, got %q", found["MyStruct"])
	}
	if found["MyEnum"] != "enum" {
		t.Errorf("expected MyEnum as enum, got %q", found["MyEnum"])
	}
	if found["MyTrait"] != "trait" {
		t.Errorf("expected MyTrait as trait, got %q", found["MyTrait"])
	}
}

func TestExtractStructure_Generic(t *testing.T) {
	content := "function myFunc()\ndef my_python_func():\nclass MyClass:\nsub my_perl_sub {\n"
	items := extractStructure("test.unknown", content)
	if len(items) < 3 {
		t.Errorf("expected at least 3 items, got %d", len(items))
	}
}

func TestExtractExports_Go(t *testing.T) {
	content := `package main

import "fmt"

func Hello() { fmt.Println("hi") }

type Server struct{}

var Version = "1.0"
`
	exports := extractExports(content, "go")
	found := make(map[string]bool)
	for _, e := range exports {
		found[e] = true
	}
	if !found["Hello"] {
		t.Errorf("expected Hello in exports, got %v", exports)
	}
	if !found["Server"] {
		t.Errorf("expected Server in exports, got %v", exports)
	}
	if !found["Version"] {
		t.Errorf("expected Version in exports, got %v", exports)
	}
}

func TestExtractExports_Python(t *testing.T) {
	content := `__all__ = ["MyClass", "helper", "process"]
`
	exports := extractExports(content, "python")
	found := make(map[string]bool)
	for _, e := range exports {
		found[e] = true
	}
	if !found["MyClass"] || !found["helper"] || !found["process"] {
		t.Errorf("expected MyClass, helper, process in exports, got %v", exports)
	}
}

func TestExtractExports_Python_NoDunderAll(t *testing.T) {
	exports := extractExports(`import os
def main():
    pass
`, "python")
	if len(exports) != 0 {
		t.Errorf("expected no exports without __all__, got %v", exports)
	}
}

func TestExtractExports_JS(t *testing.T) {
	content := `export function myFunc() {}
export class MyClass {}
export const MY_CONST = 42;
`
	exports := extractExports(content, "javascript")
	found := make(map[string]bool)
	for _, e := range exports {
		found[e] = true
	}
	if !found["myFunc"] || !found["MyClass"] || !found["MY_CONST"] {
		t.Errorf("expected myFunc, MyClass, MY_CONST in exports, got %v", exports)
	}
}

func TestExtractExports_Rust(t *testing.T) {
	content := `pub fn public_func() {}
pub struct PublicStruct {}
pub enum PublicEnum {}
fn private_func() {}
`
	exports := extractExports(content, "rust")
	if len(exports) < 1 {
		t.Errorf("expected at least 1 export, got %v", exports)
	}
	for _, e := range exports {
		if e == "private_func" {
			t.Error("private_func should not be in exports")
		}
	}
}

func TestExtractExports_UnsupportedLang(t *testing.T) {
	exports := extractExports("some content", "ruby")
	if len(exports) != 0 {
		t.Errorf("expected no exports for unsupported lang, got %v", exports)
	}
}

func TestAnalyzeFile_GoFile(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "test.go")
	content := `package test

import "fmt"

// Hello greets
func Hello() {
	fmt.Println("hello")
}
`
	if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(goFile)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analyzeFile(goFile, info)
	if err != nil {
		t.Fatalf("analyzeFile failed: %v", err)
	}
	if result.Language != "go" {
		t.Errorf("expected language go, got %q", result.Language)
	}
	if result.Lines == 0 {
		t.Error("expected non-zero lines")
	}
	if result.Size == 0 {
		t.Error("expected non-zero size")
	}
	if len(result.Dependencies) == 0 {
		t.Error("expected at least one dependency (fmt)")
	}
}

func TestAnalyzeFile_PythonFile(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "test.py")
	content := `import os

# A comment
def main():
    pass
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(pyFile)
	if err != nil {
		t.Fatal(err)
	}

	result, err := analyzeFile(pyFile, info)
	if err != nil {
		t.Fatalf("analyzeFile failed: %v", err)
	}
	if result.Language != "python" {
		t.Errorf("expected language python, got %q", result.Language)
	}
}

func TestOutputTextGrasp(t *testing.T) {
	r := &graspResult{
		Path:         "/tmp/test.go",
		Language:     "go",
		Size:         100,
		Lines:        10,
		CodeLines:    7,
		CommentLines: 2,
		BlankLines:   1,
		ModTime:      "2024-01-01 00:00:00",
		Summary:      "10 lines (7 code, 2 comments, 1 blank) in go",
		Structure:    []structItem{{Type: "function", Name: "main", Line: 5}},
		Dependencies: []string{"fmt"},
		Exports:      []string{"main"},
	}

	old := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextGrasp(r)
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextGrasp failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(pr)
	out := buf.String()
	if !strings.Contains(out, "go") {
		t.Errorf("expected output to contain 'go', got %q", out)
	}
	if !strings.Contains(out, "function") {
		t.Errorf("expected output to contain 'function', got %q", out)
	}
	if !strings.Contains(out, "fmt") {
		t.Errorf("expected output to contain 'fmt', got %q", out)
	}
}

func TestOutputTextGrasp_EmptyFields(t *testing.T) {
	r := &graspResult{
		Path:     "/tmp/empty.go",
		Language: "go",
		Size:     0,
		Lines:    0,
		ModTime:  "2024-01-01 00:00:00",
		Summary:  "0 lines (0 code, 0 comments, 0 blank) in go",
	}

	old := os.Stdout
	_, pw, _ := os.Pipe()
	os.Stdout = pw

	err := outputTextGrasp(r)
	pw.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputTextGrasp failed: %v", err)
	}
}

func TestExtractStructure_LangDispatch(t *testing.T) {
	tests := []struct {
		lang string
	}{
		{"go"},
		{"python"},
		{"javascript"},
		{"typescript"},
		{"rust"},
		{"java"},
		{"unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			items := extractStructure("function test() {}", tt.lang)
			_ = items
		})
	}
}

func TestAnalyzeFile_ReadError(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "unreadable.go")
	os.WriteFile(goFile, []byte("package main\n"), 0644)
	os.Chmod(goFile, 0000)
	defer os.Chmod(goFile, 0644)

	info, _ := os.Stat(goFile)
	_, err := analyzeFile(goFile, info)
	if err == nil {
		t.Error("expected error for unreadable file")
	}
}

func TestExtractStructure_Java(t *testing.T) {
	content := "public class Main {\n    public void run() {}\n}\ninterface Runnable {}\n"
	items := extractStructure("test.java", content)
	if len(items) < 1 {
		t.Fatalf("expected at least 1 java item, got %d", len(items))
	}
	found := false
	for _, item := range items {
		if item.Name == "Main" && item.Type == "class" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Main class, got %v", items)
	}
}

func TestCountLines_PythonBlockComment(t *testing.T) {
	content := `"""block
comment
here"""
import os
def main():
    pass
`
	_, _, comments, _ := countLines(content, "python")
	if comments < 2 {
		t.Errorf("expected at least 2 comment lines for block comment, got %d", comments)
	}
}

func TestCountLines_PythonTripleSingleQuote(t *testing.T) {
	content := `'''block
comment
here'''
import os
`
	_, _, comments, _ := countLines(content, "python")
	if comments < 2 {
		t.Errorf("expected at least 2 comment lines for single-quote block comment, got %d", comments)
	}
}

func TestCountLines_BashComments(t *testing.T) {
	content := "// comment\\necho hello\\n"
	_, _, comments, _ := countLines(content, "bash")
	if comments < 1 {
		t.Errorf("expected at least 1 comment line for bash, got %d", comments)
	}
}

func TestCountLines_RubyComments(t *testing.T) {
	content := "# comment\nputs 'hello'\n"
	_, _, comments, _ := countLines(content, "ruby")
	if comments < 1 {
		t.Errorf("expected at least 1 comment, got %d", comments)
	}
}

func TestExtractExports_TypeScriptExport(t *testing.T) {
	content := `export function myFunc() {}\nexport class MyClass {}\nexport default function() {}\n`
	exports := extractExports(content, "typescript")
	if len(exports) < 1 {
		t.Errorf("expected at least 1 TS export, got %v", exports)
	}
}

func TestExtractExports_PythonWithAll(t *testing.T) {
	content := `__all__ = ["Func1", "Func2", "Func3"]
def Func1(): pass
`
	exports := extractExports(content, "python")
	if len(exports) != 3 {
		t.Errorf("expected 3 exports, got %v", exports)
	}
}

func TestGraspCmd_InvalidAbsPath(t *testing.T) {
	graspFormat = "text"
	err := GraspCmd.RunE(GraspCmd, []string{"\x00invalid"})
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestAnalyzeFile_JSFile(t *testing.T) {
	dir := t.TempDir()
	jsFile := filepath.Join(dir, "app.js")
	os.WriteFile(jsFile, []byte("function hello() { return 1; }\nconst x = 5;\n"), 0644)
	info, _ := os.Stat(jsFile)

	result, err := analyzeFile(jsFile, info)
	if err != nil {
		t.Fatalf("analyzeFile failed: %v", err)
	}
	if result.Language != "javascript" {
		t.Errorf("expected language javascript, got %q", result.Language)
	}
}

func TestAnalyzeFile_RustFile(t *testing.T) {
	dir := t.TempDir()
	rsFile := filepath.Join(dir, "main.rs")
	os.WriteFile(rsFile, []byte("fn main() {}\nstruct Point {}\n"), 0644)
	info, _ := os.Stat(rsFile)

	result, err := analyzeFile(rsFile, info)
	if err != nil {
		t.Fatalf("analyzeFile failed: %v", err)
	}
	if result.Language != "rust" {
		t.Errorf("expected language rust, got %q", result.Language)
	}
}
