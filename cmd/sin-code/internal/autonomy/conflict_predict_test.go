// SPDX-License-Identifier: MIT
// Purpose: tests for the worktree conflict predictor (issue #319). All
// git commands are mocked via the hookable runners so no real repository
// is required. The -race flag exercises the mutex paths (M7).
package autonomy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// newMockPredictor returns a ConflictPredictor with mocked git runners.
// The mergeTree and diffNameOnly hooks are configurable per-test by
// reassigning the fields after construction.
func newMockPredictor() *ConflictPredictor {
	p := NewConflictPredictor("/fake/repo")
	p.gitMergeTree = func(root, base, branch string) mergeTreeResult {
		return mergeTreeResult{exitCode: 0} // clean by default
	}
	p.gitDiffNameOnly = func(root, spec string) (string, error) {
		return "", nil // no changes by default
	}
	return p
}

func TestNewConflictPredictor(t *testing.T) {
	p := NewConflictPredictor("/fake/repo")
	if p == nil {
		t.Fatal("expected non-nil predictor")
	}
	if p.root != "/fake/repo" {
		t.Errorf("root = %q", p.root)
	}
	if p.gitMergeTree == nil || p.gitDiffNameOnly == nil {
		t.Error("expected default runners to be wired")
	}
}

func TestPredictNoConflicts(t *testing.T) {
	p := newMockPredictor()
	// merge-tree exit 0 => clean merge.
	preds, err := p.Predict("feature")
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(preds) != 0 {
		t.Errorf("expected 0 predictions, got %d", len(preds))
	}
}

func TestPredictWithConflicts(t *testing.T) {
	p := newMockPredictor()
	p.gitMergeTree = func(root, base, branch string) mergeTreeResult {
		// exit 1 + tree SHA line then conflicted file paths.
		return mergeTreeResult{
			stdout:   "abc123def456\nmain.go\nlib/auth.go\n",
			exitCode: 1,
		}
	}
	preds, err := p.Predict("feature")
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(preds) != 2 {
		t.Fatalf("expected 2 predictions, got %d", len(preds))
	}
	files := map[string]bool{}
	for _, pr := range preds {
		if pr.ConflictType != ConflictMergeTree {
			t.Errorf("type = %q, want merge-tree", pr.ConflictType)
		}
		files[pr.File] = true
	}
	if !files["main.go"] || !files["lib/auth.go"] {
		t.Errorf("expected main.go + lib/auth.go, got %v", files)
	}
	// Severity should reflect 2 conflicts => low.
	if preds[0].Severity != SeverityLow {
		t.Errorf("Severity = %q, want low", preds[0].Severity)
	}
}

func TestPredictSkipsTreeShaLine(t *testing.T) {
	p := newMockPredictor()
	p.gitMergeTree = func(root, base, branch string) mergeTreeResult {
		return mergeTreeResult{
			stdout:   "0123456789abcdef0123456789abcdef01234567\nonly.go\n",
			exitCode: 1,
		}
	}
	preds, err := p.Predict("feature")
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(preds) != 1 {
		t.Fatalf("expected 1 prediction, got %d", len(preds))
	}
	if preds[0].File != "only.go" {
		t.Errorf("File = %q, want only.go", preds[0].File)
	}
}

func TestPredictFallbackToOverlap(t *testing.T) {
	p := newMockPredictor()
	// merge-tree errors out (unsupported flag on old git).
	p.gitMergeTree = func(root, base, branch string) mergeTreeResult {
		return mergeTreeResult{err: fmt.Errorf("unknown option: --write-tree")}
	}
	// branch-side changes.
	p.gitDiffNameOnly = func(root, spec string) (string, error) {
		if spec == "main...feature" {
			return "a.go\nb.go\nc.go\n", nil
		}
		if spec == "feature...main" {
			return "b.go\nc.go\nd.go\n", nil
		}
		return "", nil
	}
	preds, err := p.Predict("feature")
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(preds) != 2 {
		t.Fatalf("expected 2 overlapping files, got %d", len(preds))
	}
	got := map[string]ConflictType{}
	for _, pr := range preds {
		got[pr.File] = pr.ConflictType
	}
	if got["b.go"] != ConflictOverlap || got["c.go"] != ConflictOverlap {
		t.Errorf("expected b.go + c.go as overlap, got %v", got)
	}
}

