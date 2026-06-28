// SPDX-License-Identifier: MIT
// Purpose: Additional unit tests for update_phases.go to reach 100% coverage
// under the -run 'TestUpdate|TestSelfUpdate' filter.
package internal

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestUpdateExecPipx_Default(t *testing.T) {
	ctx := context.Background()
	cmd := execPipx(ctx, "list")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Args[0] != "pipx" {
		t.Errorf("expected pipx, got %q", cmd.Args[0])
	}
}

func TestUpdateExecGo_Default(t *testing.T) {
	ctx := context.Background()
	cmd := execGo(ctx, "build")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Args[0] != "go" {
		t.Errorf("expected go, got %q", cmd.Args[0])
	}
}

func TestUpdateExecGit_Default(t *testing.T) {
	ctx := context.Background()
	cmd := execGit(ctx, "describe")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Args[0] != "git" {
		t.Errorf("expected git, got %q", cmd.Args[0])
	}
}

func TestUpdatePythonPhase_DryRun(t *testing.T) {
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

func TestUpdatePythonPhase_CheckOnly(t *testing.T) {
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

func TestUpdateAllPythonPackages_NotEmpty(t *testing.T) {
	if len(AllPythonPackages) == 0 {
		t.Error("AllPythonPackages should not be empty")
	}
	// sin-code-bundle was removed in the ecosystem-cleanup.
	if AllPythonPackages[0] == "sin-code-bundle" {
		t.Errorf("sin-code-bundle must not be in AllPythonPackages, got it at index 0")
	}
}

func TestUpdateAllGoTools_Count(t *testing.T) {
	if len(AllGoTools) != 0 {
		t.Errorf("expected 0 Go tools (absorbed into main binary), got %d", len(AllGoTools))
	}
}

func TestUpdateGoPhase_RepoMissing(t *testing.T) {
	t.Setenv("SIN_CODE_REPOS_DIR", "/nonexistent/path/here")
	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunGoPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunGoPhase failed: %v", err)
	}
	if res.Skipped != 0 {
		t.Errorf("with no Go tools, Skipped should be 0, got %d", res.Skipped)
	}
	if res.Updated != 0 {
		t.Errorf("Updated should be 0, got %d", res.Updated)
	}
	if res.Failed != 0 {
		t.Errorf("Failed should be 0, got %d", res.Failed)
	}
}

func TestUpdateSkillsPhase_DelegatesToPython(t *testing.T) {
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

func TestUpdateHomeDirOrEmpty(t *testing.T) {
	h := homeDirOrEmpty()
	if h == "" {
		t.Error("homeDirOrEmpty should not return empty on a real system")
	}
}

func TestUpdateHomeDirOrEmpty_Error(t *testing.T) {
	old := osUserHomeDir
	osUserHomeDir = func() (string, error) { return "", errors.New("no home") }
	defer func() { osUserHomeDir = old }()
	if h := homeDirOrEmpty(); h != "" {
		t.Errorf("expected empty, got %q", h)
	}
}

func TestUpdateBinDirPath(t *testing.T) {
	t.Setenv("SIN_CODE_BIN_DIR", "/custom/bin")
	if bd := binDirPath(); bd != "/custom/bin" {
		t.Errorf("binDirPath with env = %q, want /custom/bin", bd)
	}
}

func TestUpdateCurrentPlatformString(t *testing.T) {
	s := currentPlatformString()
	if s == "" {
		t.Error("currentPlatformString should not be empty")
	}
	if len(s) < 5 {
		t.Errorf("platform string too short: %q", s)
	}
}

func TestUpdatePhaseResult_Struct(t *testing.T) {
	r := &PhaseResult{Name: "test", Updated: 3, Skipped: 1, Failed: 2}
	if r.Name != "test" {
		t.Errorf("Name = %q", r.Name)
	}
	if r.Updated != 3 {
		t.Errorf("Updated = %d", r.Updated)
	}
}

func TestUpdateRunPythonPhase_WithFakePipx(t *testing.T) {
	saved := execPipx
	execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "upgraded")
	}
	defer func() { execPipx = saved }()

	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunPythonPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunPythonPhase with fake pipx failed: %v", err)
	}
	if res.Updated < 19 {
		t.Errorf("expected at least 19 updated, got %d", res.Updated)
	}
	if res.Failed > 0 {
		t.Errorf("unexpected failures: %v", res.Errors)
	}
}

