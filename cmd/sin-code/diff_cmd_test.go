// SPDX-License-Identifier: MIT
// Purpose: tests for `sin-code diff` — git diff with complexity + sin-debt overlay.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/complexity"
)

// resetDiffFlags resets the package-level diff flag variables so tests
// don't leak state into each other.
func resetDiffFlags() {
	diffCached = false
	diffLast = false
	diffStat = false
	diffJSON = false
}

// gitInitRepo creates a temp directory, initialises a git repo, and
// configures a test user. Returns the directory path.
func gitInitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test User")
	gitCmd(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

// gitCmd runs a git command in dir and fails the test on error.
func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// gitCommitAll stages all changes and commits.
func gitCommitAll(t *testing.T, dir string, msg string) {
	t.Helper()
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", msg)
}

// chdirTemp changes to dir and returns a restore function.
func chdirTemp(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chdir(orig) }
}

// ── unit tests (no git needed) ────────────────────────────────────────

func TestParseNumstatCount(t *testing.T) {
	if n := parseNumstatCount("42"); n != 42 {
		t.Fatalf("expected 42, got %d", n)
	}
	if n := parseNumstatCount("-"); n != 0 {
		t.Fatalf("expected 0 for -, got %d", n)
	}
	if n := parseNumstatCount(""); n != 0 {
		t.Fatalf("expected 0 for empty, got %d", n)
	}
	if n := parseNumstatCount("abc"); n != 0 {
		t.Fatalf("expected 0 for non-numeric, got %d", n)
	}
}

func TestParseNumstat(t *testing.T) {
	input := "3\t2\tpath/to/file.go\n0\t5\tother.go\n-\t-\tbinary.png\n"
	files := parseNumstat(input)
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	if files[0].Path != "path/to/file.go" || files[0].LinesAdded != 3 || files[0].LinesRemoved != 2 {
		t.Fatalf("unexpected first file: %+v", files[0])
	}
	if files[1].Path != "other.go" || files[1].LinesAdded != 0 || files[1].LinesRemoved != 5 {
		t.Fatalf("unexpected second file: %+v", files[1])
	}
	if files[2].Path != "binary.png" || files[2].LinesAdded != 0 || files[2].LinesRemoved != 0 {
		t.Fatalf("unexpected binary file: %+v", files[2])
	}
}

func TestParseNumstatEmpty(t *testing.T) {
	files := parseNumstat("")
	if len(files) != 0 {
		t.Fatalf("expected 0 files for empty input, got %d", len(files))
	}
}

func TestSplitDiffByFile(t *testing.T) {
	input := "diff --git a/file1.go b/file1.go\n--- a/file1.go\n+++ b/file1.go\n@@ -1 +1 @@\n-old\n+new\ndiff --git a/file2.go b/file2.go\n--- a/file2.go\n+++ b/file2.go\n@@ -1 +1 @@\n-old2\n+new2\n"
	result := splitDiffByFile(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result))
	}
	if _, ok := result["file1.go"]; !ok {
		t.Fatal("expected file1.go in result")
	}
	if _, ok := result["file2.go"]; !ok {
		t.Fatal("expected file2.go in result")
	}
}

func TestExtractDiffPath(t *testing.T) {
	tests := []struct {
		section string
		want    string
	}{
		{"a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n", "file.go"},
		{"a/deep/path.go b/deep/path.go\n--- a/deep/path.go\n+++ b/deep/path.go\n", "deep/path.go"},
	}
	for _, tt := range tests {
		got := extractDiffPath(tt.section)
		if got != tt.want {
			t.Fatalf("extractDiffPath(%q) = %q, want %q", tt.section, got, tt.want)
		}
	}
}

func TestShouldAnnotateDebt(t *testing.T) {
	if !shouldAnnotateDebt("+// sin-debt: legacy, upgrade: refactor") {
		t.Fatal("expected true for sin-debt line")
	}
	if !shouldAnnotateDebt(" // sin-debt: shrink") {
		t.Fatal("expected true for context line with sin-debt")
	}
	if shouldAnnotateDebt("+func main() {}") {
		t.Fatal("expected false for non-debt line")
	}
}

func TestUniqueTags(t *testing.T) {
	findings := []complexity.Finding{
		{Tag: "yagni"},
		{Tag: "yagni"},
		{Tag: "stdlib"},
		{Tag: "delete"},
	}
	tags := uniqueTags(findings)
	if len(tags) != 3 {
		t.Fatalf("expected 3 unique tags, got %d: %v", len(tags), tags)
	}
}

func TestBuildDiffArgs(t *testing.T) {
	resetDiffFlags()
	if args := buildDiffArgs(); len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}

	resetDiffFlags()
	diffCached = true
	if args := buildDiffArgs(); len(args) != 1 || args[0] != "--cached" {
		t.Fatalf("expected --cached, got %v", args)
	}

	resetDiffFlags()
	diffLast = true
	if args := buildDiffArgs(); len(args) != 1 || args[0] != "HEAD~1..HEAD" {
		t.Fatalf("expected HEAD~1..HEAD, got %v", args)
	}
}

