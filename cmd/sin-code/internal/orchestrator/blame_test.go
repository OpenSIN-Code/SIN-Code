// SPDX-License-Identifier: MIT
// Purpose: tests for blame.go — covers Diagnosis formatting, the git helper,
// and checkAt bisection logic. Git-dependent paths use the blameGitCmd hook
// or skip when git is unavailable.
package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ── Diagnosis ──────────────────────────────────────────────────────────

func TestDiagnosis_PreExisting(t *testing.T) {
	r := &BlameResult{
		Check: Check{Name: "build"},
		// Culprit is nil → pre-existing failure
	}
	got := r.Diagnosis()
	if !strings.Contains(got, "pre-existing") {
		t.Fatalf("expected 'pre-existing' in diagnosis, got: %s", got)
	}
	if !strings.Contains(got, "build") {
		t.Fatalf("expected check name in diagnosis, got: %s", got)
	}
	if !strings.Contains(got, "do not blame") {
		t.Fatalf("expected guidance in diagnosis, got: %s", got)
	}
}

func TestDiagnosis_CulpritFound(t *testing.T) {
	r := &BlameResult{
		Culprit: &EditRecord{
			Seq:     3,
			SHA:     "abc123def456",
			Path:    "main.go",
			Summary: "refactored auth flow",
		},
		Check:      Check{Name: "test"},
		PriorGreen: 2,
	}
	got := r.Diagnosis()
	if !strings.Contains(got, "CULPRIT") {
		t.Fatalf("expected 'CULPRIT' in diagnosis, got: %s", got)
	}
	if !strings.Contains(got, "edit #3") {
		t.Fatalf("expected edit number in diagnosis, got: %s", got)
	}
	if !strings.Contains(got, "main.go") {
		t.Fatalf("expected path in diagnosis, got: %s", got)
	}
	if !strings.Contains(got, "refactored auth flow") {
		t.Fatalf("expected summary in diagnosis, got: %s", got)
	}
	if !strings.Contains(got, "Edits 1..2 are verified green") {
		t.Fatalf("expected prior green count in diagnosis, got: %s", got)
	}
	if !strings.Contains(got, "Fix or replace ONLY edit #3") {
		t.Fatalf("expected fix guidance in diagnosis, got: %s", got)
	}
}

// ── git helper ─────────────────────────────────────────────────────────

func TestGit_Command(t *testing.T) {
	origCmd := blameGitCmd
	t.Cleanup(func() { blameGitCmd = origCmd })

	called := false
	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		called = true
		if dir != "/test/dir" {
			t.Errorf("dir = %q, want %q", dir, "/test/dir")
		}
		if len(args) != 2 || args[0] != "status" || args[1] != "--short" {
			t.Errorf("args = %v, want [status --short]", args)
		}
		return []byte("output"), nil
	}

	bl := &Blamer{}
	err := bl.git(context.Background(), "/test/dir", "status", "--short")
	if err != nil {
		t.Fatalf("git returned error: %v", err)
	}
	if !called {
		t.Fatal("blameGitCmd was not called")
	}
}

func TestGit_CommandError(t *testing.T) {
	origCmd := blameGitCmd
	t.Cleanup(func() { blameGitCmd = origCmd })

	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte("fatal: not a git repo"), errors.New("exit status 128")
	}

	bl := &Blamer{}
	err := bl.git(context.Background(), "/bad/dir", "checkout", "main")
	if err == nil {
		t.Fatal("expected error from git")
	}
	if !strings.Contains(err.Error(), "not a git repo") {
		t.Fatalf("expected output in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "checkout") {
		t.Fatalf("expected args in error, got: %v", err)
	}
}

// ── checkAt ────────────────────────────────────────────────────────────