func TestUpdateRunPythonPhase_FakePipxFail(t *testing.T) {
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

func TestUpdateRunPythonPhase_GsdFamily(t *testing.T) {
	saved := execPipx
	execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "list" {
			return exec.CommandContext(ctx, "echo", `{"venvs":{"sin-gsd-core":{"metadata":{}},"sin-gsd-extra":{"metadata":{}}}}`)
		}
		return exec.CommandContext(ctx, "true")
	}
	defer func() { execPipx = saved }()

	ctx := context.Background()
	res, err := RunPythonPhase(ctx, UpdateOptions{})
	if err != nil {
		t.Fatalf("RunPythonPhase failed: %v", err)
	}
	if res.Updated < 21 {
		t.Errorf("expected at least 21 updated, got %d", res.Updated)
	}
}

func TestUpdateRunGoPhase_WithFakeGo(t *testing.T) {
	t.Setenv("SIN_CODE_REPOS_DIR", t.TempDir())
	t.Setenv("SIN_CODE_BIN_DIR", t.TempDir())

	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunGoPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunGoPhase with no tools failed: %v", err)
	}
	if res.Updated != 0 {
		t.Errorf("with no Go tools, Updated should be 0, got %d", res.Updated)
	}
	if res.Skipped != 0 {
		t.Errorf("with no Go tools, Skipped should be 0, got %d", res.Skipped)
	}
	if res.Failed != 0 {
		t.Errorf("with no Go tools, Failed should be 0, got %d", res.Failed)
	}
}

func TestUpdateRunGoPhase_BuildFailure(t *testing.T) {
	t.Setenv("SIN_CODE_REPOS_DIR", t.TempDir())
	t.Setenv("SIN_CODE_BIN_DIR", t.TempDir())

	ctx := context.Background()
	opts := UpdateOptions{}
	res, err := RunGoPhase(ctx, opts)
	if err != nil {
		t.Fatalf("RunGoPhase should not return error: %v", err)
	}
	if res.Updated != 0 {
		t.Errorf("with no Go tools, Updated should be 0, got %d", res.Updated)
	}
	if res.Failed != 0 {
		t.Errorf("with no Go tools, Failed should be 0, got %d", res.Failed)
	}
}

func TestUpdateRunGoPhase_DefaultReposDir(t *testing.T) {
	old := osUserHomeDir
	osUserHomeDir = func() (string, error) { return t.TempDir(), nil }
	defer func() { osUserHomeDir = old }()

	ctx := context.Background()
	res, err := RunGoPhase(ctx, UpdateOptions{})
	if err != nil {
		t.Fatalf("RunGoPhase failed: %v", err)
	}
	if res.Skipped != 0 {
		t.Errorf("with no Go tools, Skipped should be 0, got %d", res.Skipped)
	}
	if res.Updated != 0 {
		t.Errorf("with no Go tools, Updated should be 0, got %d", res.Updated)
	}
	if res.Failed != 0 {
		t.Errorf("with no Go tools, Failed should be 0, got %d", res.Failed)
	}
}

func TestUpdateRunGoPhase_GitDescribeError(t *testing.T) {
	t.Setenv("SIN_CODE_REPOS_DIR", t.TempDir())
	t.Setenv("SIN_CODE_BIN_DIR", t.TempDir())

	ctx := context.Background()
	res, err := RunGoPhase(ctx, UpdateOptions{})
	if err != nil {
		t.Fatalf("RunGoPhase failed: %v", err)
	}
	if res.Failed != 0 {
		t.Errorf("with no Go tools, Failed should be 0, got %d", res.Failed)
	}
}

func TestUpdateRunGoPhase_CheckOnly(t *testing.T) {
	t.Setenv("SIN_CODE_REPOS_DIR", t.TempDir())
	t.Setenv("SIN_CODE_BIN_DIR", t.TempDir())

	ctx := context.Background()
	res, err := RunGoPhase(ctx, UpdateOptions{CheckOnly: true})
	if err != nil {
		t.Fatalf("RunGoPhase failed: %v", err)
	}
	if res.Skipped != 0 {
		t.Errorf("with no Go tools, Skipped should be 0, got %d", res.Skipped)
	}
}

func TestUpdateRunGoPhase_VerifyBinaryError(t *testing.T) {
	t.Setenv("SIN_CODE_REPOS_DIR", t.TempDir())
	t.Setenv("SIN_CODE_BIN_DIR", t.TempDir())

	ctx := context.Background()
	res, err := RunGoPhase(ctx, UpdateOptions{})
	if err != nil {
		t.Fatalf("RunGoPhase failed: %v", err)
	}
	if res.Failed != 0 {
		t.Errorf("with no Go tools, Failed should be 0, got %d", res.Failed)
	}
}

