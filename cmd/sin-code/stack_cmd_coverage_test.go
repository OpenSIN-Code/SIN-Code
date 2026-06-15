// SPDX-License-Identifier: MIT
// Purpose: coverage tests for stack_cmd.go — exercises install/doctor
// subcommands and their JSON/human output paths using package-level hooks.
// Docs: stack.doc.md
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/stack"
)

type stackErrWriter struct{ err error }

func (e stackErrWriter) Write(p []byte) (int, error) { return 0, e.err }

func saveStackHooks(t *testing.T) {
	t.Helper()
	origInstall := stackInstallHook
	origDoctor := stackDoctorHook
	origFormat := stackFormatHook
	t.Cleanup(func() {
		stackInstallHook = origInstall
		stackDoctorHook = origDoctor
		stackFormatHook = origFormat
	})
}

func runStackCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewStackCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestStackInstallHumanOK(t *testing.T) {
	saveStackHooks(t)
	stackInstallHook = func(stack.InstallOptions) stack.Report {
		return stack.Report{AllOK: true, Components: []stack.Component{{Name: "x", OK: true}}}
	}
	stackFormatHook = func(stack.Report) string { return "stack ok\n" }
	out, err := runStackCmd(t, "install")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "stack ok\n" {
		t.Errorf("unexpected output: %q", out.String())
	}
}

func TestStackInstallHumanDegraded(t *testing.T) {
	saveStackHooks(t)
	stackInstallHook = func(stack.InstallOptions) stack.Report {
		return stack.Report{AllOK: false, Components: []stack.Component{{Name: "x", OK: false}}}
	}
	stackFormatHook = func(stack.Report) string { return "stack bad\n" }
	_, err := runStackCmd(t, "install")
	if err == nil || !strings.Contains(err.Error(), "stack install completed with errors") {
		t.Fatalf("expected degraded error, got %v", err)
	}
}

func TestStackInstallJSON(t *testing.T) {
	saveStackHooks(t)
	stackInstallHook = func(stack.InstallOptions) stack.Report {
		return stack.Report{AllOK: true, Components: []stack.Component{{Name: "x", OK: true}}}
	}
	out, err := runStackCmd(t, "install", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["all_ok"] != true {
		t.Errorf("expected all_ok true, got %v", result["all_ok"])
	}
}

func TestStackInstallJSONEncodeError(t *testing.T) {
	saveStackHooks(t)
	stackInstallHook = func(stack.InstallOptions) stack.Report {
		return stack.Report{AllOK: true, Components: []stack.Component{{Name: "x", OK: true}}}
	}
	cmd := NewStackCmd()
	cmd.SetArgs([]string{"install", "--json"})
	setOutAll(cmd, stackErrWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestStackInstallFlags(t *testing.T) {
	saveStackHooks(t)
	var got stack.InstallOptions
	stackInstallHook = func(opts stack.InstallOptions) stack.Report {
		got = opts
		return stack.Report{AllOK: true, Components: []stack.Component{{Name: "x", OK: true, Skipped: false}}}
	}
	_, err := runStackCmd(t, "install", "--skip-superpowers", "--skip-dox", "--skip-vane", "--agents-md", "AGENTS.md", "--vane-url", "http://x")
	if err != nil {
		t.Fatal(err)
	}
	if !got.SkipSuperpowers || !got.SkipDox || !got.SkipVane || got.AgentsMDPath != "AGENTS.md" || got.VaneURL != "http://x" {
		t.Errorf("unexpected options: %+v", got)
	}
}

func TestStackDoctorHumanOK(t *testing.T) {
	saveStackHooks(t)
	stackDoctorHook = func(string) stack.Report {
		return stack.Report{AllOK: true, Components: []stack.Component{{Name: "x", OK: true}}}
	}
	stackFormatHook = func(stack.Report) string { return "doctor ok\n" }
	out, err := runStackCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "doctor ok\n" {
		t.Errorf("unexpected output: %q", out.String())
	}
}

func TestStackDoctorHumanDegraded(t *testing.T) {
	saveStackHooks(t)
	stackDoctorHook = func(string) stack.Report {
		return stack.Report{AllOK: false, Components: []stack.Component{{Name: "x", OK: false}}}
	}
	stackFormatHook = func(stack.Report) string { return "doctor bad\n" }
	_, err := runStackCmd(t, "doctor", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "stack doctor found problems") {
		t.Fatalf("expected degraded error, got %v", err)
	}
}

func TestStackDoctorJSON(t *testing.T) {
	saveStackHooks(t)
	stackDoctorHook = func(string) stack.Report {
		return stack.Report{AllOK: true, Components: []stack.Component{{Name: "x", OK: true}}}
	}
	out, err := runStackCmd(t, "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["all_ok"] != true {
		t.Errorf("expected all_ok true, got %v", result["all_ok"])
	}
}

func TestStackDoctorJSONEncodeError(t *testing.T) {
	saveStackHooks(t)
	stackDoctorHook = func(string) stack.Report {
		return stack.Report{AllOK: true, Components: []stack.Component{{Name: "x", OK: true}}}
	}
	cmd := NewStackCmd()
	cmd.SetArgs([]string{"doctor", "--json"})
	setOutAll(cmd, stackErrWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}
