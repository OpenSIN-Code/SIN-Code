// SPDX-License-Identifier: MIT
// Purpose: integration tests for the sin-analyse-suite entry in
// mcpclient.DefaultServers(). Verifies the v3.22.0 multimodal preprocessing
// MCP is wired into the registry, short-name-mapped correctly, and reachable
// as a goNative-style stdio bridge (read-only analyse__* tools, allow policy
// per cmd/sin-code/internal/permission_defaults.go:66).
package mcpclient

import "testing"

// findAnalyse returns the ServerConfig for the sin-analyse-suite entry,
// or nil if missing. Inspection helper shared by every positive test below.
func findAnalyse(t *testing.T) *ServerConfig {
	t.Helper()
	for i := range DefaultServers() {
		s := DefaultServers()[i]
		if s.Name == "analyse" {
			return &s
		}
	}
	t.Fatal("sin-analyse-suite server not found in DefaultServers()")
	return nil
}

func TestSinAnalyseRegistered(t *testing.T) {
	s := findAnalyse(t)
	if s.Transport != "stdio" {
		t.Fatalf("sin-analyse-suite transport should be %q, got %q", "stdio", s.Transport)
	}
	if len(s.Args) != 1 || s.Args[0] != "serve" {
		t.Fatalf("sin-analyse-suite args should be [serve], got %v", s.Args)
	}
}

func TestSinAnalyseShortName(t *testing.T) {
	if got := shortName("sin-analyse-suite"); got != "analyse" {
		t.Fatalf("shortName(\"sin-analyse-suite\") should be %q, got %q", "analyse", got)
	}
}

func TestSinAnalyseUniqueShortName(t *testing.T) {
	count := 0
	for _, s := range DefaultServers() {
		if s.Name == "analyse" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("exactly one server should map to short name %q, found %d", "analyse", count)
	}
}

func TestSinAnalyseHasNonEmptyCommand(t *testing.T) {
	// Override testSkillsDir so skillsDirOrDefault returns "" and goNative
	// falls back to the bare binary name "sin-analyse". The Command field
	// must be non-empty in that path so the bridge can actually invoke it.
	orig := testSkillsDir
	testSkillsDir = stringPtr("")
	t.Cleanup(func() { testSkillsDir = orig })
	t.Setenv("SIN_SKILLS_DIR", "")

	s := findAnalyse(t)
	if s.Command == "" {
		t.Fatalf("sin-analyse-suite command must not be empty so the bridge can spawn it, got %q", s.Command)
	}
}

func TestSinAnalyseIsGoNative(t *testing.T) {
	// goNative-style entry: Name + Transport + Command all set, URL empty
	// (the bridge is stdio, not HTTP/websocket).
	orig := testSkillsDir
	testSkillsDir = stringPtr("")
	t.Cleanup(func() { testSkillsDir = orig })
	t.Setenv("SIN_SKILLS_DIR", "")

	s := findAnalyse(t)
	if s.Name == "" {
		t.Fatalf("sin-analyse-suite Name must be set, got empty")
	}
	if s.Transport != "stdio" {
		t.Fatalf("sin-analyse-suite Transport should be %q, got %q", "stdio", s.Transport)
	}
	if s.Command == "" {
		t.Fatalf("sin-analyse-suite Command must be set, got empty")
	}
	if s.URL != "" {
		t.Fatalf("sin-analyse-suite URL must be empty (stdio bridge), got %q", s.URL)
	}
	if len(s.Args) != 1 || s.Args[0] != "serve" {
		t.Fatalf("sin-analyse-suite Args should be [serve], got %v", s.Args)
	}
}

func stringPtr(s string) *string { return &s }
