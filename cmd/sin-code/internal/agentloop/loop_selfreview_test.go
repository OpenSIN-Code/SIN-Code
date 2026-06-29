// SPDX-License-Identifier: MIT
// Purpose: tests for the SelfReviewReflector — automatic adversarial
// self-review that scans changed files for incomplete-work markers.
package agentloop

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestScanContent_DetectsTODO verifies that the scanner finds TODO markers.
func TestScanContent_DetectsTODO(t *testing.T) {
	content := "package main\n\nfunc foo() {\n\t// TODO: implement this\n}\n"
	issues := scanContent("main.go", content)
	if len(issues) == 0 {
		t.Fatal("expected issues for TODO marker, got none")
	}
	found := false
	for _, is := range issues {
		if contains(is, "TODO marker") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'TODO marker' in issues, got: %v", issues)
	}
}

// TestScanContent_DetectsMultipleMarkers verifies multiple marker types.
func TestScanContent_DetectsMultipleMarkers(t *testing.T) {
	content := `package main
// TODO: do this
// FIXME: broken
var x = "dummy data"
// HACK: workaround
`
	issues := scanContent("test.go", content)
	if len(issues) < 3 {
		t.Errorf("expected at least 3 issues, got %d: %v", len(issues), issues)
	}
}

// TestScanContent_CleanCodeReturnsNoIssues verifies no false positives.
func TestScanContent_CleanCodeReturnsNoIssues(t *testing.T) {
	content := `package main
import "fmt"
func main() {
	fmt.Println("hello world")
}
`
	issues := scanContent("main.go", content)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for clean code, got %d: %v", len(issues), issues)
	}
}

// TestScanContent_DebugPrints verifies debug prints are caught.
func TestScanContent_DebugPrints(t *testing.T) {
	content := "console.log('debug here')\n"
	issues := scanContent("app.js", content)
	if len(issues) == 0 {
		t.Error("expected issue for console.log, got none")
	}
}

// TestIsExcludedFile verifies binary/generated files are skipped.
func TestIsExcludedFile(t *testing.T) {
	tests := []struct {
		path     string
		excluded bool
	}{
		{"foo.png", true},
		{"foo.jpg", true},
		{"foo.go", false},
		{"foo.py", false},
		{"foo.js", false},
		{"package-lock.json", true},
		{"go.sum", true},
		{"main.ts", false},
	}
	for _, tt := range tests {
		got := isExcludedFile(tt.path)
		if got != tt.excluded {
			t.Errorf("isExcludedFile(%q) = %v, want %v", tt.path, got, tt.excluded)
		}
	}
}

// TestNewSelfReviewReflector_NoGitRepo verifies graceful degradation
// when the workspace is not a git repo.
func TestNewSelfReviewReflector_NoGitRepo(t *testing.T) {
	tmp := t.TempDir()
	reflector := NewSelfReviewReflector(SelfReviewConfig{
		Workspace: tmp,
	})
	ref := reflector(nil, StopSnapshot{})
	if len(ref.Issues) != 0 {
		t.Errorf("expected 0 issues in non-git dir, got %d: %v", len(ref.Issues), ref.Issues)
	}
}

// TestNewSelfReviewReflector_DetectsTODOInGitRepo creates a real git repo,
// writes a file with a TODO, commits it, then modifies it and verifies
// the reflector catches the marker.
func TestNewSelfReviewReflector_DetectsTODOInGitRepo(t *testing.T) {
	tmp := t.TempDir()
	srInitGitRepo(t, tmp)

	// Write initial file and commit.
	srWriteFile(t, filepath.Join(tmp, "main.go"), "package main\nfunc main() {}\n")
	srRunGit(t, tmp, "add", "-A")
	srRunGit(t, tmp, "commit", "-m", "initial")

	// Modify with a TODO marker.
	srWriteFile(t, filepath.Join(tmp, "main.go"), "package main\nfunc main() {\n\t// TODO: implement\n}\n")

	reflector := NewSelfReviewReflector(SelfReviewConfig{
		Workspace: tmp,
	})
	ref := reflector(nil, StopSnapshot{})
	if len(ref.Issues) == 0 {
		t.Fatal("expected issues for TODO in git repo, got none")
	}
	found := false
	for _, is := range ref.Issues {
		if contains(is, "TODO marker") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'TODO marker' in issues, got: %v", ref.Issues)
	}
}

// TestNewSelfReviewReflector_CleanFileReturnsNoIssues verifies that a
// modified file without markers produces no issues.
func TestNewSelfReviewReflector_CleanFileReturnsNoIssues(t *testing.T) {
	tmp := t.TempDir()
	srInitGitRepo(t, tmp)

	srWriteFile(t, filepath.Join(tmp, "main.go"), "package main\nfunc main() {}\n")
	srRunGit(t, tmp, "add", "-A")
	srRunGit(t, tmp, "commit", "-m", "initial")

	// Modify cleanly — no markers.
	srWriteFile(t, filepath.Join(tmp, "main.go"), "package main\nfunc main() {\n\tprintln(\"hello\")\n}\n")

	reflector := NewSelfReviewReflector(SelfReviewConfig{
		Workspace: tmp,
	})
	ref := reflector(nil, StopSnapshot{})
	if len(ref.Issues) != 0 {
		t.Errorf("expected 0 issues for clean change, got %d: %v", len(ref.Issues), ref.Issues)
	}
}

// TestNewSelfReviewReflector_MaxIssuesCap verifies the MaxIssues cap.
func TestNewSelfReviewReflector_MaxIssuesCap(t *testing.T) {
	tmp := t.TempDir()
	srInitGitRepo(t, tmp)

	// Create initial file.
	srWriteFile(t, filepath.Join(tmp, "main.go"), "package main\nfunc main() {}\n")
	srRunGit(t, tmp, "add", "-A")
	srRunGit(t, tmp, "commit", "-m", "initial")

	// Modify with many TODOs.
	content := "package main\n"
	for i := 0; i < 20; i++ {
		content += "// TODO: item " + itoa(i) + "\n"
	}
	srWriteFile(t, filepath.Join(tmp, "main.go"), content)

	reflector := NewSelfReviewReflector(SelfReviewConfig{
		Workspace: tmp,
		MaxIssues: 3,
	})
	ref := reflector(nil, StopSnapshot{})
	if len(ref.Issues) > 3 {
		t.Errorf("expected at most 3 issues (MaxIssues cap), got %d", len(ref.Issues))
	}
}

// --- helpers (sr-prefixed to avoid conflicts with other test files) ---

func srInitGitRepo(t *testing.T, dir string) {
	t.Helper()
	srRunGit(t, dir, "init")
	srRunGit(t, dir, "config", "user.email", "test@test.com")
	srRunGit(t, dir, "config", "user.name", "Test")
}

func srRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
}

func srWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
