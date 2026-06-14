// SPDX-License-Identifier: MIT
// Purpose: Fast EFM tests that mock container runtimes via fake PATH binaries.
// These tests run without a real Docker daemon and are included in the fast suite.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeFakeContainerScript creates an executable shell script at the given path.
func makeFakeContainerScript(t *testing.T, path, body string) {
	t.Helper()
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake container script %s: %v", path, err)
	}
}

func TestEFM_ListWithFakeDocker(t *testing.T) {
	binDir := t.TempDir()
	// Fake docker ps output: two services, one with ports, one without.
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"),
		`printf 'web\trunning\t80/tcp\tnginx\ndb\texited\t\tpostgres\n'`)
	t.Setenv("PATH", binDir)

	get := captureStdout(t)
	if err := runEFM("list", "", 0, "text", "docker"); err != nil {
		t.Fatalf("runEFM list text failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "EFM: list") {
		t.Errorf("expected 'EFM: list' in text output, got %q", out)
	}
	if !strings.Contains(out, "web") || !strings.Contains(out, "db") {
		t.Errorf("expected services in text output, got %q", out)
	}
	if !strings.Contains(out, "80/tcp") {
		t.Errorf("expected ports in text output, got %q", out)
	}

	get = captureStdout(t)
	if err := runEFM("list", "", 0, "json", "docker"); err != nil {
		t.Fatalf("runEFM list json failed: %v", err)
	}
	outJSON := get()
	var result efmResult
	if err := json.Unmarshal([]byte(outJSON), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Action != "list" {
		t.Errorf("expected action='list', got %q", result.Action)
	}
	if len(result.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(result.Services))
	}
	if result.Runtime != "docker" {
		t.Errorf("expected runtime='docker', got %q", result.Runtime)
	}
}

func TestEFM_UpWithFakeDocker(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 0`)
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("up", stackFile, 3600, "json", "docker"); err != nil {
		t.Fatalf("runEFM up json failed: %v", err)
	}
	out := get()
	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Action != "up" {
		t.Errorf("expected action='up', got %q", result.Action)
	}
	if result.Status != "started" {
		t.Errorf("expected status='started', got %q", result.Status)
	}

	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	metadataFile := filepath.Join(metadataDir, metadataKey(stackFile))
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		t.Fatalf("expected metadata file to be written: %v", err)
	}
	var meta map[string]string
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("expected valid metadata JSON: %v", err)
	}
	if meta["ttl"] != "3600" {
		t.Errorf("expected ttl=3600, got %q", meta["ttl"])
	}
	if meta["stack"] != stackFile {
		t.Errorf("expected stack path in metadata, got %q", meta["stack"])
	}
}

func TestEFM_UpNoTTLWithFakeDocker(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 0`)
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "no-ttl-compose.yml")
	if err := os.WriteFile(stackFile, []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	if err := dockerComposeUp(stackFile, 0, "docker"); err != nil {
		t.Fatalf("dockerComposeUp failed: %v", err)
	}

	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	metadataFile := filepath.Join(metadataDir, filepath.Base(stackFile)+".meta")
	if _, err := os.Stat(metadataFile); err == nil {
		t.Error("expected no metadata file when TTL=0")
	}
}

func TestEFM_DownWithFakeDocker(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 0`)
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\nservices:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	metadataDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code", "efm")
	metadataFile := filepath.Join(metadataDir, metadataKey(stackFile))
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		t.Fatalf("mkdir metadata dir: %v", err)
	}
	if err := os.WriteFile(metadataFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write metadata file: %v", err)
	}

	if err := dockerComposeDown(stackFile, "docker"); err != nil {
		t.Fatalf("dockerComposeDown failed: %v", err)
	}
	if _, err := os.Stat(metadataFile); err == nil {
		t.Error("expected metadata file to be removed after down")
	}
}

func TestEFM_DownTextOutput(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 0`)
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("down", stackFile, 0, "text", "docker"); err != nil {
		t.Fatalf("runEFM down text failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "EFM: down") || !strings.Contains(out, "Status: stopped") {
		t.Errorf("expected down text output, got %q", out)
	}
}

