// SPDX-License-Identifier: MIT
package orchestrator

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestContractBlocksFrozenPath(t *testing.T) {
	c := &Contract{FrozenGlobs: []string{"go.sum", ".github/workflows/*"}}
	if v := c.CheckEdit("go.sum", []string{"tampered"}); len(v) == 0 {
		t.Fatal("expected frozen-path violation for go.sum")
	}
	if v := c.CheckEdit(filepath.Join(".github", "workflows", "ci.yml"), nil); len(v) == 0 {
		t.Fatal("expected frozen-path violation for workflow file")
	}
	if v := c.CheckEdit(filepath.Join("internal", "tui", "view.go"), nil); len(v) != 0 {
		t.Fatalf("unexpected violations for unrelated path: %v", v)
	}
}

func TestContractEnforcesScope(t *testing.T) {
	c := &Contract{AllowedGlobs: []string{"internal/tui/*"}}
	if v := c.CheckEdit("cmd/main.go", nil); len(v) == 0 {
		t.Fatal("expected out-of-scope violation")
	}
	if v := c.CheckEdit(filepath.Join("internal", "tui", "view.go"), nil); len(v) != 0 {
		t.Fatalf("in-scope path flagged: %v", v)
	}
	if v := c.CheckEdit(filepath.Join("internal", "tui", "widgets", "list.go"), nil); len(v) != 0 {
		t.Fatalf("nested in-scope path flagged: %v", v)
	}
}

func TestContractCatchesForbiddenContent(t *testing.T) {
	c := &Contract{ForbiddenPatterns: DefaultForbidden()}
	added := []string{`apiKey := "sk_live_abcdefghijklmnop1234"`}
	v := c.CheckEdit("internal/client.go", added)
	if len(v) == 0 {
		t.Fatal("expected forbidden-content violation for hardcoded secret")
	}
	if v[0].Kind != "forbidden-content" {
		t.Fatalf("wrong kind: %s", v[0].Kind)
	}
}

func TestContractCatchesDisabledTests(t *testing.T) {
	c := &Contract{ForbiddenPatterns: DefaultForbidden()}
	if v := c.CheckEdit("foo_test.go", []string{"\tt.Skip()"}); len(v) == 0 {
		t.Fatal("agent silently skipping a test must be a violation")
	}
}

func TestContractBlastRadius(t *testing.T) {
	c := &Contract{MaxFilesChanged: 5, MaxLinesChanged: 100}
	if v := c.CheckDiffStats(6, 50); len(v) != 1 {
		t.Fatalf("expected 1 blast-radius violation, got %d", len(v))
	}
	if v := c.CheckDiffStats(3, 50); len(v) != 0 {
		t.Fatalf("within-budget diff flagged: %v", v)
	}
}

func TestCompileContractFreezesLockfilesByDefault(t *testing.T) {
	task := &Task{ID: "t1", Title: "Fix nil pointer in TUI", Description: "crash on resize"}
	c := CompileContract(task)
	if v := c.CheckEdit("go.sum", []string{"x"}); len(v) == 0 {
		t.Fatal("lockfile must be frozen for a non-dependency task")
	}
	depTask := &Task{ID: "t2", Title: "Update dependencies", Description: "bump go.sum"}
	cd := CompileContract(depTask)
	if v := cd.CheckEdit("go.sum", []string{"x"}); len(v) != 0 {
		t.Fatal("lockfile must be editable when task is about dependencies")
	}
}

func TestViolationStringWithLine(t *testing.T) {
	v := Violation{Kind: "forbidden-content", Path: "x.go", Line: 42, Detail: "match"}
	if !strings.Contains(v.String(), "42") {
		t.Fatalf("violation string should include line, got %q", v.String())
	}
}

func TestAsChecksEmitsScope(t *testing.T) {
	c := &Contract{AllowedGlobs: []string{"internal/*"}}
	checks := c.AsChecks()
	if len(checks) != 1 || checks[0].Kind != CheckDiffScope {
		t.Fatalf("expected 1 diff-scope check, got %+v", checks)
	}
}

func TestAsChecksNoScopeEmitsNothing(t *testing.T) {
	c := &Contract{}
	checks := c.AsChecks()
	if len(checks) != 0 {
		t.Fatalf("expected no checks for empty contract, got %+v", checks)
	}
}
