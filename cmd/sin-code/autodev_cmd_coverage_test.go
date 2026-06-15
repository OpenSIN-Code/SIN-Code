// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for autodev_cmd.go.
// Docs: autodev_cmd.go
package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func resetAutodevHooks(t *testing.T) {
	t.Helper()
	orig := autodevHookVars
	t.Cleanup(func() { autodevHookVars = orig })
}

func runAutodevCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewAutodevCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestAutodevSetup(t *testing.T) {
	resetAutodevHooks(t)
	autodevHookVars.resolveAutodevBin = func() error { return nil }
	autodevHookVars.defaultBin = func() string { return "autodev" }
	var gotBin string
	var gotArgs []string
	autodevHookVars.runPassthrough = func(ctx context.Context, bin string, args ...string) error {
		gotBin = bin
		gotArgs = args
		return nil
	}
	out, err := runAutodevCmd(t, "setup")
	if err != nil {
		t.Fatal(err)
	}
	if gotBin != "autodev" {
		t.Errorf("expected bin autodev, got %q", gotBin)
	}
	if len(gotArgs) != 3 || gotArgs[0] != "init" || gotArgs[1] != "--json" || gotArgs[2] != "." {
		t.Errorf("unexpected args: %v", gotArgs)
	}
	if out.String() != "" {
		t.Errorf("expected empty output, got %q", out.String())
	}
}

func TestAutodevSetupResolveError(t *testing.T) {
	resetAutodevHooks(t)
	autodevHookVars.resolveAutodevBin = func() error { return errors.New("not installed") }
	autodevHookVars.defaultBin = func() string { return "autodev" }
	_, err := runAutodevCmd(t, "setup")
	if err == nil || !strings.Contains(err.Error(), "autodev bridge") {
		t.Fatalf("expected resolve error, got %v", err)
	}
}

func TestAutodevSetupRunError(t *testing.T) {
	resetAutodevHooks(t)
	autodevHookVars.resolveAutodevBin = func() error { return nil }
	autodevHookVars.runPassthrough = func(ctx context.Context, bin string, args ...string) error {
		return errors.New("run boom")
	}
	_, err := runAutodevCmd(t, "setup")
	if err == nil || !strings.Contains(err.Error(), "run boom") {
		t.Fatalf("expected run error, got %v", err)
	}
}

func TestAutodevDoctor(t *testing.T) {
	resetAutodevHooks(t)
	autodevHookVars.resolveAutodevBin = func() error { return nil }
	autodevHookVars.defaultBin = func() string { return "autodev" }
	var gotArgs []string
	autodevHookVars.runPassthrough = func(ctx context.Context, bin string, args ...string) error {
		gotArgs = args
		return nil
	}
	out, err := runAutodevCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "status" || gotArgs[1] != "--json" {
		t.Errorf("unexpected args: %v", gotArgs)
	}
	if out.String() != "" {
		t.Errorf("expected empty output, got %q", out.String())
	}
}

func TestAutodevDoctorResolveError(t *testing.T) {
	resetAutodevHooks(t)
	autodevHookVars.resolveAutodevBin = func() error { return errors.New("not installed") }
	autodevHookVars.defaultBin = func() string { return "autodev" }
	_, err := runAutodevCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "autodev bridge") {
		t.Fatalf("expected resolve error, got %v", err)
	}
}

func TestAutodevVersion(t *testing.T) {
	resetAutodevHooks(t)
	autodevHookVars.version = func() (string, error) { return "v0.4.0", nil }
	out, err := runAutodevCmd(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "v0.4.0\n" {
		t.Errorf("expected version output, got %q", out.String())
	}
}

func TestAutodevVersionError(t *testing.T) {
	resetAutodevHooks(t)
	autodevHookVars.version = func() (string, error) { return "", errors.New("version boom") }
	out, err := runAutodevCmd(t, "version")
	if err == nil || !strings.Contains(err.Error(), "version boom") {
		t.Fatalf("expected version error, got %v", err)
	}
	if !strings.Contains(out.String(), "autodev version probe failed") {
		t.Errorf("expected error message, got %q", out.String())
	}
}

func TestAutodevCustomBin(t *testing.T) {
	resetAutodevHooks(t)
	autodevHookVars.resolveAutodevBin = func() error { return nil }
	autodevHookVars.defaultBin = func() string { return "/opt/autodev" }
	var gotBin string
	autodevHookVars.runPassthrough = func(ctx context.Context, bin string, args ...string) error {
		gotBin = bin
		return nil
	}
	_, err := runAutodevCmd(t, "setup")
	if err != nil {
		t.Fatal(err)
	}
	if gotBin != "/opt/autodev" {
		t.Errorf("expected custom bin, got %q", gotBin)
	}
}

func TestAutodevDefaultHooks(t *testing.T) {
	resetAutodevHooks(t)
	// Exercise the default runPassthrough wrapper with a no-op command.
	_ = autodevHookVars.runPassthrough(context.Background(), "true")
}