func TestEFM_UpTextOutput(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 0`)
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("up", stackFile, 0, "text", "docker"); err != nil {
		t.Fatalf("runEFM up text failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "EFM: up") || !strings.Contains(out, "Status: started") {
		t.Errorf("expected up text output, got %q", out)
	}
}

func TestEFM_ListError(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "docker-compose"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb-compose"), `exit 1`)
	t.Setenv("PATH", binDir)

	get := captureStdout(t)
	if err := runEFM("list", "", 0, "json", "docker"); err != nil {
		t.Fatalf("runEFM list should not return error, got: %v", err)
	}
	out := get()
	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "error" {
		t.Errorf("expected status='error', got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error in result")
	}
}

func TestEFM_UpWithStack_ComposeError(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "docker-compose"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb-compose"), `exit 1`)
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("up", stackFile, 0, "json", "docker"); err != nil {
		t.Fatalf("runEFM up should not return error, got: %v", err)
	}
	out := get()
	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "error" {
		t.Errorf("expected status='error', got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error in result")
	}
}

func TestEFM_DownWithStack_ComposeError(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "docker-compose"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb-compose"), `exit 1`)
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("down", stackFile, 0, "json", "docker"); err != nil {
		t.Fatalf("runEFM down should not return error, got: %v", err)
	}
	out := get()
	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "error" {
		t.Errorf("expected status='error', got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error in result")
	}
}

func TestEFM_StatusWithStack_AllRunning(t *testing.T) {
	binDir := t.TempDir()
	// docker compose ps with all running states.
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"),
		`if [ "$1" = "compose" ]; then shift; fi
		case "$*" in *"{{.State}}"*) printf 'running\nrunning\n' ;; esac
		exit 0`)
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("status", stackFile, 0, "json", "docker"); err != nil {
		t.Fatalf("runEFM status json failed: %v", err)
	}
	out := get()
	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Action != "status" {
		t.Errorf("expected action='status', got %q", result.Action)
	}
	if result.Status != "all running" {
		t.Errorf("expected status='all running', got %q", result.Status)
	}
}

func TestEFM_StatusWithStack_Partial(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"),
		`if [ "$1" = "compose" ]; then shift; fi
		case "$*" in *"{{.State}}"*) printf 'running\npaused\n' ;; esac
		exit 0`)
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("status", stackFile, 0, "text", "docker"); err != nil {
		t.Fatalf("runEFM status text failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "Status: partial") {
		t.Errorf("expected 'Status: partial' in text output, got %q", out)
	}
}

func TestEFM_StatusWithStack_NoContainers(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 0`)
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("status", stackFile, 0, "text", "docker"); err != nil {
		t.Fatalf("runEFM status text failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "Status: no containers running") {
		t.Errorf("expected 'Status: no containers running' in text output, got %q", out)
	}
}

