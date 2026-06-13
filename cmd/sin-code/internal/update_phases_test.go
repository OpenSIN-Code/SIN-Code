// SPDX-License-Identifier: MIT
// Purpose: Unit tests for update phases (Python/Go/Skills).
package internal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func withFakeBinaries(t *testing.T) {
	t.Helper()
	// Reset execPipx to use testdata fake
	execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "pipx", args...)
	}
	execGo = func(ctx context.Context, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "go", args...)
	}
	execGit = func(ctx context.Context, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "git", args...)
	}
}

func TestRunPythonPhase_DryRun(t *testing.T) {
	withFakeBinaries(t)
	ctx := context.Background()
	opts := UpdateOptions{DryRun: true}
	res, err := RunPythonPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunPythonPhase(dry-run) failed: %v", err)
	}
	if res.Updated != 0 {
		t.Errorf("dry-run should have Updated=0, got %d", res.Updated)
	}
	if res.Skipped == 0 {
		t.Error("dry-run should have Skipped > 0")
	}
}

func TestRunPythonPhase_CheckOnly(t *testing.T) {
	withFakeBinaries(t)
	ctx := context.Background()
	opts := UpdateOptions{CheckOnly: true}
	res, err := RunPythonPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunPythonPhase(check) failed: %v", err)
	}
	if res.Updated != 0 {
		t.Errorf("check should have Updated=0, got %d", res.Updated)
	}
}

func TestAllPythonPackages_NotEmpty(t *testing.T) {
	if len(AllPythonPackages) == 0 {
		t.Error("AllPythonPackages should not be empty")
	}
	// verify sin-code-bundle is first
	if AllPythonPackages[0] != "sin-code-bundle" {
		t.Errorf("first package should be sin-code-bundle, got %s", AllPythonPackages[0])
	}
}

func TestAllGoTools_Count(t *testing.T) {
	if len(AllGoTools) != 7 {
		t.Errorf("expected 7 Go tools, got %d", len(AllGoTools))
	}
	names := make(map[string]bool)
	for _, tool := range AllGoTools {
		names[tool.Name] = true
	}
	expected := []string{"discover", "execute", "map", "grasp", "scout", "harvest", "orchestrate"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing Go tool: %s", name)
		}
	}
}

func TestRunGoPhase_RepoMissing(t *testing.T) {
	withFakeBinaries(t)
	t.Setenv("SIN_CODE_REPOS_DIR", "/nonexistent/path/here")
	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunGoPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunGoPhase failed: %v", err)
	}
	if res.Skipped != 7 {
		t.Errorf("all 7 should be skipped when repos missing, got Skipped=%d", res.Skipped)
	}
	if res.Updated != 0 {
		t.Errorf("Updated should be 0 when repos missing, got %d", res.Updated)
	}
}

func TestRunSkillsPhase_DelegatesToPython(t *testing.T) {
	withFakeBinaries(t)
	ctx := context.Background()
	opts := UpdateOptions{DryRun: true}
	res, err := RunSkillsPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunSkillsPhase failed: %v", err)
	}
	if res.Updated != 0 {
		t.Errorf("dry-run skills should have Updated=0, got %d", res.Updated)
	}
	if res.Skipped == 0 {
		t.Error("dry-run skills should have Skipped > 0")
	}
}

func TestHomeDirOrEmpty(t *testing.T) {
	h := homeDirOrEmpty()
	if h == "" {
		t.Error("homeDirOrEmpty should not return empty on a real system")
	}
}

func TestBinDirPath(t *testing.T) {
	t.Setenv("SIN_CODE_BIN_DIR", "/custom/bin")
	if bd := binDirPath(); bd != "/custom/bin" {
		t.Errorf("binDirPath with env = %q, want /custom/bin", bd)
	}
}

func TestCurrentPlatformString(t *testing.T) {
	s := currentPlatformString()
	if s == "" {
		t.Error("currentPlatformString should not be empty")
	}
	// format should be "os/arch"
	if len(s) < 5 {
		t.Errorf("platform string too short: %q", s)
	}
}

