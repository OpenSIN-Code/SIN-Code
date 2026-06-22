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
	// Use a real temp directory with no project markers so the test is
	// independent of whatever happens to live in /tmp on this machine.
	dir := t.TempDir()
	result := detectProjectType(dir)
	if result != "generic" {
		t.Errorf("expected 'generic' for empty temp dir, got %q", result)
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
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	oldArgs := SecurityCmd.Args
	defer func() { SecurityCmd.Args = oldArgs }()
	resetSecurityCmdFlags(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n"), 0644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "go"), "# fake go vet\n", 0)
	os.Setenv("PATH", binDir)

	SecurityCmd.SetArgs([]string{dir})
	SecurityCmd.Flags().Set("type", "go")
	SecurityCmd.Flags().Set("tools", "go vet")
	SecurityCmd.SetOut(new(strings.Builder))
	SecurityCmd.SetErr(new(strings.Builder))
	_ = captureStdout(t)
	if err := SecurityCmd.Execute(); err != nil {
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