func TestEFM_StatusWithStack_ComposeError(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "docker-compose"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb-compose"), `exit 1`)
	t.Setenv("PATH", binDir)

	dir := t.TempDir()
	stackFile := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(stackFile, []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	get := captureStdout(t)
	if err := runEFM("status", stackFile, 0, "json", "docker"); err != nil {
		t.Fatalf("runEFM status json should not return error, got: %v", err)
	}
	out := get()
	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
	if result.Status != "error" {
		t.Errorf("expected status='error', got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error in result")
	}
}

func TestEFM_RunComposeCandidatesFallback(t *testing.T) {
	binDir := t.TempDir()
	// docker fails, orb succeeds. isModern("orb") prepends "compose".
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb"), `exit 0`)
	t.Setenv("PATH", binDir)

	if err := runComposeCandidates("docker", []string{"-f", "x.yml", "up", "-d"}, false); err != nil {
		t.Fatalf("runComposeCandidates should fallback to orb, got: %v", err)
	}
}

func TestEFM_RunComposeCandidatesNoRuntime(t *testing.T) {
	// Empty PATH so no runtime is found.
	t.Setenv("PATH", "")
	err := runComposeCandidates("docker", []string{"-f", "x.yml", "up", "-d"}, false)
	if err == nil {
		t.Fatal("expected error when no runtime is found")
	}
	if !strings.Contains(err.Error(), "no container runtime binary found") {
		t.Errorf("expected 'no container runtime binary found' error, got %q", err.Error())
	}
}

func TestEFM_RunComposeCaptureError(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "docker-compose"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb-compose"), `exit 1`)
	t.Setenv("PATH", binDir)

	_, err := runComposeCapture("docker", []string{"-f", "x.yml", "ps"})
	if err == nil {
		t.Fatal("expected error from runComposeCapture")
	}
}

func TestEFM_ContainerCommand(t *testing.T) {
	cmd := containerCommand("docker", "ps")
	if cmd.Args[0] != "docker" {
		t.Errorf("expected args[0] 'docker', got %q", cmd.Args[0])
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "ps" {
		t.Errorf("expected args [docker ps], got %v", cmd.Args)
	}
	empty := containerCommand("", "ps")
	if empty.Args[0] != detectContainerRuntime() {
		t.Errorf("expected empty runtime to fall back to detected runtime, got %q", empty.Args[0])
	}
}

func TestEFM_IsModern(t *testing.T) {
	if !isModern("docker") {
		t.Error("expected docker to be modern")
	}
	if !isModern("orb") {
		t.Error("expected orb to be modern")
	}
	if isModern("docker-compose") {
		t.Error("expected docker-compose to not be modern")
	}
	if isModern("podman") {
		t.Error("expected podman to not be modern")
	}
}

func TestEFM_ComposeCandidates(t *testing.T) {
	assertOrder := func(rt string, want []string) {
		t.Helper()
		got := composeCandidates(rt)
		if len(got) != len(want) {
			t.Errorf("composeCandidates(%q) length = %d, want %d", rt, len(got), len(want))
			return
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("composeCandidates(%q)[%d] = %q, want %q", rt, i, got[i], want[i])
			}
		}
	}

	assertOrder("orb", []string{"orb", "orb-compose", "docker", "docker-compose"})
	assertOrder("docker", []string{"docker", "docker-compose", "orb", "orb-compose"})

	// Empty rt delegates to detectContainerRuntime. Force it to docker by
	// controlling PATH so the result is deterministic across platforms.
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 0`)
	t.Setenv("PATH", binDir)
	assertOrder("", []string{"docker", "docker-compose", "orb", "orb-compose"})
}

func TestEFM_ResolveContainerRuntime(t *testing.T) {
	if got := resolveContainerRuntime("orb"); got != "orb" {
		t.Errorf("resolveContainerRuntime(orb) = %q, want orb", got)
	}
	if got := resolveContainerRuntime("docker"); got != "docker" {
		t.Errorf("resolveContainerRuntime(docker) = %q, want docker", got)
	}
	if got := resolveContainerRuntime("auto"); got != "orb" && got != "docker" {
		t.Errorf("resolveContainerRuntime(auto) = %q, want orb or docker", got)
	}
	if got := resolveContainerRuntime(""); got != "orb" && got != "docker" {
		t.Errorf("resolveContainerRuntime() = %q, want orb or docker", got)
	}
	if got := resolveContainerRuntime("unknown"); got != "orb" && got != "docker" {
		t.Errorf("resolveContainerRuntime(unknown) = %q, want orb or docker", got)
	}
}

func TestEFM_DetectContainerRuntime(t *testing.T) {
	oldGOOS := efmGOOS
	defer func() { efmGOOS = oldGOOS }()

	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 0`)
	// On darwin, orb is preferred. On linux, docker is used directly.
	if runtime.GOOS == "darwin" {
		// Only docker present → falls back to docker.
		t.Setenv("PATH", binDir)
		if got := detectContainerRuntime(); got != "docker" {
			t.Errorf("detectContainerRuntime on darwin with only docker = %q, want docker", got)
		}
		// Both orb and docker present → orb wins.
		makeFakeContainerScript(t, filepath.Join(binDir, "orb"), `exit 0`)
		if got := detectContainerRuntime(); got != "orb" {
			t.Errorf("detectContainerRuntime on darwin with orb and docker = %q, want orb", got)
		}
	} else {
		t.Setenv("PATH", binDir)
		if got := detectContainerRuntime(); got != "docker" {
			t.Errorf("detectContainerRuntime on linux with docker = %q, want docker", got)
		}
	}

	// Linux branch: only docker is checked.
	efmGOOS = "linux"
	t.Setenv("PATH", binDir)
	if got := detectContainerRuntime(); got != "docker" {
		t.Errorf("detectContainerRuntime on linux = %q, want docker", got)
	}

	// Linux branch with no docker found still returns docker default.
	t.Setenv("PATH", "")
	if got := detectContainerRuntime(); got != "docker" {
		t.Errorf("detectContainerRuntime on linux without docker = %q, want docker", got)
	}
}