func TestPhaseResult_Struct(t *testing.T) {
	r := &PhaseResult{Name: "test", Updated: 3, Skipped: 1, Failed: 2}
	if r.Name != "test" {
		t.Errorf("Name = %q", r.Name)
	}
	if r.Updated != 3 {
		t.Errorf("Updated = %d", r.Updated)
	}
}


func setFakePath(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	pipxDir := filepath.Join(wd, "testdata", "fake_pipx")
	goDir := filepath.Join(wd, "testdata", "fake_go")
	combined := pipxDir + string(os.PathListSeparator) + goDir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", combined)
}

func TestRunPythonPhase_WithFakePipx(t *testing.T) {
	// Mock execPipx to return a successful fake
	saved := execPipx
	execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
		// Return a command that just succeeds (fake upgrade)
		return exec.CommandContext(ctx, "echo", "upgraded")
	}
	defer func() { execPipx = saved }()

	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunPythonPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunPythonPhase with fake pipx failed: %v", err)
	}
	// 20 packages should be "upgraded"
	if res.Updated < 20 {
		t.Errorf("expected at least 20 updated, got %d", res.Updated)
	}
	if res.Failed > 0 {
		t.Errorf("unexpected failures: %v", res.Errors)
	}
}

func TestRunPythonPhase_FakePipxFail(t *testing.T) {
	saved := execPipx
	execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	}
	defer func() { execPipx = saved }()

	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunPythonPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunPythonPhase should not return error on pipx fail: %v", err)
	}
	if res.Failed < 1 {
		t.Errorf("expected at least 1 failure, got Failed=%d", res.Failed)
	}
}

func TestRunGoPhase_WithFakeGo(t *testing.T) {
	td := t.TempDir()
	t.Setenv("SIN_CODE_REPOS_DIR", td)
	binDir := t.TempDir()
	t.Setenv("SIN_CODE_BIN_DIR", binDir)

	repoPath := filepath.Join(td, "SIN-Code-Discover-Tool")
	cmdPath := filepath.Join(repoPath, "cmd", "discover")
	if err := os.MkdirAll(cmdPath, 0755); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	savedExecGit := execGit
	execGit = func(ctx context.Context, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "describe" {
			return exec.CommandContext(ctx, "echo", "v1.2.0-3-g4c5a78d")
		}
		return savedExecGit(ctx, args...)
	}
	defer func() { execGit = savedExecGit }()

	savedExecGo := execGo
	execGo = func(ctx context.Context, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "build" {
			// Extract the -o output path and create a fake binary
			var outPath string
			for i, a := range args {
				if a == "-o" && i+1 < len(args) {
					outPath = args[i+1]
				}
			}
			if outPath != "" {
				// Create a script that runs the verifyBinaryVersion check
				script := fmt.Sprintf("#!/bin/sh\necho v1.2.0-3-g4c5a78d\n")
				os.WriteFile(outPath, []byte(script), 0755)
			}
			return exec.CommandContext(ctx, "true")
		}
		return savedExecGo(ctx, args...)
	}
	defer func() { execGo = savedExecGo }()

	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunGoPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunGoPhase with fake go failed: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("expected 1 updated (discover), got Updated=%d (Failed=%d, Skipped=%d, Errors=%v)", res.Updated, res.Failed, res.Skipped, res.Errors)
	}
	if res.Skipped != 6 {
		t.Errorf("expected 6 skipped (missing repos), got Skipped=%d", res.Skipped)
	}
}

