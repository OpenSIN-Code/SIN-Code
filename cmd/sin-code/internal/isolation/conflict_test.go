// SPDX-License-Identifier: MIT
// Purpose: tests for issue #319 — conflict prediction via git merge-tree.
package isolation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestPredictConflictsClean(t *testing.T) {
	root := newRepo(t)

	// Main adds a new file.
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "new.txt")
	runGit(t, root, "commit", "-q", "-m", "main new")

	// Feature branches from the initial commit and adds a different file.
	runGit(t, root, "checkout", "-q", "-b", "feature", "HEAD~1")
	if err := os.WriteFile(filepath.Join(root, "feature.txt"), []byte("feature"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "feature.txt")
	runGit(t, root, "commit", "-q", "-m", "feature")
	runGit(t, root, "checkout", "-q", "main")

	report, err := PredictConflicts(root, "main", "feature")
	if err != nil {
		t.Fatalf("PredictConflicts: %v", err)
	}
	if !report.Clean {
		t.Fatalf("expected clean merge, got %+v", report)
	}
	if report.Tree == "" {
		t.Fatal("expected a tree hash")
	}
	if len(report.ConflictPaths) != 0 {
		t.Fatalf("expected no conflict paths, got %v", report.ConflictPaths)
	}
}

func TestPredictConflictsConflict(t *testing.T) {
	root := newRepo(t)

	// Both branches modify the same file with different content.
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

	report, err := PredictConflicts(root, "main", "feature")
	if err != nil {
		t.Fatalf("PredictConflicts: %v", err)
	}
	if report.Clean {
		t.Fatalf("expected conflict, got clean: %+v", report)
	}
	if len(report.ConflictPaths) != 1 || report.ConflictPaths[0] != "README" {
		t.Fatalf("expected README conflict, got %v", report.ConflictPaths)
	}
	if len(report.Messages) == 0 {
		t.Fatal("expected conflict messages")
	}
}

func TestPredictConflictsUnknownTarget(t *testing.T) {
	root := newRepo(t)
	_, err := PredictConflicts(root, "does-not-exist", "main")
	if err == nil {
		t.Fatal("expected error for unknown target branch")
	}
}

func TestPredictWorktreeConflictsExistingBranch(t *testing.T) {
	root := newRepo(t)

	// Create a pre-existing worktree-feature branch that conflicts with main.
	runGit(t, root, "checkout", "-q", "-b", "worktree-feature")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("worktree"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-q", "-am", "worktree")

	runGit(t, root, "checkout", "-q", "main")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-q", "-am", "main")

	report, err := PredictWorktreeConflicts(root, "feature", "main")
	if err != nil {
		t.Fatalf("PredictWorktreeConflicts: %v", err)
	}
	if report.Clean {
		t.Fatalf("expected conflict, got clean: %+v", report)
	}
	if report.Source != "worktree-feature" {
		t.Fatalf("expected source worktree-feature, got %q", report.Source)
	}
	if len(report.ConflictPaths) != 1 || report.ConflictPaths[0] != "README" {
		t.Fatalf("expected README conflict, got %v", report.ConflictPaths)
	}
}

func TestPredictWorktreeConflictsFallbackToHead(t *testing.T) {
	root := newRepo(t)

	// main modifies README; no worktree-foo branch exists, so source=HEAD.
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-q", "-am", "main")

	// Feature branches from the initial commit and also modifies README.
	runGit(t, root, "checkout", "-q", "-b", "feature", "HEAD~1")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("feature"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-q", "-am", "feature")
	runGit(t, root, "checkout", "-q", "main")

	// Predict against the feature branch; source falls back to HEAD (main),
	// which conflicts with feature.
	report, err := PredictWorktreeConflicts(root, "foo", "feature")
	if err != nil {
		t.Fatalf("PredictWorktreeConflicts: %v", err)
	}
	if report.Clean {
		t.Fatalf("expected conflict, got clean: %+v", report)
	}
	if report.Source != "HEAD" {
		t.Fatalf("expected source HEAD, got %q", report.Source)
	}
	if report.Target != "feature" {
		t.Fatalf("expected target feature, got %q", report.Target)
	}
	if len(report.ConflictPaths) != 1 || report.ConflictPaths[0] != "README" {
		t.Fatalf("expected README conflict, got %v", report.ConflictPaths)
	}
}
