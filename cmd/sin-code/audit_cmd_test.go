package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/audit"
)

func TestAuditCommandComplexityJSON(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), `package demo

// sin-debt: legacy interface
type Reader interface { Read() }

func NewReader() Reader { return &reader{} }
type reader struct{}
func (r *reader) Read() {}
`)

	cmd := NewAuditCmd()
	cmd.SetArgs([]string{"complexity", dir, "--format", "json", "--tags", "yagni"})
	out := captureOutput(t, cmd)

	var res audit.Result
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(res.Findings) == 0 {
		t.Fatalf("expected findings, got: %s", out)
	}
	for _, f := range res.Findings {
		if f.Tag != "yagni" {
			t.Fatalf("expected only yagni tag, got %v", f)
		}
	}
}

func TestAuditCommandComplexityText(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), `package demo
var VerboseFlag bool
`)

	cmd := NewAuditCmd()
	cmd.SetArgs([]string{"complexity", dir, "--tags", "delete"})
	out := captureOutput(t, cmd)
	if !strings.Contains(out, "delete:") {
		t.Fatalf("expected delete finding in output:\n%s", out)
	}
	if !strings.Contains(out, "net:") {
		t.Fatalf("expected net summary in output:\n%s", out)
	}
}

func TestAuditCommandComplexityMarkdown(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), `package demo
func Exported() {}
`)

	cmd := NewAuditCmd()
	cmd.SetArgs([]string{"complexity", dir, "--format", "markdown", "--tags", "shrink"})
	out := captureOutput(t, cmd)
	if !strings.HasPrefix(out, "# Complexity Audit") {
		t.Fatalf("expected markdown header, got:\n%s", out)
	}
}

func TestAuditCommandUnknownTag(t *testing.T) {
	cmd := NewAuditCmd()
	cmd.SetArgs([]string{"complexity", ".", "--tags", "bad"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unknown tag")
	}
}

func TestAuditCommandInvalidRank(t *testing.T) {
	cmd := NewAuditCmd()
	cmd.SetArgs([]string{"complexity", ".", "--rank", "bad"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid rank")
	}
}

func TestAuditCommandStrictThreshold(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), `package demo
var VerboseFlag bool
var DebugConfig string
var OtherFlag string
`)

	cmd := NewAuditCmd()
	cmd.SetArgs([]string{"complexity", dir, "--tags", "delete", "--strict", "--max-net-lines", "1"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected strict threshold error")
	}
}

func TestCEOAUDITCommand(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), `package demo
var VerboseFlag bool
`)

	cmd := NewCEOAUDITCmd()
	cmd.SetArgs([]string{dir, "--format", "json", "--tags", "delete"})
	out := captureOutput(t, cmd)

	if !strings.Contains(out, "complexity-audit") {
		t.Fatalf("expected complexity gate in output:\n%s", out)
	}
	if !strings.Contains(out, "net:") {
		t.Fatalf("expected net summary in output:\n%s", out)
	}
}

func TestCEOAUDITCommand48Gates(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), `package demo
`)

	cmd := NewCEOAUDITCmd()
	cmd.SetArgs([]string{dir, "--format", "json"})
	out := captureOutput(t, cmd)

	var result ceoResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(result.Gates) != 48 {
		t.Fatalf("expected 48 gates, got %d", len(result.Gates))
	}
	found := false
	for _, g := range result.Gates {
		if g.Name == "complexity-audit" {
			found = true
		}
	}
	if !found {
		t.Fatal("complexity-audit gate not found")
	}
}

func TestCEOAUDITCommandSecurityGateWarnsOnIssues(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module test\n")

	// Put a fake go vet that reports issues.
	binDir := t.TempDir()
	makeFakeSecurityToolForCEO(t, filepath.Join(binDir, "go"), "vet found issues", 1)
	os.Setenv("PATH", binDir)

	cmd := NewCEOAUDITCmd()
	cmd.SetArgs([]string{dir, "--format", "json"})
	out := captureOutput(t, cmd)

	var result ceoResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	var secGate *ceoGate
	for i := range result.Gates {
		if result.Gates[i].Name == "security-scan" {
			secGate = &result.Gates[i]
			break
		}
	}
	if secGate == nil {
		t.Fatal("security-scan gate not found")
	}
	if secGate.Status != "warn" {
		t.Fatalf("expected security-scan status warn, got %s", secGate.Status)
	}
}

func TestCEOAUDITCommandSecurityGateStrictFails(t *testing.T) {
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module test\n")

	binDir := t.TempDir()
	makeFakeSecurityToolForCEO(t, filepath.Join(binDir, "go"), "vet found issues", 1)
	os.Setenv("PATH", binDir)

	cmd := NewCEOAUDITCmd()
	cmd.SetArgs([]string{dir, "--format", "json", "--strict"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected strict mode to fail with security issues")
	}
}

func TestAuditSecuritySubcommand(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "config.env"), `api_key = "supersecret123456789"`)

	cmd := NewAuditCmd()
	cmd.SetArgs([]string{"security", dir, "--format", "json"})
	out := captureOutput(t, cmd)

	if !strings.Contains(out, "issues found") && !strings.Contains(out, "\"issues\":") {
		t.Fatalf("expected security issues in output:\n%s", out)
	}
}

func makeFakeSecurityToolForCEO(t *testing.T, path, output string, exit int) {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\nexit %d\n", output, exit)
	os.WriteFile(path, []byte(script), 0o755)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func captureOutput(t *testing.T, cmd *cobra.Command) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	if err := cmd.Execute(); err != nil {
		os.Stdout = old
		fmt.Fprintf(old, "command error: %v\n", err)
	}
	w.Close()
	os.Stdout = old
	b := make([]byte, 65536)
	n, _ := r.Read(b)
	return string(b[:n])
}
