// SPDX-License-Identifier: MIT
// Purpose: coverage tests for skills_cmd.go — exercises list/install with fake
// skillsmith.Smith instances so no real install happens.
// Docs: cmd/sin-code/skills_cmd.go.doc.md
package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/Songmu/skillsmith"
	"github.com/Songmu/skillsmith/agentskills"

	"github.com/OpenSIN-Code/SIN-Code/skills"
)

func TestResolveSkillsVersion(t *testing.T) {
	orig := skillsVersionHook
	skillsVersionHook = func() string { return "v1.2.3" }
	defer func() { skillsVersionHook = orig }()
	if got := resolveSkillsVersion(); got != "v1.2.3" {
		t.Errorf("expected v1.2.3, got %q", got)
	}
}

func TestResolveSkillsVersion_DevFallback(t *testing.T) {
	orig := skillsVersionHook
	skillsVersionHook = func() string { return "dev" }
	defer func() { skillsVersionHook = orig }()
	if got := resolveSkillsVersion(); got != "v0.0.0-dev" {
		t.Errorf("expected v0.0.0-dev, got %q", got)
	}
}

func TestResolveSkillsVersion_SemverPassThrough(t *testing.T) {
	orig := skillsVersionHook
	skillsVersionHook = func() string { return "v3.14.0" }
	defer func() { skillsVersionHook = orig }()
	if got := resolveSkillsVersion(); got != "v3.14.0" {
		t.Errorf("expected v3.14.0, got %q", got)
	}
}

func runSkillsCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewSkillsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestSkillsCmd_NewSkillsCmd(t *testing.T) {
	cmd := NewSkillsCmd()
	if cmd.Use != "skills" {
		t.Errorf("Use = %q, want skills", cmd.Use)
	}
	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"list", "install"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing subcommand %q in %q", want, joined)
		}
	}
}

func TestSkillsDefaultHooks(t *testing.T) {
	// Verify the default hooks are wired and the embedded FS can be reached.
	listFS, err := skills.ListFS()
	if err != nil {
		t.Fatal(err)
	}
	smith, err := skillsNewSmithHook("sin-code", resolveSkillsVersion(), listFS)
	if err != nil {
		t.Fatal(err)
	}
	list, err := smith.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Error("expected at least one bundled skill")
	}
}

func TestSkillsCmd_List_Json(t *testing.T) {
	orig := skillsNewSmithHook
	skillsNewSmithHook = func(name, version string, fs fs.FS) (*skillsmith.Smith, error) {
		return skillsmith.New(name, version, fs)
	}
	defer func() { skillsNewSmithHook = orig }()
	out, err := runSkillsCmd(t, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[") {
		t.Errorf("expected JSON array, got %q", out.String())
	}
}

func TestSkillsCmd_List_Error(t *testing.T) {
	orig := skillsNewSmithHook
	skillsNewSmithHook = func(name, version string, fs fs.FS) (*skillsmith.Smith, error) {
		return nil, errors.New("new smith boom")
	}
	defer func() { skillsNewSmithHook = orig }()
	_, err := runSkillsCmd(t, "list")
	if err == nil || !strings.Contains(err.Error(), "new smith boom") {
		t.Fatalf("expected smith error, got %v", err)
	}
}

func TestSkillsCmd_Install_Error(t *testing.T) {
	orig := skillsNewSmithHook
	skillsNewSmithHook = func(name, version string, fs fs.FS) (*skillsmith.Smith, error) {
		return nil, errors.New("new smith boom")
	}
	defer func() { skillsNewSmithHook = orig }()
	_, err := runSkillsCmd(t, "install", "foo")
	if err == nil || !strings.Contains(err.Error(), "new smith boom") {
		t.Fatalf("expected smith error, got %v", err)
	}
}

func TestSkillsCmd_Install(t *testing.T) {
	orig := skillsNewSmithHook
	origDir := skillsInstallDirHook
	skillsNewSmithHook = func(name, version string, fs fs.FS) (*skillsmith.Smith, error) {
		return skillsmith.New(name, version, fs)
	}
	skillsInstallDirHook = func(scope string) (string, error) {
		return "/tmp/skills-test", nil
	}
	defer func() {
		skillsNewSmithHook = orig
		skillsInstallDirHook = origDir
	}()
	out, err := runSkillsCmd(t, "install", "skill-code-create", "--dry-run")
	if err != nil {
		t.Fatalf("install error: %v\noutput: %s", err, out.String())
	}
	if !strings.Contains(out.String(), "installed") {
		t.Errorf("expected install output, got %q", out.String())
	}
}

// _unusedAgentskillsImport prevents goimports from removing the agentskills import.
var _unusedAgentskillsImport = agentskills.Discover
