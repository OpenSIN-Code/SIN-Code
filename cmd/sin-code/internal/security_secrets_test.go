// SPDX-License-Identifier: MIT
// Purpose: Tests for the `security scan secrets` vendored scanner integration.
package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeSecretsScanner returns a shell script that echoes the provided JSON and
// exits 0. The caller is responsible for cleaning up the file.
func fakeSecretsScanner(t *testing.T, json string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell script test is Unix-specific")
	}
	path := filepath.Join(t.TempDir(), "sin-secrets")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\n", json)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSecurityScanSecrets_Help(t *testing.T) {
	cmd := NewSecurityScanSecretsCmd()
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help command failed: %v", err)
	}
}

func TestSecurityScanSecrets_NoFindings(t *testing.T) {
	json := `{
  "path": "/tmp",
  "status": "passed",
  "findings": [],
  "summary": {
    "critical": 0,
    "high": 0,
    "medium": 0,
    "low": 0,
    "files_scanned": 5,
    "secrets_found": 0,
    "by_type": {},
    "by_file": {}
  },
  "scan_duration_seconds": 0.05,
  "timestamp": "2026-06-23T00:00:00Z"
}`
	fake := fakeSecretsScanner(t, json)
	oldLocator := secretsBinLocator
	secretsBinLocator = func(string, bool) (string, error) { return fake, nil }
	defer func() { secretsBinLocator = oldLocator }()

	cmd := NewSecurityScanSecretsCmd()
	cmd.SetArgs([]string{t.TempDir()})
	out := captureStdout(t)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out(), "No leaked secrets detected") {
		t.Errorf("expected no-secrets message, got:\n%s", out())
	}
}

func TestSecurityScanSecrets_WithFindings(t *testing.T) {
	json := `{
  "path": "/tmp",
  "status": "failed",
  "findings": [
    {
      "rule_id": "SECRETS-001",
      "rule_name": "OpenAI API Key",
      "severity": "critical",
      "secret_type": "api-key",
      "file": "config.py",
      "line": 3,
      "column": 10,
      "match": "sk-1234567890abcdef1234567890abcdef",
      "context": "api_key = \"sk-1234567890abcdef1234567890abcdef\"",
      "remediation": "Remove from code.",
      "confidence": "High",
      "entropy": 4.32,
      "is_verified": false
    }
  ],
  "summary": {
    "critical": 1,
    "high": 0,
    "medium": 0,
    "low": 0,
    "files_scanned": 1,
    "secrets_found": 1,
    "by_type": {"api-key": 1},
    "by_file": {"config.py": 1}
  },
  "scan_duration_seconds": 0.12,
  "timestamp": "2026-06-23T00:00:00Z"
}`
	fake := fakeSecretsScanner(t, json)
	oldLocator := secretsBinLocator
	secretsBinLocator = func(string, bool) (string, error) { return fake, nil }
	defer func() { secretsBinLocator = oldLocator }()

	cmd := NewSecurityScanSecretsCmd()
	cmd.SetArgs([]string{t.TempDir()})
	out := captureStdout(t)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr := out()
	for _, want := range []string{"SECRETS-001", "OpenAI API Key", "config.py:3", "sk-1", "Remove from code."} {
		if !strings.Contains(outStr, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, outStr)
		}
	}
	if strings.Contains(outStr, "sk-1234567890abcdef1234567890abcdef") {
		t.Error("raw secret should be masked in output")
	}
}

func TestSecurityScanSecrets_Strict(t *testing.T) {
	json := `{
  "path": "/tmp",
  "status": "failed",
  "findings": [
    {
      "rule_id": "SECRETS-001",
      "rule_name": "OpenAI API Key",
      "severity": "critical",
      "secret_type": "api-key",
      "file": "config.py",
      "line": 3,
      "match": "sk-1234567890abcdef1234567890abcdef",
      "context": "api_key",
      "remediation": "Remove from code.",
      "confidence": "High",
      "entropy": 4.32,
      "is_verified": false
    }
  ],
  "summary": {
    "critical": 1,
    "high": 0,
    "medium": 0,
    "low": 0,
    "files_scanned": 1,
    "secrets_found": 1,
    "by_type": {},
    "by_file": {}
  },
  "scan_duration_seconds": 0.1,
  "timestamp": "2026-06-23T00:00:00Z"
}`
	fake := fakeSecretsScanner(t, json)
	oldLocator := secretsBinLocator
	secretsBinLocator = func(string, bool) (string, error) { return fake, nil }
	defer func() { secretsBinLocator = oldLocator }()

	cmd := NewSecurityScanSecretsCmd()
	cmd.SetArgs([]string{"--strict", t.TempDir()})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	_ = captureStdout(t)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected strict mode to return error when secrets found")
	}
}

