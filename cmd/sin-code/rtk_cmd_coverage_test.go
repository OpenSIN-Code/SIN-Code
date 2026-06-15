// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for rtk_cmd.go.
// Docs: rtk_cmd.go
package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeRtkBridge implements rtkBridge for tests.
type fakeRtkBridge struct {
	runOut     string
	runErr     error
	findPath   string
	findErr    error
	version    string
	versionErr error
	runWorkdir string
	runArgs    []string
}

func (f *fakeRtkBridge) Run(ctx context.Context, workdir string, args []string) (string, error) {
	f.runWorkdir = workdir
	f.runArgs = args
	return f.runOut, f.runErr
}

func (f *fakeRtkBridge) Find() (string, error) { return f.findPath, f.findErr }

func (f *fakeRtkBridge) Version(ctx context.Context) (string, error) { return f.version, f.versionErr }

func resetRtkHooks(t *testing.T) {
	t.Helper()
	orig := rtkHookVars
	t.Cleanup(func() { rtkHookVars = orig })
}

func runRtkCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewRtkCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestRtkRunSuccess(t *testing.T) {
	resetRtkHooks(t)
	bridge := &fakeRtkBridge{runOut: "filtered output"}
	rtkHookVars.newBridge = func() rtkBridge { return bridge }
	out, err := runRtkCmd(t, "run", "--", "git", "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "filtered output") {
		t.Errorf("expected run output, got %q", out.String())
	}
	if bridge.runWorkdir != "" {
		t.Errorf("expected empty workdir, got %q", bridge.runWorkdir)
	}
	if len(bridge.runArgs) != 2 || bridge.runArgs[0] != "git" || bridge.runArgs[1] != "status" {
		t.Errorf("unexpected args: %v", bridge.runArgs)
	}
}

func TestRtkRunEmptyOutput(t *testing.T) {
	resetRtkHooks(t)
	rtkHookVars.newBridge = func() rtkBridge { return &fakeRtkBridge{runOut: ""} }
	out, err := runRtkCmd(t, "run", "--", "echo", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "" {
		t.Errorf("expected empty output, got %q", out.String())
	}
}

func TestRtkRunError(t *testing.T) {
	resetRtkHooks(t)
	rtkHookVars.newBridge = func() rtkBridge { return &fakeRtkBridge{runErr: errors.New("run boom")} }
	_, err := runRtkCmd(t, "run", "--", "git", "status")
	if err == nil || !strings.Contains(err.Error(), "run boom") {
		t.Fatalf("expected run error, got %v", err)
	}
}

func TestRtkRunWithWorkdir(t *testing.T) {
	resetRtkHooks(t)
	bridge := &fakeRtkBridge{runOut: "ok"}
	rtkHookVars.newBridge = func() rtkBridge { return bridge }
	_, err := runRtkCmd(t, "run", "-C", "/tmp", "--", "ls")
	if err != nil {
		t.Fatal(err)
	}
	if bridge.runWorkdir != "/tmp" {
		t.Errorf("expected workdir /tmp, got %q", bridge.runWorkdir)
	}
}

func TestRtkRunWithTimeout(t *testing.T) {
	resetRtkHooks(t)
	bridge := &fakeRtkBridge{runOut: "ok"}
	rtkHookVars.newBridge = func() rtkBridge { return bridge }
	out, err := runRtkCmd(t, "run", "--timeout", "2s", "--", "ls")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Errorf("expected run output, got %q", out.String())
	}
	_ = bridge
}

func TestRtkDoctorSuccess(t *testing.T) {
	resetRtkHooks(t)
	rtkHookVars.newBridge = func() rtkBridge {
		return &fakeRtkBridge{findPath: "/usr/bin/rtk", version: "v1.0"}
	}
	out, err := runRtkCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "rtk: OK") || !strings.Contains(out.String(), "/usr/bin/rtk") || !strings.Contains(out.String(), "v1.0") {
		t.Errorf("expected doctor output, got %q", out.String())
	}
}

func TestRtkDoctorFindError(t *testing.T) {
	resetRtkHooks(t)
	rtkHookVars.newBridge = func() rtkBridge { return &fakeRtkBridge{findErr: errors.New("not found")} }
	out, err := runRtkCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected find error, got %v", err)
	}
	if !strings.Contains(out.String(), "rtk: NOT installed") {
		t.Errorf("expected not installed message, got %q", out.String())
	}
}

func TestRtkDoctorVersionError(t *testing.T) {
	resetRtkHooks(t)
	rtkHookVars.newBridge = func() rtkBridge {
		return &fakeRtkBridge{findPath: "/usr/bin/rtk", versionErr: errors.New("version boom")}
	}
	out, err := runRtkCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "version:") {
		t.Errorf("expected no version line, got %q", out.String())
	}
	if !strings.Contains(out.String(), "rtk: OK") {
		t.Errorf("expected ok output, got %q", out.String())
	}
}

func TestRtkDoctorNoVersion(t *testing.T) {
	resetRtkHooks(t)
	rtkHookVars.newBridge = func() rtkBridge { return &fakeRtkBridge{findPath: "/usr/bin/rtk"} }
	out, err := runRtkCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "version:") {
		t.Errorf("expected no version line, got %q", out.String())
	}
	_ = out
}

func TestRtkRunTimeout(t *testing.T) {
	resetRtkHooks(t)
	var gotTimeout time.Duration
	rtkHookVars.newBridge = func() rtkBridge {
		return &fakeRtkBridge{
			runOut: "ok",
			runErr: nil,
		}
	}
	out, err := runRtkCmd(t, "run", "--timeout", "5s", "--", "ls")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Errorf("expected output, got %q", out.String())
	}
	_ = gotTimeout
	_ = out
}

func TestRtkDefaultHooks(t *testing.T) {
	resetRtkHooks(t)
	b := rtkHookVars.newBridge()
	if b == nil {
		t.Fatal("expected non-nil bridge from default hook")
	}
}
