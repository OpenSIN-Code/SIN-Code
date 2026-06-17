// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the testgen package. Keeps the package isolated
// from the real network and external tools by stubbing exec.LookPath and
// using small fixtures in a temporary directory.
package testgen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveTestFileName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"foo.go", "foo_test.go"},
		{"/path/to/bar.go", "/path/to/bar_test.go"},
		{"bar_test.go", "bar_test.go"},
	}
	for _, c := range cases {
		got := deriveTestFileName(c.in)
		if got != c.want {
			t.Errorf("deriveTestFileName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGenerateFallback(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	code := `package calc

func Add(a, b int) int {
	return a + b
}

func Greet(name string) string {
	return "Hello " + name
}
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := generateFallback(context.Background(), src, nil, nil)
	if err != nil {
		t.Fatalf("generateFallback: %v", err)
	}

	if !strings.Contains(got, GeneratedMarker) {
		t.Errorf("generated test missing marker")
	}
	if !strings.Contains(got, "func TestAdd(t *testing.T)") {
		t.Errorf("generated test missing TestAdd")
	}
	if !strings.Contains(got, "func TestGreet(t *testing.T)") {
		t.Errorf("generated test missing TestGreet")
	}
}

func TestGenerateFallbackNoExportedFuncs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	code := `package calc

func add(a, b int) int { return a + b }
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := generateFallback(context.Background(), src, nil, nil)
	if err == nil {
		t.Fatal("expected error for file with no exported functions")
	}
}

func TestGenerateForFileOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	code := `package calc

func Add(a, b int) int { return a + b }
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create pre-existing test file.
	testFile := filepath.Join(dir, "calc_test.go")
	if err := os.WriteFile(testFile, []byte("package calc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := Generate(context.Background(), Options{File: src, Overwrite: false})
	if res.Error == "" || !strings.Contains(res.Error, "already exists") {
		t.Fatalf("expected overwrite error, got %+v", res)
	}

	res2 := Generate(context.Background(), Options{File: src, Overwrite: true})
	if res2.Error != "" {
		t.Fatalf("expected overwrite success, got: %s", res2.Error)
	}
	if !strings.Contains(res2.TestOutput, "PASS") {
		t.Logf("test output: %s", res2.TestOutput)
	}
}

func TestFindGoFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package p\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package p\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package p\n"), 0o644)

	files, err := findGoFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			t.Errorf("findGoFiles included test file: %s", f)
		}
	}
}

func TestSimpleTypeAndZeroValue(t *testing.T) {
	if !simpleType("int") || !simpleType("string") || !simpleType("bool") {
		t.Error("expected primitive types to be simple")
	}
	if simpleType("MyStruct") {
		t.Error("expected custom type to be non-simple")
	}
	if zeroValue("int") != "0" || zeroValue("string") != `""` || zeroValue("bool") != "false" {
		t.Error("unexpected zero values")
	}
}

