// SPDX-License-Identifier: MIT
// Purpose: tests for the always-on SinCode Loop System baseline — env toggle,
// additive/deduped merge into Resolve, the prompt preamble, and the actual
// shell predicates (fail-open outside git, fail-closed on a real code-only
// diff). The predicate tests build a throwaway git repo so the scripts run for
// real, not mocked.
package goalcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineEnabledDefaultsOn(t *testing.T) {
	t.Setenv(baselineEnvVar, "")
	if !BaselineEnabled(false) {
		t.Fatal("baseline must default ON when env unset and flag false")
	}
	if BaselineEnabled(true) {
		t.Fatal("--no-baseline (disable=true) must turn it off")
	}
}

func TestBaselineEnabledEnvToggle(t *testing.T) {
	for _, v := range []string{"off", "0", "false", "no", "OFF", "Disabled"} {
		t.Setenv(baselineEnvVar, v)
		if BaselineEnabled(false) {
			t.Fatalf("SIN_BASELINE=%q should disable baseline", v)
		}
	}
	for _, v := range []string{"on", "1", "true", "", "yes", "anything"} {
		t.Setenv(baselineEnvVar, v)
		if !BaselineEnabled(false) {
			t.Fatalf("SIN_BASELINE=%q should leave baseline ON", v)
		}
	}
}

func TestBaselineSemanticCriteriaNonEmpty(t *testing.T) {
	crit := BaselineSemanticCriteria()
	if len(crit) < 4 {
		t.Fatalf("expected a rich baseline rubric, got %d criteria", len(crit))
	}
	joined := strings.ToLower(strings.Join(crit, " "))
	for _, want := range []string{"test", "debug", "documentation", "changelog", "readme", ".doc.md"} {
		if !strings.Contains(joined, want) {
			t.Errorf("baseline rubric should mention %q", want)
		}
	}
}

func TestBaselineChecksOnlyForGoRepo(t *testing.T) {
	if got := BaselineChecks(t.TempDir()); got != nil {
		t.Fatalf("no go.mod -> no baseline checks, got %+v", got)
	}
	ws := t.TempDir()
	writeFile(t, ws, "go.mod", "module x\n")
	checks := BaselineChecks(ws)
	for _, name := range []string{"baseline-tests-changed-with-code", "baseline-changelog-updated", "baseline-codoc-present"} {
		if !hasCheckNamed(checks, name) {
			t.Errorf("expected baseline check %q, got %+v", name, checks)
		}
	}
}

