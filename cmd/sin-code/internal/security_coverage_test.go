// SPDX-License-Identifier: MIT
// Purpose: Coverage tests for runGrypeSCA and runWithTimeout edge cases.
// Uses fake grype binaries and a json marshal hook; no real security scanners required.
// Docs: security.doc.md
package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityFake_GrypeSCANotFound(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", "")

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	status, _, _, errStr := runGrypeSCA(dir, 5)
	if status != "not_found" {
		t.Errorf("expected status 'not_found', got %q", status)
	}
	if errStr == "" {
		t.Error("expected non-empty error string for not_found status")
	}
}

func TestSecurityFake_GrypeSCAOk(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\nrequire github.com/foo/bar v1.2.3\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "grype"), `{"matches": []}`, 0)
	os.Setenv("PATH", binDir)

	status, issues, output, errStr := runGrypeSCA(dir, 5)
	if status != "ok" {
		t.Errorf("expected status 'ok', got %q", status)
	}
	if issues != 0 {
		t.Errorf("expected 0 issues, got %d", issues)
	}
	if output == "" {
		t.Error("expected non-empty JSON output")
	}
	if errStr != "" {
		t.Errorf("expected empty error string, got %q", errStr)
	}
}

func TestSecurityFake_GrypeSCAIssues(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\nrequire github.com/foo/bar v1.2.3\n"), 0o644)

	binDir := t.TempDir()
	fixture := `{
  "matches": [
    {
      "vulnerability": {
        "id": "CVE-2024-1234",
        "severity": "High",
        "description": "Bad thing",
        "fix": {
          "versions": ["v1.2.4"],
          "state": "fixed"
        }
      },
      "artifact": {
        "name": "github.com/foo/bar",
        "version": "v1.2.3",
        "type": "go-module"
      }
    }
  ]
}`
	makeFakeSecurityTool(t, filepath.Join(binDir, "grype"), fixture, 0)
	os.Setenv("PATH", binDir)

	status, issues, output, errStr := runGrypeSCA(dir, 5)
	if status != "issues" {
		t.Errorf("expected status 'issues', got %q", status)
	}
	if issues != 1 {
		t.Errorf("expected 1 issue, got %d", issues)
	}
	if output == "" {
		t.Error("expected non-empty JSON output")
	}
	if errStr != "" {
		t.Errorf("expected empty error string, got %q", errStr)
	}
}

func TestSecurityFake_GrypeSCAScanError(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	// Invalid go.mod forces sca.Scanner.Scan to return a parse error.
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("not a go.mod file\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "grype"), `{"matches": []}`, 0)
	os.Setenv("PATH", binDir)

	status, _, _, errStr := runGrypeSCA(dir, 5)
	if status != "error" {
		t.Errorf("expected status 'error', got %q", status)
	}
	if errStr == "" {
		t.Error("expected non-empty error string for scan error")
	}
}

func TestSecurityFake_GrypeSCAJsonError(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\nrequire github.com/foo/bar v1.2.3\n"), 0o644)

	binDir := t.TempDir()
	makeFakeSecurityTool(t, filepath.Join(binDir, "grype"), `{"matches": []}`, 0)
	os.Setenv("PATH", binDir)

	oldMarshal := jsonMarshalIndent
	jsonMarshalIndent = func(v any, prefix, indent string) ([]byte, error) {
		return nil, fmt.Errorf("marshal error")
	}
	t.Cleanup(func() { jsonMarshalIndent = oldMarshal })

	status, _, _, errStr := runGrypeSCA(dir, 5)
	if status != "error" {
		t.Errorf("expected status 'error', got %q", status)
	}
	if errStr == "" {
		t.Error("expected non-empty error string for JSON marshal error")
	}
}

func TestSecurityFake_RunWithTimeoutZero(t *testing.T) {
	out, err := runWithTimeout("echo", []string{"hello"}, "", 0)
	if err != nil {
		t.Fatalf("runWithTimeout with timeout=0: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("expected 'hello', got %q", string(out))
	}
}
