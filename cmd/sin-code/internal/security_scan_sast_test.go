// SPDX-License-Identifier: MIT
// Purpose: unit tests for the SAST scanner integration.
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

// makeFakeSASTScanner writes a script that prints the given JSON and exits 0.
func makeFakeSASTScanner(t *testing.T, dir, output string) string {
	t.Helper()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".bat"
	}
	bin := filepath.Join(dir, "sin-sast"+ext)
	var script string
	if runtime.GOOS == "windows" {
		script = fmt.Sprintf("@echo off\necho %s\nexit /b 0\n", output)
	} else {
		script = fmt.Sprintf("#!/bin/sh\necho '%s'\nexit 0\n", output)
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestFindSASTScannerBinary_EnvVar(t *testing.T) {
	binDir := t.TempDir()
	bin := makeFakeSASTScanner(t, binDir, `{}`)

	old := os.Getenv("SIN_SAST_BIN")
	os.Setenv("SIN_SAST_BIN", bin)
	defer os.Setenv("SIN_SAST_BIN", old)

	found, err := findSASTScannerBinary(".", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != bin {
		t.Errorf("expected %s, got %s", bin, found)
	}
}

func TestFindSASTScannerBinary_Path(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	binDir := t.TempDir()
	makeFakeSASTScanner(t, binDir, `{}`)
	os.Setenv("PATH", binDir)

	found, err := findSASTScannerBinary(".", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(found) != "sin-sast"+binaryExt() {
		t.Errorf("expected sin-sast binary, got %s", found)
	}
}

func TestFindSASTScannerBinary_NoBuild(t *testing.T) {
	old := os.Getenv("SIN_SAST_BIN")
	os.Unsetenv("SIN_SAST_BIN")
	defer os.Setenv("SIN_SAST_BIN", old)

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	oldCacheDir := sastCacheDir
	tmpCache := t.TempDir()
	sastCacheDir = func() (string, error) { return tmpCache, nil }
	defer func() { sastCacheDir = oldCacheDir }()

	// Use the workspace as scan path so the vendored SAST module is found,
	// but the cache is empty and noBuild prevents building.
	_, err := findSASTScannerBinary(".", true)
	if err == nil {
		t.Fatal("expected error when binary missing and no-build set")
	}
	if !strings.Contains(err.Error(), "--no-build") {
		t.Errorf("expected --no-build in error, got %q", err.Error())
	}
}

func TestRunSASTScanner_ParsesJSON(t *testing.T) {
	binDir := t.TempDir()
	output := `{
  "path": "/tmp/test",
  "status": "warning",
  "findings": [
    {
      "rule_id": "SAST-006",
      "rule_name": "Hardcoded API Key",
      "severity": "high",
      "cwe": "CWE-798",
      "owasp": "A07:2021",
      "language": "go",
      "file": "main.go",
      "line": 12,
      "column": 5,
      "match": "api_key = \"secret123\"",
      "context": "api_key = \"secret123\"",
      "remediation": "Use environment variables or a secret manager",
      "confidence": "high",
      "description": "Detects hardcoded API keys"
    }
  ],
  "summary": {
    "critical": 0,
    "high": 1,
    "medium": 0,
    "low": 0,
    "files_scanned": 1,
    "lines_scanned": 42,
    "rules_triggered": 1,
    "by_language": {"go": 1},
    "by_owasp": {"A07:2021": 1}
  },
  "scan_duration_seconds": 0.5,
  "timestamp": "2026-06-23T12:00:00Z"
}`
	bin := makeFakeSASTScanner(t, binDir, output)

	result, err := runSASTScanner(bin, ".", "low", "", "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "warning" {
		t.Errorf("expected status warning, got %s", result.Status)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	f := result.Findings[0]
	if f.RuleID != "SAST-006" {
		t.Errorf("expected rule SAST-006, got %s", f.RuleID)
	}
	if f.Severity != "high" {
		t.Errorf("expected severity high, got %s", f.Severity)
	}
	if result.Summary.High != 1 {
		t.Errorf("expected 1 high finding, got %d", result.Summary.High)
	}
}

func TestRunSASTScanner_HandlesExitErrorWithFindings(t *testing.T) {
	binDir := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".bat"
	}
	bin := filepath.Join(binDir, "sin-sast"+ext)
	output := `{"status":"failed","findings":[{"severity":"critical","rule_id":"SAST-001"}],"summary":{"critical":1,"high":0,"medium":0,"low":0,"files_scanned":1,"lines_scanned":1,"rules_triggered":1,"by_language":{},"by_owasp":{}},"scan_duration_seconds":0.1,"timestamp":"2026-06-23T12:00:00Z"}`
	var script string
	if runtime.GOOS == "windows" {
		script = fmt.Sprintf("@echo off\necho %s\nexit /b 1\n", output)
	} else {
		script = fmt.Sprintf("#!/bin/sh\necho '%s'\nexit 1\n", output)
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := runSASTScanner(bin, ".", "low", "", "", 5)
	if err != nil {
		t.Fatalf("expected no error when findings present, got %v", err)
	}
	if result.Summary.Critical != 1 {
		t.Errorf("expected 1 critical finding, got %d", result.Summary.Critical)
	}
}

func TestPrintSASTResult_NoFindings(t *testing.T) {
	r := &sastScanResult{
		Path:                ".",
		Status:              "passed",
		Findings:            []sastScanFinding{},
		ScanDurationSeconds: 0.1,
		Summary:             sastScanSummary{FilesScanned: 1, LinesScanned: 10},
	}
	get := captureStdout(t)
	printSASTResult(r)
	out := get()
	if !strings.Contains(out, "No SAST findings") {
		t.Errorf("expected 'No SAST findings' in output, got %q", out)
	}
}

func TestPrintSASTResult_WithFindings(t *testing.T) {
	r := &sastScanResult{
		Path:   ".",
		Status: "warning",
		Findings: []sastScanFinding{
			{RuleID: "SAST-006", RuleName: "Hardcoded API Key", Severity: "high", File: "main.go", Line: 10, Match: "api_key", CWE: "CWE-798", OWASP: "A07", Remediation: "Use env vars"},
			{RuleID: "SAST-001", RuleName: "SQL Injection", Severity: "critical", File: "db.go", Line: 5, Match: "query", CWE: "CWE-89", OWASP: "A03", Remediation: "Use parameterized queries"},
		},
		Summary: sastScanSummary{High: 1, Critical: 1, FilesScanned: 2},
	}
	get := captureStdout(t)
	printSASTResult(r)
	out := get()
	if !strings.Contains(out, "SAST-001") {
		t.Errorf("expected SAST-001 in output, got %q", out)
	}
	if !strings.Contains(out, "SAST-006") {
		t.Errorf("expected SAST-006 in output, got %q", out)
	}
	// Critical should appear before high due to severity ranking.
	criticalIdx := strings.Index(out, "SAST-001")
	highIdx := strings.Index(out, "SAST-006")
	if criticalIdx == -1 || highIdx == -1 || criticalIdx > highIdx {
		t.Errorf("expected critical before high; criticalIdx=%d highIdx=%d", criticalIdx, highIdx)
	}
}

func TestSecurityScanSastCmd_Help(t *testing.T) {
	cmd := NewSecurityScanSastCmd()
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help command failed: %v", err)
	}
}

func TestSecurityScanSastCmd_ExecuteWithMock(t *testing.T) {
	oldLocator := sastBinLocator
	defer func() { sastBinLocator = oldLocator }()

	dir := t.TempDir()
	output := `{"status":"passed","findings":[],"summary":{"critical":0,"high":0,"medium":0,"low":0,"files_scanned":1,"lines_scanned":1,"rules_triggered":0,"by_language":{},"by_owasp":{}},"scan_duration_seconds":0.1,"timestamp":"2026-06-23T12:00:00Z"}`
	bin := makeFakeSASTScanner(t, dir, output)

	sastBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return bin, nil
	}

	cmd := NewSecurityScanSastCmd()
	cmd.SetArgs([]string{dir, "--format", "json"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(new(strings.Builder))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(out.String(), `"status": "passed"`) {
		t.Errorf("expected JSON status passed, got %q", out.String())
	}
}

func TestSecurityScanSastCmd_StrictCritical(t *testing.T) {
	oldLocator := sastBinLocator
	defer func() { sastBinLocator = oldLocator }()

	dir := t.TempDir()
	output := `{"status":"failed","findings":[{"severity":"critical","rule_id":"SAST-001","rule_name":"SQL Injection","file":"main.go","line":1,"match":"exec","cwe":"CWE-89","owasp":"A03","remediation":"parametrize"}],"summary":{"critical":1,"high":0,"medium":0,"low":0,"files_scanned":1,"lines_scanned":1,"rules_triggered":1,"by_language":{},"by_owasp":{}},"scan_duration_seconds":0.1,"timestamp":"2026-06-23T12:00:00Z"}`
	bin := makeFakeSASTScanner(t, dir, output)

	sastBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return bin, nil
	}

	cmd := NewSecurityScanSastCmd()
	cmd.SetArgs([]string{dir, "--strict"})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected strict mode to fail with critical findings")
	}
}
