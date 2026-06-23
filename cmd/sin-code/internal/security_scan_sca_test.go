// SPDX-License-Identifier: MIT
// Purpose: unit tests for the SCA scanner integration.
// All tests are hermetic: no real network, no real vendored builds.
package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeFakeSCAScanner writes a script that prints the given JSON and exits 0.
// The returned path is the fake binary location.
func makeFakeSCAScanner(t *testing.T, dir, output string) string {
	t.Helper()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".bat"
	}
	bin := filepath.Join(dir, "sin-sca-go"+ext)
	payload := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(payload, []byte(output), 0o644); err != nil {
		t.Fatal(err)
	}
	var script string
	if runtime.GOOS == "windows" {
		script = fmt.Sprintf("@echo off\ntype %s\nexit /b 0\n", payload)
	} else {
		script = fmt.Sprintf("#!/bin/sh\ncat %s\nexit 0\n", payload)
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestFindSCAScannerBinary_EnvVar(t *testing.T) {
	binDir := t.TempDir()
	bin := makeFakeSCAScanner(t, binDir, `{}`)

	old := os.Getenv("SIN_SCA_BIN")
	os.Setenv("SIN_SCA_BIN", bin)
	defer os.Setenv("SIN_SCA_BIN", old)

	found, err := findSCAScannerBinary(".", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != bin {
		t.Errorf("expected %s, got %s", bin, found)
	}
}

func TestFindSCAScannerBinary_Path(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	binDir := t.TempDir()
	makeFakeSCAScanner(t, binDir, `{}`)
	os.Setenv("PATH", binDir)

	found, err := findSCAScannerBinary(".", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(found) != "sin-sca-go"+binaryExt() {
		t.Errorf("expected sin-sca-go binary, got %s", found)
	}
}

func TestFindSCAScannerBinary_NoBuild(t *testing.T) {
	old := os.Getenv("SIN_SCA_BIN")
	os.Unsetenv("SIN_SCA_BIN")
	defer os.Setenv("SIN_SCA_BIN", old)

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	oldCacheDir := scaCacheDir
	tmpCache := t.TempDir()
	scaCacheDir = func() (string, error) { return tmpCache, nil }
	defer func() { scaCacheDir = oldCacheDir }()

	_, err := findSCAScannerBinary(".", true)
	if err == nil {
		t.Fatal("expected error when binary missing and no-build set")
	}
	if !strings.Contains(err.Error(), "--no-build") {
		t.Errorf("expected --no-build in error, got %q", err.Error())
	}
}

func TestFindSCAModuleRoot(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "SIN-Code-SCA-Tool-Go")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := findSCAModuleRoot(filepath.Join(root, "cmd", "sin-code"))
	if got != moduleDir {
		t.Errorf("expected module root %q, got %q", moduleDir, got)
	}
}

func TestFindSCAModuleRoot_NotFound(t *testing.T) {
	if got := findSCAModuleRoot(t.TempDir()); got != "" {
		t.Errorf("expected empty root, got %q", got)
	}
}

func TestRunSCAScanner_ParsesJSON(t *testing.T) {
	binDir := t.TempDir()
	output := `{
  "project_path": "/tmp/test",
  "ecosystem": "Go",
  "packages_scanned": 5,
  "vulnerabilities": [
    {
      "id": "GHSA-123",
      "package": "github.com/foo/bar",
      "version": "v1.0.0",
      "severity": "high",
      "summary": "Bad thing",
      "fixed_in": "v1.1.0",
      "references": ["https://example.com"],
      "aliases": ["CVE-2026-1234"]
    }
  ],
  "summary": {
    "total": 1,
    "critical": 0,
    "high": 1,
    "medium": 0,
    "low": 0,
    "unknown": 0
  }
}`
	bin := makeFakeSCAScanner(t, binDir, output)

	findings, err := runSCAScanner(bin, ".", scanConfig{Timeout: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Scanner != "sca" {
		t.Errorf("expected scanner sca, got %s", f.Scanner)
	}
	if f.RuleID != "GHSA-123" {
		t.Errorf("expected id GHSA-123, got %s", f.RuleID)
	}
	if f.Severity != "high" {
		t.Errorf("expected severity high, got %s", f.Severity)
	}
	if f.Package != "github.com/foo/bar" {
		t.Errorf("expected package github.com/foo/bar, got %s", f.Package)
	}
	if f.Version != "v1.0.0" {
		t.Errorf("expected version v1.0.0, got %s", f.Version)
	}
	if f.Remediation != "Upgrade to v1.1.0" {
		t.Errorf("expected remediation, got %s", f.Remediation)
	}
}

func TestRunSCAScanner_SeverityFilter(t *testing.T) {
	binDir := t.TempDir()
	output := `{
  "project_path": "/tmp/test",
  "ecosystem": "npm",
  "packages_scanned": 10,
  "vulnerabilities": [
    {"id": "GHSA-CRIT", "package": "pkg-a", "version": "1.0.0", "severity": "CRITICAL", "summary": "crit"},
    {"id": "GHSA-LOW", "package": "pkg-b", "version": "2.0.0", "severity": "low", "summary": "low"}
  ],
  "summary": {"total": 2, "critical": 1, "high": 0, "medium": 0, "low": 1, "unknown": 0}
}`
	bin := makeFakeSCAScanner(t, binDir, output)

	findings, err := runSCAScanner(bin, ".", scanConfig{Severity: "high", Timeout: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding after filter, got %d", len(findings))
	}
	if findings[0].RuleID != "GHSA-CRIT" {
		t.Errorf("expected critical finding, got %s", findings[0].RuleID)
	}

	result, err := runSCAScannerFull(bin, ".", scanConfig{Severity: "high", Timeout: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary.Total != 1 || result.Summary.Critical != 1 {
		t.Errorf("expected filtered summary critical=1, total=1, got %+v", result.Summary)
	}
}

func TestRunSCAScanner_HandlesExitErrorWithJSON(t *testing.T) {
	binDir := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".bat"
	}
	bin := filepath.Join(binDir, "sin-sca-go"+ext)
	output := `{"project_path": "/tmp/test", "ecosystem": "Go", "packages_scanned": 1, "vulnerabilities": [{"id": "GHSA-001", "package": "p", "version": "v1", "severity": "critical", "summary": "x"}], "summary": {"total": 1, "critical": 1, "high": 0, "medium": 0, "low": 0, "unknown": 0}}`
	payload := filepath.Join(binDir, "payload.json")
	if err := os.WriteFile(payload, []byte(output), 0o644); err != nil {
		t.Fatal(err)
	}
	var script string
	if runtime.GOOS == "windows" {
		script = fmt.Sprintf("@echo off\ntype %s\nexit /b 1\n", payload)
	} else {
		script = fmt.Sprintf("#!/bin/sh\ncat %s\nexit 1\n", payload)
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	findings, err := runSCAScanner(bin, ".", scanConfig{Timeout: 5})
	if err != nil {
		t.Fatalf("expected no error when JSON is present despite exit error, got %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestRunSCAScanner_ParsesJSONWithLogPrefix(t *testing.T) {
	binDir := t.TempDir()
	output := "🔍 Detected ecosystem: Go\n📦 Found 91 packages\n" + `{
  "project_path": "/tmp/test",
  "ecosystem": "Go",
  "packages_scanned": 91,
  "vulnerabilities": [],
  "summary": {
    "total": 0,
    "critical": 0,
    "high": 0,
    "medium": 0,
    "low": 0,
    "unknown": 0
  }
}`
	bin := makeFakeSCAScanner(t, binDir, output)

	findings, err := runSCAScanner(bin, ".", scanConfig{Timeout: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"json only", `{"a":1}`, `{"a":1}`},
		{"log prefix", "log line\n{\"a\":1}\n", "{\"a\":1}"},
		{"emoji prefix", "🔍 Detected\n📦 Found\n{\"a\":1}\n", "{\"a\":1}"},
		{"array", "[1,2,3]", "[1,2,3]"},
		{"no json", "just text", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractJSON([]byte(c.input))
			if string(got) != c.want {
				t.Errorf("got %q, want %q", string(got), c.want)
			}
		})
	}
}

func TestPrintSCAResult_NoFindings(t *testing.T) {
	r := &scaScanResult{
		ProjectPath:     ".",
		Ecosystem:       "Go",
		PackagesScanned: 5,
		Vulnerabilities: []SecurityFinding{},
		Summary:         summarizeSCAFindings(nil),
	}
	get := captureStdout(t)
	printSCAResult(r)
	out := get()
	if !strings.Contains(out, "No vulnerable dependencies") {
		t.Errorf("expected no-vulns message, got %q", out)
	}
}

func TestPrintSCAResult_WithFindings(t *testing.T) {
	findings := []SecurityFinding{
		{Scanner: "sca", RuleID: "GHSA-002", Package: "pkg-b", Version: "2.0.0", Severity: "high", Description: "bad"},
		{Scanner: "sca", RuleID: "GHSA-001", Package: "pkg-a", Version: "1.0.0", Severity: "critical", Description: "worse"},
	}
	r := &scaScanResult{
		ProjectPath:     ".",
		Ecosystem:       "npm",
		PackagesScanned: 10,
		Vulnerabilities: findings,
		Summary:         summarizeSCAFindings(findings),
	}
	get := captureStdout(t)
	printSCAResult(r)
	out := get()
	if !strings.Contains(out, "GHSA-001") {
		t.Errorf("expected GHSA-001, got %q", out)
	}
	if !strings.Contains(out, "GHSA-002") {
		t.Errorf("expected GHSA-002, got %q", out)
	}
	// Critical should appear before high due to severity ranking.
	criticalIdx := strings.Index(out, "GHSA-001")
	highIdx := strings.Index(out, "GHSA-002")
	if criticalIdx == -1 || highIdx == -1 || criticalIdx > highIdx {
		t.Errorf("expected critical before high; criticalIdx=%d highIdx=%d", criticalIdx, highIdx)
	}
}

func TestSecurityScanScaCmd_Help(t *testing.T) {
	cmd := NewSecurityScanScaCmd()
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help command failed: %v", err)
	}
}

func TestSecurityScanScaCmd_ExecuteWithMock(t *testing.T) {
	oldLocator := scaBinLocator
	defer func() { scaBinLocator = oldLocator }()

	dir := t.TempDir()
	output := `{"project_path": "/tmp/test", "ecosystem": "Go", "packages_scanned": 3, "vulnerabilities": [], "summary": {"total": 0, "critical": 0, "high": 0, "medium": 0, "low": 0, "unknown": 0}}`
	bin := makeFakeSCAScanner(t, dir, output)

	scaBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return bin, nil
	}

	cmd := NewSecurityScanScaCmd()
	cmd.SetArgs([]string{dir, "--format", "json"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(new(strings.Builder))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(out.String(), `"project_path"`) {
		t.Errorf("expected JSON project_path, got %q", out.String())
	}
}

func TestSecurityScanScaCmd_Strict(t *testing.T) {
	oldLocator := scaBinLocator
	defer func() { scaBinLocator = oldLocator }()

	dir := t.TempDir()
	output := `{"project_path": "/tmp/test", "ecosystem": "Go", "packages_scanned": 1, "vulnerabilities": [{"id": "GHSA-001", "package": "p", "version": "v1", "severity": "critical", "summary": "x"}], "summary": {"total": 1, "critical": 1, "high": 0, "medium": 0, "low": 0, "unknown": 0}}`
	bin := makeFakeSCAScanner(t, dir, output)

	scaBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return bin, nil
	}

	cmd := NewSecurityScanScaCmd()
	cmd.SetArgs([]string{dir, "--strict"})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected strict mode to fail with findings")
	}
}

func TestSecurityScanScaCmd_Tree(t *testing.T) {
	cmd := NewSecurityScanCmd()
	names := make([]string, 0, len(cmd.Commands()))
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	if !strings.Contains(strings.Join(names, ","), "sca") {
		t.Errorf("expected 'sca' subcommand, got %v", names)
	}
}

func TestSecurityCmd_HasScaSubcommand(t *testing.T) {
	found := false
	for _, c := range SecurityCmd.Commands() {
		if c.Name() == "scan" {
			for _, sc := range c.Commands() {
				if sc.Name() == "sca" {
					found = true
					break
				}
			}
			break
		}
	}
	if !found {
		t.Error("expected SecurityCmd to have 'scan sca' subcommand")
	}
}