func TestResolveIncludesBaselineAdditively(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "go.mod", "module x\n")
	c, err := Resolve(ResolveOptions{
		Workspace:       ws,
		Criteria:        []string{"custom criterion"},
		AutoDetect:      true,
		IncludeBaseline: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Custom criterion preserved AND baseline rubric appended.
	if !hasCriterion(c.SemanticCriteria, "custom criterion") {
		t.Fatal("explicit criterion dropped after baseline merge")
	}
	base := BaselineSemanticCriteria()
	if !hasCriterion(c.SemanticCriteria, base[0]) {
		t.Fatal("baseline semantic criteria not merged in")
	}
	if !hasCheckNamed(c.DeterministicChecks, "baseline-tests-changed-with-code") {
		t.Fatal("baseline deterministic checks not merged in")
	}
	// Auto-detect's go build still present (additive, not replaced).
	if !hasCheckNamed(c.DeterministicChecks, "go build") {
		t.Fatal("auto-detect checks lost after baseline merge")
	}
}

func TestResolveBaselineIsDeduped(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "go.mod", "module x\n")
	// Resolving twice-worth of baseline (also pass one of its criteria inline)
	// must not duplicate entries.
	base := BaselineSemanticCriteria()
	c, err := Resolve(ResolveOptions{
		Workspace:       ws,
		Criteria:        []string{base[0]},
		IncludeBaseline: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, cr := range c.SemanticCriteria {
		if cr == base[0] {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("baseline criterion duplicated %d times", count)
	}
}

func TestResolveWithoutBaselineUnchanged(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "go.mod", "module x\n")
	c, err := Resolve(ResolveOptions{Workspace: ws, AutoDetect: true, IncludeBaseline: false})
	if err != nil {
		t.Fatal(err)
	}
	if hasCheckNamed(c.DeterministicChecks, "baseline-tests-changed-with-code") {
		t.Fatal("baseline must not leak in when IncludeBaseline=false")
	}
	if len(c.SemanticCriteria) != 0 {
		t.Fatalf("no semantic criteria expected, got %v", c.SemanticCriteria)
	}
}

func TestPreambleRendersCriteria(t *testing.T) {
	if Preamble(GoalContract{}) != "" {
		t.Fatal("empty contract must yield empty preamble")
	}
	p := Preamble(GoalContract{SemanticCriteria: []string{"do tests", "update docs"}})
	if !strings.Contains(p, "DEFINITION OF DONE") {
		t.Fatal("preamble missing header")
	}
	if !strings.Contains(p, "do tests") || !strings.Contains(p, "update docs") {
		t.Fatal("preamble missing criteria")
	}
}

// ── predicate script behavior (run for real in a throwaway git repo) ─────────

func TestBaselinePredicatesFailOpenOutsideGit(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "go.mod", "module x\n")
	writeFile(t, ws, "main.go", "package main\nfunc main(){}\n")
	// No git repo here: every baseline predicate must exit 0 (cannot judge).
	for _, c := range BaselineChecks(ws) {
		if code := runPredicate(t, ws, c.Cmd); code != 0 {
			t.Errorf("%s should fail-open outside git, exit=%d", c.Name, code)
		}
	}
}

func TestBaselineTestsPredicateFailsOnCodeOnlyDiff(t *testing.T) {
	ws := newGitRepo(t)
	// Commit a baseline so HEAD exists, then add code with no test.
	writeFile(t, ws, "go.mod", "module x\n")
	gitCommit(t, ws, "init")
	writeFile(t, ws, "feature.go", "package x\nfunc F() int { return 1 }\n")
	gitAdd(t, ws)

	checks := map[string][]string{}
	for _, c := range BaselineChecks(ws) {
		checks[c.Name] = c.Cmd
	}
	if code := runPredicate(t, ws, checks["baseline-tests-changed-with-code"]); code == 0 {
		t.Fatal("code-only diff must fail the tests predicate")
	}
	// Adding a test file flips it green.
	writeFile(t, ws, "feature_test.go", "package x\nimport \"testing\"\nfunc TestF(t *testing.T){ if F()!=1 {t.Fail()} }\n")
	gitAdd(t, ws)
	if code := runPredicate(t, ws, checks["baseline-tests-changed-with-code"]); code != 0 {
		t.Fatalf("test added alongside code should pass, exit=%d", code)
	}
}

func TestBaselineChangelogAndCoDocPredicates(t *testing.T) {
	ws := newGitRepo(t)
	writeFile(t, ws, "go.mod", "module x\n")
	writeFile(t, ws, "CHANGELOG.md", "# Changelog\n")
	gitCommit(t, ws, "init")
	// New code in a package with neither CHANGELOG touch nor CoDoc.
	writeFile(t, ws, "pkg/thing.go", "package pkg\nfunc T(){}\n")
	gitAdd(t, ws)

	checks := map[string][]string{}
	for _, c := range BaselineChecks(ws) {
		checks[c.Name] = c.Cmd
	}
	if code := runPredicate(t, ws, checks["baseline-changelog-updated"]); code == 0 {
		t.Fatal("untouched CHANGELOG with code change must fail")
	}
	if code := runPredicate(t, ws, checks["baseline-codoc-present"]); code == 0 {
		t.Fatal("package without .doc.md must fail the CoDoc predicate")
	}
	// Touch CHANGELOG and add the CoDoc -> both pass.
	writeFile(t, ws, "CHANGELOG.md", "# Changelog\n- did a thing\n")
	writeFile(t, ws, "pkg/pkg.doc.md", "# pkg\n")
	gitAdd(t, ws)
	if code := runPredicate(t, ws, checks["baseline-changelog-updated"]); code != 0 {
		t.Fatalf("touched CHANGELOG should pass, exit=%d", code)
	}
	if code := runPredicate(t, ws, checks["baseline-codoc-present"]); code != 0 {
		t.Fatalf("CoDoc present should pass, exit=%d", code)
	}
}

func TestBaselinePredicatesPassOnDocsOnlyDiff(t *testing.T) {
	ws := newGitRepo(t)
	writeFile(t, ws, "go.mod", "module x\n")
	gitCommit(t, ws, "init")
	// Only docs changed: no production .go file -> every predicate passes.
	writeFile(t, ws, "README.md", "# hello\n")
	gitAdd(t, ws)
	for _, c := range BaselineChecks(ws) {
		if code := runPredicate(t, ws, c.Cmd); code != 0 {
			t.Errorf("%s should pass on docs-only diff, exit=%d", c.Name, code)
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runPredicate(t *testing.T, dir string, cmd []string) int {
	t.Helper()
	if len(cmd) == 0 {
		t.Fatal("empty predicate command")
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		t.Logf("predicate output: %s", strings.TrimSpace(string(out)))
		return ee.ExitCode()
	}
	t.Fatalf("predicate run error: %v", err)
	return -1
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ws := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t.test"},
		{"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		c := exec.Command("git", args...)
		c.Dir = ws
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return ws
}

func gitAdd(t *testing.T, ws string) {
	t.Helper()
	c := exec.Command("git", "add", "-A")
	c.Dir = ws
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
}

func gitCommit(t *testing.T, ws, msg string) {
	t.Helper()
	gitAdd(t, ws)
	c := exec.Command("git", "commit", "-q", "-m", msg)
	c.Dir = ws
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
}
