// SPDX-License-Identifier: MIT
// Purpose: tests for autonomous commit/push behaviour (loop-007) — clean-tree
// no-op, dirty-tree commit, and the main-branch push refusal guard.
package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "feature/x"},
		{"config", "user.email", "test@test.local"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func TestAutoCommitCleanTreeNoOp(t *testing.T) {
	dir := initRepo(t)
	// Empty repo, clean tree => no error, no commit created.
	if err := AutoCommit(context.Background(), CommitOptions{Workspace: dir}); err != nil {
		t.Fatalf("clean tree should be a no-op, got %v", err)
	}
	// No commits should exist.
	cmd := exec.Command("git", "rev-list", "--count", "--all")
	cmd.Dir = dir
	out, _ := cmd.CombinedOutput()
	if strings.TrimSpace(string(out)) != "0" {
		t.Fatalf("expected 0 commits on clean tree, got %q", out)
	}
}

func TestAutoCommitDirtyTreeCommits(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := AutoCommit(context.Background(), CommitOptions{
		Workspace: dir,
		Message:   "feat(agent): complete goal #42 in 7 turns\n\nbody",
	})
	if err != nil {
		t.Fatalf("dirty tree commit failed: %v", err)
	}
	log := gitOut(t, dir, "log", "--oneline")
	if !strings.Contains(log, "complete goal #42") {
		t.Fatalf("commit message not found in log: %q", log)
	}
	// Tree should now be clean.
	st := gitOut(t, dir, "status", "--porcelain")
	if strings.TrimSpace(st) != "" {
		t.Fatalf("tree should be clean after commit, got %q", st)
	}
}

func TestAutoCommitRefusesMainPush(t *testing.T) {
	dir := initRepo(t)
	gitOut(t, dir, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_ALLOW_MAIN_PUSH", "")
	err := AutoCommit(context.Background(), CommitOptions{
		Workspace:  dir,
		Message:    "x",
		PushRemote: "origin",
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to push directly to main") {
		t.Fatalf("expected main-push refusal, got %v", err)
	}
	// The commit itself should still have happened (refusal is at push stage).
	log := gitOut(t, dir, "log", "--oneline")
	if strings.TrimSpace(log) == "" {
		t.Fatalf("commit should happen before push refusal")
	}
}

func TestAutoCommitMainPushAllowedFlag(t *testing.T) {
	dir := initRepo(t)
	gitOut(t, dir, "checkout", "-b", "master")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_ALLOW_MAIN_PUSH", "1")
	// Push will fail (no remote configured) but NOT with the refusal message.
	err := AutoCommit(context.Background(), CommitOptions{
		Workspace: dir, Message: "x", PushRemote: "origin",
	})
	if err != nil && strings.Contains(err.Error(), "refusing to push") {
		t.Fatalf("SIN_ALLOW_MAIN_PUSH=1 should bypass refusal, got %v", err)
	}
}

func TestTruncate(t *testing.T) {
	cases := map[string]struct{ n, in int }{}
	_ = cases
	if got := truncate("hello world", 8); got != "hello..." {
		t.Fatalf("truncate: got %q", got)
	}
	if got := truncate("short", 80); got != "short" {
		t.Fatalf("truncate no-op: got %q", got)
	}
}
