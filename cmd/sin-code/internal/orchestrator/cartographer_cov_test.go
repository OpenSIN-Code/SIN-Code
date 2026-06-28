// SPDX-License-Identifier: MIT
// Purpose: coverage tests for cartographer.go helper functions that run
// without the "coverage" build tag.
package orchestrator

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestLastSegment(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a/b/c", "c"},
		{"a/b", "b"},
		{"a", "a"},
		{"", ""},
		{"a/b/c/d", "d"},
		{"/deep/nested/path", "path"},
	}
	for _, c := range cases {
		got := lastSegment(c.in)
		if got != c.want {
			t.Errorf("lastSegment(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLastSegmentNoSlash(t *testing.T) {
	got := lastSegment("noslash")
	if got != "noslash" {
		t.Errorf("lastSegment(%q) = %q, want %q", "noslash", got, "noslash")
	}
}

func TestAffectedByPathMatch(t *testing.T) {
	affected := map[string]bool{
		"repo/foo": true,
		"repo/bar": true,
	}
	// file path contains the last segment of an affected package
	if !affectedByPath(affected, "src/repo/foo/file.go") {
		t.Error("expected true for foo match")
	}
}

func TestAffectedByPathNoMatch(t *testing.T) {
	affected := map[string]bool{
		"repo/foo": true,
	}
	if affectedByPath(affected, "src/repo/baz/file.go") {
		t.Error("expected false for no match")
	}
}

func TestAffectedByPathEmpty(t *testing.T) {
	if affectedByPath(map[string]bool{}, "anything.go") {
		t.Error("expected false for empty affected map")
	}
}

func TestCallNameIdent(t *testing.T) {
	src := `package x
func caller() {
	callee()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var callExprs []*ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			callExprs = append(callExprs, call)
		}
		return true
	})
	if len(callExprs) == 0 {
		t.Fatal("expected at least one call expression")
	}
	got := callName(callExprs[0], "x")
	if got != "x.callee" {
		t.Errorf("callName(ident) = %q, want %q", got, "x.callee")
	}
}

func TestCallNameSelector(t *testing.T) {
	src := `package x
import "fmt"
func caller() {
	fmt.Println("hi")
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found string
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if name := callName(call, "x"); name != "" {
				found = name
			}
		}
		return true
	})
	if found != "fmt.Println" {
		t.Errorf("callName(selector) = %q, want %q", found, "fmt.Println")
	}
}

func TestCallNameOtherType(t *testing.T) {
	// Call expressions that are not Ident or SelectorExpr should return "".
	src := `package x
func caller() {
	(func(){})()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if got := callName(call, "x"); got != "" {
				t.Errorf("callName(anon) = %q, want empty", got)
			}
		}
		return true
	})
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0.5, 0.5},
		{1.5, 1.0},
		{-0.5, 0.0},
		{0.0, 0.0},
		{1.0, 1.0},
	}
	for _, c := range cases {
		got := normalize(c.in)
		if got != c.want {
			t.Errorf("normalize(%f) = %f, want %f", c.in, got, c.want)
		}
	}
}

func TestClamp(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0.5, 0.5},
		{1.5, 1.0},
		{-0.5, 0.0},
		{0.0, 0.0},
		{1.0, 1.0},
	}
	for _, c := range cases {
		got := clamp(c.in)
		if got != c.want {
			t.Errorf("clamp(%f) = %f, want %f", c.in, got, c.want)
		}
	}
}

func TestSignatureLine(t *testing.T) {
	src := "line1\nfunc foo() {\nline3\n"
	got := signatureLine(src, 2)
	if got != "func foo() {" {
		t.Errorf("signatureLine = %q, want %q", got, "func foo() {")
	}
}

func TestSignatureLineOutOfRange(t *testing.T) {
	src := "only one line\n"
	got := signatureLine(src, 10)
	if got != "" {
		t.Errorf("signatureLine(out of range) = %q, want empty", got)
	}
}

func TestCartographerFindingsZero(t *testing.T) {
	c := NewCartographer("")
	if findings := c.Findings(0); len(findings) != 0 {
		t.Errorf("Findings(0) should return nil, got %d", len(findings))
	}
}

func TestCartographerFindingsNegative(t *testing.T) {
	c := NewCartographer("")
	if findings := c.Findings(-1); len(findings) != 0 {
		t.Errorf("Findings(-1) should return nil, got %d", len(findings))
	}
}

func TestCartographerFindingsWithSymbols(t *testing.T) {
	dir := t.TempDir()
	must_write(t, filepath_join(dir, "x.go"), "package x\nfunc Hello() {}\nfunc World() {}\n")
	must_write(t, filepath_join(dir, "go.mod"), "module x\n\ngo 1.22\n")
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	findings := c.Findings(1)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Tag != TagVerify {
		t.Errorf("expected TagVerify, got %s", findings[0].Tag)
	}
}
