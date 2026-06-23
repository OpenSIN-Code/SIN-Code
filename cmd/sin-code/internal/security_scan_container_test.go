// SPDX-License-Identifier: MIT
// Purpose: tests for the container security scanner.
package internal

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/security/sca"
)

func TestDetectContainerScanRuntimeImpl_ExplicitDockerMissing(t *testing.T) {
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", oldPath)

	_, _, err := detectContainerScanRuntimeImpl("docker")
	if err == nil {
		t.Fatal("expected error when docker not in PATH")
	}
}

func TestContainerRunArgs_AppleVsDocker(t *testing.T) {
	apple := containerRunArgs("/usr/local/bin/container", "alpine:latest", containerVolume{src: "/host", dst: "/guest"})
	if len(apple) < 4 || apple[2] != "--volume" {
		t.Fatalf("expected Apple --volume flag, got %v", apple)
	}

	docker := containerRunArgs("/usr/local/bin/docker", "alpine:latest", containerVolume{src: "/host", dst: "/guest"})
	if len(docker) < 4 || docker[2] != "-v" {
		t.Fatalf("expected Docker -v flag, got %v", docker)
	}
}

func TestParseContainerScanners(t *testing.T) {
	got := parseContainerScanners("secrets, sast, sca")
	want := []string{"secrets", "sast", "sca"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestSummarizeContainerFindings(t *testing.T) {
	findings := []SecurityFinding{
		{Severity: "critical"},
		{Severity: "High"},
		{Severity: "medium"},
		{Severity: "low"},
		{Severity: "info"},
	}
	s := summarizeContainerFindings(findings)
	if s.Critical != 1 || s.High != 1 || s.Medium != 1 || s.Low != 1 || s.Info != 1 || s.Total != 5 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestOrderContainerFindingsBySeverity(t *testing.T) {
	findings := []SecurityFinding{
		{Severity: "low", RuleID: "B"},
		{Severity: "critical", RuleID: "A"},
		{Severity: "high", RuleID: "C"},
	}
	orderContainerFindingsBySeverity(findings)
	if findings[0].Severity != "critical" || findings[1].Severity != "high" || findings[2].Severity != "low" {
		t.Fatalf("unexpected order: %v", findings)
	}
}

func TestRunContainerScanner_ParsesFindings(t *testing.T) {
	oldExec := containerExecCommand
	containerExecCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.Command("echo", `{"findings":[{"rule_id":"S-1","rule_name":"Hardcoded","severity":"high","file":"main.go","line":1,"secret_type":"api-key"}]}`)
	}
	defer func() { containerExecCommand = oldExec }()

	findings, err := runContainerScanner("secrets", "/fake/container", []string{"run", "--rm", "alpine"}, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "S-1" {
		t.Fatalf("unexpected rule id: %q", findings[0].RuleID)
	}
}

func TestRunContainerScanner_ExecFailure(t *testing.T) {
	oldExec := containerExecCommand
	containerExecCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.Command("false")
	}
	defer func() { containerExecCommand = oldExec }()

	_, err := runContainerScanner("secrets", "/fake/container", []string{"run"}, 5*time.Second)
	if err == nil {
		t.Fatal("expected error from failing exec with no output")
	}
}

func TestParseContainerSASTOutput(t *testing.T) {
	out := []byte(`{
		"findings": [
			{"rule_id":"SAST-1","rule_name":"SQL Injection","severity":"high","file":"db.go","line":42,"description":"bad","cwe":"CWE-89","owasp":"A03","remediation":"use prepared statements"}
		]
	}`)
	findings, err := parseContainerSASTOutput(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 || findings[0].CWE != "CWE-89" {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestParseContainerSCAOutput(t *testing.T) {
	out := []byte(`{
		"vulnerabilities": [
			{"id":"CVE-2024-1234","package":"log4j","version":"2.14","severity":"critical","fixed_in":["2.17"]}
		]
	}`)
	findings, err := parseContainerSCAOutput(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 || findings[0].Severity != "critical" {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestResolveContainerScanner_Unsupported(t *testing.T) {
	_, err := resolveContainerScanner("unknown", ".")
	if err == nil {
		t.Fatal("expected error for unsupported scanner")
	}
}

func TestContainerScanResultSummary(t *testing.T) {
	result := containerScanResult{
		Runtime:  "apple",
		Target:   "alpine:latest",
		Kind:     "image",
		Scanners: []string{"secrets"},
		Findings: []SecurityFinding{{RuleID: "S-1", Severity: "high"}},
		Summary:  summarizeContainerFindings([]SecurityFinding{{RuleID: "S-1", Severity: "high"}}),
	}
	if result.Summary.High != 1 || result.Summary.Total != 1 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
}

func TestContainerScanRuntimeFlags(t *testing.T) {
	cmd := NewSecurityScanContainerCmd()
	if cmd.Use != "container [image|path]" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	flags := []string{"runtime", "image", "scanners", "format", "strict", "timeout"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Fatalf("missing flag %q", f)
		}
	}
}

// Ensure the sca package import is used and compiles.
func TestSCAImportCompiles(t *testing.T) {
	_ = sca.Result{}
}
