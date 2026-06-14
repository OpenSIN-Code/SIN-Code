// SPDX-License-Identifier: MIT
// Purpose: Tests for printSecurityResult and SecurityCmd strict mode. (st-cov1)
package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func captureSecurityPrint(t *testing.T, r SecurityResult) string {
	t.Helper()
	get := captureStdout(t)
	printSecurityResult(r)
	return get()
}

func TestPrintSecurityResult_AllStatuses(t *testing.T) {
	r := SecurityResult{
		ProjectType: "go",
		Path:        "/tmp",
		Duration:    1 * time.Second,
		Tools: []ToolResult{
			{Name: "govulncheck", Status: "ok", Duration: "1s"},
			{Name: "gosec", Status: "issues", Issues: 3, Duration: "1s"},
			{Name: "go vet", Status: "error", Duration: "1s", Error: "boom"},
			{Name: "bandit", Status: "not_found", Duration: "1s"},
			{Name: "safety", Status: "skipped", Duration: "1s"},
		},
		Summary: SecuritySummary{ToolsRun: 3, Issues: 3, Errors: 1, NotFound: 1, Skipped: 1},
	}
	out := captureSecurityPrint(t, r)
	for _, want := range []string{"Security Scan Summary", "govulncheck", "gosec", "go vet", "bandit", "safety", "3 issues"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got %q", want, out)
		}
	}
}

func TestPrintSecurityResult_NoIssues(t *testing.T) {
	r := SecurityResult{
		ProjectType: "generic",
		Path:        "/tmp",
		Duration:    1 * time.Second,
		Tools:       []ToolResult{{Name: "secrets grep", Status: "ok", Duration: "1s"}},
		Summary:     SecuritySummary{ToolsRun: 1},
	}
	out := captureSecurityPrint(t, r)
	if !strings.Contains(out, "No security issues detected") {
		t.Errorf("expected no issues message, got %q", out)
	}
}

func TestPrintSecurityResult_Strict(t *testing.T) {
	r := SecurityResult{
		ProjectType: "go",
		Path:        "/tmp",
		Duration:    1 * time.Second,
		Strict:      true,
		Tools:       []ToolResult{{Name: "gosec", Status: "issues", Issues: 5, Duration: "1s"}},
		Summary:     SecuritySummary{ToolsRun: 1, Issues: 5},
	}
	out := captureSecurityPrint(t, r)
	if !strings.Contains(out, "Strict mode") {
		t.Errorf("expected strict mode message, got %q", out)
	}
}

func resetSecurityCmdFlags(t *testing.T) {
	t.Helper()
	SecurityCmd.Flags().Set("type", "auto")
	SecurityCmd.Flags().Set("tools", "")
	SecurityCmd.Flags().Set("format", "text")
	SecurityCmd.Flags().Set("timeout", "300")
	SecurityCmd.Flags().Set("strict", "false")
}

func TestSecurityCmd_StrictWithIssues(t *testing.T) {
	oldArgs := SecurityCmd.Args
	defer func() { SecurityCmd.Args = oldArgs }()
	resetSecurityCmdFlags(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.env"), []byte(`password = "supersecret12345"`), 0o644)

	SecurityCmd.SetArgs([]string{dir})
	SecurityCmd.Flags().Set("type", "generic")
	SecurityCmd.Flags().Set("strict", "true")
	SecurityCmd.Flags().Set("format", "text")
	SecurityCmd.SetOut(new(strings.Builder))
	SecurityCmd.SetErr(new(strings.Builder))
	_ = captureStdout(t)
	err := SecurityCmd.Execute()
	if err == nil {
		t.Fatal("expected strict mode to return error when issues found")
	}
}

func TestSecurityCmd_JSON(t *testing.T) {
	oldArgs := SecurityCmd.Args
	defer func() { SecurityCmd.Args = oldArgs }()
	resetSecurityCmdFlags(t)

	SecurityCmd.SetArgs([]string{"."})
	SecurityCmd.Flags().Set("type", "generic")
	SecurityCmd.Flags().Set("format", "json")
	SecurityCmd.Flags().Set("strict", "false")
	SecurityCmd.SetOut(new(strings.Builder))
	SecurityCmd.SetErr(new(strings.Builder))
	_ = captureStdout(t)
	if err := SecurityCmd.Execute(); err != nil {
		t.Fatalf("security json failed: %v", err)
	}
}
