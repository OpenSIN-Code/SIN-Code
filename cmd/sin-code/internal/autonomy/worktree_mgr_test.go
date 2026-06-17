// SPDX-License-Identifier: MIT
// Purpose: tests for issue #329 — WorktreeManager auto-merge and auto-prune.
package autonomy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newMockWorktreeManager() *WorktreeManager {
	m := NewWorktreeManager("/fake/repo")
	m.gitWorktreeAdd = func(root, branch, path string) (string, error) {
		return "created", nil
	}
	m.gitWorktreeList = func(root string) (string, error) {
		return "", nil
	}
	m.gitWorktreePrune = func(root string) (string, error) {
		return "pruned", nil
	}
	m.gitWorktreeRemove = func(root, path string, force bool) (string, error) {
		return "removed", nil
	}
	m.gitMerge = func(root, branch string) (string, error) {
		return "merged", nil
	}
	m.gitCheckout = func(root, branch string) (string, error) {
		return "checked out", nil
	}
	m.gitStatus = func(path string) (string, error) {
		return "", nil // clean by default
	}
	m.gitBranchDelete = func(root, branch string) (string, error) {
		return "deleted", nil
	}
	return m
}

func TestWorktreeManagerCreate(t *testing.T) {
	m := newMockWorktreeManager()
	path, err := m.Create("feature-branch")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
	if !strings.Contains(path, "feature-branch") {
		t.Errorf("expected path to contain branch name, got %q", path)
	}
}

func TestWorktreeManagerCreateEmptyBranch(t *testing.T) {
	m := newMockWorktreeManager()
	_, err := m.Create("")
	if err == nil {
		t.Error("expected error for empty branch")
	}
}

func TestWorktreeManagerMerge(t *testing.T) {
	m := newMockWorktreeManager()
	if err := m.Merge("feature-branch"); err != nil {
		t.Fatalf("Merge: %v", err)
	}
}

func TestWorktreeManagerMergeError(t *testing.T) {
	m := newMockWorktreeManager()
	m.gitMerge = func(root, branch string) (string, error) {
		return "conflict", fmt.Errorf("merge conflict")
	}
	if err := m.Merge("bad-branch"); err == nil {
		t.Error("expected merge error")
	}
}

func TestWorktreeManagerPrune(t *testing.T) {
	m := newMockWorktreeManager()
	if err := m.Prune(); err != nil {
		t.Fatalf("Prune: %v", err)
	}
}

func TestWorktreeManagerList(t *testing.T) {
	m := newMockWorktreeManager()
	m.gitWorktreeList = func(root string) (string, error) {
		return "worktree /fake/repo\nHEAD abc123\nbranch refs/heads/main\n\nworktree /fake/repo/.sin-code/worktrees/feature\nHEAD def456\nbranch refs/heads/feature\n\n", nil
	}
	m.gitStatus = func(path string) (string, error) {
		if strings.Contains(path, "feature") {
			return " M file.go\n", nil // dirty
		}
		return "", nil
	}

	infos, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(infos))
	}
	if infos[0].Branch != "main" {
		t.Errorf("expected main, got %q", infos[0].Branch)
	}
	if infos[1].Branch != "feature" {
		t.Errorf("expected feature, got %q", infos[1].Branch)
	}
	if !infos[1].Dirty {
		t.Error("expected feature worktree to be dirty")
	}
	if infos[0].Dirty {
		t.Error("expected main worktree to be clean")
	}
}

func TestWorktreeManagerAutoMerge(t *testing.T) {
	m := newMockWorktreeManager()
	m.gitWorktreeList = func(root string) (string, error) {
		return "worktree /fake/repo\nHEAD abc123\nbranch refs/heads/main\n\nworktree /fake/repo/.sin-code/worktrees/goal-42\nHEAD def456\nbranch refs/heads/goal-42\n\n", nil
	}

	if err := m.AutoMerge("42"); err != nil {
		t.Fatalf("AutoMerge: %v", err)
	}
}

func TestWorktreeManagerAutoMergeDirty(t *testing.T) {
	m := newMockWorktreeManager()
	m.gitWorktreeList = func(root string) (string, error) {
		return "worktree /fake/repo\nHEAD abc123\nbranch refs/heads/main\n\nworktree /fake/repo/.sin-code/worktrees/goal-42\nHEAD def456\nbranch refs/heads/goal-42\n\n", nil
	}
	m.gitStatus = func(path string) (string, error) {
		return " M dirty.go\n", nil
	}

	err := m.AutoMerge("42")
	if err == nil {
		t.Fatal("expected error for dirty worktree")
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Errorf("expected 'dirty' in error, got %v", err)
	}
}

