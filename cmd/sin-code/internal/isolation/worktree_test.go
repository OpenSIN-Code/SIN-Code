// SPDX-License-Identifier: MIT
// Purpose: race-clean tests for the git-worktree isolation package.
package isolation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo creates a fresh git repo at t.TempDir(), commits one file
// ("README") and returns the repo root. Fails the test on any git
// error — tests already require `git` on PATH.
func newRepo(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-q", "-m", "init")
	return root
}

func TestCreateAndList(t *testing.T) {
	root := newRepo(t)
	wt, err := Create(root, "feature-a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(wt, filepath.Join(".sin-code", "worktrees", "feature-a")) {
		t.Fatalf("unexpected worktree path: %q", wt)
	}
	if _, err := os.Stat(filepath.Join(wt, "README")); err != nil {
		t.Fatalf("README must exist in new worktree: %v", err)
	}
	infos, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) < 2 {
		t.Fatalf("want >=2 worktrees (root + feature-a), got %d", len(infos))
	}
	var hasBranch bool
	for _, i := range infos {
		if i.Path == wt && strings.HasSuffix(i.Branch, "worktree-feature-a") {
			hasBranch = true
		}
	}
	if !hasBranch {
		t.Fatalf("branch worktree-feature-a not registered: %+v", infos)
	}
}

func TestCreateRejectsInvalidNames(t *testing.T) {
	root := newRepo(t)
	cases := []string{
		"",          // empty
		".",         // traverses
		"..",        // traverses
		"../escape", // explicit traversal
		".hidden",   // leading dot
		"-flag",     // leading hyphen (git option confusion)
		"a/b",       // slash
		"a\\b",      // backslash
		"a:b",       // colon (windows-illegal)
	}
	for _, n := range cases {
		if _, err := Create(root, n); err == nil {
			t.Errorf("Create(%q): want error, got nil", n)
		}
	}
}

func TestCreateIdempotentName(t *testing.T) {
	root := newRepo(t)
	_, err := Create(root, "dup")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "dup"); err == nil {
		t.Fatal("second Create with same name must error")
	}
}

func TestRemoveCleanWorktree(t *testing.T) {
	root := newRepo(t)
	wt, err := Create(root, "clean")
	if err != nil {
		t.Fatal(err)
	}
	// Create() auto-locks while the agent runs; the session-end
	// cleanup path either Unlock()s first or passes force=true.
	// Verify both code paths:
	if err := Unlock(root, wt); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, wt, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree dir should be gone; stat err = %v", err)
	}
}

func TestRemoveRefusesLockedNotForce(t *testing.T) {
	root := newRepo(t)
	wt, err := Create(root, "lkonly")
	if err != nil {
		t.Fatal(err)
	}
	// auto-locked by Create(); remove without unlock or force
	// must refuse.
	err = Remove(root, wt, false)
	if err == nil {
		t.Fatal("Remove must refuse a locked worktree without force")
	}
	rerr, ok := err.(ErrRefusal)
	if !ok {
		t.Fatalf("want ErrRefusal, got %T", err)
	}
	if !strings.Contains(rerr.Reason, "locked") {
		t.Fatalf("reason must mention lock; got %q", rerr.Reason)
	}
	// force=true with --force --force overrides both dirty + lock.
	if err := Remove(root, wt, true); err != nil {
		t.Fatalf("force remove must bypass lock: %v", err)
	}
}

func TestRemoveRefusesDirty(t *testing.T) {
	root := newRepo(t)
	wt, err := Create(root, "dirty")
	if err != nil {
		t.Fatal(err)
	}
	// Unlock so the dirty-check — not the lock — is the
	// reason for refusal.
	if err := Unlock(root, wt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "new.txt"),
		[]byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Remove(root, wt, false)
	if err == nil {
		t.Fatal("Remove must refuse dirty worktree when force=false")
	}
	if _, ok := err.(ErrRefusal); !ok {
		t.Fatalf("want ErrRefusal, got %T: %v", err, err)
	}
	// Force still works
	if err := Remove(root, wt, true); err != nil {
		t.Fatalf("force remove must succeed: %v", err)
	}
}

func TestRemoveRefusesMainWorktree(t *testing.T) {
	root := newRepo(t)
	// Even with force=true we never remove the main worktree.
	err := Remove(root, root, true)
	if err == nil {
		t.Fatal("Remove must refuse the main worktree")
	}
	if _, ok := err.(ErrRefusal); !ok {
		t.Fatalf("want ErrRefusal, got %T", err)
	}
}

func TestHasUncommitted(t *testing.T) {
	root := newRepo(t)
	if dirty, why, err := HasUncommitted(root); err != nil || dirty {
		t.Fatalf("fresh repo must be clean: dirty=%v why=%q err=%v", dirty, why, err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"),
		[]byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, why, err := HasUncommitted(root)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("untracked file must mark tree dirty")
	}
	if !strings.Contains(why, "untracked.txt") {
		t.Fatalf("reason must mention file: %q", why)
	}
}

func TestNotARepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := Create(dir, "x"); err == nil {
		t.Fatal("Create outside a git repo must error")
	} else if _, ok := err.(ErrNotARepo); !ok {
		t.Fatalf("want ErrNotARepo, got %T: %v", err, err)
	}
}

func TestLockUnlock(t *testing.T) {
	root := newRepo(t)
	wt, err := Create(root, "lk")
	if err != nil {
		t.Fatal(err)
	}
	// Create() auto-locks, so Unlock must first succeed.
	if err := Unlock(root, wt); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if err := Lock(root, wt); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	infos, _ := List(root)
	for _, i := range infos {
		if i.Path == wt && !i.Locked {
			t.Fatalf("worktree must report locked=true after Lock(): %+v", i)
		}
	}
}
