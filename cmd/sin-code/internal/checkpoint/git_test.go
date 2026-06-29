// SPDX-License-Identifier: MIT
// Purpose: tests for git-based workspace checkpoints (issue #483).
package checkpoint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a temporary git repo with one initial commit
// and returns its path. Uses a per-test HOME so the shared DB does
// not collide with real user data.
func initGitRepo(t *testing.T) string {
	t.Helper()
	// Isolate HOME so the shared checkpoints.db goes to a temp dir.
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo := t.TempDir()
	mustRun(t, repo, "git", "init", "-q")
	mustRun(t, repo, "git", "config", "user.email", "test@test.test")
	mustRun(t, repo, "git", "config", "user.name", "Test")
	// Initial commit so HEAD exists.
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, repo, "git", "add", "README.md")
	mustRun(t, repo, "git", "commit", "-q", "-m", "init")
	return repo
}

func mustRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v in %s: %v\n%s", name, args, dir, err, out)
	}
}

func TestGitStore_CreateAndList(t *testing.T) {
	repo := initGitRepo(t)
	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cp, err := st.Create(context.Background(), "first checkpoint")
	if err != nil {
		t.Fatal(err)
	}
	if cp.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if cp.GitRef == "" {
		t.Fatal("expected non-empty git ref")
	}

	// Verify the tag was actually created in git.
	out, err := exec.Command("git", "-C", repo, "tag", "-l").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), cp.GitRef) {
		t.Errorf("tag %s not found in git tag list: %s", cp.GitRef, out)
	}

	// List should return 1 checkpoint.
	list, err := st.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(list))
	}
	if list[0].ID != cp.ID {
		t.Errorf("expected id %s, got %s", cp.ID, list[0].ID)
	}
	if list[0].Message != "first checkpoint" {
		t.Errorf("expected message %q, got %q", "first checkpoint", list[0].Message)
	}
}

func TestGitStore_RollbackRestoresFiles(t *testing.T) {
	repo := initGitRepo(t)
	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Write a file and commit it.
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, repo, "git", "add", "a.txt")
	mustRun(t, repo, "git", "commit", "-q", "-m", "add a.txt")

	// Create a checkpoint at this state.
	cp, err := st.Create(context.Background(), "before mutation")
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the file.
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Rollback.
	if err := st.Rollback(context.Background(), cp.ID); err != nil {
		t.Fatal(err)
	}

	// The working tree should have the original content.
	got, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original\n" {
		t.Errorf("expected original content after rollback, got %q", got)
	}
}

func TestGitStore_DiffShowsChanges(t *testing.T) {
	repo := initGitRepo(t)
	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cp, err := st.Create(context.Background(), "baseline")
	if err != nil {
		t.Fatal(err)
	}

	// Make a new commit that changes a file.
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, repo, "git", "add", "README.md")
	mustRun(t, repo, "git", "commit", "-q", "-m", "change readme")

	diff, err := st.Diff(context.Background(), cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff == "" {
		t.Error("expected non-empty diff between checkpoint and HEAD")
	}
	if !strings.Contains(diff, "README.md") {
		t.Errorf("expected diff to mention README.md, got: %s", diff)
	}
}

func TestGitStore_DeleteRemovesTagAndRow(t *testing.T) {
	repo := initGitRepo(t)
	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cp, err := st.Create(context.Background(), "to be deleted")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.Delete(context.Background(), cp.ID); err != nil {
		t.Fatal(err)
	}

	// Tag should be gone.
	out, err := exec.Command("git", "-C", repo, "tag", "-l", cp.GitRef).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "" {
		t.Errorf("expected tag to be deleted, still found: %s", out)
	}

	// Row should be gone.
	list, err := st.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 checkpoints after delete, got %d", len(list))
	}
}

func TestGitStore_GetUnknownID(t *testing.T) {
	repo := initGitRepo(t)
	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.Get(context.Background(), "nonexistent"); err == nil {
		t.Error("expected error for unknown id")
	}
}

func TestGitStore_RollbackUnknownID(t *testing.T) {
	repo := initGitRepo(t)
	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.Rollback(context.Background(), "nope"); err == nil {
		t.Error("expected error for rollback of unknown id")
	}
}

func TestGitStore_CreateNotAGitRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := t.TempDir() // not a git repo

	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.Create(context.Background(), "test"); err == nil {
		t.Error("expected error when creating checkpoint outside a git repo")
	}
}

func TestGitStore_WorkspaceIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repo1 := initGitRepoCustom(t, home)
	repo2 := initGitRepoCustom(t, home)

	st1, err := OpenGit(repo1)
	if err != nil {
		t.Fatal(err)
	}
	defer st1.Close()

	st2, err := OpenGit(repo2)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()

	// Create in repo1 only.
	if _, err := st1.Create(context.Background(), "repo1 cp"); err != nil {
		t.Fatal(err)
	}

	// repo2 should see 0 checkpoints.
	list2, err := st2.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 0 {
		t.Errorf("expected 0 checkpoints in repo2, got %d", len(list2))
	}

	// repo1 should see 1.
	list1, err := st1.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list1) != 1 {
		t.Errorf("expected 1 checkpoint in repo1, got %d", len(list1))
	}
}

func TestGitStore_ListNewestFirst(t *testing.T) {
	repo := initGitRepo(t)
	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Create three checkpoints. Each commit advances HEAD so IDs differ.
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(string(rune('a'+i))), 0o644); err != nil {
			t.Fatal(err)
		}
		mustRun(t, repo, "git", "add", "f.txt")
		mustRun(t, repo, "git", "commit", "-q", "-m", "commit")
		if _, err := st.Create(context.Background(), "cp"); err != nil {
			t.Fatal(err)
		}
	}

	list, err := st.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if !list[0].CreatedAt.After(list[1].CreatedAt) || !list[1].CreatedAt.After(list[2].CreatedAt) {
		t.Errorf("expected newest-first ordering, got %v < %v < %v",
			list[0].CreatedAt, list[1].CreatedAt, list[2].CreatedAt)
	}
}

func TestGitStore_CreateIdempotentOnSameCommit(t *testing.T) {
	repo := initGitRepo(t)
	st, err := OpenGit(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Two creates at the same HEAD with the same message may produce
	// the same ID (same timestamp nano is unlikely but the tag
	// already-exists path should be handled gracefully). The key
	// guarantee: neither call errors.
	if _, err := st.Create(context.Background(), "same msg"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(context.Background(), "same msg"); err != nil {
		t.Fatal(err)
	}
}

// initGitRepoCustom is like initGitRepo but takes an explicit HOME
// for workspace-isolation tests.
func initGitRepoCustom(t *testing.T, home string) string {
	t.Helper()
	repo := t.TempDir()
	mustRun(t, repo, "git", "init", "-q")
	mustRun(t, repo, "git", "config", "user.email", "test@test.test")
	mustRun(t, repo, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, repo, "git", "add", "README.md")
	mustRun(t, repo, "git", "commit", "-q", "-m", "init")
	return repo
}