func TestWorktreeManagerAutoMergeNotFound(t *testing.T) {
	m := newMockWorktreeManager()
	m.gitWorktreeList = func(root string) (string, error) {
		return "worktree /fake/repo\nHEAD abc123\nbranch refs/heads/main\n\n", nil
	}

	err := m.AutoMerge("999")
	if err == nil {
		t.Fatal("expected error for missing worktree")
	}
	if !strings.Contains(err.Error(), "no worktree") {
		t.Errorf("expected 'no worktree' in error, got %v", err)
	}
}

func TestWorktreeManagerAutoPrune(t *testing.T) {
	m := newMockWorktreeManager()
	m.gitWorktreeList = func(root string) (string, error) {
		return "worktree /fake/repo\nHEAD abc123\nbranch refs/heads/main\n\nworktree /fake/repo/.sin-code/worktrees/old\nHEAD def456\nbranch refs/heads/old\n\n", nil
	}
	removedPaths := []string{}
	m.gitWorktreeRemove = func(root, path string, force bool) (string, error) {
		removedPaths = append(removedPaths, path)
		return "removed", nil
	}

	if err := m.AutoPrune(1 * time.Hour); err != nil {
		t.Fatalf("AutoPrune: %v", err)
	}

	// Should have removed the non-main, non-dirty worktree.
	found := false
	for _, p := range removedPaths {
		if strings.Contains(p, "old") {
			found = true
		}
	}
	if !found {
		t.Error("expected old worktree to be pruned")
	}
}

func newRealRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-q", "-m", "init")
	return root
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestWorktreeManagerPredictConflictsClean(t *testing.T) {
	root := newRealRepo(t)

	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "new.txt")
	runGit(t, root, "commit", "-q", "-m", "main new")

	runGit(t, root, "checkout", "-q", "-b", "feature", "HEAD~1")
	if err := os.WriteFile(filepath.Join(root, "feature.txt"), []byte("feature"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "feature.txt")
	runGit(t, root, "commit", "-q", "-m", "feature")
	runGit(t, root, "checkout", "-q", "main")

	m := NewWorktreeManager(root)
	r, err := m.PredictConflicts("feature", "main")
	if err != nil {
		t.Fatalf("PredictConflicts: %v", err)
	}
	if !r.Clean {
		t.Fatalf("expected clean merge, got %+v", r)
	}
	if r.Tree == "" {
		t.Fatal("expected a tree hash")
	}
}

func TestWorktreeManagerPredictConflictsConflict(t *testing.T) {
	root := newRealRepo(t)

	runGit(t, root, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("feature"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-q", "-am", "feature")

	runGit(t, root, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-q", "-am", "main")

	m := NewWorktreeManager(root)
	r, err := m.PredictConflicts("feature", "main")
	if err != nil {
		t.Fatalf("PredictConflicts: %v", err)
	}
	if r.Clean {
		t.Fatal("expected conflict")
	}
	if len(r.ConflictPaths) != 1 || r.ConflictPaths[0] != "README" {
		t.Fatalf("unexpected conflict paths: %v", r.ConflictPaths)
	}
}

func TestWorktreeManagerPredictConflictsUnknownTarget(t *testing.T) {
	root := newRealRepo(t)
	m := NewWorktreeManager(root)
	_, err := m.PredictConflicts("feature", "missing")
	if err == nil {
		t.Fatal("expected error for unknown target branch")
	}
}

func TestWorktreeManagerPredictConflictsWithSource(t *testing.T) {
	root := newRealRepo(t)

	runGit(t, root, "checkout", "-q", "-b", "worktree-foo")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("wt"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-q", "-am", "worktree")

	runGit(t, root, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-q", "-am", "main")

	m := NewWorktreeManager(root)
	r, err := m.PredictConflictsWithSource("worktree-foo", "main")
	if err != nil {
		t.Fatalf("PredictConflictsWithSource: %v", err)
	}
	if r.Clean {
		t.Fatal("expected conflict")
	}
	if r.Source != "worktree-foo" {
		t.Fatalf("expected source worktree-foo, got %q", r.Source)
	}
	if len(r.ConflictPaths) != 1 || r.ConflictPaths[0] != "README" {
		t.Fatalf("unexpected conflict paths: %v", r.ConflictPaths)
	}
}
