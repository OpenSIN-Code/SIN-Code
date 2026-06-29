// SPDX-License-Identifier: MIT
// Purpose: tests for the `sin-code doctor` check functions. Tests the
// pure-logic check functions, not the cobra command wrapper.
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
)

// restoreHook restores a func() string hook to its original value.
func restoreHook(old func() string) func() string { return old }

// ── parseGoVersion ─────────────────────────────────────────────────────

func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{"go version go1.23.4 darwin/arm64", 1, 23, true},
		{"go version go1.26.4 linux/amd64", 1, 26, true},
		{"go version go2.0.0 linux/amd64", 2, 0, true},
		{"go version go1.22.0 darwin/arm64", 1, 22, true},
		{"not a version string", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, tc := range tests {
		major, minor, ok := parseGoVersion(tc.input)
		if ok != tc.wantOK {
			t.Errorf("parseGoVersion(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
		}
		if ok && (major != tc.wantMajor || minor != tc.wantMinor) {
			t.Errorf("parseGoVersion(%q) = (%d, %d), want (%d, %d)",
				tc.input, major, minor, tc.wantMajor, tc.wantMinor)
		}
	}
}

// ── checkGoToolchain ───────────────────────────────────────────────────

func TestCheckGoToolchain_Pass(t *testing.T) {
	old := doctorGoVersionHook
	t.Cleanup(func() { doctorGoVersionHook = old })
	doctorGoVersionHook = func() (string, error) {
		return "go version go1.26.4 darwin/arm64", nil
	}
	r := checkGoToolchain()
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckGoToolchain_TooOld(t *testing.T) {
	old := doctorGoVersionHook
	t.Cleanup(func() { doctorGoVersionHook = old })
	doctorGoVersionHook = func() (string, error) {
		return "go version go1.22.0 darwin/arm64", nil
	}
	r := checkGoToolchain()
	if r.Status != statusFail {
		t.Fatalf("expected fail, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckGoToolchain_NotInstalled(t *testing.T) {
	old := doctorGoVersionHook
	t.Cleanup(func() { doctorGoVersionHook = old })
	doctorGoVersionHook = func() (string, error) {
		return "", os.ErrNotExist
	}
	r := checkGoToolchain()
	if r.Status != statusFail {
		t.Fatalf("expected fail, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckGoToolchain_Unparseable(t *testing.T) {
	old := doctorGoVersionHook
	t.Cleanup(func() { doctorGoVersionHook = old })
	doctorGoVersionHook = func() (string, error) {
		return "garbage output", nil
	}
	r := checkGoToolchain()
	if r.Status != statusFail {
		t.Fatalf("expected fail, got %s: %s", r.Status, r.Detail)
	}
}

// ── checkSinCodeBinary ─────────────────────────────────────────────────

func TestCheckSinCodeBinary_Dev(t *testing.T) {
	old := doctorSinCodeVersionHook
	t.Cleanup(func() { doctorSinCodeVersionHook = restoreHook(old) })
	doctorSinCodeVersionHook = func() string { return "dev" }
	r := checkSinCodeBinary()
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckSinCodeBinary_Empty(t *testing.T) {
	old := doctorSinCodeVersionHook
	t.Cleanup(func() { doctorSinCodeVersionHook = restoreHook(old) })
	doctorSinCodeVersionHook = func() string { return "" }
	r := checkSinCodeBinary()
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckSinCodeBinary_Versioned(t *testing.T) {
	old := doctorSinCodeVersionHook
	t.Cleanup(func() { doctorSinCodeVersionHook = restoreHook(old) })
	doctorSinCodeVersionHook = func() string { return "v3.24.0" }
	r := checkSinCodeBinary()
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

// ── checkDBFile ────────────────────────────────────────────────────────

func TestCheckDBFile_NotExist(t *testing.T) {
	r := checkDBFile("test-db", "/nonexistent/path/test.db", "not created yet")
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
	if r.Name != "test-db" {
		t.Errorf("expected name test-db, got %s", r.Name)
	}
}

func TestCheckDBFile_EmptyPath(t *testing.T) {
	r := checkDBFile("test-db", "", "not created yet")
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckDBFile_IsDir(t *testing.T) {
	tmp := t.TempDir()
	r := checkDBFile("test-db", tmp, "not created yet")
	if r.Status != statusFail {
		t.Fatalf("expected fail, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckDBFile_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkDBFile("test-db", path, "not created yet")
	if r.Status != statusWarn {
		t.Fatalf("expected warn for empty file, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckDBFile_ValidFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "valid.db")
	if err := os.WriteFile(path, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkDBFile("test-db", path, "not created yet")
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckDBFile_Unreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — permissions are ignored")
	}
	tmp := t.TempDir()
	path := filepath.Join(tmp, "noperm.db")
	if err := os.WriteFile(path, []byte("data"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	r := checkDBFile("test-db", path, "not created yet")
	if r.Status != statusFail {
		t.Fatalf("expected fail for unreadable file, got %s: %s", r.Status, r.Detail)
	}
}

// ── checkSessionDB / checkLessonsDB / checkGoalsDB / checkLedgerDB ────

func TestCheckSessionDB_Hooked(t *testing.T) {
	old := doctorSessionDBPathHook
	t.Cleanup(func() { doctorSessionDBPathHook = old })
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sessions.db")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	doctorSessionDBPathHook = func() string { return path }
	r := checkSessionDB()
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
	if r.Name != "session-db" {
		t.Errorf("expected name session-db, got %s", r.Name)
	}
}

func TestCheckLessonsDB_Hooked(t *testing.T) {
	old := doctorLessonsDBPathHook
	t.Cleanup(func() { doctorLessonsDBPathHook = old })
	doctorLessonsDBPathHook = func() string { return "/nonexistent/lessons.db" }
	r := checkLessonsDB()
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckGoalsDB_Hooked(t *testing.T) {
	old := doctorGoalsDBPathHook
	t.Cleanup(func() { doctorGoalsDBPathHook = old })
	tmp := t.TempDir()
	path := filepath.Join(tmp, "goals.db")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	doctorGoalsDBPathHook = func() string { return path }
	r := checkGoalsDB()
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckLedgerDB_Hooked(t *testing.T) {
	old := doctorLedgerDBPathHook
	t.Cleanup(func() { doctorLedgerDBPathHook = old })
	doctorLedgerDBPathHook = func() string { return "/nonexistent/ledger.db" }
	r := checkLedgerDB()
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

// ── checkMCPServers ────────────────────────────────────────────────────

func TestCheckMCPServers_AllRunnable(t *testing.T) {
	old := doctorSkillStatusHook
	t.Cleanup(func() { doctorSkillStatusHook = old })
	doctorSkillStatusHook = func(ctx context.Context) []skillmgr.SkillStatus {
		return []skillmgr.SkillStatus{
			{Name: "websearch", Installed: true, Runnable: true},
			{Name: "scheduler", Installed: true, Runnable: true},
		}
	}
	r := checkMCPServers(context.Background())
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckMCPServers_NoneRunnable(t *testing.T) {
	old := doctorSkillStatusHook
	t.Cleanup(func() { doctorSkillStatusHook = old })
	doctorSkillStatusHook = func(ctx context.Context) []skillmgr.SkillStatus {
		return []skillmgr.SkillStatus{
			{Name: "websearch", Installed: false, Runnable: false},
			{Name: "scheduler", Installed: false, Runnable: false},
		}
	}
	r := checkMCPServers(context.Background())
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckMCPServers_SomeRunnable(t *testing.T) {
	old := doctorSkillStatusHook
	t.Cleanup(func() { doctorSkillStatusHook = old })
	doctorSkillStatusHook = func(ctx context.Context) []skillmgr.SkillStatus {
		return []skillmgr.SkillStatus{
			{Name: "websearch", Installed: true, Runnable: true},
			{Name: "scheduler", Installed: false, Runnable: false},
		}
	}
	r := checkMCPServers(context.Background())
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckMCPServers_Empty(t *testing.T) {
	old := doctorSkillStatusHook
	t.Cleanup(func() { doctorSkillStatusHook = old })
	doctorSkillStatusHook = func(ctx context.Context) []skillmgr.SkillStatus {
		return nil
	}
	r := checkMCPServers(context.Background())
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

// ── checkExternalTools ─────────────────────────────────────────────────

func TestCheckExternalTools_AllFound(t *testing.T) {
	old := doctorLookPathHook
	t.Cleanup(func() { doctorLookPathHook = old })
	doctorLookPathHook = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	r := checkExternalTools()
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckExternalTools_SomeMissing(t *testing.T) {
	old := doctorLookPathHook
	t.Cleanup(func() { doctorLookPathHook = old })
	missing := map[string]bool{"ruff": true, "node": true}
	doctorLookPathHook = func(file string) (string, error) {
		if missing[file] {
			return "", os.ErrNotExist
		}
		return "/usr/bin/" + file, nil
	}
	r := checkExternalTools()
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckExternalTools_AllMissing(t *testing.T) {
	old := doctorLookPathHook
	t.Cleanup(func() { doctorLookPathHook = old })
	doctorLookPathHook = func(file string) (string, error) {
		return "", os.ErrNotExist
	}
	r := checkExternalTools()
	if r.Status != statusFail {
		t.Fatalf("expected fail, got %s: %s", r.Status, r.Detail)
	}
}

// ── checkModulePath ────────────────────────────────────────────────────

func TestCheckModulePath_Correct(t *testing.T) {
	old := doctorGoModPathHook
	t.Cleanup(func() { doctorGoModPathHook = old })
	tmp := t.TempDir()
	goMod := filepath.Join(tmp, "go.mod")
	content := "module github.com/OpenSIN-Code/SIN-Code\n\ngo 1.26.4\n"
	if err := os.WriteFile(goMod, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	doctorGoModPathHook = func() string { return goMod }
	r := checkModulePath()
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckModulePath_Wrong(t *testing.T) {
	old := doctorGoModPathHook
	t.Cleanup(func() { doctorGoModPathHook = old })
	tmp := t.TempDir()
	goMod := filepath.Join(tmp, "go.mod")
	content := "module github.com/OpenSIN-Code/SIN-Code-OLD-NAME\n\ngo 1.26.4\n"
	if err := os.WriteFile(goMod, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	doctorGoModPathHook = func() string { return goMod }
	r := checkModulePath()
	if r.Status != statusFail {
		t.Fatalf("expected fail, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckModulePath_NoFile(t *testing.T) {
	old := doctorGoModPathHook
	t.Cleanup(func() { doctorGoModPathHook = old })
	doctorGoModPathHook = func() string { return "/nonexistent/go.mod" }
	r := checkModulePath()
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckModulePath_NoModuleDirective(t *testing.T) {
	old := doctorGoModPathHook
	t.Cleanup(func() { doctorGoModPathHook = old })
	tmp := t.TempDir()
	goMod := filepath.Join(tmp, "go.mod")
	content := "go 1.26.4\n"
	if err := os.WriteFile(goMod, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	doctorGoModPathHook = func() string { return goMod }
	r := checkModulePath()
	if r.Status != statusWarn {
		t.Fatalf("expected warn, got %s: %s", r.Status, r.Detail)
	}
}

// ── checkCGO ───────────────────────────────────────────────────────────

func TestCheckCGO_Disabled(t *testing.T) {
	old := doctorCGOEnabledHook
	t.Cleanup(func() { doctorCGOEnabledHook = old })
	doctorCGOEnabledHook = func() bool { return false }
	r := checkCGO()
	if r.Status != statusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckCGO_Enabled(t *testing.T) {
	old := doctorCGOEnabledHook
	t.Cleanup(func() { doctorCGOEnabledHook = old })
	doctorCGOEnabledHook = func() bool { return true }
	r := checkCGO()
	if r.Status != statusFail {
		t.Fatalf("expected fail, got %s: %s", r.Status, r.Detail)
	}
}

func TestCheckCGO_CompileTime(t *testing.T) {
	r := checkCGO()
	switch r.Status {
	case statusPass:
		if cgoEnabled {
			t.Errorf("cgoEnabled=true but check passed")
		}
	case statusFail:
		if !cgoEnabled {
			t.Errorf("cgoEnabled=false but check failed")
		}
	default:
		t.Errorf("unexpected status %q (cgoEnabled=%v)", r.Status, cgoEnabled)
	}
}

// ── hasFail ────────────────────────────────────────────────────────────

func TestHasFail(t *testing.T) {
	if hasFail([]CheckResult{{Status: statusPass}, {Status: statusWarn}}) {
		t.Error("expected false for pass+warn")
	}
	if !hasFail([]CheckResult{{Status: statusPass}, {Status: statusFail}}) {
		t.Error("expected true when fail present")
	}
	if hasFail(nil) {
		t.Error("expected false for nil")
	}
	if hasFail([]CheckResult{}) {
		t.Error("expected false for empty")
	}
}

// ── formatDoctorTable ──────────────────────────────────────────────────

func TestFormatDoctorTable(t *testing.T) {
	results := []CheckResult{
		{Name: "go-toolchain", Status: statusPass, Detail: "go1.26.4"},
		{Name: "config-file", Status: statusWarn, Detail: "not configured"},
		{Name: "cgo-enabled", Status: statusFail, Detail: "CGO_ENABLED=1"},
	}
	out := formatDoctorTable(results)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !contains(out, "go-toolchain") {
		t.Error("output missing go-toolchain")
	}
	if !contains(out, "PASS") {
		t.Error("output missing PASS")
	}
	if !contains(out, "WARN") {
		t.Error("output missing WARN")
	}
	if !contains(out, "FAIL") {
		t.Error("output missing FAIL")
	}
}

// ── runAllChecks ───────────────────────────────────────────────────────

func TestRunAllChecks_Count(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := runAllChecks(ctx)
	if len(results) != 11 {
		t.Errorf("expected 11 checks, got %d", len(results))
	}
}

func TestRunAllChecks_Names(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := runAllChecks(ctx)
	want := []string{
		"go-toolchain",
		"sin-code-binary",
		"config-file",
		"session-db",
		"lessons-db",
		"goals-db",
		"ledger-db",
		"mcp-servers",
		"external-tools",
		"module-path",
		"cgo-enabled",
	}
	for i, w := range want {
		if i >= len(results) {
			t.Errorf("missing result at index %d: want %s", i, w)
			continue
		}
		if results[i].Name != w {
			t.Errorf("result[%d].Name = %q, want %q", i, results[i].Name, w)
		}
	}
}

func TestRunAllChecks_ValidStatuses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := runAllChecks(ctx)
	for _, r := range results {
		switch r.Status {
		case statusPass, statusWarn, statusFail:
		default:
			t.Errorf("check %q has invalid status %q", r.Name, r.Status)
		}
	}
}

// ── NewDoctorCmd ───────────────────────────────────────────────────────

func TestNewDoctorCmd(t *testing.T) {
	cmd := NewDoctorCmd()
	if cmd == nil {
		t.Fatal("NewDoctorCmd returned nil")
	}
	if cmd.Use != "doctor" {
		t.Errorf("Use = %q, want %q", cmd.Use, "doctor")
	}
	if !cmd.Flags().HasFlags() {
		t.Error("expected flags to be registered")
	}
	jsonFlag := cmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Error("expected --json flag")
	}
	quietFlag := cmd.Flags().Lookup("quiet")
	if quietFlag == nil {
		t.Error("expected --quiet flag")
	}
}

// ── helper ─────────────────────────────────────────────────────────────

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
