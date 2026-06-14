// SPDX-License-Identifier: MIT
// Purpose: coverage tests for edit.go CLI and error branches (st-cov2).
package internal

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureEditCmd(t *testing.T, args []string, setup func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	setup()
	err := EditCmd.RunE(EditCmd, args)
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("EditCmd failed: %v", err)
	}
	return string(out)
}

func TestEditCmd_RunE_Text(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() {}\n"), 0644)

	oldAnchor := editAnchor
	oldNewText := editNewText
	oldFormat := editFormat
	oldGetwd := editGetwd
	defer func() { editAnchor = oldAnchor; editNewText = oldNewText; editFormat = oldFormat; editGetwd = oldGetwd }()

	editGetwd = func() (string, error) { return dir, nil }
	editAnchor = "3:" + LineHash("func Hello() {}")
	editNewText = "func World() {}"
	editFormat = "text"

	out := captureEditCmd(t, []string{"f.go"}, func() {})
	if !strings.Contains(out, "edited") {
		t.Errorf("expected edited output, got %q", out)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "func World()") {
		t.Errorf("expected edit applied, got %q", data)
	}
}

func TestEditCmd_RunE_JSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() {}\n"), 0644)

	oldAnchor := editAnchor
	oldNewText := editNewText
	oldFormat := editFormat
	oldGetwd := editGetwd
	defer func() { editAnchor = oldAnchor; editNewText = oldNewText; editFormat = oldFormat; editGetwd = oldGetwd }()

	editGetwd = func() (string, error) { return dir, nil }
	editAnchor = "3:" + LineHash("func Hello() {}")
	editNewText = "func World() {}"
	editFormat = "json"

	out := captureEditCmd(t, []string{"f.go"}, func() {})
	if !strings.Contains(out, "\"path\"") {
		t.Errorf("expected JSON output, got %q", out)
	}
}

func TestEditCmd_RunE_DryRun(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() {}\n"), 0644)

	oldAnchor := editAnchor
	oldNewText := editNewText
	oldDryRun := editDryRun
	oldGetwd := editGetwd
	defer func() { editAnchor = oldAnchor; editNewText = oldNewText; editDryRun = oldDryRun; editGetwd = oldGetwd }()

	editGetwd = func() (string, error) { return dir, nil }
	editAnchor = "3:" + LineHash("func Hello() {}")
	editNewText = "func World() {}"
	editDryRun = true

	out := captureEditCmd(t, []string{"f.go"}, func() {})
	if !strings.Contains(out, "---") {
		t.Errorf("expected diff output, got %q", out)
	}
	data, _ := os.ReadFile(p)
	if strings.Contains(string(data), "func World()") {
		t.Error("dry run must not modify the file")
	}
}

