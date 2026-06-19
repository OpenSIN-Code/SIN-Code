// SPDX-License-Identifier: MIT
// Purpose: tests for worktree_goal.go (issue #390). Uses hookable git
// function fields so no real git binary is invoked.
package autonomy

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func newMockWorktreeGoalManager(root string) *WorktreeGoalManager {
	m := NewWorktreeGoalManager(root)
	m.gitWorktreeAdd = func(root, branch, path string) (string, error) {
		return "created " + branch, nil
	}
	m.gitWorktreeRemove = func(root, path string, force bool) (string, error) {
		return "removed " + path, nil
	}
	m.gitWorktreeList = func(root string) (string, error) {
		return "worktree " + root, nil
	}
	return m
}

func TestWorktreeGoalCreate(t *testing.T) {
	m := newMockWorktreeGoalManager("/fake/repo")
	path, err := m.CreateWorktree("abc123")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if !strings.Contains(path, "goal-abc123") {
		t.Errorf("expected path to contain goal-abc123, got %q", path)
	}
	if p, ok := m.GetWorktree("abc123"); !ok || p != path {
		t.Errorf("expected worktree tracked at %q, got %q ok=%v", path, p, ok)
	}
}

func TestWorktreeGoalCreateEmptyID(t *testing.T) {
	m := newMockWorktreeGoalManager("/fake/repo")
	_, err := m.CreateWorktree("")
	if err == nil {
		t.Fatal("expected error for empty goalID")
	}
}

func TestWorktreeGoalCreateDuplicate(t *testing.T) {
	m := newMockWorktreeGoalManager("/fake/repo")
	if _, err := m.CreateWorktree("dup"); err != nil {
		t.Fatalf("first CreateWorktree: %v", err)
	}
	_, err := m.CreateWorktree("dup")
	if err == nil {
		t.Fatal("expected error for duplicate goalID")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got %v", err)
	}
}

func TestWorktreeGoalRemove(t *testing.T) {
	m := newMockWorktreeGoalManager("/fake/repo")
	if _, err := m.CreateWorktree("rm1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if err := m.RemoveWorktree("rm1"); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, ok := m.GetWorktree("rm1"); ok {
		t.Error("expected worktree to be removed from map")
	}
}

func TestWorktreeGoalRemoveNonexistent(t *testing.T) {
	m := newMockWorktreeGoalManager("/fake/repo")
	if err := m.RemoveWorktree("nope"); err != nil {
		t.Errorf("expected nil for nonexistent worktree, got %v", err)
	}
}

func TestWorktreeGoalRemoveGitError(t *testing.T) {
	m := newMockWorktreeGoalManager("/fake/repo")
	m.gitWorktreeRemove = func(root, path string, force bool) (string, error) {
		return "locked", fmt.Errorf("worktree locked")
	}
	if _, err := m.CreateWorktree("err1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	err := m.RemoveWorktree("err1")
	if err == nil {
		t.Fatal("expected git error")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Errorf("expected 'locked' in error, got %v", err)
	}
}

func TestWorktreeGoalListWorktrees(t *testing.T) {
	m := newMockWorktreeGoalManager("/fake/repo")
	if _, err := m.CreateWorktree("a"); err != nil {
		t.Fatalf("CreateWorktree a: %v", err)
	}
	if _, err := m.CreateWorktree("b"); err != nil {
		t.Fatalf("CreateWorktree b: %v", err)
	}
	list := m.ListWorktrees()
	if len(list) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(list))
	}
	if _, ok := list["a"]; !ok {
		t.Error("expected worktree 'a' in list")
	}
	if _, ok := list["b"]; !ok {
		t.Error("expected worktree 'b' in list")
	}
	list["c"] = "injected"
	if _, ok := m.GetWorktree("c"); ok {
		t.Error("mutating returned map should not affect internal state")
	}
}

func TestWorktreeGoalCleanupAndConcurrent(t *testing.T) {
	m := newMockWorktreeGoalManager("/fake/repo")
	for _, id := range []string{"c1", "c2", "c3"} {
		if _, err := m.CreateWorktree(id); err != nil {
			t.Fatalf("CreateWorktree %s: %v", id, err)
		}
	}
	if err := m.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(m.ListWorktrees()) != 0 {
		t.Errorf("expected 0 worktrees after cleanup, got %d", len(m.ListWorktrees()))
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("g%d", i)
			_, _ = m.CreateWorktree(id)
		}(i)
	}
	wg.Wait()
	if len(m.ListWorktrees()) != 10 {
		t.Errorf("expected 10 concurrent worktrees, got %d", len(m.ListWorktrees()))
	}
	_ = m.Cleanup()
}
