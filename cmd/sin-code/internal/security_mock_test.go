// SPDX-License-Identifier: MIT
// Purpose: Mock external security tools to cover tool-runner execution paths. (st-cov1)
package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeFakeSecurityTool creates an executable shell script at the given path.
func makeFakeSecurityTool(t *testing.T, path, output string, exit int) {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\nexit %d\n", output, exit)
	os.WriteFile(path, []byte(script), 0o755)
}

func fakeBinDir(t *testing.T, tools map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, output := range tools {
		makeFakeSecurityTool(t, filepath.Join(dir, name), output, 0)
	}
	return dir
}

func TestSecurityFake_WithFakeGosec(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	binDir := fakeBinDir(t, map[string]string{
		"gosec": `{"Issues": [{"severity": "HIGH"}, {"severity": "MEDIUM"}]}`,
	})
	// Make fake gosec exit non-zero so runner treats it as issues found.
	makeFakeSecurityTool(t, filepath.Join(binDir, "gosec"), `{"Issues": [{"severity": "HIGH"}, {"severity": "MEDIUM"}]}`, 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "go", "gosec", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "gosec" && tr.Status == "issues" && tr.Issues >= 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fake gosec to report issues, got %+v", res.Tools)
	}
}

func TestSecurityFake_WithFakeBandit(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "bandit"), `{"results": [{"issue_severity": "HIGH"}, {"issue_severity": "MEDIUM"}]}`, 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "python", "bandit", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "bandit" && tr.Status == "issues" && tr.Issues >= 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fake bandit to report issues, got %+v", res.Tools)
	}
}

func TestSecurityFake_WithFakeNpm(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "npm"), `{"vulnerabilities": {"a": {"via": "V1"}, "b": {"via": "V2"}}}`, 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "node", "npm audit", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "npm audit" && tr.Status == "issues" && tr.Issues >= 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fake npm audit to report issues, got %+v", res.Tools)
	}
}

func TestSecurityFake_WithFakeGovulncheck(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	binDir := t.TempDir()
	// govulncheck reports issues by counting substrings in stdout even on exit 0.
	makeFakeSecurityTool(t, filepath.Join(binDir, "govulncheck"), `Vulnerability #12345: os/exec
GO-2024-1234`, 0)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "go", "govulncheck", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "govulncheck" && tr.Status == "issues" && tr.Issues >= 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fake govulncheck to report issues, got %+v", res.Tools)
	}
}

func TestSecurityFake_WithFakeSafety(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "safety"), `{"vulnerabilities": [{"vulnerability_id": "V1"}]}`, 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "python", "safety", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "safety" && tr.Status == "issues" && tr.Issues >= 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fake safety to report issues, got %+v", res.Tools)
	}
}

func TestSecurityFake_ToolErrorWithoutPattern(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "gosec"), "boom", 1)
	makeFakeSecurityTool(t, filepath.Join(binDir, "go"), "boom", 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "go", "gosec,go vet", 5)
	haveError := false
	haveGosecError := false
	for _, tr := range res.Tools {
		if tr.Status == "error" {
			haveError = true
		}
		if tr.Name == "gosec" && tr.Status == "error" {
			haveGosecError = true
		}
	}
	if !haveError || !haveGosecError {
		t.Errorf("expected gosec and go vet to report errors, got %+v", res.Tools)
	}
}

func TestSecurityFake_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	// Create an executable file to exercise the executable branch.
	execFile := filepath.Join(dir, "script.sh")
	os.WriteFile(execFile, []byte("#!/bin/sh\n"), 0o755)
	// Add a world-writable file to exercise the world-writable branch.
	ww := filepath.Join(dir, "worldwritable.txt")
	os.WriteFile(ww, []byte("x"), 0o666)

	res := runSecurityScan(dir, "generic", "file permissions", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "file permissions" && tr.Status == "ok" && strings.Contains(tr.Output, "executable") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected file permissions to report executable files, got %+v", res.Tools)
	}
}

func TestSecurityFake_ToolOkPath(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "gosec"), "no issues", 0)
	makeFakeSecurityTool(t, filepath.Join(binDir, "go"), "vet ok", 0)
	makeFakeSecurityTool(t, filepath.Join(binDir, "govulncheck"), "no vulns", 0)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "go", "", 5)
	okCount := 0
	for _, tr := range res.Tools {
		if tr.Status == "ok" {
			okCount++
		}
	}
	if okCount < 1 {
		t.Errorf("expected at least one ok status, got %+v", res.Tools)
	}
}

func TestSecurityFake_GosecNotFound(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", "")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	res := runSecurityScan(dir, "go", "gosec", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "gosec" && tr.Status == "not_found" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected gosec not_found status, got %+v", res.Tools)
	}
}

