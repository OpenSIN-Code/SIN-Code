package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityCmd_DetectGoProject(t *testing.T) {
	// Build a synthetic Go project instead of hardcoding a developer's
	// machine-specific checkout path: the test must pass everywhere.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result := detectProjectType(dir)
	if result != "go" {
		t.Errorf("expected 'go' for dir with go.mod, got %q", result)
	}
}

func TestSecurityCmd_DetectGenericProject(t *testing.T) {
	result := detectProjectType("/tmp")
	if result != "generic" {
		t.Errorf("expected 'generic' for /tmp, got %q", result)
	}
}

func TestSecurityCmd_ParseToolFilter(t *testing.T) {
	m := parseToolFilter("govulncheck,gosec")
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if !m["govulncheck"] {
		t.Error("expected govulncheck in filter")
	}
	if !m["gosec"] {
		t.Error("expected gosec in filter")
	}
	if m["bandit"] {
		t.Error("did not expect bandit in filter")
	}
}

func TestSecurityCmd_ParseToolFilterEmpty(t *testing.T) {
	m := parseToolFilter("")
	if m != nil {
		t.Error("expected nil for empty filter")
	}
}

func TestSecurityCmd_RunGoProject(t *testing.T) {
	// Run security scan on the current project (Go)
	SecurityCmd.SetArgs([]string{".", "--type", "go", "--tools", "go vet"})
	SecurityCmd.SetOut(new(strings.Builder))
	SecurityCmd.SetErr(new(strings.Builder))
	err := SecurityCmd.Execute()
	if err != nil {
		t.Fatalf("security command failed: %v", err)
	}
}

func TestSecurityCmd_FileExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(existing, []byte("module m\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(existing) {
		t.Error("expected freshly created go.mod to exist")
	}
	if fileExists(filepath.Join(dir, "missing", "file.txt")) {
		t.Error("expected nonexistent file to not exist")
	}
}