func TestInGitRepo(t *testing.T) {
	dir := t.TempDir()
	if inGitRepo(dir) {
		t.Fatal("expected false for non-git directory")
	}

	repo := gitInitRepo(t)
	if !inGitRepo(repo) {
		t.Fatal("expected true for git repo")
	}
}

// ── integration tests (git repo required) ─────────────────────────────

func TestDiffCommandNotGitRepo(t *testing.T) {
	dir := t.TempDir()
	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestDiffCommandNoChanges(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "README.md"), "hello\n")
	gitCommitAll(t, dir, "initial")

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{})
	out := captureOutput(t, cmd)
	if !strings.Contains(out, "No changes") {
		t.Fatalf("expected 'No changes', got: %s", out)
	}
}

func TestDiffCommandWithChanges(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n")
	gitCommitAll(t, dir, "initial")

	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n\nfunc Foo() {}\n")

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{})
	out := captureOutput(t, cmd)
	if !strings.Contains(out, "demo.go") {
		t.Fatalf("expected demo.go in output, got: %s", out)
	}
	if !strings.Contains(out, "Summary") {
		t.Fatalf("expected Summary in output, got: %s", out)
	}
}

func TestDiffCommandJSON(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n")
	gitCommitAll(t, dir, "initial")

	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n\nfunc Foo() {}\n")

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	diffJSON = true
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--json"})
	out := captureOutput(t, cmd)

	var result diffResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}
	if result.Files[0].Path != "demo.go" {
		t.Fatalf("expected demo.go, got %s", result.Files[0].Path)
	}
	if result.Files[0].LinesAdded != 2 {
		t.Fatalf("expected 2 lines added, got %d", result.Files[0].LinesAdded)
	}
}

func TestDiffCommandStat(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n")
	gitCommitAll(t, dir, "initial")

	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n\nfunc Foo() {}\n")

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	diffStat = true
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--stat"})
	out := captureOutput(t, cmd)
	if !strings.Contains(out, "Summary") {
		t.Fatalf("expected Summary in stat output, got: %s", out)
	}
	if strings.Contains(out, "diff --git") {
		t.Fatalf("stat mode should not show full diff, got: %s", out)
	}
}

func TestDiffCommandWithSinDebt(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n")
	gitCommitAll(t, dir, "initial")

	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n\n// sin-debt: legacy, upgrade: refactor\nfunc Foo() {}\n")

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{})
	out := captureOutput(t, cmd)
	if !strings.Contains(out, "⚡") {
		t.Fatalf("expected ⚡ annotation for sin-debt marker, got: %s", out)
	}
	if !strings.Contains(out, "sin-debt:") {
		t.Fatalf("expected sin-debt content in output, got: %s", out)
	}
}

func TestDiffCommandWithComplexity(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n")
	gitCommitAll(t, dir, "initial")

	mustWrite(t, filepath.Join(dir, "demo.go"), `package demo

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
`)

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{})
	out := captureOutput(t, cmd)
	if !strings.Contains(out, "demo.go") {
		t.Fatalf("expected demo.go in output, got: %s", out)
	}
}

func TestDiffCommandCached(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n")
	gitCommitAll(t, dir, "initial")

	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n\nfunc Staged() {}\n")
	gitCmd(t, dir, "add", "demo.go")

	mustWrite(t, filepath.Join(dir, "other.go"), "package demo\n\nfunc Unstaged() {}\n")

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	diffCached = true
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--cached"})
	out := captureOutput(t, cmd)
	if !strings.Contains(out, "demo.go") {
		t.Fatalf("expected demo.go in cached diff, got: %s", out)
	}
	if strings.Contains(out, "other.go") {
		t.Fatalf("cached diff should not show unstaged other.go, got: %s", out)
	}
}

func TestDiffCommandLast(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n")
	gitCommitAll(t, dir, "first commit")

	mustWrite(t, filepath.Join(dir, "demo.go"), "package demo\n\nfunc Bar() {}\n")
	gitCommitAll(t, dir, "second commit")

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	diffLast = true
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--last"})
	out := captureOutput(t, cmd)
	if !strings.Contains(out, "demo.go") {
		t.Fatalf("expected demo.go in --last output, got: %s", out)
	}
	if !strings.Contains(out, "Bar") {
		t.Fatalf("expected Bar() in --last diff, got: %s", out)
	}
}

func TestDiffCommandJSONNoChanges(t *testing.T) {
	dir := gitInitRepo(t)
	mustWrite(t, filepath.Join(dir, "README.md"), "hello\n")
	gitCommitAll(t, dir, "initial")

	restore := chdirTemp(t, dir)
	defer restore()

	resetDiffFlags()
	diffJSON = true
	cmd := NewDiffCmd()
	cmd.SetArgs([]string{"--json"})
	out := captureOutput(t, cmd)

	var result diffResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON for no changes: %v\n%s", err, out)
	}
	if len(result.Files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(result.Files))
	}
}