func TestSecurityFake_BanditNotFound(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", "")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644)

	res := runSecurityScan(dir, "python", "bandit", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "bandit" && tr.Status == "not_found" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bandit not_found status, got %+v", res.Tools)
	}
}

func TestSecurityFake_SafetyOk(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "safety"), `{"vulnerabilities": []}`, 0)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "python", "safety", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "safety" && tr.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected safety ok status, got %+v", res.Tools)
	}
}

func TestSecurityFake_NpmAuditNotFound(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", "")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644)

	res := runSecurityScan(dir, "node", "npm audit", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "npm audit" && tr.Status == "not_found" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected npm audit not_found status, got %+v", res.Tools)
	}
}

func TestSecurityFake_GoVetNotFound(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", "")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	res := runSecurityScan(dir, "go", "go vet", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "go vet" && tr.Status == "not_found" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected go vet not_found status, got %+v", res.Tools)
	}
}

func TestSecurityFake_GovulncheckNotFound(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", "")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	res := runSecurityScan(dir, "go", "govulncheck", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "govulncheck" && tr.Status == "not_found" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected govulncheck not_found status, got %+v", res.Tools)
	}
}

func TestSecurityFake_FilePermissionsError(t *testing.T) {
	res := runSecurityScan("/nonexistent/path/for/permissions", "generic", "file permissions", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "file permissions" && tr.Status == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected file permissions error status, got %+v", res.Tools)
	}
}

func TestSecurityCmd_AutoDetect(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "go"), "# fake go vet\n", 0)
	os.Setenv("PATH", binDir)

	oldArgs := SecurityCmd.Args
	defer func() { SecurityCmd.Args = oldArgs }()
	resetSecurityCmdFlags(t)

	SecurityCmd.SetArgs([]string{dir})
	SecurityCmd.Flags().Set("type", "auto")
	SecurityCmd.Flags().Set("tools", "go vet")
	SecurityCmd.Flags().Set("format", "json")
	SecurityCmd.Flags().Set("strict", "false")
	SecurityCmd.SetOut(new(strings.Builder))
	SecurityCmd.SetErr(new(strings.Builder))
	_ = captureStdout(t)
	if err := SecurityCmd.Execute(); err != nil {
		t.Fatalf("security auto detect failed: %v", err)
	}
}

func TestSecurityFake_GovulncheckError(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "govulncheck"), "govulncheck failed", 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "go", "govulncheck", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "govulncheck" && tr.Status == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected govulncheck error status, got %+v", res.Tools)
	}
}

func TestSecurityFake_GoVetError(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "go"), "", 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "go", "go vet", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "go vet" && tr.Status == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected go vet error status, got %+v", res.Tools)
	}
}

func TestSecurityFake_BanditError(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "bandit"), "bandit error", 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "python", "bandit", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "bandit" && tr.Status == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bandit error status, got %+v", res.Tools)
	}
}

func TestSecurityFake_BanditOk(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "bandit"), `{"results": []}`, 0)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "python", "bandit", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "bandit" && tr.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bandit ok status, got %+v", res.Tools)
	}
}

func TestSecurityFake_SafetyNotFound(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", "")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644)

	res := runSecurityScan(dir, "python", "safety", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "safety" && tr.Status == "not_found" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected safety not_found status, got %+v", res.Tools)
	}
}

func TestSecurityFake_SafetyError(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hi')\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "safety"), "safety crashed", 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "python", "safety", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "safety" && tr.Status == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected safety error status, got %+v", res.Tools)
	}
}

func TestSecurityFake_NpmAuditOk(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "npm"), `{"vulnerabilities": {}}`, 0)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "node", "npm audit", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "npm audit" && tr.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected npm audit ok status, got %+v", res.Tools)
	}
}

func TestSecurityFake_FilePermissionsRootUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("chmod-based unreadable root test is Unix/non-root specific")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)

	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)

	res := runSecurityScan(dir, "generic", "file permissions", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "file permissions" && tr.Status == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected file permissions error status for unreadable root, got %+v", res.Tools)
	}
}

func TestSecurityFake_FilePermissionsSkipDir(t *testing.T) {
	dir := t.TempDir()

	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(gitDir, "hook"), []byte("#!/bin/sh\n"), 0o755)

	nodeDir := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(nodeDir, "bin.js"), []byte("x"), 0o755)

	res := runSecurityScan(dir, "generic", "file permissions", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "file permissions" && tr.Status == "ok" && strings.Contains(tr.Output, "No executable files found") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected .git and node_modules to be skipped, got %+v", res.Tools)
	}
}

