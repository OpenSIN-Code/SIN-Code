// SPDX-License-Identifier: MIT
// Purpose: unit tests for container.go. Uses a fake `docker` binary on PATH so
// the suite does not require a real Docker daemon.
package autonomy

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type recordingRunner struct {
	image, workspace, command string
	output                    string
	err                       error
}

func (r *recordingRunner) RunInContainer(ctx context.Context, image, workspace, command string) (string, error) {
	r.image = image
	r.workspace = workspace
	r.command = command
	return r.output, r.err
}

func TestNoopRunner(t *testing.T) {
	var runner ContainerRunner = &NoopRunner{}
	out, err := runner.RunInContainer(context.Background(), "img", "/ws", "cmd")
	if err == nil {
		t.Fatal("expected NoopRunner to return an error")
	}
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("expected error to mention 'disabled', got %v", err)
	}
}

func TestDockerRunner_EmptyImage(t *testing.T) {
	runner := NewDockerRunner()
	_, err := runner.RunInContainer(context.Background(), "", "/ws", "cmd")
	if err == nil {
		t.Fatal("expected error for empty image")
	}
	if !strings.Contains(err.Error(), "image is empty") {
		t.Errorf("expected image error, got %v", err)
	}
}

func TestDockerRunner_EmptyWorkspace(t *testing.T) {
	runner := NewDockerRunner()
	_, err := runner.RunInContainer(context.Background(), "alpine", "", "cmd")
	if err == nil {
		t.Fatal("expected error for empty workspace")
	}
}

func TestDockerRunner_EmptyCommand(t *testing.T) {
	runner := NewDockerRunner()
	_, err := runner.RunInContainer(context.Background(), "alpine", "/ws", "")
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestDockerRunner_RunInContainer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker script is Unix-specific")
	}

	tmp := t.TempDir()
	docker := filepath.Join(tmp, "docker")
	script := `#!/bin/sh
printf '%s\n' "$@"`
	if err := os.WriteFile(docker, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	runner := NewDockerRunner()
	out, err := runner.RunInContainer(context.Background(), "alpine:latest", "/tmp/ws", "echo hello")
	if err != nil {
		t.Fatalf("fake docker run failed: %v", err)
	}

	for _, want := range []string{"--rm", "-v", "/tmp/ws:/workspace", "-w", "/workspace", "alpine:latest", "sh", "-c", "echo hello"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing expected arg %q:\n%s", want, out)
		}
	}
}

func TestRecordingRunner(t *testing.T) {
	rec := &recordingRunner{output: "ok"}
	out, err := rec.RunInContainer(context.Background(), "img", "/ws", "cmd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Errorf("expected output 'ok', got %q", out)
	}
	if rec.image != "img" || rec.workspace != "/ws" || rec.command != "cmd" {
		t.Errorf("unexpected recorded args: image=%q workspace=%q command=%q", rec.image, rec.workspace, rec.command)
	}
}