func TestCheckAt_NoWorkdir(t *testing.T) {
	// Empty workdir → checkAt short-circuits to (true, nil)
	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	log := &EditLog{
		Edits: []EditRecord{{Seq: 1, SHA: "abc", Path: "x.go", Summary: "s"}},
	}
	ok, err := bl.checkAt(context.Background(), log, "abc", Check{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected true for empty workdir")
	}
}

func TestCheckAt_EmptySHA(t *testing.T) {
	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	log := &EditLog{
		Workdir: "/some/dir",
		Edits:   []EditRecord{{Seq: 1, SHA: "abc", Path: "x.go", Summary: "s"}},
	}
	// Empty SHA → short-circuit to (true, nil)
	ok, err := bl.checkAt(context.Background(), log, "", Check{Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected true for empty SHA")
	}
}

func TestCheckAt_WithGit(t *testing.T) {
	// Override blameGitCmd to track checkout/restore calls, and
	// verifierRunCheck to control the pass/fail result.
	origCmd := blameGitCmd
	origRunCheck := verifierRunCheck
	t.Cleanup(func() {
		blameGitCmd = origCmd
		verifierRunCheck = origRunCheck
	})

	var gitCalls []string
	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		gitCalls = append(gitCalls, strings.Join(args, " "))
		return []byte(""), nil
	}

	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true}
	}

	bl := &Blamer{Verifier: NewVerifier("/fake")}
	log := &EditLog{
		TaskID:  "task-1",
		Workdir: "/fake/repo",
		Edits: []EditRecord{
			{Seq: 1, SHA: "aaa11111aaaa", Path: "a.go", Summary: "first"},
			{Seq: 2, SHA: "bbb22222bbbb", Path: "b.go", Summary: "second"},
		},
	}

	ok, err := bl.checkAt(context.Background(), log, "aaa11111aaaa", Check{Kind: CheckBuild, Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected true (verifier passes)")
	}

	// Should have checkout'd the target SHA, then checkout'd back to tip
	if len(gitCalls) < 2 {
		t.Fatalf("expected at least 2 git calls, got %d: %v", len(gitCalls), gitCalls)
	}
	if !strings.Contains(gitCalls[0], "aaa11111") {
		t.Errorf("first call should checkout target SHA: %v", gitCalls)
	}
	// Last call should restore to tip (bbb22222)
	lastCall := gitCalls[len(gitCalls)-1]
	if !strings.Contains(lastCall, "bbb22222") {
		t.Errorf("last call should restore tip: %v", gitCalls)
	}
}

func TestCheckAt_GitCheckoutError(t *testing.T) {
	origCmd := blameGitCmd
	t.Cleanup(func() { blameGitCmd = origCmd })

	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "checkout" {
			return []byte("error: pathspec"), errors.New("exit 1")
		}
		return []byte(""), nil
	}

	bl := &Blamer{Verifier: NewVerifier("/fake")}
	log := &EditLog{
		TaskID:  "task-1",
		Workdir: "/fake/repo",
		Edits: []EditRecord{
			{Seq: 1, SHA: "aaa11111aaaa", Path: "a.go", Summary: "first"},
			{Seq: 2, SHA: "bbb22222bbbb", Path: "b.go", Summary: "second"},
		},
	}

	_, err := bl.checkAt(context.Background(), log, "aaa11111aaaa", Check{Kind: CheckBuild, Name: "test"})
	if err == nil {
		t.Fatal("expected error from checkout failure")
	}
	if !strings.Contains(err.Error(), "blame checkout") {
		t.Fatalf("expected 'blame checkout' in error, got: %v", err)
	}
}

// ── Blame bisection ────────────────────────────────────────────────────

