// SPDX-License-Identifier: MIT
// Purpose: coverage tests for commands.go skill subcommand surface.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
)

func TestNewSkillCmd_HasDoctor(t *testing.T) {
	cmd := NewSkillCmd()
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"status", "doctor", "install", "list", "uninstall"} {
		if !names[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}

func runSkillCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewSkillCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	return &out, cmd.Execute()
}

func TestSkillCmd_Doctor(t *testing.T) {
	orig := skillmgrDoctorHook
	skillmgrDoctorHook = func(ctx context.Context) []skillmgr.SkillStatus {
		return []skillmgr.SkillStatus{
			{Name: "websearch", Installed: true, Runnable: true, Detail: "1 tools"},
			{Name: "scheduler", Installed: false, Runnable: false, Detail: "not installed: /tmp/scheduler"},
		}
	}
	defer func() { skillmgrDoctorHook = orig }()

	out, err := runSkillCmd(t, "doctor")
	if err != nil {
		t.Fatalf("doctor error: %v\noutput: %s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "websearch") || !strings.Contains(s, "scheduler") {
		t.Errorf("expected both skills in output, got %q", s)
	}
	if !strings.Contains(s, "1 skill(s) are not runnable") {
		t.Errorf("expected sick summary, got %q", s)
	}
}

func TestSkillCmd_DoctorAllHealthy(t *testing.T) {
	orig := skillmgrDoctorHook
	skillmgrDoctorHook = func(ctx context.Context) []skillmgr.SkillStatus {
		return []skillmgr.SkillStatus{
			{Name: "websearch", Installed: true, Runnable: true, Detail: "1 tools"},
		}
	}
	defer func() { skillmgrDoctorHook = orig }()

	out, err := runSkillCmd(t, "doctor")
	if err != nil {
		t.Fatalf("doctor error: %v\noutput: %s", err, out.String())
	}
	if !strings.Contains(out.String(), "All known ecosystem skills are runnable") {
		t.Errorf("expected healthy summary, got %q", out.String())
	}
}

func TestSkillCmd_DoctorJson(t *testing.T) {
	orig := skillmgrDoctorHook
	skillmgrDoctorHook = func(ctx context.Context) []skillmgr.SkillStatus {
		return []skillmgr.SkillStatus{
			{Name: "websearch", Installed: true, Runnable: true, Detail: "1 tools"},
		}
	}
	defer func() { skillmgrDoctorHook = orig }()

	out, err := runSkillCmd(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor json error: %v\noutput: %s", err, out.String())
	}
	var sts []skillmgr.SkillStatus
	if err := json.Unmarshal(out.Bytes(), &sts); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out.String())
	}
	if len(sts) != 1 || sts[0].Name != "websearch" {
		t.Errorf("unexpected json output: %+v", sts)
	}
}

func TestSkillCmd_InstallAll(t *testing.T) {
	orig := skillmgrInstallAllHook
	skillmgrInstallAllHook = func(ctx context.Context) ([]skillmgr.SkillStatus, error) {
		return []skillmgr.SkillStatus{
			{Name: "websearch", Installed: true, Runnable: true, Detail: "1 tools"},
			{Name: "scheduler", Installed: false, Runnable: false, Detail: "git clone failed"},
		}, nil
	}
	defer func() { skillmgrInstallAllHook = orig }()

	out, err := runSkillCmd(t, "install", "all")
	if err != nil {
		t.Fatalf("install all error: %v\noutput: %s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "OK   websearch") {
		t.Errorf("expected websearch OK, got %q", s)
	}
	if !strings.Contains(s, "FAIL scheduler") {
		t.Errorf("expected scheduler FAIL, got %q", s)
	}
}

func TestSkillCmd_InstallAllJson(t *testing.T) {
	orig := skillmgrInstallAllHook
	skillmgrInstallAllHook = func(ctx context.Context) ([]skillmgr.SkillStatus, error) {
		return []skillmgr.SkillStatus{
			{Name: "websearch", Installed: true, Runnable: true, Detail: "1 tools"},
		}, nil
	}
	defer func() { skillmgrInstallAllHook = orig }()

	out, err := runSkillCmd(t, "install", "all", "--json")
	if err != nil {
		t.Fatalf("install all json error: %v\noutput: %s", err, out.String())
	}
	var sts []skillmgr.SkillStatus
	if err := json.Unmarshal(out.Bytes(), &sts); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out.String())
	}
	if len(sts) != 1 || sts[0].Name != "websearch" {
		t.Errorf("unexpected json output: %+v", sts)
	}
}

func TestSkillCmd_InstallAllReturnsError(t *testing.T) {
	orig := skillmgrInstallAllHook
	skillmgrInstallAllHook = func(ctx context.Context) ([]skillmgr.SkillStatus, error) {
		return []skillmgr.SkillStatus{
			{Name: "scheduler", Installed: false, Runnable: false, Detail: "git clone failed"},
		}, &skillInstallError{count: 1}
	}
	defer func() { skillmgrInstallAllHook = orig }()

	_, err := runSkillCmd(t, "install", "all")
	if err == nil {
		t.Fatal("expected error when install all fails")
	}
}

// skillInstallError is a minimal error type for testing the install-all error path.
type skillInstallError struct {
	count int
}

func (e *skillInstallError) Error() string {
	return "1 skill(s) failed to install"
}
