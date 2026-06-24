// SPDX-License-Identifier: MIT
// Purpose: tests for the unified `security scan all` orchestrator.
package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var errTest = fmt.Errorf("simulated error")

// writeFakeScanner writes a small shell script that prints the given JSON and
// exits 0, so the scan orchestrator can be exercised without vendored tools.
func writeFakeScanner(t *testing.T, dir, name, output string) string {
	t.Helper()
	bin := filepath.Join(dir, name)
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", output)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake scanner: %v", err)
	}
	return bin
}

func TestRunSecurityScanAll_NoFindings(t *testing.T) {
	oldSecrets := secretsBinLocator
	oldSast := sastBinLocator
	secretsBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return "", errTest
	}
	sastBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return "", errTest
	}
	defer func() {
		secretsBinLocator = oldSecrets
		sastBinLocator = oldSast
	}()

	findings, issues, err := runSecurityScanAll(t.TempDir(), "low", 5, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issues != 0 || len(findings) != 0 {
		t.Fatalf("expected no findings, got %d issues / %d findings", issues, len(findings))
	}
}

func TestRunSecurityScanAll_AggregatesFindings(t *testing.T) {
	dir := t.TempDir()
	secretsOut := `{"summary":{"secrets_found":1,"files_scanned":1},"findings":[{"rule_id":"SEC-1","rule_name":"API Key","severity":"high","file":"main.go","line":10,"secret_type":"api-key"}]}`
	sastOut := `{"summary":{"critical":0,"high":1,"medium":0,"low":0,"files_scanned":1},"findings":[{"rule_id":"SAST-1","rule_name":"SQL Injection","severity":"high","file":"db.go","line":42,"cwe":"CWE-89","owasp":"A03","description":"bad"}]}`
	secretsBin := writeFakeScanner(t, dir, "sin-secrets", secretsOut)
	sastBin := writeFakeScanner(t, dir, "sin-sast", sastOut)

	oldSecrets := secretsBinLocator
	oldSast := sastBinLocator
	secretsBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return secretsBin, nil
	}
	sastBinLocator = func(scanPath string, noBuild bool) (string, error) {
		return sastBin, nil
	}
	defer func() {
		secretsBinLocator = oldSecrets
		sastBinLocator = oldSast
	}()

	findings, issues, err := runSecurityScanAll(t.TempDir(), "low", 5, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	if issues != 2 {
		t.Fatalf("expected 2 issues, got %d", issues)
	}
}

func TestPrintSecurityScanAllResult(t *testing.T) {
	// Smoke test: should not panic.
	findings := []SecurityFinding{
		{RuleID: "S-1", RuleName: "Test", Severity: "high", File: "main.go", Line: 1, CWE: "CWE-79", OWASP: "A03", Remediation: "fix it"},
	}
	printSecurityScanAllResult(t.TempDir(), 1, findings)
}

func TestNewSecurityScanAllCmd_Flags(t *testing.T) {
	cmd := NewSecurityScanAllCmd()
	flags := []string{"severity", "format", "strict", "timeout", "no-build"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Fatalf("missing flag %q", f)
		}
	}
}
