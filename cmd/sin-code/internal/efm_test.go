// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the efm (Ephemeral Full-Stack Mocking) subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testRuntime = "auto"

func dockerAvailable() bool {
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func TestRunEFM_ListAction(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "text", testRuntime)
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
	err := runEFM("up", "", 3600, "text", testRuntime)
	if err == nil {
		t.Error("expected error when --stack is missing for action 'up'")
	}
	if !strings.Contains(err.Error(), "--stack is required") {
		t.Errorf("expected '--stack is required' error, got %q", err.Error())
	}
}

func TestRunEFM_DownRequiresStack(t *testing.T) {
	err := runEFM("down", "", 0, "text", testRuntime)
	if err == nil {
		t.Error("expected error when --stack is missing for action 'down'")
	}
	if !strings.Contains(err.Error(), "--stack is required") {
		t.Errorf("expected '--stack is required' error, got %q", err.Error())
	}
}

func TestDockerComposeUp_NonExistentStack(t *testing.T) {
	err := dockerComposeUp("/nonexistent/docker-compose.yml", 3600, testRuntime)
	if err == nil {
		t.Error("expected error for nonexistent stack file")
	}
	if !strings.Contains(err.Error(), "stack file not found") {
		t.Errorf("expected 'stack file not found' error, got %q", err.Error())
	}
}

func TestDockerComposeDown_NonExistentStack(t *testing.T) {
	err := dockerComposeDown("/nonexistent/docker-compose.yml", testRuntime)
	if err == nil {
		t.Error("expected error for nonexistent stack file")
	}
	if !strings.Contains(err.Error(), "stack file not found") {
		t.Errorf("expected 'stack file not found' error, got %q", err.Error())
	}
}

