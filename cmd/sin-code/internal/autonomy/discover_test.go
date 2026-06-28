// SPDX-License-Identifier: MIT
// Purpose: tests for autonomous backlog discovery — TODO/FIXME scanning,
// MASTER_TODO parsing, dedup keys, and enqueue integration (AGENTS.md §8).
package autonomy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTodoMarkerParsing(t *testing.T) {
	cases := map[string]struct{ marker, note string }{
		"// TODO: fix the thing":  {"TODO", "fix the thing"},
		"//FIXME broken":          {"FIXME", "broken"},
		"# TODO: python style":    {"TODO", "python style"},
		"  // XXX danger":         {"XXX", "danger"},
		"/* HACK temporary */":    {"HACK", "temporary"},
		"no marker here":          {"", ""},
		`s := "TODO in a string"`: {"", ""}, // not in a comment prefix
	}
	for line, want := range cases {
		m, note := todoMarker(line)
		if m != want.marker {
			t.Errorf("line %q: marker want %q got %q", line, want.marker, m)
		}
		if m != "" && note != want.note {
			t.Errorf("line %q: note want %q got %q", line, want.note, note)
		}
	}
}

func TestDiscoverComments(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "main.go", "package main\n// TODO: implement foo\nfunc main(){}\n")
	writeFile(t, ws, "util.py", "# FIXME: handle edge case\n")
	writeFile(t, ws, "README.md", "TODO this should be ignored (not a code file)\n")

	findings, err := Discover(DiscoverConfig{Workspace: ws, ScanComments: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (go + py), got %d: %+v", len(findings), findings)
	}
	for _, f := range findings {
		if f.DedupKey == "" {
			t.Fatal("finding missing dedup key")
		}
		if f.Prompt == "" {
			t.Fatal("finding missing prompt")
		}
	}
}

func TestDiscoverSkipsVendorAndGit(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "node_modules/dep/index.js", "// TODO: ignore me\n")
	writeFile(t, ws, ".git/hooks/x.sh", "# TODO: ignore me too\n")
	writeFile(t, ws, "real.go", "// TODO: keep me\n")

	findings, err := Discover(DiscoverConfig{Workspace: ws, ScanComments: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected only the real finding, got %d: %+v", len(findings), findings)
	}
}

func TestDiscoverMasterTodo(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "MASTER_TODO.md", `# Roadmap
- [ ] Build the widget
- [x] Already done
* [ ] Another task
Some prose.
`)
	findings, err := Discover(DiscoverConfig{Workspace: ws, ScanMaster: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 unchecked items, got %d: %+v", len(findings), findings)
	}
	for _, f := range findings {
		if f.Source != "master_todo" {
			t.Fatalf("unexpected source %q", f.Source)
		}
	}
}

func TestDiscoverDedupWithinScan(t *testing.T) {
	ws := t.TempDir()
	// Same marker text on two lines collapses to one finding via dedup key.
	writeFile(t, ws, "a.go", "// TODO: same note\n")
	writeFile(t, ws, "b.go", "// TODO: same note\n")
	findings, err := Discover(DiscoverConfig{Workspace: ws, ScanComments: true})
	if err != nil {
		t.Fatal(err)
	}
	// Different files → different dedup keys (path is part of the key), so 2.
	if len(findings) != 2 {
		t.Fatalf("expected 2 (path-scoped keys), got %d", len(findings))
	}
}

func TestDiscoverMaxFindings(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "a.go", "// TODO: one\n// FIXME: two\n// XXX: three\n")
	findings, err := Discover(DiscoverConfig{Workspace: ws, ScanComments: true, MaxFindings: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 2 {
		t.Fatalf("MaxFindings not respected, got %d", len(findings))
	}
}

func TestEnqueueFindingsDedups(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	findings := []Finding{
		{Source: "todo", Prompt: "do a", DedupKey: "k1"},
		{Source: "todo", Prompt: "do b", DedupKey: "k2"},
		{Source: "todo", Prompt: "do a again", DedupKey: "k1"}, // dup
	}
	n, err := EnqueueFindings(ctx, q, "/tmp", findings, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 enqueued (k1 deduped), got %d", n)
	}
	pending, _ := q.List(ctx, StatusPending)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending goals, got %d", len(pending))
	}
}

func TestEnqueueFindingsIdempotentAcrossScans(t *testing.T) {
	q := openTestQueue(t)
	ctx := context.Background()
	findings := []Finding{{Source: "todo", Prompt: "x", DedupKey: "stable"}}
	n1, _ := EnqueueFindings(ctx, q, "/tmp", findings, 3)
	n2, _ := EnqueueFindings(ctx, q, "/tmp", findings, 3)
	if n1 != 1 || n2 != 0 {
		t.Fatalf("re-scan should not duplicate: n1=%d n2=%d", n1, n2)
	}
}

func TestPriorityFor(t *testing.T) {
	cases := []struct {
		marker string
		want   int
	}{
		{"FIXME", 2},
		{"XXX", 2},
		{"HACK", 1},
		{"TODO", 0},
		{"unknown", 0},
		{"", 0},
	}
	for _, c := range cases {
		got := priorityFor(c.marker)
		if got != c.want {
			t.Errorf("priorityFor(%q) = %d, want %d", c.marker, got, c.want)
		}
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  Build   the widget  ", "build the widget"},
		{"UPPER CASE", "upper case"},
		{"single", "single"},
		{"", ""},
		{"\t\n tabs\n\t", "tabs"},
	}
	for _, c := range cases {
		got := normalizeKey(c.in)
		if got != c.want {
			t.Errorf("normalizeKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDiscoverMaxFindingsDefault(t *testing.T) {
	ws := t.TempDir()
	// With MaxFindings=0, default of 50 should apply.
	writeFile(t, ws, "a.go", "// TODO: one\n")
	findings, err := Discover(DiscoverConfig{Workspace: ws, ScanComments: true, MaxFindings: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding with default MaxFindings, got %d", len(findings))
	}
}

func TestDiscoverBothSources(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "main.go", "// TODO: implement\n")
	writeFile(t, ws, "MASTER_TODO.md", "- [ ] Build feature\n")
	findings, err := Discover(DiscoverConfig{Workspace: ws, ScanComments: true, ScanMaster: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (1 comment + 1 master), got %d", len(findings))
	}
}

func TestDiscoverNoSources(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "main.go", "// TODO: implement\n")
	findings, err := Discover(DiscoverConfig{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings with no scan flags, got %d", len(findings))
	}
}

func TestScanMasterTodoNoFile(t *testing.T) {
	ws := t.TempDir()
	// No MASTER_TODO.md file — should return nil (not an error).
	_, err := Discover(DiscoverConfig{Workspace: ws, ScanMaster: true})
	if err != nil {
		t.Fatalf("expected no error when file missing, got %v", err)
	}
}
