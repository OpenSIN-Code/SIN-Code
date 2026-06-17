// SPDX-License-Identifier: MIT
// Purpose: tests for issue #326 — Status snapshot / readiness report.
// All external operations are injected via SnapshotHooks so tests run
// hermetically without git, go build, or real databases.
package internal

import (
	"fmt"
	"strings"
	"testing"
)

// helperSnapshot returns a Snapshot with all hooks stubbed to deterministic
// values, so every test starts from a known state.
func helperSnapshot() *Snapshot {
	return NewSnapshotWithHooks(SnapshotHooks{
		Workdir: ".",
		Git: func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref":
				return "main", nil
			case len(args) >= 1 && args[0] == "status":
				return "", nil // clean tree
			case len(args) >= 2 && args[0] == "rev-list" && args[1] == "--count":
				if len(args) >= 3 && args[2] == "@{u}..HEAD" {
					return "0", nil
				}
				if len(args) >= 3 && args[2] == "HEAD..@{u}" {
					return "0", nil
				}
			}
			return "", nil
		},
		Exec: func(name string, args ...string) (string, error) {
			// Simulate clean build + vet + all tests pass.
			if name == "go" && len(args) > 0 {
				switch args[0] {
				case "build":
					return "", nil
				case "vet":
					return "", nil
				case "test":
					return "ok\tpkg/a\t0.1s\nok\tpkg/b\t0.2s\n", nil
				}
			}
			return "", nil
		},
		SessionCount: func() (int, error) { return 3, nil },
		TodoCounts: func() (int, int, int, error) { return 5, 1, 2, nil },
		MCPStatus: func() (map[string]bool, []string, error) {
			return map[string]bool{
					"websearch":  true,
					"scheduler":  true,
					"browser":    false,
				},
				[]string{"websearch", "scheduler", "browser"},
				nil
		},
		SkillsCount:  func() (int, error) { return 37, nil },
		ConfigValues: func() (string, string, string) {
			return "claude-mythos-5", "anthropic", "poc"
		},
	})
}

// ── Test 1: Collect populates all fields ────────────────────────────────────

func TestSnapshot_Collect_PopulatesAllFields(t *testing.T) {
	s := helperSnapshot()
	if err := s.Collect(); err != nil {
		t.Fatalf("Collect() error: %v", err)
	}
	d := s.data

	if d.GoVersion == "" {
		t.Error("GoVersion should be non-empty")
	}
	if !d.BuildPass {
		t.Error("BuildPass should be true")
	}
	if !d.VetPass {
		t.Error("VetPass should be true")
	}
	if d.TestsPass != 2 {
		t.Errorf("TestsPass = %d, want 2", d.TestsPass)
	}
	if d.TestsFail != 0 {
		t.Errorf("TestsFail = %d, want 0", d.TestsFail)
	}
	if d.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want %q", d.GitBranch, "main")
	}
	if !d.GitClean {
		t.Error("GitClean should be true")
	}
	if d.GitAhead != 0 || d.GitBehind != 0 {
		t.Errorf("GitAhead=%d GitBehind=%d, want 0/0", d.GitAhead, d.GitBehind)
	}
	if d.ConfigModel != "claude-mythos-5" {
		t.Errorf("ConfigModel = %q, want %q", d.ConfigModel, "claude-mythos-5")
	}
	if d.ConfigProvider != "anthropic" {
		t.Errorf("ConfigProvider = %q, want %q", d.ConfigProvider, "anthropic")
	}
	if d.ConfigVerifyMode != "poc" {
		t.Errorf("ConfigVerifyMode = %q, want %q", d.ConfigVerifyMode, "poc")
	}
	if len(d.MCPServers) != 3 {
		t.Errorf("MCPServers len = %d, want 3", len(d.MCPServers))
	}
	if d.Skills != 37 {
		t.Errorf("Skills = %d, want 37", d.Skills)
	}
	if d.Sessions != 3 {
		t.Errorf("Sessions = %d, want 3", d.Sessions)
	}
	if d.TodosOpen != 5 || d.TodosBlocked != 1 || d.TodosReady != 2 {
		t.Errorf("Todos open/blocked/ready = %d/%d/%d, want 5/1/2", d.TodosOpen, d.TodosBlocked, d.TodosReady)
	}
}

