// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the efm (Ephemeral Full-Stack Mocking) subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunEFM_ListAction(t *testing.T) {
	efmAction = "list"
	efmStack = ""
	efmTTL = 0
	efmFormat = "text"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "text")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM list failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "EFM: list") {
		t.Errorf("expected 'EFM: list' in output, got %q", out)
	}
}

func TestRunEFM_UpRequiresStack(t *testing.T) {
	err := runEFM("up", "", 3600, "text")
	if err == nil {
		t.Error("expected error when --stack is missing for action 'up'")
	}
	if !strings.Contains(err.Error(), "--stack is required") {
		t.Errorf("expected '--stack is required' error, got %q", err.Error())
	}
}

func TestRunEFM_DownRequiresStack(t *testing.T) {
	err := runEFM("down", "", 0, "text")
	if err == nil {
		t.Error("expected error when --stack is missing for action 'down'")
	}
	if !strings.Contains(err.Error(), "--stack is required") {
		t.Errorf("expected '--stack is required' error, got %q", err.Error())
	}
}

func TestDockerComposeUp_NonExistentStack(t *testing.T) {
	err := dockerComposeUp("/nonexistent/docker-compose.yml", 3600)
	if err == nil {
		t.Error("expected error for nonexistent stack file")
	}
	if !strings.Contains(err.Error(), "stack file not found") {
		t.Errorf("expected 'stack file not found' error, got %q", err.Error())
	}
}

func TestDockerComposeDown_NonExistentStack(t *testing.T) {
	err := dockerComposeDown("/nonexistent/docker-compose.yml")
	if err == nil {
		t.Error("expected error for nonexistent stack file")
	}
	if !strings.Contains(err.Error(), "stack file not found") {
		t.Errorf("expected 'stack file not found' error, got %q", err.Error())
	}
}

func TestRunEFM_InvalidAction(t *testing.T) {
	err := runEFM("invalid", "", 0, "text")
	if err == nil {
		t.Error("expected error for invalid action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("expected 'unknown action' error, got %q", err.Error())
	}
}

func TestRunEFM_StatusNoStack(t *testing.T) {
	efmAction = "status"
	efmStack = ""
	efmTTL = 0
	efmFormat = "text"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("status", "", 0, "text")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM status without stack failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "EFM: status") {
		t.Errorf("expected 'EFM: status' in output, got %q", out)
	}
}

func TestDockerComposeStatus_NonExistentStack(t *testing.T) {
	_, err := dockerComposeStatus("/nonexistent/docker-compose.yml")
	if err == nil {
		t.Error("expected error for nonexistent stack file in status")
	}
}

func TestRunEFM_JSONOutput(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM list json failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON output, got parse error: %v", err)
	}
	if result.Action != "list" {
		t.Errorf("expected action='list', got %q", result.Action)
	}
}

func TestFilterServices(t *testing.T) {
	services := []efmService{
		{Name: "myapp-web-1", Status: "running", Image: "nginx"},
		{Name: "myapp-db-1", Status: "running", Image: "postgres"},
		{Name: "other-app-1", Status: "running", Image: "redis"},
	}

	filtered := filterServices(services, "myapp.yml")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered services, got %d", len(filtered))
	}
	for _, svc := range filtered {
		if !strings.HasPrefix(svc.Name, "myapp") {
			t.Errorf("expected service name to start with 'myapp', got %q", svc.Name)
		}
	}
}

func TestFilterServices_EmptyStack(t *testing.T) {
	services := []efmService{
		{Name: "svc-1", Status: "running"},
	}
	filtered := filterServices(services, "")
	if len(filtered) != 0 {
		t.Errorf("expected 0 filtered services for empty stack, got %d", len(filtered))
	}
}

func TestFilterServices_NoMatch(t *testing.T) {
	services := []efmService{
		{Name: "app-1", Status: "running"},
	}
	filtered := filterServices(services, "nonexistent/docker-compose.yml")
	if len(filtered) != 0 {
		t.Errorf("expected 0 filtered services for non-matching stack, got %d", len(filtered))
	}
}