func TestEFM_ResolveComposeRuntime(t *testing.T) {
	if got := resolveComposeRuntime("docker"); got != "docker" {
		t.Errorf("resolveComposeRuntime(docker) = %q, want docker", got)
	}
	if got := resolveComposeRuntime("auto"); got != "orb" && got != "docker" {
		t.Errorf("resolveComposeRuntime(auto) = %q, want orb or docker", got)
	}
	if got := resolveComposeRuntime(""); got != "orb" && got != "docker" {
		t.Errorf("resolveComposeRuntime() = %q, want orb or docker", got)
	}
}

func TestEFM_UpMissingStack(t *testing.T) {
	if err := runEFM("up", "", 0, "text", "docker"); err == nil {
		t.Fatal("expected error for up without stack")
	}
}

func TestEFM_DownMissingStack(t *testing.T) {
	if err := runEFM("down", "", 0, "text", "docker"); err == nil {
		t.Fatal("expected error for down without stack")
	}
}

func TestEFM_StatusWithoutStack(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"),
		`printf 'web\trunning\t80/tcp\tnginx\n'`)
	t.Setenv("PATH", binDir)

	get := captureStdout(t)
	if err := runEFM("status", "", 0, "text", "docker"); err != nil {
		t.Fatalf("runEFM status without stack failed: %v", err)
	}
	out := get()
	if !strings.Contains(out, "EFM: status") {
		t.Errorf("expected status output, got %q", out)
	}
}

func TestEFM_StatusWithoutStack_Error(t *testing.T) {
	binDir := t.TempDir()
	makeFakeContainerScript(t, filepath.Join(binDir, "docker"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "docker-compose"), `exit 1`)
	makeFakeContainerScript(t, filepath.Join(binDir, "orb-compose"), `exit 1`)
	t.Setenv("PATH", binDir)

	get := captureStdout(t)
	if err := runEFM("status", "", 0, "json", "docker"); err != nil {
		t.Fatalf("runEFM status without stack should not return error, got: %v", err)
	}
	out := get()
	var result efmResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}
	if result.Status != "error" {
		t.Errorf("expected status='error', got %q", result.Status)
	}
}