func TestSecurityScanSecrets_JSONOutput(t *testing.T) {
	json := `{
  "path": "/tmp",
  "status": "passed",
  "findings": [],
  "summary": {"files_scanned": 2, "secrets_found": 0},
  "scan_duration_seconds": 0.05,
  "timestamp": "2026-06-23T00:00:00Z"
}`
	fake := fakeSecretsScanner(t, json)
	oldLocator := secretsBinLocator
	secretsBinLocator = func(string, bool) (string, error) { return fake, nil }
	defer func() { secretsBinLocator = oldLocator }()

	cmd := NewSecurityScanSecretsCmd()
	cmd.SetArgs([]string{"--format", "json", t.TempDir()})
	cmd.SetOut(new(strings.Builder))
	out := captureStdout(t)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr := out()
	if !strings.Contains(outStr, `"status": "passed"`) {
		t.Errorf("expected JSON status, got:\n%s", outStr)
	}
}

func TestSecurityScanSecrets_LocatorNotFound(t *testing.T) {
	oldLocator := secretsBinLocator
	secretsBinLocator = func(string, bool) (string, error) { return "", fmt.Errorf("not found") }
	defer func() { secretsBinLocator = oldLocator }()

	cmd := NewSecurityScanSecretsCmd()
	cmd.SetArgs([]string{t.TempDir()})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	_ = captureStdout(t)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when scanner not found")
	}
}

func TestSecurityScanSecrets_RunScannerInvalidJSON(t *testing.T) {
	fake := fakeSecretsScanner(t, "not-json")
	oldLocator := secretsBinLocator
	secretsBinLocator = func(string, bool) (string, error) { return fake, nil }
	defer func() { secretsBinLocator = oldLocator }()

	cmd := NewSecurityScanSecretsCmd()
	cmd.SetArgs([]string{t.TempDir()})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	_ = captureStdout(t)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid scanner JSON")
	}
}

func TestFindSecretsModuleRoot(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "SIN-Code-Secrets-Scanner")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := findSecretsModuleRoot(filepath.Join(root, "cmd", "sin-code"))
	if got != moduleDir {
		t.Errorf("expected module root %q, got %q", moduleDir, got)
	}
}

func TestFindSecretsModuleRoot_NotFound(t *testing.T) {
	if got := findSecretsModuleRoot(t.TempDir()); got != "" {
		t.Errorf("expected empty root, got %q", got)
	}
}

func TestMaskSecret(t *testing.T) {
	if got := maskSecuritySecret("short"); got != "*****" {
		t.Errorf("expected short mask, got %q", got)
	}
	got := maskSecuritySecret("sk-1234567890abcdef")
	if !strings.HasPrefix(got, "sk-1") || !strings.HasSuffix(got, "cdef") {
		t.Errorf("expected prefix/suffix preserved, got %q", got)
	}
	if strings.Contains(got, "234567890abcd") {
		t.Error("middle of secret should be masked")
	}
}

func TestSecurityScanCmd_Tree(t *testing.T) {
	cmd := NewSecurityScanCmd()
	if cmd.Name() != "scan" {
		t.Errorf("expected command name 'scan', got %q", cmd.Name())
	}
	subNames := make([]string, 0, len(cmd.Commands()))
	for _, c := range cmd.Commands() {
		subNames = append(subNames, c.Name())
	}
	if !strings.Contains(strings.Join(subNames, ","), "secrets") {
		t.Errorf("expected 'secrets' subcommand, got %v", subNames)
	}
}

func TestSecurityCmd_HasScanSubcommand(t *testing.T) {
	found := false
	for _, c := range SecurityCmd.Commands() {
		if c.Name() == "scan" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected SecurityCmd to have 'scan' subcommand")
	}
}

func TestSecurityScanSecrets_FlagsPassedToScanner(t *testing.T) {
	var capturedArgs []string
	fake := filepath.Join(t.TempDir(), "sin-secrets")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$CAPTURE_FILE"
printf '{"path":"/tmp","status":"passed","findings":[],"summary":{"files_scanned":0,"secrets_found":0},"scan_duration_seconds":0,"timestamp":"2026-06-23T00:00:00Z"}'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	captureFile := filepath.Join(t.TempDir(), "args")

	oldLocator := secretsBinLocator
	secretsBinLocator = func(string, bool) (string, error) { return fake, nil }
	defer func() { secretsBinLocator = oldLocator }()

	cmd := NewSecurityScanSecretsCmd()
	cmd.SetArgs([]string{"--severity", "high", "--types", "api-key", "--no-entropy", "--exclude", "*.test", t.TempDir()})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	_ = captureStdout(t)

	// Set the capture file path in the script's environment.
	os.Setenv("CAPTURE_FILE", captureFile)
	defer os.Unsetenv("CAPTURE_FILE")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	captured, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("could not read captured args: %v", err)
	}
	capturedArgs = strings.Split(string(captured), "\n")
	for _, want := range []string{"--severity", "high", "--types", "api-key", "--check-entropy=false", "--exclude", "*.test"} {
		found := false
		for _, arg := range capturedArgs {
			if arg == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected scanner args to contain %q, got %v", want, capturedArgs)
		}
	}
}