func TestDockerComposeUp_TTLMetadata(t *testing.T) {
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0644)

	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	_ = os.MkdirAll(metadataDir, 0755)
	metadataFile := filepath.Join(metadataDir, filepath.Base(stackFile)+".meta")
	_ = os.Remove(metadataFile)

	err := dockerComposeUp(stackFile, 3600)
	if err == nil {
		data, readErr := os.ReadFile(metadataFile)
		if readErr != nil {
			t.Fatalf("expected metadata file to exist, got error: %v", readErr)
		}
		var meta map[string]string
		if jsonErr := json.Unmarshal(data, &meta); jsonErr != nil {
			t.Fatalf("expected valid JSON metadata, got parse error: %v", jsonErr)
		}
		if meta["ttl"] != "3600" {
			t.Errorf("expected ttl=3600, got %q", meta["ttl"])
		}
		if meta["stack"] == "" {
			t.Error("expected non-empty stack path in metadata")
		}
		if meta["started"] == "" {
			t.Error("expected non-empty started timestamp in metadata")
		}
		if meta["expires"] == "" {
			t.Error("expected non-empty expires timestamp in metadata")
		}
	} else {
		if !strings.Contains(err.Error(), "docker") && !strings.Contains(err.Error(), "compose") {
			t.Errorf("expected docker-related error, got: %v", err)
		}
	}
}

func TestOutputTextEFM(t *testing.T) {
	result := efmResult{
		Action:   "list",
		Status:   "ok",
		Duration: "100ms",
		Services: []efmService{
			{Name: "web-1", Status: "running", Image: "nginx", Ports: []string{"8080:80"}},
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextEFM(result); err != nil {
		t.Fatalf("outputTextEFM failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "EFM: list") {
		t.Errorf("expected 'EFM: list' in output, got %q", out)
	}
	if !strings.Contains(out, "Status: ok") {
		t.Errorf("expected 'Status: ok' in output, got %q", out)
	}
	if !strings.Contains(out, "web-1") {
		t.Errorf("expected 'web-1' in services, got %q", out)
	}
	if !strings.Contains(out, "nginx") {
		t.Errorf("expected 'nginx' image in output, got %q", out)
	}
}

func TestOutputTextEFM_WithStack(t *testing.T) {
	result := efmResult{
		Action:   "up",
		Stack:    "docker-compose.yml",
		Status:   "started",
		Duration: "500ms",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextEFM(result); err != nil {
		t.Fatalf("outputTextEFM failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Stack: docker-compose.yml") {
		t.Errorf("expected 'Stack: docker-compose.yml' in output, got %q", out)
	}
}

func TestOutputTextEFM_WithError(t *testing.T) {
	result := efmResult{
		Action:  "up",
		Stack:   "docker-compose.yml",
		Status:  "error",
		Error:   "docker not available",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextEFM(result); err != nil {
		t.Fatalf("outputTextEFM failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Error: docker not available") {
		t.Errorf("expected error message in output, got %q", out)
	}
}

func TestOutputTextEFM_NoServices(t *testing.T) {
	result := efmResult{
		Action:   "list",
		Status:   "ok",
		Duration: "50ms",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextEFM(result); err != nil {
		t.Fatalf("outputTextEFM failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if strings.Contains(out, "Services:") {
		t.Errorf("did not expect 'Services:' when no services present, got %q", out)
	}
}

func TestEfmCmd_ListViaRunEFM(t *testing.T) {
	efmAction = "list"
	efmStack = ""
	efmTTL = 0
	efmFormat = "text"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := EfmCmd.RunE(EfmCmd, []string{})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("EfmCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "EFM: list") {
		t.Errorf("expected 'EFM: list' in output, got %q", out)
	}
}

func TestEfmCmd_UpWithoutStack(t *testing.T) {
	efmAction = "up"
	efmStack = ""
	efmTTL = 3600
	efmFormat = "text"

	err := EfmCmd.RunE(EfmCmd, []string{})
	if err == nil {
		t.Error("expected error when --stack is missing for up")
	}
}

func TestEfmCmd_DownWithoutStack(t *testing.T) {
	efmAction = "down"
	efmStack = ""
	efmTTL = 0
	efmFormat = "text"

	err := EfmCmd.RunE(EfmCmd, []string{})
	if err == nil {
		t.Error("expected error when --stack is missing for down")
	}
}

func TestEfmCmd_InvalidAction(t *testing.T) {
	efmAction = "destroy"
	efmStack = ""
	efmTTL = 0
	efmFormat = "text"

	err := EfmCmd.RunE(EfmCmd, []string{})
	if err == nil {
		t.Error("expected error for invalid action")
	}
}