func TestSecurityFake_FilePermissionsInfoError(t *testing.T) {
	old := dirEntryInfo
	defer func() { dirEntryInfo = old }()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644)

	dirEntryInfo = func(d fs.DirEntry) (fs.FileInfo, error) {
		if d.Name() == "a.txt" {
			return nil, fmt.Errorf("info error")
		}
		return d.Info()
	}

	res := runSecurityScan(dir, "generic", "file permissions", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "file permissions" && tr.Status == "ok" && strings.Contains(tr.Output, "No executable files found") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected file permissions ok after skipping Info-error entry, got %+v", res.Tools)
	}
}

func TestSecurityFake_FilePermissionsWorldWritable(t *testing.T) {
	dir := t.TempDir()
	ww := filepath.Join(dir, "worldwritable.txt")
	os.WriteFile(ww, []byte("x"), 0o644)
	if err := os.Chmod(ww, 0o666); err != nil {
		t.Fatal(err)
	}

	res := runSecurityScan(dir, "generic", "file permissions", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "file permissions" && tr.Status == "ok" && strings.Contains(tr.Output, "world-writable") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected file permissions to report world-writable file, got %+v", res.Tools)
	}
}

func TestSecurityFake_NpmAuditError(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "npm"), "npm audit failed", 1)
	os.Setenv("PATH", binDir)

	res := runSecurityScan(dir, "node", "npm audit", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "npm audit" && tr.Status == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected npm audit error status, got %+v", res.Tools)
	}
}

func TestSecurityFake_SecretsGrepOk(t *testing.T) {
	dir := t.TempDir()
	res := runSecurityScan(dir, "generic", "secrets grep", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "secrets grep" && tr.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected secrets grep ok status, got %+v", res.Tools)
	}
}

func TestSecurity_DetectNodeProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o644)
	if got := detectProjectType(dir); got != "node" {
		t.Errorf("expected 'node', got %q", got)
	}
}

func TestSecurity_DetectPythonProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask==2.0\n"), 0o644)
	if got := detectProjectType(dir); got != "python" {
		t.Errorf("expected 'python', got %q", got)
	}
}

func TestSecurityFake_FilePermissionsSkipUnreadableEntry(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("chmod-based unreadable entry test is Unix/non-root specific")
	}

	dir := t.TempDir()
	sub := filepath.Join(dir, "unreadable")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(sub, "a.txt"), []byte("x"), 0o644)
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(sub, 0o755)

	res := runSecurityScan(dir, "generic", "file permissions", 5)
	found := false
	for _, tr := range res.Tools {
		if tr.Name == "file permissions" && tr.Status == "ok" && strings.Contains(tr.Output, "No executable files found") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected file permissions ok after skipping unreadable entry, got %+v", res.Tools)
	}
}

func TestSecurity_CountLinesSimple(t *testing.T) {
	if got := countLinesSimple("a\nb\nc"); got != 3 {
		t.Errorf("countLinesSimple(a\\nb\\nc) = %d, want 3", got)
	}
	if got := countLinesSimple(""); got != 1 {
		t.Errorf("countLinesSimple(\"\") = %d, want 1", got)
	}
}

func TestSecurityFake_PrintError(t *testing.T) {
	r := SecurityResult{
		ProjectType: "go",
		Path:        "/tmp",
		Duration:    1,
		Tools:       []ToolResult{{Name: "go vet", Status: "error", Error: "boom", Duration: "1ms"}},
		Summary:     SecuritySummary{ToolsRun: 1, Errors: 1},
	}
	out := captureSecurityPrint(t, r)
	if !strings.Contains(out, "ERROR") {
		t.Errorf("expected output to contain ERROR, got %q", out)
	}
}

func TestSecurityFake_PrintNotFound(t *testing.T) {
	r := SecurityResult{
		ProjectType: "go",
		Path:        "/tmp",
		Duration:    1,
		Tools:       []ToolResult{{Name: "govulncheck", Status: "not_found", Duration: "1ms"}},
		Summary:     SecuritySummary{NotFound: 1},
	}
	out := captureSecurityPrint(t, r)
	if !strings.Contains(out, "not installed") {
		t.Errorf("expected output to contain 'not installed', got %q", out)
	}
}

func TestSecurityFake_PrintNonStrictIssues(t *testing.T) {
	r := SecurityResult{
		ProjectType: "generic",
		Path:        "/tmp",
		Duration:    1,
		Tools:       []ToolResult{{Name: "secrets grep", Status: "issues", Issues: 1, Duration: "1ms"}},
		Summary:     SecuritySummary{ToolsRun: 1, Issues: 1},
	}
	out := captureSecurityPrint(t, r)
	if !strings.Contains(out, "review recommended") {
		t.Errorf("expected output to contain 'review recommended', got %q", out)
	}
}
