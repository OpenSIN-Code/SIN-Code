package internal

import (
	"strings"
	"testing"
)

func TestSecurityCmd_DetectGoProject(t *testing.T) {
	result := detectProjectType("/Users/jeremy/dev/SIN-Code-Bundle")
	if result != "go" {
		t.Errorf("expected 'go' for SIN-Code-Bundle, got %q", result)
	}
}

func TestSecurityCmd_DetectGenericProject(t *testing.T) {
	result := detectProjectType("/tmp")
	if result != "generic" {
		t.Errorf("expected 'generic' for /tmp, got %q", result)
	}
}

func TestSecurityCmd_ParseToolFilter(t *testing.T) {
	m := parseToolFilter("govulncheck,gosec")
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if !m["govulncheck"] {
		t.Error("expected govulncheck in filter")
	}
	if !m["gosec"] {
		t.Error("expected gosec in filter")
	}
	if m["bandit"] {
		t.Error("did not expect bandit in filter")
	}
}

func TestSecurityCmd_ParseToolFilterEmpty(t *testing.T) {
	m := parseToolFilter("")
	if m != nil {
		t.Error("expected nil for empty filter")
	}
}

func TestSecurityCmd_RunGoProject(t *testing.T) {
	// Run security scan on the current project (Go)
	SecurityCmd.SetArgs([]string{".", "--type", "go", "--tools", "go vet"})
	SecurityCmd.SetOut(new(strings.Builder))
	SecurityCmd.SetErr(new(strings.Builder))
	err := SecurityCmd.Execute()
	if err != nil {
		t.Fatalf("security command failed: %v", err)
	}
}

func TestSecurityCmd_FileExists(t *testing.T) {
	if !fileExists("/Users/jeremy/dev/SIN-Code-Bundle/go.mod") {
		t.Error("expected go.mod to exist in SIN-Code-Bundle")
	}
	if fileExists("/nonexistent/path/file.txt") {
		t.Error("expected nonexistent file to not exist")
	}
}