func TestEditCmd_RunE_InvalidPath(t *testing.T) {
	if err := EditCmd.RunE(EditCmd, []string{"\x00invalid"}); err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestApplyAnchorEdit_ParseAnchorError(t *testing.T) {
	_, err := applyAnchorEdit([]string{"a"}, editRequest{Anchor: "bad"}, &editResult{})
	if err == nil {
		t.Fatal("expected error for invalid anchor")
	}
}

func TestApplyAnchorEdit_ResolveError(t *testing.T) {
	_, err := applyAnchorEdit([]string{"a"}, editRequest{Anchor: "1:deadbeef"}, &editResult{})
	if err == nil {
		t.Fatal("expected error for unresolvable anchor")
	}
}

func TestApplyAnchorEdit_EndAnchorParseError(t *testing.T) {
	lines := []string{"a", "b", "c"}
	_, err := applyAnchorEdit(lines, editRequest{
		Anchor:    "1:" + LineHash("a"),
		EndAnchor: "bad",
	}, &editResult{})
	if err == nil {
		t.Fatal("expected error for invalid end anchor")
	}
}

func TestApplyAnchorEdit_EndAnchorResolveError(t *testing.T) {
	lines := []string{"a", "b", "c"}
	_, err := applyAnchorEdit(lines, editRequest{
		Anchor:    "1:" + LineHash("a"),
		EndAnchor: "2:deadbeef",
	}, &editResult{})
	if err == nil {
		t.Fatal("expected error for unresolvable end anchor")
	}
}

func TestApplyAnchorEdit_InvalidInsert_Cli(t *testing.T) {
	lines := []string{"a", "b", "c"}
	_, err := applyAnchorEdit(lines, editRequest{
		Anchor:   "1:" + LineHash("a"),
		NewText:  "x",
		Insert:   "middle",
	}, &editResult{})
	if err == nil {
		t.Fatal("expected error for invalid insert")
	}
}

func TestApplyStringEdit_OldStringNotFound(t *testing.T) {
	_, err := applyStringEdit([]string{"a"}, "abc", editRequest{OldString: "x"}, &editResult{}, new(bool))
	if err == nil {
		t.Fatal("expected error when old string not found")
	}
}

func TestApplyStringEdit_ReplaceAll(t *testing.T) {
	res := &editResult{}
	updated, err := applyStringEdit([]string{"x", "x"}, "x\nx\n", editRequest{OldString: "x", NewString: "y", ReplaceAll: true}, res, new(bool))
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 2 || updated[0] != "y" || updated[1] != "y" {
		t.Fatalf("expected all replaced, got %v", updated)
	}
}

func TestApplySymbolEdit_NoEngine(t *testing.T) {
	_, err := applySymbolEdit([]string{"x"}, "x.txt", "x", editRequest{Symbol: "foo"}, &editResult{})
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestApplySymbolEdit_NotFound_Cli(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\n"), 0644)
	data, _ := os.ReadFile(p)
	_, err := applySymbolEdit([]string{"package main", "func Hello() {}"}, p, string(data), editRequest{Symbol: "Missing"}, &editResult{})
	if err == nil {
		t.Fatal("expected error for missing symbol")
	}
}

func TestApplySymbolEdit_Ambiguous_Cli(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\nfunc Hello() {}\n"), 0644)
	data, _ := os.ReadFile(p)
	_, err := applySymbolEdit([]string{"package main", "func Hello() {}", "func Hello() {}"}, p, string(data), editRequest{Symbol: "Hello"}, &editResult{})
	if err == nil {
		t.Fatal("expected error for ambiguous symbol")
	}
}

func TestApplySymbolEdit_InvalidInsert_Cli(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\n"), 0644)
	data, _ := os.ReadFile(p)
	_, err := applySymbolEdit([]string{"package main", "func Hello() {}"}, p, string(data), editRequest{Symbol: "Hello", Insert: "middle"}, &editResult{})
	if err == nil {
		t.Fatal("expected error for invalid insert")
	}
}

func TestApplySymbolEdit_InvalidRange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\n"), 0644)
	data, _ := os.ReadFile(p)
	// Force an invalid range by overriding the parsed outline via a malformed file? Instead, use a small
	// trick: the symbol's EndLine will be within the line slice, but the function checks startIdx<0 etc.
	// We can pass a single-line content so EndLine-1 > len(lines)-1.
	_, err := applySymbolEdit([]string{"package main"}, p, string(data), editRequest{Symbol: "Hello"}, &editResult{})
	if err == nil {
		t.Fatal("expected error for invalid range")
	}
}

func TestApplyEdit_FileNotFound(t *testing.T) {
	_, err := applyEdit("/nonexistent/path.go", editRequest{Anchor: "1:deadbeef"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestApplyEdit_SymbolError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	os.WriteFile(p, []byte("hello\n"), 0644)
	_, err := applyEdit(p, editRequest{Symbol: "Foo", NewText: "bar"})
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestApplyEdit_InsertAfterMissingNewText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\n"), 0644)
	_, err := applyEdit(p, editRequest{
		Anchor: "2:" + LineHash("func Hello() {}"),
		Insert: "after",
	})
	if err == nil {
		t.Fatal("expected error for insert without new-text")
	}
}

func TestApplyEdit_SymbolInsertMissingNewText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\n"), 0644)
	_, err := applyEdit(p, editRequest{
		Symbol: "Hello",
		Insert: "before",
	})
	if err == nil {
		t.Fatal("expected error for symbol insert without new-text")
	}
}

func TestApplyEdit_SymbolReplaceMissingNewText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\n"), 0644)
	_, err := applyEdit(p, editRequest{Symbol: "Hello"})
	if err == nil {
		t.Fatal("expected error for symbol replace without new-text")
	}
}

func TestUnifiedDiff_NoChange(t *testing.T) {
	before := []string{"a", "b", "c"}
	after := []string{"a", "b", "c"}
	if diff := unifiedDiff("f.txt", before, after); diff != "" {
		t.Fatalf("expected empty diff, got %q", diff)
	}
}
