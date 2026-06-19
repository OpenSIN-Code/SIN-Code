// SPDX-License-Identifier: MIT
// Purpose: WorktreeGoalManager — one git worktree per autonomous goal
// (issue #390). Unlike WorktreeManager (which manages worktrees by branch
// name), this manager keys worktrees by goal ID, tracks paths in an
// in-memory map, and provides Cleanup to remove all goal worktrees at
// once. Git commands are shell-outs via os/exec with hookable function
// fields for testability (M2: no CGO; M7: sync.Mutex guards the map).
package autonomy

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// WorktreeGoalManager provisions one git worktree per autonomous goal and
// tracks the goal→path mapping in memory. It is safe for concurrent use
// (M7): every public method acquires mu.
type WorktreeGoalManager struct {
	root      string
	mu        sync.Mutex
	worktrees map[string]string

	gitWorktreeAdd    func(root, branch, path string) (string, error)
	gitWorktreeRemove func(root, path string, force bool) (string, error)
	gitWorktreeList   func(root string) (string, error)
}

// goalGitRunner is the default command runner that shells out to git.
type goalGitRunner struct{}

func (goalGitRunner) worktreeAdd(root, branch, path string) (string, error) {
	cmd := exec.Command("git", "worktree", "add", "-b", branch, path, "HEAD")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (goalGitRunner) worktreeRemove(root, path string, force bool) (string, error) {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (goalGitRunner) worktreeList(root string) (string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

var defaultGoalGitRunner = goalGitRunner{}

// NewWorktreeGoalManager creates a WorktreeGoalManager rooted at the given
// git repository path. The root should be the toplevel of the git work tree.
func NewWorktreeGoalManager(root string) *WorktreeGoalManager {
	return &WorktreeGoalManager{
		root:              root,
		worktrees:         make(map[string]string),
		gitWorktreeAdd:    defaultGoalGitRunner.worktreeAdd,
		gitWorktreeRemove: defaultGoalGitRunner.worktreeRemove,
		gitWorktreeList:   defaultGoalGitRunner.worktreeList,
	}
}

// CreateWorktree provisions a new git worktree for the given goal ID and
// returns the absolute path. The branch is named "goal-<goalID>" and the
// worktree is placed at <root>/.sin-code/worktrees/goal-<goalID>.
func (m *WorktreeGoalManager) CreateWorktree(goalID string) (string, error) {
	if goalID == "" {
		return "", fmt.Errorf("worktree_goal: goalID required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.worktrees[goalID]; exists {
		return "", fmt.Errorf("worktree_goal: worktree for goal %s already exists", goalID)
	}

	branch := "goal-" + goalID
	path := m.root + "/.sin-code/worktrees/" + branch

	out, err := m.gitWorktreeAdd(m.root, branch, path)
	if err != nil {
		return "", fmt.Errorf("worktree_goal: create %s: %s: %w", goalID, strings.TrimSpace(out), err)
	}

	m.worktrees[goalID] = path
	return path, nil
}

// RemoveWorktree removes the worktree for the given goal ID. If the goal
// has no worktree, it returns nil (idempotent).
func (m *WorktreeGoalManager) RemoveWorktree(goalID string) error {
	if goalID == "" {
		return fmt.Errorf("worktree_goal: goalID required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	path, exists := m.worktrees[goalID]
	if !exists {
		return nil
	}

	out, err := m.gitWorktreeRemove(m.root, path, true)
	if err != nil {
		return fmt.Errorf("worktree_goal: remove %s: %s: %w", goalID, strings.TrimSpace(out), err)
	}

	delete(m.worktrees, goalID)
	return nil
}

// GetWorktree returns the path of the worktree for the given goal ID and
// whether it exists.
func (m *WorktreeGoalManager) GetWorktree(goalID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, ok := m.worktrees[goalID]
	return path, ok
}

// ListWorktrees returns a copy of the goal→path mapping. The returned map
// is safe to mutate by the caller.
func (m *WorktreeGoalManager) ListWorktrees() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.worktrees))
	for k, v := range m.worktrees {
		out[k] = v
	}
	return out
}

// Cleanup removes all tracked worktrees. Errors from individual removals
// are collected; the first error is returned but remaining worktrees are
// still attempted.
func (m *WorktreeGoalManager) Cleanup() error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.worktrees))
	for id := range m.worktrees {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var firstErr error
	for _, id := range ids {
		if err := m.RemoveWorktree(id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
