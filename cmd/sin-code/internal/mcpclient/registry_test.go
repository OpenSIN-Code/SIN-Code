// SPDX-License-Identifier: MIT
// Purpose: tests for the built-in ecosystem registry in mcpclient.
package mcpclient

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultServersWebsearchUsesLocalBinaryWhenPresent(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "web_search_bundle", "sin-websearch")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SIN_SKILLS_DIR", dir)
	for _, s := range DefaultServers() {
		if s.Name != "websearch" {
			continue
		}
		if s.Command != bin {
			t.Fatalf("websearch command should use local binary %q, got %q", bin, s.Command)
		}
		if len(s.Args) != 1 || s.Args[0] != "serve" {
			t.Fatalf("websearch args should be [serve], got %v", s.Args)
		}
		return
	}
	t.Fatal("websearch server not found in DefaultServers")
}

func TestDefaultServersWebsearchFallsBackToPathBinary(t *testing.T) {
	// Use a HOME that has no sin-code skills checkout so the default skills
	// dir check fails and the registry falls back to the binary on PATH.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIN_SKILLS_DIR", "")
	for _, s := range DefaultServers() {
		if s.Name != "websearch" {
			continue
		}
		if s.Command != "sin-websearch" {
			t.Fatalf("websearch command should fall back to %q, got %q", "sin-websearch", s.Command)
		}
		return
	}
	t.Fatal("websearch server not found in DefaultServers")
}

func TestDefaultServersWebsearchUsesDefaultSkillsDir(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "share", "sin-code", "skills", "web_search_bundle", "sin-websearch")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SIN_SKILLS_DIR", "")
	t.Setenv("HOME", home)
	for _, s := range DefaultServers() {
		if s.Name != "websearch" {
			continue
		}
		if s.Command != bin {
			t.Fatalf("websearch command should use default skills dir binary %q, got %q", bin, s.Command)
		}
		return
	}
	t.Fatal("websearch server not found in DefaultServers")
}

func TestDefaultServersFallbacksWhenNoSkillsDir(t *testing.T) {
	// Force UserHomeDir to fail so skillsDirOrDefault returns empty and both
	// py and goNative fall back to the binary-on-PATH command.
	orig := userHomeDirHook
	userHomeDirHook = func() (string, error) { return "", errors.New("no home") }
	t.Cleanup(func() { userHomeDirHook = orig })
	t.Setenv("SIN_SKILLS_DIR", "")

	for _, s := range DefaultServers() {
		switch s.Name {
		case "websearch":
			if s.Command != "sin-websearch" {
				t.Fatalf("websearch fallback command mismatch: %q", s.Command)
			}
		case "scheduler":
			if s.Command != "sin-scheduler" {
				t.Fatalf("scheduler fallback command mismatch: %q", s.Command)
			}
		}
	}
}

func TestDefaultServersPythonSkillWithSkillsDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)

	for _, s := range DefaultServers() {
		if s.Name != "scheduler" {
			continue
		}
		want := filepath.Join(dir, "SIN-Code-Scheduler-Skill", "mcp_server.py")
		if s.Command != "python3" || len(s.Args) != 1 || s.Args[0] != want {
			t.Fatalf("scheduler should use python3 + skills dir, got %+v", s)
		}
		return
	}
	t.Fatal("scheduler server not found in DefaultServers")
}

func TestShortNameDefaultReturnsRepo(t *testing.T) {
	if got := shortName("Unknown-Repo-Name"); got != "Unknown-Repo-Name" {
		t.Fatalf("expected repo name unchanged, got %q", got)
	}
}