func TestRunGoPhase_BuildFailure(t *testing.T) {
	setFakePath(t)
	td := t.TempDir()
	t.Setenv("SIN_CODE_REPOS_DIR", td)
	t.Setenv("SIN_CODE_BIN_DIR", t.TempDir())

	repoPath := filepath.Join(td, "SIN-Code-Discover-Tool")
	cmdPath := filepath.Join(repoPath, "cmd", "discover")
	if err := os.MkdirAll(cmdPath, 0755); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	savedExecGit := execGit
	execGit = func(ctx context.Context, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "describe" {
			cmd := exec.CommandContext(ctx, "echo", "v1.2.0")
			cmd.Dir = repoPath
			return cmd
		}
		return savedExecGit(ctx, args...)
	}
	defer func() { execGit = savedExecGit }()

	savedExecGo := execGo
	execGo = func(ctx context.Context, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "build" {
			return exec.CommandContext(ctx, "false")
		}
		return savedExecGo(ctx, args...)
	}
	defer func() { execGo = savedExecGo }()

	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunGoPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunGoPhase should not return error on build failure: %v", err)
	}
	if res.Updated != 0 {
		t.Errorf("all builds should fail, got Updated=%d", res.Updated)
	}
	if res.Failed != 1 {
		t.Errorf("expected 1 failure, got Failed=%d", res.Failed)
	}
}

func TestVerifyBinaryVersion_Success(t *testing.T) {
	// Test verifyBinaryVersion with echo as the "binary"
	ctx := context.Background()
	// echo will output nothing useful, so we use a script
	td := t.TempDir()
	script := filepath.Join(td, "testbin")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho v1.2.0"), 0755); err != nil {
		t.Fatal(err)
	}
	err := verifyBinaryVersion(ctx, script, "v1.2.0")
	if err != nil {
		t.Errorf("verifyBinaryVersion should succeed: %v", err)
	}
}

func TestVerifyBinaryVersion_Mismatch(t *testing.T) {
	ctx := context.Background()
	td := t.TempDir()
	script := filepath.Join(td, "testbin2")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho v2.0.0"), 0755); err != nil {
		t.Fatal(err)
	}
	err := verifyBinaryVersion(ctx, script, "v1.2.0")
	if err == nil {
		t.Error("verifyBinaryVersion should fail on version mismatch")
	}
}

func TestGitDescribeVersion_Success(t *testing.T) {
	ctx := context.Background()
	savedExecGit := execGit
	execGit = func(ctx context.Context, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "v1.2.0-3-g4c5a78d")
	}
	defer func() { execGit = savedExecGit }()

	version, err := gitDescribeVersion(ctx, "/tmp")
	if err != nil {
		t.Fatalf("gitDescribeVersion failed: %v", err)
	}
	if version != "v1.2.0-3-g4c5a78d" {
		t.Errorf("version = %q, want v1.2.0-3-g4c5a78d", version)
	}
}

func TestListGsdFamily_Success(t *testing.T) {
	saved := execPipx
	execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
		// Return JSON with one sin-gsd-test package
		return exec.CommandContext(ctx, "echo", `{"venvs":{"sin-gsd-test":{"metadata":{}}}}`) 
	}
	defer func() { execPipx = saved }()

	ctx := context.Background()
	pkgs, err := listGsdFamily(ctx)
	if err != nil {
		t.Fatalf("listGsdFamily failed: %v", err)
	}
	if len(pkgs) != 1 {
		t.Errorf("expected 1 gsd package, got %d", len(pkgs))
	}
	if pkgs[0] != "sin-gsd-test" {
		t.Errorf("expected sin-gsd-test, got %s", pkgs[0])
	}
}

func TestRunUpdate_FullFlow_DryRun(t *testing.T) {
	setFakePath(t)
	t.Setenv("SIN_CODE_REPOS_DIR", "/nonexistent/path")
	td := t.TempDir()
	UpdateCmd.Flags().Set("rollback", "false")
	UpdateCmd.Flags().Set("python-only", "false")
	UpdateCmd.Flags().Set("go-only", "false")
	UpdateCmd.Flags().Set("skills-only", "false")
	UpdateCmd.Flags().Set("check", "false")
	UpdateCmd.Flags().Set("dry-run", "true")
	UpdateCmd.Flags().Set("force", "false")
	UpdateCmd.Flags().Set("skip-doctor", "true")
	UpdateCmd.Flags().Set("state-root", td)
	UpdateCmd.Flags().Set("keep-snapshots", "10")

	err := UpdateCmd.Execute()
	if err != nil {
		t.Fatalf("UpdateCmd --dry-run failed: %v", err)
	}
	UpdateCmd.Flags().Set("dry-run", "false")
}
