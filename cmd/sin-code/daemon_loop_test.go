// SPDX-License-Identifier: MIT
// Purpose: tests for the loop-engineering daemon helpers — post-goal template
// rendering (loop-001), changed-file glob matching (loop-001), and the
// decomposition directive content (loop-005).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	got, err := renderTemplate(
		"Record this work:\n{{ .Summary }}\nturns={{ .Turns }}",
		map[string]any{"Summary": "added X", "Turns": 7, "SessionID": "s1"})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(got, "added X") || !strings.Contains(got, "turns=7") {
		t.Fatalf("template not rendered correctly: %q", got)
	}
}

func TestRenderTemplateInvalid(t *testing.T) {
	if _, err := renderTemplate("{{ .Unclosed", map[string]any{}); err == nil {
		t.Fatal("expected error for malformed template")
	}
}

func TestBuildDecompositionDirective(t *testing.T) {
	d := buildDecompositionDirective(4)
	for _, must := range []string{
		"spawn_subgoal", "go build", "go test -race", "go vet",
		"TODO/FIXME", "CHANGELOG", "stop-gate", "max depth 4",
	} {
		if !strings.Contains(d, must) {
			t.Errorf("directive missing %q", must)
		}
	}
}

func TestChangedFilesMatch(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@t.local")
	run("config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(dir, "first.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "first")
	// Second commit changes a .go file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "second")

	if !changedFilesMatch(dir, "*.go") {
		t.Error("expected *.go to match the last commit's main.go")
	}
	if changedFilesMatch(dir, "*.md") {
		t.Error("did not expect *.md to match")
	}
}

func TestChangedFilesMatchFailOpen(t *testing.T) {
	// A directory that is not a git repo => fail-open (returns true).
	if !changedFilesMatch(t.TempDir(), "*.go") {
		t.Error("non-git dir should fail-open to true")
	}
}

func TestTruncatePrompt(t *testing.T) {
	if got := truncatePrompt("hello world", 8); got != "hello..." {
		t.Fatalf("got %q", got)
	}
	if got := truncatePrompt("short", 80); got != "short" {
		t.Fatalf("got %q", got)
	}
}