func TestPredictEmptyBranch(t *testing.T) {
	p := newMockPredictor()
	if _, err := p.Predict(""); err == nil {
		t.Error("expected error for empty branch")
	}
}

func TestSeverityLevels(t *testing.T) {
	p := newMockPredictor()
	cases := []struct {
		n    int
		want SeverityLevel
	}{
		{0, SeverityNone},
		{1, SeverityLow},
		{3, SeverityLow},
		{4, SeverityMedium},
		{9, SeverityMedium},
		{10, SeverityHigh},
		{25, SeverityHigh},
	}
	for _, c := range cases {
		if got := p.Severity(c.n); got != string(c.want) {
			t.Errorf("Severity(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestSafeToMergeClean(t *testing.T) {
	p := newMockPredictor()
	safe, reason := p.SafeToMerge("feature")
	if !safe {
		t.Errorf("expected safe, reason=%q", reason)
	}
	if !strings.Contains(reason, "no conflicts") {
		t.Errorf("reason = %q", reason)
	}
}

func TestSafeToMergeConflicted(t *testing.T) {
	p := newMockPredictor()
	p.gitMergeTree = func(root, base, branch string) mergeTreeResult {
		return mergeTreeResult{
			stdout:   "abc123\nmain.go\nlib.go\n",
			exitCode: 1,
		}
	}
	safe, reason := p.SafeToMerge("feature")
	if safe {
		t.Error("expected not safe")
	}
	if !strings.Contains(reason, "2 conflict") {
		t.Errorf("reason = %q", reason)
	}
	if !strings.Contains(reason, "main.go") {
		t.Errorf("reason should list files: %q", reason)
	}
}

func TestOverlappingFiles(t *testing.T) {
	p := newMockPredictor()
	p.gitDiffNameOnly = func(root, spec string) (string, error) {
		switch spec {
		case "main...feature":
			return "x.go\ny.go\nz.go\n", nil
		case "feature...main":
			return "y.go\nz.go\nw.go\n", nil
		}
		return "", nil
	}
	got, err := p.OverlappingFiles("feature")
	if err != nil {
		t.Fatalf("OverlappingFiles: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 overlaps, got %d (%v)", len(got), got)
	}
	// Should be sorted.
	if got[0] != "y.go" || got[1] != "z.go" {
		t.Errorf("got %v, want [y.go z.go]", got)
	}
}

func TestOverlappingFilesNoOverlap(t *testing.T) {
	p := newMockPredictor()
	p.gitDiffNameOnly = func(root, spec string) (string, error) {
		switch spec {
		case "main...feature":
			return "a.go\n", nil
		case "feature...main":
			return "b.go\n", nil
		}
		return "", nil
	}
	got, err := p.OverlappingFiles("feature")
	if err != nil {
		t.Fatalf("OverlappingFiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 overlaps, got %d", len(got))
	}
}

func TestOverlappingFilesEmptyBranch(t *testing.T) {
	p := newMockPredictor()
	if _, err := p.OverlappingFiles(""); err == nil {
		t.Error("expected error for empty branch")
	}
}

func TestOverlappingFilesDiffError(t *testing.T) {
	p := newMockPredictor()
	p.gitDiffNameOnly = func(root, spec string) (string, error) {
		return "", fmt.Errorf("git failed")
	}
	if _, err := p.OverlappingFiles("feature"); err == nil {
		t.Error("expected error when diff fails")
	}
}

func TestParseMergeTreeConflicts(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"abc\nfile1.go\nfile2.go\n", []string{"file1.go", "file2.go"}},
		{"abc\n", nil},
		{"", nil},
		{"  \nabc\ngo.mod\n", []string{"go.mod"}},
	}
	for _, c := range cases {
		got := parseMergeTreeConflicts(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseMergeTreeConflicts(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseMergeTreeConflicts(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestParseDiffNameOnlyDedup(t *testing.T) {
	got := parseDiffNameOnly("a.go\nb.go\na.go\n\nb.go\n")
	want := []string{"a.go", "b.go"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestConflictPredictorConcurrent(t *testing.T) {
	p := newMockPredictor()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			branch := fmt.Sprintf("feature-%d", i)
			_, _ = p.Predict(branch)
			_, _ = p.OverlappingFiles(branch)
			_ = p.Severity(i)
		}(i)
	}
	wg.Wait()
}

// --- cpGitRunner real-git tests ---

// initGitRepo creates a temp git repo with an initial commit on main.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cpRunGit(t, dir, "init", "-b", "main")
	cpRunGit(t, dir, "config", "user.email", "test@test.com")
	cpRunGit(t, dir, "config", "user.name", "Test")
	cpWriteGitFile(t, dir, "README.md", "init\n")
	cpRunGit(t, dir, "add", "-A")
	cpRunGit(t, dir, "commit", "-m", "init")
	return dir
}

func cpRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

func cpWriteGitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCpGitRunnerMergeTreeClean(t *testing.T) {
	dir := initGitRepo(t)
	// Create a branch that adds a new file (no conflict).
	cpRunGit(t, dir, "checkout", "-b", "feature")
	cpWriteGitFile(t, dir, "new.go", "package main\n")
	cpRunGit(t, dir, "add", "-A")
	cpRunGit(t, dir, "commit", "-m", "add new.go")
	cpRunGit(t, dir, "checkout", "main")

	runner := cpGitRunner{}
	res := runner.mergeTree(dir, "main", "feature")
	// exit 0 = clean merge, no error.
	if res.err != nil && res.exitCode != 0 {
		t.Fatalf("expected clean merge, got err=%v exit=%d", res.err, res.exitCode)
	}
}

func TestCpGitRunnerMergeTreeConflict(t *testing.T) {
	dir := initGitRepo(t)
	// Modify same file on both branches to create a conflict.
	cpWriteGitFile(t, dir, "shared.go", "package main\n// branch version\n")
	cpRunGit(t, dir, "checkout", "-b", "feature")
	cpWriteGitFile(t, dir, "shared.go", "package main\n// branch version\n")
	cpRunGit(t, dir, "add", "-A")
	cpRunGit(t, dir, "commit", "-m", "branch change")
	cpRunGit(t, dir, "checkout", "main")
	cpWriteGitFile(t, dir, "shared.go", "package main\n// main version\n")
	cpRunGit(t, dir, "add", "-A")
	cpRunGit(t, dir, "commit", "-m", "main change")

	runner := cpGitRunner{}
	res := runner.mergeTree(dir, "main", "feature")
	// exit 1 = conflicts detected, err should be nil (suppressed).
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if res.exitCode != 1 {
		t.Fatalf("expected exit code 1 (conflicts), got %d", res.exitCode)
	}
	// stdout should contain the conflicted file.
	if !strings.Contains(res.stdout, "shared.go") {
		t.Errorf("expected 'shared.go' in stdout, got: %q", res.stdout)
	}
}

func TestCpGitRunnerDiffNameOnly(t *testing.T) {
	dir := initGitRepo(t)
	// Add a file on a feature branch.
	cpRunGit(t, dir, "checkout", "-b", "feature")
	cpWriteGitFile(t, dir, "a.go", "package main\n")
	cpWriteGitFile(t, dir, "b.go", "package main\n")
	cpRunGit(t, dir, "add", "-A")
	cpRunGit(t, dir, "commit", "-m", "add files")
	cpRunGit(t, dir, "checkout", "main")

	runner := cpGitRunner{}
	out, err := runner.diffNameOnly(dir, "main...feature")
	if err != nil {
		t.Fatalf("diffNameOnly: %v", err)
	}
	files := parseDiffNameOnly(out)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestCpGitRunnerDiffNameOnlyNoChanges(t *testing.T) {
	dir := initGitRepo(t)
	runner := cpGitRunner{}
	out, err := runner.diffNameOnly(dir, "main...main")
	if err != nil {
		t.Fatalf("diffNameOnly: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty diff, got %q", out)
	}
}