// ── Test 2: RenderMarkdown produces expected sections ───────────────────────

func TestSnapshot_RenderMarkdown_ContainsAllSections(t *testing.T) {
	s := helperSnapshot()
	_ = s.Collect()
	md := s.RenderMarkdown()

	required := []string{
		"# SIN-Code Readiness Report",
		"## Build",
		"## Git",
		"## Configuration",
		"## MCP Servers",
		"## Skills",
		"## Sessions",
		"## Todos",
		"## Verdict",
	}
	for _, section := range required {
		if !strings.Contains(md, section) {
			t.Errorf("RenderMarkdown() missing section %q", section)
		}
	}
}

// ── Test 3: RenderMarkdown shows correct verdict for clean system ──────────

func TestSnapshot_RenderMarkdown_VerdictReady(t *testing.T) {
	s := helperSnapshot()
	_ = s.Collect()
	md := s.RenderMarkdown()
	if !strings.Contains(md, "READY FOR PRODUCTION") {
		t.Errorf("clean system should be READY FOR PRODUCTION, got:\n%s", md)
	}
}

// ── Test 4: RenderMarkdown shows attention needed when build fails ─────────

func TestSnapshot_RenderMarkdown_VerdictAttentionOnBuildFail(t *testing.T) {
	s := helperSnapshot()
	s.exec = func(name string, args ...string) (string, error) {
		if name == "go" && len(args) > 0 && args[0] == "build" {
			return "", fmt.Errorf("build failed")
		}
		return "", nil
	}
	_ = s.Collect()
	if !s.NeedsAttention() {
		t.Error("NeedsAttention should be true when build fails")
	}
	md := s.RenderMarkdown()
	if !strings.Contains(md, "ATTENTION NEEDED") && !strings.Contains(md, "NOT READY") {
		t.Errorf("failing build should not be READY, got verdict: %s", s.verdict())
	}
}

// ── Test 5: RenderMarkdown shows NOT READY when multiple failures ──────────

func TestSnapshot_RenderMarkdown_VerdictNotReady(t *testing.T) {
	s := helperSnapshot()
	s.exec = func(name string, args ...string) (string, error) {
		if name == "go" && len(args) > 0 {
			switch args[0] {
			case "build":
				return "", fmt.Errorf("build failed")
			case "vet":
				return "", fmt.Errorf("vet failed")
			case "test":
				return "FAIL\tpkg/a\t0.1s\nFAIL\tpkg/b\t0.2s\n", nil
			}
		}
		return "", nil
	}
	s.git = func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "status" {
			return "M file.go", nil // dirty
		}
		if len(args) > 0 && args[0] == "rev-parse" {
			return "main", nil
		}
		return "0", nil
	}
	_ = s.Collect()
	md := s.RenderMarkdown()
	if !strings.Contains(md, "NOT READY") {
		t.Errorf("3+ issues should be NOT READY, got:\n%s", md)
	}
}

// ── Test 6: MCP servers render in order with correct status ────────────────

func TestSnapshot_RenderMarkdown_MCPOrderAndStatus(t *testing.T) {
	s := helperSnapshot()
	_ = s.Collect()
	md := s.RenderMarkdown()

	// websearch and scheduler should show ✅, browser should show ❌
	if !strings.Contains(md, "websearch: ✅") {
		t.Error("websearch should show ✅")
	}
	if !strings.Contains(md, "scheduler: ✅") {
		t.Error("scheduler should show ✅")
	}
	if !strings.Contains(md, "browser: ❌") {
		t.Error("browser should show ❌")
	}

	// Verify order: websearch before scheduler before browser
	wsIdx := strings.Index(md, "websearch")
	scIdx := strings.Index(md, "scheduler")
	brIdx := strings.Index(md, "browser")
	if !(wsIdx < scIdx && scIdx < brIdx) {
		t.Errorf("MCP servers not in expected order: ws=%d sc=%d br=%d", wsIdx, scIdx, brIdx)
	}
}

// ── Test 7: Collect is resilient — one hook failing doesn't abort ──────────

