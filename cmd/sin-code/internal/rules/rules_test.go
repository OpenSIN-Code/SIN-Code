// SPDX-License-Identifier: MIT
// Purpose: race-clean tests for the path-scoped rule loader.
package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRule drops a rule .md file into workspace/.sin-code/rules/.
func writeRule(t *testing.T, workspace, name, body string) {
	t.Helper()
	dir := filepath.Join(workspace, ".sin-code", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStoreLoad(t *testing.T) {
	root := t.TempDir()
	writeRule(t, root, "loop-style", `---
name: loop-style
description: coding style for the SIN-Code agent loop
paths:
  - "cmd/sin-code/internal/agentloop/**"
  - "cmd/sin-code/internal/loopbuilder/**"
---

# Loop Style
- Always run race tests after non-trivial edits.
`)
	s := New(root)
	n, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 rule, got %d", n)
	}
	r, ok := s.Get("loop-style")
	if !ok {
		t.Fatal("loop-style missing")
	}
	if !strings.Contains(string(r.Body), "Always run race tests") {
		t.Fatalf("body wrong: %q", r.Body)
	}
	if len(r.Globs) != 2 {
		t.Fatalf("want 2 globs, got %d", len(r.Globs))
	}
}

func TestForPath(t *testing.T) {
	root := t.TempDir()
	writeRule(t, root, "a-path", `---
name: a-path
description: only agentloop matches
paths:
  - "cmd/sin-code/internal/agentloop/**"
---
A body.
`)
	writeRule(t, root, "b-path", `---
name: b-path
description: only loopbuilder matches
paths:
  - "cmd/sin-code/internal/loopbuilder/**"
---
B body.
`)
	writeRule(t, root, "always-on", `---
name: always-on
description: matches every path
always_on: true
---
Always body.
`)
	s := New(root)
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	got := s.ForPath("/Users/x/src/cmd/sin-code/internal/agentloop/loop.go")
	if len(got) != 2 {
		t.Fatalf("agentloop should match both a-path and always-on; got %v", namesOf(got))
	}
	got = s.ForPath("/Users/x/src/cmd/sin-code/internal/orchestrator/dispatcher.go")
	if len(got) != 1 || got[0].Name != "always-on" {
		t.Fatalf("dispatcher should match only always-on; got %v", namesOf(got))
	}
}

func TestGlobMatching(t *testing.T) {
	cases := []struct {
		glob  string
		path  string
		want  bool
	}{
		{`cmd/sin-code/internal/agentloop/**`, `/Users/x/cmd/sin-code/internal/agentloop/loop.go`, true},
		{`cmd/sin-code/internal/agentloop/**`, `/Users/x/cmd/sin-code/internal/orchestrator/x.go`, false},
		{`cmd/sin-code/internal/agentloop/*`, `/Users/x/cmd/sin-code/internal/agentloop/loop.go`, true},
		{`cmd/sin-code/internal/agentloop/*`, `/Users/x/cmd/sin-code/internal/agentloop/sub/x.go`, false},
		{`**`, `/Users/x/src/anything.go`, true},
		{`*.go`, `/Users/x/foo.go`, true},
		{`*.go`, `/Users/x/foo.txt`, false},
	}
	for _, c := range cases {
		if got := match(c.glob, c.path); got != c.want {
			t.Errorf("match(%q, %q) = %v, want %v", c.glob, c.path, got, c.want)
		}
	}
}

func TestFrontmatterParse(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{
			name: "valid_singleline_paths",
			raw: `---
name: r1
paths: ["a/**", "b/**"]
---
body`,
		},
		{
			name: "valid_multiline_paths",
			raw: `---
name: r1
paths:
  - "a/**"
  - "b/**"
---
body`,
		},
		{
			name:    "missing_open_fence",
			raw:     `name: x\ndescription: y\n---\nbody`,
			wantErr: true,
		},
		{
			name:    "missing_close_fence",
			raw:     "---\nname: x\n\nbody without close",
			wantErr: true,
		},
		{
			name: "always_on",
			raw: `---
name: r2
always_on: true
---
all paths`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := splitFrontmatter([]byte(c.raw))
			if c.wantErr {
				if err == nil {
					t.Errorf("want error, got nil")
				}
				// splitFrontmatter doesn't have a file path, so
				// it returns a plain error (not ErrInvalidFrontmatter);
				// parseFile() rewraps. The contract is just
				// "non-nil error".
				return
			}
			if err != nil {
				t.Errorf("unexpected: %v", err)
			}
		})
	}
}

func TestDuplicateRuleRejected(t *testing.T) {
	root := t.TempDir()
	writeRule(t, root, "dup", "---\nname: dup\n---\nA\n")
	writeRule(t, root, "dup-other", "---\nname: dup\n---\nB\n")
	s := New(root)
	if _, err := s.Load(); err == nil {
		t.Fatal("want duplicate error")
	} else if _, ok := err.(ErrDuplicateRule); !ok {
		t.Fatalf("want ErrDuplicateRule, got %T", err)
	}
}

func TestMissingDirCreatesEmpty(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	n, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("want 0 rules on fresh dir, got %d", n)
	}
	if names := s.Names(); len(names) != 0 {
		t.Fatalf("Names must be empty: %v", names)
	}
}

func TestBodyIsBytesStableAfterReload(t *testing.T) {
	root := t.TempDir()
	writeRule(t, root, "stable", "---\nname: stable\ndescription: deterministic\n---\nStatic body.\n")
	s := New(root)
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	r1, _ := s.Get("stable")
	if _, err := s.Load(); err != nil { // idempotent
		t.Fatal(err)
	}
	r2, _ := s.Get("stable")
	if string(r1.Body) != string(r2.Body) {
		t.Fatalf("reload must be idempotent: %q vs %q", r1.Body, r2.Body)
	}
}

func namesOf(rs []Rule) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}
