// SPDX-License-Identifier: MIT
// Purpose: Unit tests for chat tool functions that can be exercised without
// a full agent loop. Covers argBool parsing, sin_test, and sin_test_generate.
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArgBool(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		key  string
		def  bool
		want bool
	}{
		{"missing", map[string]any{}, "x", true, true},
		{"bool true", map[string]any{"x": true}, "x", false, true},
		{"bool false", map[string]any{"x": false}, "x", true, false},
		{"string true", map[string]any{"x": "true"}, "x", false, true},
		{"string yes", map[string]any{"x": "YES"}, "x", false, true},
		{"string 1", map[string]any{"x": "1"}, "x", false, true},
		{"string false", map[string]any{"x": "false"}, "x", true, false},
		{"float64 1", map[string]any{"x": float64(1)}, "x", false, true},
		{"int 0", map[string]any{"x": 0}, "x", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := argBool(c.args, c.key, c.def); got != c.want {
				t.Errorf("argBool(%v, %q, %v) = %v, want %v", c.args, c.key, c.def, got, c.want)
			}
		})
	}
}

func TestToolTestGoProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package calc\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc_test.go"), []byte("package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"expected 3\")\n\t}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	out, err := toolTest(context.Background(), map[string]any{
		"target":  ".",
		"race":    "false",
		"cover":   "false",
		"json":    "true",
		"timeout": "30s",
	})
	if err != nil {
		t.Fatalf("toolTest: %v", err)
	}
	if !strings.Contains(out, `"status": "PASS"`) {
		t.Fatalf("expected PASS status, got: %s", out)
	}
}

func TestToolTestGenerateForFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "calc.go")
	if err := os.WriteFile(src, []byte("package calc\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	out, err := toolTestGenerate(context.Background(), map[string]any{
		"file":      "calc.go",
		"overwrite": "false",
	})
	if err != nil {
		t.Fatalf("toolTestGenerate: %v", err)
	}
	if !strings.Contains(out, "test_passed") {
		t.Fatalf("expected test_passed in output, got: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "calc_test.go")); err != nil {
		t.Fatalf("expected calc_test.go to be generated: %v", err)
	}
}

func TestMaybeGenerateTest(t *testing.T) {
	// Disable by default.
	autoGenerateTests = false
	if got := maybeGenerateTest("foo.go"); got != "" {
		t.Fatalf("expected empty when disabled, got %q", got)
	}

	autoGenerateTests = true
	if got := maybeGenerateTest("foo.txt"); got != "" {
		t.Fatalf("expected empty for non-.go, got %q", got)
	}
	if got := maybeGenerateTest("foo_test.go"); got != "" {
		t.Fatalf("expected empty for _test.go, got %q", got)
	}

	// Generate for a real .go file.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/auto\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "auto.go")
	if err := os.WriteFile(src, []byte("package auto\n\nfunc Auto(x int) int { return x + 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	got := maybeGenerateTest("auto.go")
	if !strings.Contains(got, "auto-generate") {
		t.Fatalf("expected auto-generate note, got %q", got)
	}

	// Restore default.
	autoGenerateTests = false
}

func TestToolWriteAutoGenerate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/write\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	autoGenerateTests = true
	defer func() { autoGenerateTests = false }()

	out, err := toolWrite("write.go", "package write\n\nfunc Write(x int) int { return x }\n")
	if err != nil {
		t.Fatalf("toolWrite: %v", err)
	}
	if !strings.Contains(out, "auto-generate") {
		t.Fatalf("expected auto-generate note in output, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "write_test.go")); err != nil {
		t.Fatalf("expected write_test.go to be generated: %v", err)
	}
}

func TestToolQualityGate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/qg\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qg.go"), []byte("package qg\n\nfunc Answer() int { return 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qg_test.go"), []byte("package qg\n\nimport \"testing\"\n\nfunc TestAnswer(t *testing.T) {\n\tif Answer() != 42 {\n\t\tt.Fatal(\"expected 42\")\n\t}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	out, err := toolQualityGate(context.Background(), map[string]any{
		"steps":    "build,vet,test",
		"race":     "false",
		"json":     "true",
		"timeout":  "30s",
		"coverage": "0",
	})
	if err != nil {
		t.Fatalf("toolQualityGate: %v", err)
	}
	if !strings.Contains(out, "\"status\": \"PASS\"") {
		t.Fatalf("expected PASS status, got: %s", out)
	}
	if !strings.Contains(out, "\"name\": \"test\"") {
		t.Fatalf("expected test step in output, got: %s", out)
	}
}