func TestSnapshot_Collect_ResilientToHookFailures(t *testing.T) {
	s := helperSnapshot()
	// Make git fail entirely — Collect should still populate other fields.
	s.git = func(args ...string) (string, error) {
		return "", fmt.Errorf("git not found")
	}
	// Make todoCounts fail.
	s.todoCounts = func() (int, int, int, error) {
		return 0, 0, 0, fmt.Errorf("todo db locked")
	}
	// Make MCP fail.
	s.mcpStatus = func() (map[string]bool, []string, error) {
		return nil, nil, fmt.Errorf("mcp unavailable")
	}

	if err := s.Collect(); err != nil {
		t.Fatalf("Collect() should not return error even when hooks fail: %v", err)
	}
	d := s.data

	// Build/vet/tests should still be populated.
	if !d.BuildPass {
		t.Error("BuildPass should still be true even when git fails")
	}
	// Git fields should be zero-values (collection failed silently).
	if d.GitBranch != "" {
		t.Errorf("GitBranch should be empty when git fails, got %q", d.GitBranch)
	}
	// Todo fields should be zero.
	if d.TodosOpen != 0 {
		t.Errorf("TodosOpen should be 0 when todo hook fails, got %d", d.TodosOpen)
	}
	// MCP should be nil/empty.
	if len(d.MCPServers) != 0 {
		t.Errorf("MCPServers should be empty when mcp hook fails, got %d", len(d.MCPServers))
	}
}

// ── Test 8: NeedsAttention + verdict logic ──────────────────────────────────

func TestSnapshot_NeedsAttention_Logic(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(s *Snapshot)
		want     bool
		verdict  string
	}{
		{
			name:    "all clean",
			mutate:  func(s *Snapshot) {},
			want:    false,
			verdict: "READY FOR PRODUCTION ✅",
		},
		{
			name: "build fail only",
			mutate: func(s *Snapshot) {
				s.data.BuildPass = false
			},
			want:    true,
			verdict: "ATTENTION NEEDED ⚠️",
		},
		{
			name: "dirty git only",
			mutate: func(s *Snapshot) {
				s.data.GitClean = false
			},
			want:    true,
			verdict: "ATTENTION NEEDED ⚠️",
		},
		{
			name: "tests fail only",
			mutate: func(s *Snapshot) {
				s.data.TestsFail = 3
			},
			want:    true,
			verdict: "ATTENTION NEEDED ⚠️",
		},
		{
			name: "all fail",
			mutate: func(s *Snapshot) {
				s.data.BuildPass = false
				s.data.VetPass = false
				s.data.TestsFail = 5
				s.data.GitClean = false
			},
			want:    true,
			verdict: "NOT READY ❌",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := helperSnapshot()
			_ = s.Collect()
			c.mutate(s)
			got := s.NeedsAttention()
			if got != c.want {
				t.Errorf("NeedsAttention() = %v, want %v", got, c.want)
			}
			v := s.verdict()
			if v != c.verdict {
				t.Errorf("verdict() = %q, want %q", v, c.verdict)
			}
		})
	}
}

// ── Test 9 (bonus): RenderMarkdown calls Collect if not yet called ─────────

func TestSnapshot_RenderMarkdown_AutoCollects(t *testing.T) {
	s := helperSnapshot()
	// Don't call Collect — RenderMarkdown should call it.
	md := s.RenderMarkdown()
	if !strings.Contains(md, "# SIN-Code Readiness Report") {
		t.Error("RenderMarkdown should auto-collect and produce the report")
	}
	if !s.collected {
		t.Error("RenderMarkdown should set collected=true after auto-collect")
	}
}

// ── Test 10 (bonus): NewSnapshot defaults produce no panics ────────────────

func TestSnapshot_NewSnapshot_DefaultsNoPanic(t *testing.T) {
	s := NewSnapshot()
	// Override the heavy hooks so we don't actually run go build/test.
	s.exec = func(name string, args ...string) (string, error) { return "", nil }
	s.git = func(args ...string) (string, error) { return "", nil }
	if err := s.Collect(); err != nil {
		t.Fatalf("Collect with defaults should not error: %v", err)
	}
	md := s.RenderMarkdown()
	if !strings.Contains(md, "## Verdict") {
		t.Error("default snapshot should still render a verdict")
	}
}
