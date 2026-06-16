// SPDX-License-Identifier: MIT
// Purpose: tests for the loop-engineering extensions to goal contracts —
// post-completion goals (loop-001), the new-test-coverage gate (loop-002),
// scope-hint decomposition (loop-005), and doc/changelog freshness gates
// (loop-006). Predicate scripts are exercised against real temp git repos.
package goalcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitInit creates a throwaway git repo with one committed file so that
// `git diff HEAD` works, then returns the workspace path.
func gitInit(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(ws, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "seed")
	return ws
}

// runScript runs a predicate script via `sh -c` in dir and returns its exit
// success and combined output.
func runScript(t *testing.T, dir, script string) (bool, string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return err == nil, string(out)
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAutoDetectPostGoals(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "CHANGELOG.md", "# Changelog\n## [Unreleased]\n")
	write(t, ws, "MASTER_TODO.md", "- [ ] thing\n")
	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	goals := autoDetectPostGoals(ws)
	if len(goals) != 3 {
		t.Fatalf("expected 3 post-goals (changelog, master_todo, docs), got %d", len(goals))
	}
	// CHANGELOG and docs post-goals must be gated on *.go changes.
	gated := 0
	for _, g := range goals {
		if g.OnlyIfChanged == "*.go" {
			gated++
		}
		if g.PromptTemplate == "" || len(g.Criteria) == 0 {
			t.Fatalf("post-goal missing template/criteria: %+v", g)
		}
	}
	if gated != 2 {
		t.Fatalf("expected 2 *.go-gated post-goals, got %d", gated)
	}
}

func TestAutoDetectPostGoalsEmptyRepo(t *testing.T) {
	if g := autoDetectPostGoals(t.TempDir()); len(g) != 0 {
		t.Fatalf("expected no post-goals in bare repo, got %d", len(g))
	}
}

func TestAutoDetectScopeHints(t *testing.T) {
	if h := autoDetectScopeHints("refactor the entire auth system"); len(h) != 1 {
		t.Fatalf("large-scope prompt should yield 1 hint, got %d", len(h))
	}
	if h := autoDetectScopeHints("fix a typo in the README"); len(h) != 0 {
		t.Fatalf("small prompt should yield no hint, got %d", len(h))
	}
}

func TestResolveInjectsLoopCriteria(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "go.mod", "module x\n")
	write(t, ws, "CHANGELOG.md", "# Changelog\n")
	c, err := Resolve(ResolveOptions{Workspace: ws, AutoDetect: true, Prompt: "add a small flag"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCheckNamed(c.DeterministicChecks, "new-test-coverage") {
		t.Fatal("expected new-test-coverage check")
	}
	if !hasCheckNamed(c.DeterministicChecks, "changelog-updated") {
		t.Fatal("expected changelog-updated check")
	}
	// Test + 2 doc/changelog semantic criteria must be present.
	if len(c.SemanticCriteria) < 3 {
		t.Fatalf("expected >=3 semantic criteria, got %d: %v", len(c.SemanticCriteria), c.SemanticCriteria)
	}
}

func TestResolveOptOutToggles(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "go.mod", "module x\n")
	write(t, ws, "CHANGELOG.md", "# Changelog\n")
	c, err := Resolve(ResolveOptions{
		Workspace: ws, AutoDetect: true,
		NoTestCriterion: true, NoDocCriterion: true, NoPostGoals: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasCheckNamed(c.DeterministicChecks, "new-test-coverage") {
		t.Fatal("new-test-coverage should be disabled by NoTestCriterion")
	}
	if hasCheckNamed(c.DeterministicChecks, "changelog-updated") {
		t.Fatal("changelog-updated should be disabled by NoDocCriterion")
	}
	if len(c.PostCompletionGoals) != 0 {
		t.Fatal("post-goals should be disabled by NoPostGoals")
	}
}

func TestNewTestCoverageScript(t *testing.T) {
	// Production .go file with no test in the package -> fail.
	ws := gitInit(t)
	write(t, ws, "pkg/foo.go", "package pkg\nfunc Foo() int { return 1 }\n")
	if ok, out := runScript(t, ws, newTestCoverageScript); ok {
		t.Fatalf("expected failure for untested package, output: %s", out)
	}
	// Add a test file in the same package -> pass.
	write(t, ws, "pkg/foo_test.go", "package pkg\nimport \"testing\"\nfunc TestFoo(t *testing.T){ if Foo()!=1 {t.Fatal(\"x\")} }\n")
	if ok, out := runScript(t, ws, newTestCoverageScript); !ok {
		t.Fatalf("expected pass once test exists, output: %s", out)
	}
}

func TestNewTestCoverageScriptOnlyTestChanged(t *testing.T) {
	ws := gitInit(t)
	write(t, ws, "pkg/foo_test.go", "package pkg\n")
	if ok, _ := runScript(t, ws, newTestCoverageScript); !ok {
		t.Fatal("changing only test files must pass")
	}
}

func TestChangelogUpdatedScript(t *testing.T) {
	ws := gitInit(t)
	write(t, ws, "CHANGELOG.md", "# Changelog\n")
	// commit the changelog so it is tracked but unchanged
	commit(t, ws)
	write(t, ws, "main.go", "package main\nfunc main(){}\n")
	if ok, out := runScript(t, ws, changelogUpdatedScript); ok {
		t.Fatalf("expected failure when CHANGELOG not updated, output: %s", out)
	}
	write(t, ws, "CHANGELOG.md", "# Changelog\n- did a thing\n")
	if ok, out := runScript(t, ws, changelogUpdatedScript); !ok {
		t.Fatalf("expected pass once CHANGELOG touched, output: %s", out)
	}
}

func TestDocMdFreshnessScript(t *testing.T) {
	ws := gitInit(t)
	write(t, ws, "pkg/foo.doc.md", "# pkg docs\n")
	commit(t, ws)
	write(t, ws, "pkg/foo.go", "package pkg\nfunc Foo(){}\n")
	if ok, out := runScript(t, ws, docMdFreshnessScript); ok {
		t.Fatalf("expected failure for stale doc.md, output: %s", out)
	}
	write(t, ws, "pkg/foo.doc.md", "# pkg docs\nupdated\n")
	if ok, out := runScript(t, ws, docMdFreshnessScript); !ok {
		t.Fatalf("expected pass once doc.md updated, output: %s", out)
	}
}

func commit(t *testing.T, ws string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "wip"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}