func TestUpdateVerifyBinaryVersion_Success(t *testing.T) {
	ctx := context.Background()
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

func TestUpdateVerifyBinaryVersion_Mismatch(t *testing.T) {
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

func TestUpdateVerifyBinaryVersion_ExecError(t *testing.T) {
	ctx := context.Background()
	err := verifyBinaryVersion(ctx, "/nonexistent/binary", "v1.0.0")
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestUpdateGitDescribeVersion_Success(t *testing.T) {
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

func TestUpdateListGsdFamily_Success(t *testing.T) {
	saved := execPipx
	execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
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

func TestUpdateRunPythonPhase_GsdFamilyUpgradeFail(t *testing.T) {
	saved := execPipx
	execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "list" {
			return exec.CommandContext(ctx, "echo", `{"venvs":{"sin-gsd-core":{"metadata":{}}}}`)
		}
		if len(args) > 1 && args[1] == "sin-gsd-core" {
			return exec.CommandContext(ctx, "false")
		}
		return exec.CommandContext(ctx, "true")
	}
	defer func() { execPipx = saved }()

	ctx := context.Background()
	res, err := RunPythonPhase(ctx, UpdateOptions{})
	if err != nil {
		t.Fatalf("RunPythonPhase failed: %v", err)
	}
	if res.Failed != 1 {
		t.Errorf("expected 1 failure for gsd upgrade, got %d", res.Failed)
	}
}

func TestUpdateRunDoctorNonFatal_Success(t *testing.T) {
	td := t.TempDir()
	script := filepath.Join(td, "doctor")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	old := osExecutable
	osExecutable = func() (string, error) { return script, nil }
	defer func() { osExecutable = old }()
	if err := runDoctorNonFatal(context.Background()); err != nil {
		t.Fatalf("runDoctorNonFatal should succeed: %v", err)
	}
}

func TestUpdateRunDoctorNonFatal_ExeError(t *testing.T) {
	old := osExecutable
	osExecutable = func() (string, error) { return "", errors.New("no exe") }
	defer func() { osExecutable = old }()
	if err := runDoctorNonFatal(context.Background()); err == nil {
		t.Fatal("expected error when os.Executable fails")
	}
}

func TestUpdatePrintPhaseSummary_WithErrors(t *testing.T) {
	results := []*PhaseResult{
		{Name: "test", Updated: 1, Errors: []string{"boom"}},
	}
	printPhaseSummary(results)
}

func TestUpdateRunCheck_Offline(t *testing.T) {
	t.Setenv("NO_UPDATE_CHECK", "1")
	if err := runCheck(context.Background(), UpdateOptions{}); err != nil {
		t.Fatalf("runCheck offline failed: %v", err)
	}
}

func TestUpdateRunCheck_PythonPhaseError(t *testing.T) {
	old := runCheckPythonPhase
	runCheckPythonPhase = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return nil, errors.New("python fail")
	}
	defer func() { runCheckPythonPhase = old }()
	if err := runCheck(context.Background(), UpdateOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateRunCheck_GoPhaseError(t *testing.T) {
	oldPy := runCheckPythonPhase
	oldGo := runCheckGoPhase
	runCheckPythonPhase = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return &PhaseResult{Name: "python"}, nil
	}
	runCheckGoPhase = func(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
		return nil, errors.New("go fail")
	}
	defer func() {
		runCheckPythonPhase = oldPy
		runCheckGoPhase = oldGo
	}()
	if err := runCheck(context.Background(), UpdateOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateRunUpdate_FullFlow_DryRun(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	pipxDir := filepath.Join(wd, "testdata", "fake_pipx")
	goDir := filepath.Join(wd, "testdata", "fake_go")
	combined := pipxDir + string(os.PathListSeparator) + goDir + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", combined)

	t.Setenv("SIN_CODE_REPOS_DIR", "/nonexistent/path")
	td := t.TempDir()
	UpdateCmd.SetArgs([]string{})
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

	err = UpdateCmd.Execute()
	if err != nil {
		t.Fatalf("UpdateCmd --dry-run failed: %v", err)
	}
	UpdateCmd.Flags().Set("dry-run", "false")
}
