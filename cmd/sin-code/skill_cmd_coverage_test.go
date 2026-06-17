// SPDX-License-Identifier: MIT
// Purpose: coverage tests for skill_cmd.go — exercises status/install using
// package-level hooks so tests never clone real skill repos.
// Docs: skill_cmd.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
)

type skillErrWriter struct{ err error }

func (e skillErrWriter) Write(p []byte) (int, error) { return 0, e.err }

func saveSkillHooks(t *testing.T) {
	t.Helper()
	origStatus := skillmgrStatusHook
	origKnown := skillmgrKnownSkillsHook
	origInstall := skillmgrInstallHook
	t.Cleanup(func() {
		skillmgrStatusHook = origStatus
		skillmgrKnownSkillsHook = origKnown
		skillmgrInstallHook = origInstall
	})
}

func runSkillCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewSkillCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestSkillCmd_NewSkillCmd(t *testing.T) {
	cmd := NewSkillCmd()
	if cmd.Use != "skill" {
		t.Errorf("Use = %q, want skill", cmd.Use)
	}
	names := []string{}
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"status", "install"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing subcommand %q in %q", want, joined)
		}
	}
}

func TestSkillCmd_Status_Empty(t *testing.T) {
	saveSkillHooks(t)
	skillmgrStatusHook = func(context.Context) []skillmgr.SkillStatus { return nil }
	out, err := runSkillCmd(t, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "SKILL") {
		t.Errorf("expected header, got %q", out.String())
	}
}

func TestSkillCmd_Status_WithRows(t *testing.T) {
	saveSkillHooks(t)
	skillmgrStatusHook = func(context.Context) []skillmgr.SkillStatus {
		return []skillmgr.SkillStatus{
			{Name: "alpha", Installed: true, Runnable: true, Detail: "ok"},
			{Name: "beta", Installed: false, Runnable: false, Detail: "missing"},
		}
	}
	out, err := runSkillCmd(t, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "alpha") {
		t.Errorf("expected alpha, got %q", out.String())
	}
	if !strings.Contains(out.String(), "beta") {
		t.Errorf("expected beta, got %q", out.String())
	}
}

func TestSkillCmd_Status_JSON(t *testing.T) {
	saveSkillHooks(t)
	skillmgrStatusHook = func(context.Context) []skillmgr.SkillStatus {
		return []skillmgr.SkillStatus{
			{Name: "alpha", Installed: true, Runnable: true, Detail: "ok"},
		}
	}
	out, err := runSkillCmd(t, "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []skillmgr.SkillStatus
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json decode: %v: %q", err, out.String())
	}
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestSkillCmd_Status_JSONEncodeError(t *testing.T) {
	saveSkillHooks(t)
	skillmgrStatusHook = func(context.Context) []skillmgr.SkillStatus { return nil }
	cmd := NewSkillCmd()
	cmd.SetArgs([]string{"status", "--json"})
	setOutAll(cmd, skillErrWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestSkillCmd_Install_Single(t *testing.T) {
	saveSkillHooks(t)
	skillmgrInstallHook = func(context.Context, string) (*skillmgr.SkillStatus, error) {
		return &skillmgr.SkillStatus{Name: "alpha", Runnable: true, Detail: "ok"}, nil
	}
	out, err := runSkillCmd(t, "install", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "OK   alpha") {
		t.Errorf("expected OK alpha, got %q", out.String())
	}
}

func TestSkillCmd_Install_All(t *testing.T) {
	saveSkillHooks(t)
	skillmgrKnownSkillsHook = func() map[string]string { return map[string]string{"alpha": "a", "beta": "b"} }
	calls := 0
	skillmgrInstallHook = func(context.Context, string) (*skillmgr.SkillStatus, error) {
		calls++
		return &skillmgr.SkillStatus{Name: "x", Runnable: true, Detail: "ok"}, nil
	}
	_, err := runSkillCmd(t, "install", "all")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("expected 2 install calls, got %d", calls)
	}
}

func TestSkillCmd_Install_SingleError(t *testing.T) {
	saveSkillHooks(t)
	skillmgrInstallHook = func(context.Context, string) (*skillmgr.SkillStatus, error) {
		return nil, errors.New("install boom")
	}
	cmd := NewSkillCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"install", "alpha"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "1 skill(s) failed") {
		t.Fatalf("expected aggregate error, got %v", err)
	}
	if !strings.Contains(out.String(), "FAIL alpha") {
		t.Errorf("expected FAIL alpha, got output %q", out.String())
	}
}

func TestSkillCmd_Install_PartialError(t *testing.T) {
	saveSkillHooks(t)
	skillmgrInstallHook = func(_ context.Context, n string) (*skillmgr.SkillStatus, error) {
		if n == "alpha" {
			return &skillmgr.SkillStatus{Name: n, Runnable: true, Detail: "ok"}, nil
		}
		return nil, errors.New("install boom")
	}
	cmd := NewSkillCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"install", "alpha", "beta"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "1 skill(s) failed") {
		t.Fatalf("expected aggregate error, got %v", err)
	}
	if !strings.Contains(out.String(), "OK   alpha") {
		t.Errorf("expected OK alpha, got %q", out.String())
	}
	if !strings.Contains(out.String(), "FAIL beta") {
		t.Errorf("expected FAIL beta, got %q", out.String())
	}
}