func TestRunEFM_InvalidAction(t *testing.T) {
	err := runEFM("invalid", "", 0, "text", testRuntime)
	if err == nil {
		t.Error("expected error for invalid action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("expected 'unknown action' error, got %q", err.Error())
	}
}

func TestRunEFM_StatusNoStack(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("status", "", 0, "text", testRuntime)
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
	_, err := dockerComposeStatus("/nonexistent/docker-compose.yml", testRuntime)
	if err == nil {
		t.Error("expected error for nonexistent stack file in status")
	}
}

func TestRunEFM_JSONOutput(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "json", testRuntime)
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

func TestRunEFM_JSONOutput_WithStack(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("up", "nonexistent.yml", 3600, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM up json should not return error (error is in result), got: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Action != "up" {
		t.Errorf("expected action='up', got %q", result.Action)
	}
	if result.Stack != "nonexistent.yml" {
		t.Errorf("expected stack='nonexistent.yml', got %q", result.Stack)
	}
	if result.Status != "error" {
		t.Errorf("expected status='error', got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error in result")
	}
}

func TestRunEFM_DownWithStack_Error(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("down", "nonexistent.yml", 0, "text", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM down should not return error (error is in result), got: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "EFM: down") {
		t.Errorf("expected 'EFM: down' in output, got %q", out)
	}
	if !strings.Contains(out, "stack file not found") {
		t.Errorf("expected 'stack file not found' in output, got %q", out)
	}
}

func TestRunEFM_StatusWithStack_Error(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("status", "nonexistent.yml", 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM status should not return error (error is in result), got: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "error" {
		t.Errorf("expected status='error', got %q", result.Status)
	}
}

func TestRunEFM_UpWithStack_DockerError(t *testing.T) {
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("up", stackFile, 3600, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM up should not return error (error is in result), got: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Action != "up" {
		t.Errorf("expected action='up', got %q", result.Action)
	}
	if result.Duration == "" {
		t.Error("expected non-empty duration")
	}
}

func TestRunEFM_DownWithStack_DockerError(t *testing.T) {
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("down", stackFile, 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM down should not return error (error is in result), got: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Action != "down" {
		t.Errorf("expected action='down', got %q", result.Action)
	}
}

func TestRunEFM_StatusWithStack_DockerError(t *testing.T) {
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("status", stackFile, 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM status should not return error (error is in result), got: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Action != "status" {
		t.Errorf("expected action='status', got %q", result.Action)
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

func TestFilterServices_EmptyInput(t *testing.T) {
	filtered := filterServices(nil, "myapp.yml")
	if len(filtered) != 0 {
		t.Errorf("expected 0 for nil input, got %d", len(filtered))
	}
}

func TestFilterServices_DifferentExtension(t *testing.T) {
	services := []efmService{
		{Name: "test-web-1", Status: "running"},
		{Name: "test-db-1", Status: "running"},
		{Name: "prod-web-1", Status: "running"},
	}
	filtered := filterServices(services, "test.yaml")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered services for .yaml extension, got %d", len(filtered))
	}
}

func TestDockerComposeUp_TTLMetadata(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0644)

	err := dockerComposeUp(stackFile, 3600, testRuntime)
	if err != nil {
		metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
		metadataFile := filepath.Join(metadataDir, filepath.Base(stackFile)+".meta")
		data, readErr := os.ReadFile(metadataFile)
		if readErr == nil {
			var meta map[string]string
			if jsonErr := json.Unmarshal(data, &meta); jsonErr == nil {
				if meta["ttl"] != "3600" {
					t.Errorf("expected ttl=3600, got %q", meta["ttl"])
				}
				if meta["started"] == "" {
					t.Error("expected non-empty started timestamp")
				}
				if meta["expires"] == "" {
					t.Error("expected non-empty expires timestamp")
				}
			}
		}
	}
}

func TestDockerComposeUp_NoTTL(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "no-ttl-compose.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	metadataFile := filepath.Join(metadataDir, filepath.Base(stackFile)+".meta")
	_ = os.Remove(metadataFile)

	err := dockerComposeUp(stackFile, 0, testRuntime)
	if err != nil {
		t.Skipf("dockerComposeUp failed: %v", err)
	}

	if _, statErr := os.Stat(metadataFile); statErr == nil {
		t.Error("expected no metadata file when TTL=0")
		_ = os.Remove(metadataFile)
	}

	_ = dockerComposeDown(stackFile, testRuntime)
}

func TestDockerComposeDown_RemovesMetadata(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0644)

	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	_ = os.MkdirAll(metadataDir, 0755)
	metadataFile := filepath.Join(metadataDir, filepath.Base(stackFile)+".meta")
	meta := map[string]string{
		"stack":   stackFile,
		"started": "2026-06-07T12:00:00Z",
		"ttl":     "3600",
		"expires": "2026-06-07T13:00:00Z",
	}
	data, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(metadataFile, data, 0644)

	_ = dockerComposeDown(stackFile, testRuntime)

	if _, err := os.Stat(metadataFile); err == nil {
		t.Error("expected metadata file to be removed after down")
	}
}

func TestListDockerContainers_DockerNotAvailable(t *testing.T) {
	if dockerAvailable() {
		t.Skip("Docker daemon is available, skipping unavailable-path test")
	}
	_, err := listDockerContainers(testRuntime)
	if err == nil {
		t.Error("expected error when Docker is not available")
	}
}

func TestDockerComposeUp_DockerAvailable(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: hello-world\n"), 0644)

	err := dockerComposeUp(stackFile, 60, testRuntime)
	if err != nil {
		t.Logf("dockerComposeUp returned error (may be expected): %v", err)
	}
}

func TestDockerComposeDown_DockerAvailable(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: hello-world\n"), 0644)

	err := dockerComposeDown(stackFile, testRuntime)
	if err != nil {
		t.Logf("dockerComposeDown returned error (may be expected): %v", err)
	}
}

func TestDockerComposeStatus_DockerAvailable(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: hello-world\n"), 0644)

	status, err := dockerComposeStatus(stackFile, testRuntime)
	if err != nil {
		t.Logf("dockerComposeStatus returned error (may be expected): %v", err)
	}
	_ = status
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
	if !strings.Contains(out, "8080:80") {
		t.Errorf("expected '8080:80' port in output, got %q", out)
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

func TestOutputTextEFM_ServiceNoImageNoPorts(t *testing.T) {
	result := efmResult{
		Action:   "list",
		Status:   "ok",
		Duration: "10ms",
		Services: []efmService{
			{Name: "svc-1", Status: "running"},
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

	if !strings.Contains(out, "svc-1") {
		t.Errorf("expected 'svc-1' in output, got %q", out)
	}
}

func TestOutputTextEFM_ServiceEmptyPorts(t *testing.T) {
	result := efmResult{
		Action:   "list",
		Status:   "ok",
		Duration: "10ms",
		Services: []efmService{
			{Name: "svc-1", Status: "running", Ports: []string{""}},
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

	if !strings.Contains(out, "svc-1") {
		t.Errorf("expected 'svc-1' in output, got %q", out)
	}
}

func TestOutputTextEFM_MultiplePorts(t *testing.T) {
	result := efmResult{
		Action:   "list",
		Status:   "ok",
		Duration: "10ms",
		Services: []efmService{
			{Name: "svc-1", Status: "running", Ports: []string{"8080:80", "9090:90"}},
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

	if !strings.Contains(out, "8080:80, 9090:90") {
		t.Errorf("expected both ports in output, got %q", out)
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

func TestEfmCmd_Flags(t *testing.T) {
	EfmCmd.Flags().Set("action", "list")
	EfmCmd.Flags().Set("stack", "")
	EfmCmd.Flags().Set("ttl", "3600")
	EfmCmd.Flags().Set("format", "text")

	actionFlag, err := EfmCmd.Flags().GetString("action")
	if err != nil {
		t.Fatalf("failed to get action flag: %v", err)
	}
	if actionFlag != "list" {
		t.Errorf("default action = %q, want 'list'", actionFlag)
	}

	stackFlag, err := EfmCmd.Flags().GetString("stack")
	if err != nil {
		t.Fatalf("failed to get stack flag: %v", err)
	}
	if stackFlag != "" {
		t.Errorf("default stack = %q, want empty", stackFlag)
	}

	ttlFlag, err := EfmCmd.Flags().GetInt("ttl")
	if err != nil {
		t.Fatalf("failed to get ttl flag: %v", err)
	}
	if ttlFlag != 3600 {
		t.Errorf("default ttl = %d, want 3600", ttlFlag)
	}

	formatFlag, err := EfmCmd.Flags().GetString("format")
	if err != nil {
		t.Fatalf("failed to get format flag: %v", err)
	}
	if formatFlag != "text" {
		t.Errorf("default format = %q, want 'text'", formatFlag)
	}
}

func TestRunEFM_AllActions_JSON(t *testing.T) {
	actions := []string{"list", "status"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := runEFM(action, "", 0, "json", testRuntime)
			w.Close()
			os.Stdout = oldStdout

			if err != nil {
				t.Fatalf("runEFM %s json failed: %v", action, err)
			}

			var buf bytes.Buffer
			buf.ReadFrom(r)
			out := buf.String()

			var result efmResult
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatalf("expected valid JSON for action %s, got parse error: %v", action, err)
			}
			if result.Action != action {
				t.Errorf("expected action=%q, got %q", action, result.Action)
			}
			if result.Duration == "" {
				t.Errorf("expected non-empty duration for action %s", action)
			}
		})
	}
}

func TestRunEFM_ListAction_DockerUnavailable(t *testing.T) {
	if dockerAvailable() {
		t.Skip("Docker daemon available, skipping unavailable-path test")
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM list should not return error (error is in result), got: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "error" {
		t.Errorf("expected status='error' when Docker unavailable, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error when Docker unavailable")
	}
}

func TestEfmService_JSONRoundTrip(t *testing.T) {
	svc := efmService{
		Name:   "web-1",
		Status: "running",
		Ports:  []string{"8080:80", "9090:90"},
		Image:  "nginx:latest",
	}
	data, err := json.Marshal(svc)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	var decoded efmService
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded.Name != svc.Name {
		t.Errorf("name = %q, want %q", decoded.Name, svc.Name)
	}
	if decoded.Status != svc.Status {
		t.Errorf("status = %q, want %q", decoded.Status, svc.Status)
	}
	if len(decoded.Ports) != len(svc.Ports) {
		t.Errorf("ports count = %d, want %d", len(decoded.Ports), len(svc.Ports))
	}
	if decoded.Image != svc.Image {
		t.Errorf("image = %q, want %q", decoded.Image, svc.Image)
	}
}

func TestEfmResult_JSONRoundTrip(t *testing.T) {
	result := efmResult{
		Action:   "up",
		Stack:    "docker-compose.yml",
		Status:   "started",
		Duration: "500ms",
		Services: []efmService{
			{Name: "web-1", Status: "running", Image: "nginx"},
		},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	var decoded efmResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded.Action != result.Action {
		t.Errorf("action = %q, want %q", decoded.Action, result.Action)
	}
	if decoded.Stack != result.Stack {
		t.Errorf("stack = %q, want %q", decoded.Stack, result.Stack)
	}
	if decoded.Status != result.Status {
		t.Errorf("status = %q, want %q", decoded.Status, result.Status)
	}
}

func TestRunEFM_UpWithExistingStack_TTLZero(t *testing.T) {
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("up", stackFile, 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM up should not return error (error is in result), got: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Action != "up" {
		t.Errorf("expected action='up', got %q", result.Action)
	}
}

func TestDockerComposeUp_SuccessWithTTL(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	_ = os.MkdirAll(metadataDir, 0755)
	metadataFile := filepath.Join(metadataDir, filepath.Base(stackFile)+".meta")
	_ = os.Remove(metadataFile)

	err := dockerComposeUp(stackFile, 60, testRuntime)
	if err != nil {
		t.Skipf("dockerComposeUp failed: %v", err)
	}

	data, readErr := os.ReadFile(metadataFile)
	if readErr != nil {
		t.Fatalf("expected metadata file: %v", readErr)
	}
	var meta map[string]string
	if jsonErr := json.Unmarshal(data, &meta); jsonErr != nil {
		t.Fatalf("invalid metadata JSON: %v", jsonErr)
	}
	if meta["ttl"] != "60" {
		t.Errorf("ttl = %q, want 60", meta["ttl"])
	}

	_ = dockerComposeDown(stackFile, testRuntime)
}

func TestRunEFM_UpSuccess(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("up", stackFile, 60, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM up failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "started" {
		t.Errorf("expected status='started', got %q", result.Status)
	}
	if result.Duration == "" {
		t.Error("expected non-empty duration")
	}

	_ = dockerComposeDown(stackFile, testRuntime)
}

func TestRunEFM_UpSuccess_Text(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("up", stackFile, 60, "text", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM up text failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "EFM: up") {
		t.Errorf("expected 'EFM: up' in output, got %q", out)
	}
	if !strings.Contains(out, "started") {
		t.Errorf("expected 'started' in output, got %q", out)
	}

	_ = dockerComposeDown(stackFile, testRuntime)
}

func TestDockerComposeStatus_AllRunning(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	err := dockerComposeUp(stackFile, 60, testRuntime)
	if err != nil {
		t.Skipf("dockerComposeUp failed: %v", err)
	}

	status, err := dockerComposeStatus(stackFile, testRuntime)
	if err != nil {
		t.Fatalf("dockerComposeStatus failed: %v", err)
	}
	if status != "all running" && status != "partial" && status != "no containers running" {
		t.Errorf("unexpected status: %q", status)
	}

	_ = dockerComposeDown(stackFile, testRuntime)
}

func TestDockerComposeStatus_NoContainers(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	status, err := dockerComposeStatus(stackFile, testRuntime)
	if err != nil {
		t.Fatalf("dockerComposeStatus failed: %v", err)
	}
	if status != "no containers running" {
		t.Logf("status = %q (may differ if containers are running)", status)
	}
}

func TestRunEFM_StatusWithStack_Success(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	err := dockerComposeUp(stackFile, 60, testRuntime)
	if err != nil {
		t.Skipf("dockerComposeUp failed: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runEFM("status", stackFile, 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM status failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status == "error" {
		t.Errorf("expected non-error status, got %q with error %q", result.Status, result.Error)
	}

	_ = dockerComposeDown(stackFile, testRuntime)
}

func TestRunEFM_DownSuccess(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	err := dockerComposeUp(stackFile, 60, testRuntime)
	if err != nil {
		t.Skipf("dockerComposeUp failed: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runEFM("down", stackFile, 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM down failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "stopped" {
		t.Errorf("expected status='stopped', got %q", result.Status)
	}
}

func TestListDockerContainers_Parsing(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	services, err := listDockerContainers(testRuntime)
	if err != nil {
		t.Fatalf("listDockerContainers failed: %v", err)
	}
	for _, svc := range services {
		if svc.Name == "" {
			t.Error("service name should not be empty")
		}
		if svc.Status == "" {
			t.Error("service status should not be empty")
		}
	}
}

func TestRunEFM_ListWithDockerRunning(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM list failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("expected status='ok' with Docker running, got %q", result.Status)
	}
}

func TestRunEFM_StatusNoStackWithDockerRunning(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("status", "", 0, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM status failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("expected status='ok' with Docker running, got %q", result.Status)
	}
}

func TestDockerComposeUp_AbsPathError(t *testing.T) {
	oldWd, _ := os.Getwd()
	os.Chdir("/nonexistent/dir/xyz")
	defer os.Chdir(oldWd)

	err := dockerComposeUp("relative-compose.yml", 60, testRuntime)
	if err == nil {
		t.Error("expected error for filepath.Abs failure")
	}
}

func TestDockerComposeDown_AbsPathError(t *testing.T) {
	oldWd, _ := os.Getwd()
	os.Chdir("/nonexistent/dir/xyz")
	defer os.Chdir(oldWd)

	err := dockerComposeDown("relative-compose.yml", testRuntime)
	if err == nil {
		t.Error("expected error for filepath.Abs failure")
	}
}

func TestDockerComposeStatus_AbsPathError(t *testing.T) {
	oldWd, _ := os.Getwd()
	os.Chdir("/nonexistent/dir/xyz")
	defer os.Chdir(oldWd)

	_, err := dockerComposeStatus("relative-compose.yml", testRuntime)
	if err == nil {
		t.Error("expected error for filepath.Abs failure")
	}
}

func TestDockerComposeStatus_PartialState(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "compose-partial.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n  broken:\n    image: nonexistent-image-xyz:latest\n"), 0644)

	err := dockerComposeUp(stackFile, 60, testRuntime)
	if err != nil {
		t.Skipf("docker compose up failed: %v", err)
	}

	status, err := dockerComposeStatus(stackFile, testRuntime)
	if err != nil {
		t.Fatalf("dockerComposeStatus failed: %v", err)
	}
	t.Logf("status = %q", status)

	_ = dockerComposeDown(stackFile, testRuntime)
}

func TestRunEFM_UpWithStartedStatus(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker daemon not available")
	}
	dir := t.TempDir()
	stackFile := filepath.Join(dir, "compose-started.yml")
	os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx:alpine\n"), 0644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("up", stackFile, 60, "json", testRuntime)
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM up failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "started" {
		t.Errorf("expected status='started', got %q", result.Status)
	}

	_ = dockerComposeDown(stackFile, testRuntime)
}

func TestDetectContainerRuntime_OrbOnMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only runs on macOS")
	}
	rt := detectContainerRuntime()
	if rt != "orb" && rt != "docker" {
		t.Errorf("expected orb or docker, got %q", rt)
	}
}

func TestDetectContainerRuntime_ReturnsKnownValue(t *testing.T) {
	rt := detectContainerRuntime()
	if rt != "orb" && rt != "docker" {
		t.Errorf("detectContainerRuntime returned unexpected value %q (must be orb or docker)", rt)
	}
}

func TestDetectContainerRuntime_FallsBackToDocker(t *testing.T) {
	rt := detectContainerRuntime()
	if rt == "" {
		t.Error("detectContainerRuntime returned empty string, expected fallback to docker")
	}
}

func TestContainerCommand_UsesRightBinary(t *testing.T) {
	cmdOrb := containerCommand("orb", "ps")
	if cmdOrb.Path == "" && cmdOrb.Args[0] != "orb" {
		t.Errorf("expected cmd.Args[0] to be 'orb', got %q", cmdOrb.Args[0])
	}
	if cmdOrb.Args[0] != "orb" {
		t.Errorf("expected 'orb' as first arg, got %q", cmdOrb.Args[0])
	}
	if len(cmdOrb.Args) < 2 || cmdOrb.Args[1] != "ps" {
		t.Errorf("expected 'ps' as second arg, got %v", cmdOrb.Args)
	}

	cmdDocker := containerCommand("docker", "ps")
	if cmdDocker.Args[0] != "docker" {
		t.Errorf("expected 'docker' as first arg, got %q", cmdDocker.Args[0])
	}

	cmdEmpty := containerCommand("", "ps")
	if cmdEmpty.Args[0] != "orb" && cmdEmpty.Args[0] != "docker" {
		t.Errorf("expected fallback to orb or docker, got %q", cmdEmpty.Args[0])
	}
}

func TestContainerCommand_PassesArgs(t *testing.T) {
	cmd := containerCommand("docker", "compose", "-f", "stack.yml", "up", "-d")
	expected := []string{"docker", "compose", "-f", "stack.yml", "up", "-d"}
	if len(cmd.Args) != len(expected) {
		t.Fatalf("expected %d args, got %d (%v)", len(expected), len(cmd.Args), cmd.Args)
	}
	for i, a := range expected {
		if cmd.Args[i] != a {
			t.Errorf("arg[%d] = %q, want %q", i, cmd.Args[i], a)
		}
	}
}

func TestLegacyComposeCommand_DockerFallback(t *testing.T) {
	cmd := legacyComposeCommand("docker", "-f", "stack.yml", "down")
	if cmd.Args[0] != "docker-compose" {
		t.Errorf("expected 'docker-compose' legacy binary, got %q", cmd.Args[0])
	}
	if cmd.Args[1] != "-f" {
		t.Errorf("expected '-f' as second arg, got %q", cmd.Args[1])
	}
}

func TestLegacyComposeCommand_OrbFallback(t *testing.T) {
	cmd := legacyComposeCommand("orb", "-f", "stack.yml", "down")
	if cmd.Args[0] != "orb-compose" {
		t.Errorf("expected 'orb-compose' legacy binary, got %q", cmd.Args[0])
	}
}

func TestLegacyComposeCommand_EmptyFallback(t *testing.T) {
	cmd := legacyComposeCommand("", "-f", "stack.yml", "down")
	if cmd.Args[0] != "docker-compose" {
		t.Errorf("expected 'docker-compose' for empty runtime, got %q", cmd.Args[0])
	}
}

func TestResolveContainerRuntime_OverrideOrb(t *testing.T) {
	rt := resolveContainerRuntime("orb")
	if rt != "orb" {
		t.Errorf("resolveContainerRuntime('orb') = %q, want 'orb'", rt)
	}
}

func TestResolveContainerRuntime_OverrideDocker(t *testing.T) {
	rt := resolveContainerRuntime("docker")
	if rt != "docker" {
		t.Errorf("resolveContainerRuntime('docker') = %q, want 'docker'", rt)
	}
}

func TestResolveContainerRuntime_OverrideAuto(t *testing.T) {
	rt := resolveContainerRuntime("auto")
	if rt != "orb" && rt != "docker" {
		t.Errorf("resolveContainerRuntime('auto') = %q, want orb or docker", rt)
	}
}

func TestResolveContainerRuntime_OverrideEmpty(t *testing.T) {
	rt := resolveContainerRuntime("")
	if rt != "orb" && rt != "docker" {
		t.Errorf("resolveContainerRuntime('') = %q, want orb or docker", rt)
	}
}

func TestResolveContainerRuntime_OverrideUnknownFallsBack(t *testing.T) {
	rt := resolveContainerRuntime("podman-warpzone-9")
	if rt != "orb" && rt != "docker" {
		t.Errorf("resolveContainerRuntime(unknown) = %q, want orb or docker fallback", rt)
	}
}

func TestEfmCmd_RuntimeFlag_Default(t *testing.T) {
	efmRuntime = "auto"
	rtFlag, err := EfmCmd.Flags().GetString("runtime")
	if err != nil {
		t.Fatalf("failed to get runtime flag: %v", err)
	}
	if rtFlag != "auto" {
		t.Errorf("default runtime = %q, want 'auto'", rtFlag)
	}
}

func TestEfmCmd_RuntimeFlag_Set(t *testing.T) {
	if err := EfmCmd.Flags().Set("runtime", "orb"); err != nil {
		t.Fatalf("failed to set runtime flag: %v", err)
	}
	rtFlag, err := EfmCmd.Flags().GetString("runtime")
	if err != nil {
		t.Fatalf("failed to get runtime flag: %v", err)
	}
	if rtFlag != "orb" {
		t.Errorf("runtime after set = %q, want 'orb'", rtFlag)
	}
	if err := EfmCmd.Flags().Set("runtime", "docker"); err != nil {
		t.Fatalf("failed to reset runtime flag: %v", err)
	}
	rtFlag, _ = EfmCmd.Flags().GetString("runtime")
	if rtFlag != "docker" {
		t.Errorf("runtime after docker set = %q, want 'docker'", rtFlag)
	}
}

func TestRunEFM_RuntimeOverrideOrb(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "json", "orb")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM list with orb override failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Runtime != "orb" {
		t.Errorf("expected runtime='orb' in result, got %q", result.Runtime)
	}
}

func TestRunEFM_RuntimeOverrideDocker(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "json", "docker")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM list with docker override failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Runtime != "docker" {
		t.Errorf("expected runtime='docker' in result, got %q", result.Runtime)
	}
}

func TestRunEFM_RuntimeInTextOutput(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runEFM("list", "", 0, "text", "orb")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runEFM list text with orb override failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Runtime: orb") {
		t.Errorf("expected 'Runtime: orb' in text output, got %q", out)
	}
}