func TestBlame_EmptyLog(t *testing.T) {
	bl := &Blamer{Verifier: NewVerifier(t.TempDir())}
	_, err := bl.Blame(context.Background(), &EditLog{}, Check{Name: "x"})
	if err == nil {
		t.Fatal("expected error for empty edit log")
	}
	if !strings.Contains(err.Error(), "empty edit log") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBlame_PreExistingFailure(t *testing.T) {
	// Base already fails → Culprit stays nil, no bisection needed.
	// We use a non-empty Workdir + mocked git + mocked verifier to
	// make the base check fail.
	origCmd := blameGitCmd
	origRunCheck := verifierRunCheck
	t.Cleanup(func() {
		blameGitCmd = origCmd
		verifierRunCheck = origRunCheck
	})

	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte(""), nil
	}
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: false}
	}

	bl := &Blamer{Verifier: NewVerifier("/fake")}
	log := &EditLog{
		TaskID:  "t1",
		Base:    "base-sha",
		Workdir: "/fake/repo",
		Edits: []EditRecord{
			{Seq: 1, SHA: "aaa11111aaaa", Path: "a.go", Summary: "first"},
			{Seq: 2, SHA: "bbb22222bbbb", Path: "b.go", Summary: "second"},
		},
	}

	res, err := bl.Blame(context.Background(), log, Check{Kind: CheckBuild, Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Culprit != nil {
		t.Fatalf("expected nil culprit for pre-existing failure, got %+v", res.Culprit)
	}
	if res.Bisections < 1 {
		t.Errorf("expected at least 1 bisection (base check), got %d", res.Bisections)
	}
}

func TestBlame_BisectionFindsCulprit(t *testing.T) {
	// Edits 1-2 pass, edit 3 fails. Bisection should find edit 3.
	origCmd := blameGitCmd
	origRunCheck := verifierRunCheck
	t.Cleanup(func() {
		blameGitCmd = origCmd
		verifierRunCheck = origRunCheck
	})

	// Track which SHA is currently checked out so verifierRunCheck can
	// return the right result.
	var currentSHA string
	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "checkout" {
			currentSHA = args[2]
		}
		return []byte(""), nil
	}

	edits := []EditRecord{
		{Seq: 1, SHA: "aaaa1111aaaa", Path: "a.go", Summary: "first"},
		{Seq: 2, SHA: "bbbb2222bbbb", Path: "b.go", Summary: "second"},
		{Seq: 3, SHA: "cccc3333cccc", Path: "c.go", Summary: "third"},
	}
	// Edits with Seq <= 2 pass; edit 3 fails.
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		for _, e := range edits {
			if strings.HasPrefix(currentSHA, e.SHA[:8]) {
				return CheckResult{Check: c, Passed: e.Seq <= 2}
			}
		}
		return CheckResult{Check: c, Passed: false}
	}

	bl := &Blamer{Verifier: NewVerifier("/fake")}
	log := &EditLog{
		TaskID:  "t1",
		Workdir: "/fake/repo",
		Edits:   edits,
	}

	res, err := bl.Blame(context.Background(), log, Check{Kind: CheckBuild, Name: "check"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Culprit == nil {
		t.Fatal("expected non-nil culprit")
	}
	if res.Culprit.Seq != 3 {
		t.Errorf("expected culprit seq 3, got %d", res.Culprit.Seq)
	}
	if res.PriorGreen != 2 {
		t.Errorf("expected priorGreen 2, got %d", res.PriorGreen)
	}
}

func TestBlame_BisectionFindsFirstEdit(t *testing.T) {
	// All edits fail → culprit is edit 1, priorGreen = 0.
	origCmd := blameGitCmd
	origRunCheck := verifierRunCheck
	t.Cleanup(func() {
		blameGitCmd = origCmd
		verifierRunCheck = origRunCheck
	})

	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte(""), nil
	}

	edits := []EditRecord{
		{Seq: 1, SHA: "aaaa1111aaaa", Path: "a.go", Summary: "first"},
		{Seq: 2, SHA: "bbbb2222bbbb", Path: "b.go", Summary: "second"},
		{Seq: 3, SHA: "cccc3333cccc", Path: "c.go", Summary: "third"},
	}
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: false} // all fail
	}

	bl := &Blamer{Verifier: NewVerifier("/fake")}
	log := &EditLog{
		TaskID:  "t1",
		Workdir: "/fake/repo",
		Edits:   edits,
	}

	res, err := bl.Blame(context.Background(), log, Check{Kind: CheckBuild, Name: "check"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Culprit == nil {
		t.Fatal("expected non-nil culprit")
	}
	if res.Culprit.Seq != 1 {
		t.Errorf("expected culprit seq 1, got %d", res.Culprit.Seq)
	}
	if res.PriorGreen != 0 {
		t.Errorf("expected priorGreen 0, got %d", res.PriorGreen)
	}
}

func TestBlame_AllPassBisectionFindsLast(t *testing.T) {
	// All edits pass → bisection narrows to last edit.
	origCmd := blameGitCmd
	origRunCheck := verifierRunCheck
	t.Cleanup(func() {
		blameGitCmd = origCmd
		verifierRunCheck = origRunCheck
	})

	blameGitCmd = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return []byte(""), nil
	}

	edits := []EditRecord{
		{Seq: 1, SHA: "aaaa1111aaaa", Path: "a.go", Summary: "first"},
		{Seq: 2, SHA: "bbbb2222bbbb", Path: "b.go", Summary: "second"},
	}
	verifierRunCheck = func(ctx context.Context, c Check, workdir string) CheckResult {
		return CheckResult{Check: c, Passed: true} // all pass
	}

	bl := &Blamer{Verifier: NewVerifier("/fake")}
	log := &EditLog{
		TaskID:  "t1",
		Workdir: "/fake/repo",
		Edits:   edits,
	}

	res, err := bl.Blame(context.Background(), log, Check{Kind: CheckBuild, Name: "check"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Culprit == nil {
		t.Fatal("expected non-nil culprit")
	}
	if res.Culprit.Seq != 2 {
		t.Errorf("expected culprit seq 2 (last), got %d", res.Culprit.Seq)
	}
}