func TestEFM_UnknownAction(t *testing.T) {
	if err := runEFM("unknown", "", 0, "text", "docker"); err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestEFM_DetectContainerRuntime_DarwinNoBinary(t *testing.T) {
	oldGOOS := efmGOOS
	defer func() { efmGOOS = oldGOOS }()
	efmGOOS = "darwin"
	t.Setenv("PATH", "")
	if got := detectContainerRuntime(); got != "docker" {
		t.Errorf("detectContainerRuntime on darwin with no binaries = %q, want docker", got)
	}
}

func TestEFM_ContainerCommand_EmptyFallback(t *testing.T) {
	old := efmDetectRuntime
	efmDetectRuntime = func() string { return "" }
	defer func() { efmDetectRuntime = old }()

	cmd := containerCommand("", "ps")
	if cmd.Args[0] != "docker" {
		t.Errorf("expected fallback to docker, got %q", cmd.Args[0])
	}
}

func TestEFM_DockerComposeUp_FileNotFound(t *testing.T) {
	if err := dockerComposeUp("/nonexistent/path/stack.yml", 0, "docker"); err == nil {
		t.Fatal("expected error for missing stack file")
	}
}

func TestEFM_DockerComposeDown_FileNotFound(t *testing.T) {
	if err := dockerComposeDown("/nonexistent/path/stack.yml", "docker"); err == nil {
		t.Fatal("expected error for missing stack file")
	}
}

func TestEFM_DockerComposeStatus_FileNotFound(t *testing.T) {
	if _, err := dockerComposeStatus("/nonexistent/path/stack.yml", "docker"); err == nil {
		t.Fatal("expected error for missing stack file")
	}
}

func TestEFM_DockerComposeUp_AbsError(t *testing.T) {
	old := efmFilepathAbs
	efmFilepathAbs = func(path string) (string, error) { return "", fmt.Errorf("forced abs error") }
	defer func() { efmFilepathAbs = old }()

	if err := dockerComposeUp("x.yml", 0, "docker"); err == nil {
		t.Fatal("expected error for abs failure")
	}
}

func TestEFM_DockerComposeDown_AbsError(t *testing.T) {
	old := efmFilepathAbs
	efmFilepathAbs = func(path string) (string, error) { return "", fmt.Errorf("forced abs error") }
	defer func() { efmFilepathAbs = old }()

	if err := dockerComposeDown("x.yml", "docker"); err == nil {
		t.Fatal("expected error for abs failure")
	}
}

func TestEFM_DockerComposeStatus_AbsError(t *testing.T) {
	old := efmFilepathAbs
	efmFilepathAbs = func(path string) (string, error) { return "", fmt.Errorf("forced abs error") }
	defer func() { efmFilepathAbs = old }()

	if _, err := dockerComposeStatus("x.yml", "docker"); err == nil {
		t.Fatal("expected error for abs failure")
	}
}

func TestEFM_RunComposeCapture_NoRuntime(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := runComposeCapture("docker", []string{"-f", "x.yml", "ps"})
	if err == nil {
		t.Fatal("expected error when no runtime is found")
	}
	if !strings.Contains(err.Error(), "no container runtime binary found") {
		t.Errorf("expected 'no container runtime binary found', got %q", err.Error())
	}
}

func TestEFM_ListDockerContainers_NoRuntime(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := listDockerContainers("docker")
	if err == nil {
		t.Fatal("expected error when no runtime is found")
	}
}

func TestEFM_ListDockerContainers_LookpathSkip(t *testing.T) {
	binDir := t.TempDir()
	// Only docker-compose is present, not docker/orb.
	makeFakeContainerScript(t, filepath.Join(binDir, "docker-compose"),
		`printf 'web\trunning\t80/tcp\tnginx\n'`)
	t.Setenv("PATH", binDir)

	_, err := listDockerContainers("docker")
	if err != nil {
		t.Fatalf("expected docker-compose to be tried: %v", err)
	}
}

func TestEFM_FilterServices(t *testing.T) {
	services := []efmService{
		{Name: "stack_web", Status: "running"},
		{Name: "stack_db", Status: "exited"},
		{Name: "other_app", Status: "running"},
	}
	filtered := filterServices(services, "stack")
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered services, got %d", len(filtered))
	}
}

func TestEFM_OutputTextEFM_Error(t *testing.T) {
	get := captureStdout(t)
	outputTextEFM(efmResult{Action: "up", Status: "error", Error: "boom"})
	out := get()
	if !strings.Contains(out, "boom") {
		t.Errorf("expected error text in output, got %q", out)
	}
}
