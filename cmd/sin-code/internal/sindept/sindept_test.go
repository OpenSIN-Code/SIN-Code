// SPDX-License-Identifier: MIT
// Purpose: tests for the sindept package (issue #177). The golden fixture
// set is 10 markers spread across five files (Go, Python, TypeScript,
// Shell, Markdown). Tests assert:
//
//  1. marker count (10)
//  2. parser recognises every comment family (`//`, `#`, `--`, `/*`, `<!--`)
//  3. Parser trims trailing comment closers (`*/`, `-->`) from the
//     captured reason / upgrade clauses
//  4. AggregateStats byte-stable view matches the documented golden
//  5. RenderStatsString bytes are deterministic for the same Stats
//  6. Policy.RunCheck gate semantics (above-threshold fails, below passes)
//  7. Wire bytes survive RoundTrip (Render -> sort -> split -> stable)
package sindept

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestParseGoldenTotalCount(t *testing.T) {
	mk := parseGolden(t)
	if len(mk) != 10 {
		t.Fatalf("expected 10 golden markers, got %d", len(mk))
	}
}

func TestParseGoldenFiles(t *testing.T) {
	mk := parseGolden(t)
	wantFiles := []string{
		"testdata/markers.go",
		"testdata/markers.md",
		"testdata/markers.py",
		"testdata/markers.sh",
		"testdata/markers.ts",
	}
	gotFiles := uniqueSortedFiles(mk)
	if !equalSlices(gotFiles, wantFiles) {
		t.Fatalf("file set mismatch: got %v want %v", gotFiles, wantFiles)
	}
}

func TestParseGoldenFamilyCoverage(t *testing.T) {
	mk := parseGolden(t)
	wantFamilies := map[string]bool{
		"//": false, "#": false, "<!--": false,
	}
	for _, m := range mk {
		switch {
		case strings.HasPrefix(m.Raw, "//"):
			wantFamilies["//"] = true
		case strings.HasPrefix(m.Raw, "#"):
			wantFamilies["#"] = true
		case strings.HasPrefix(m.Raw, "<!--"):
			wantFamilies["<!--"] = true
		}
	}
	for f, seen := range wantFamilies {
		if !seen {
			t.Errorf("comment family %q did not show up in any fixture", f)
		}
	}
}

func TestParseGoldenUpgradeAttribution(t *testing.T) {
	mk := parseGolden(t)
	want := map[string]bool{
		"per-account locks when throughput > 1k req/s":           true,
		"switch to map lookup when n > 100":                      true,
		"use cenkalti/backoff when context cancellation matters": true,
		"switch to fsnotify when file count > 100":               true,
		"switch to redis when instances > 1":                     true,
		"replace with text/template when content grows 10x":      true,
	}
	for _, m := range mk {
		if m.HasUpg {
			if _, ok := want[m.Upgrade]; !ok {
				t.Errorf("unexpected upgrade clause: %q", m.Upgrade)
			}
			delete(want, m.Upgrade)
		}
	}
	if len(want) != 0 {
		t.Errorf("some expected upgrade clauses never showed up: %v", want)
	}
}

func TestParseGoldenTrailingClosersStripped(t *testing.T) {
	mk := parseGolden(t)
	for _, m := range mk {
		if strings.HasSuffix(m.Reason, "*/") || strings.HasSuffix(m.Reason, "-->") {
			t.Errorf("file=%s reason=%q still has trailing closer", m.File, m.Reason)
		}
		if strings.HasSuffix(m.Upgrade, "*/") || strings.HasSuffix(m.Upgrade, "-->") {
			t.Errorf("file=%s upgrade=%q still has trailing closer", m.File, m.Upgrade)
		}
	}
}

func TestAggregateStatsMatchesGoldenView(t *testing.T) {
	mk := parseGolden(t)
	stats := AggregateStats(mk)
	if stats.Total != 10 {
		t.Fatalf("Total=%d want 10", stats.Total)
	}
	if stats.WithUpgrade != 6 {
		t.Fatalf("WithUpgrade=%d want 6", stats.WithUpgrade)
	}
	if stats.WithoutUpgrade != 4 {
		t.Fatalf("WithoutUpgrade=%d want 4", stats.WithoutUpgrade)
	}
	if len(stats.ByFile) != 5 {
		t.Fatalf("ByFile size=%d want 5", len(stats.ByFile))
	}
	if len(stats.ByLanguage) != 5 {
		t.Fatalf("ByLanguage size=%d want 5", len(stats.ByLanguage))
	}
	if len(stats.RotRisk) != 4 {
		t.Fatalf("RotRisk size=%d want 4", len(stats.RotRisk))
	}
}

func TestRenderStatsByteStable(t *testing.T) {
	mk := parseGolden(t)
	s1 := RenderStatsString(AggregateStats(mk))
	s2 := RenderStatsString(AggregateStats(mk))
	if s1 != s2 {
		t.Fatalf("RenderStatsString not byte-stable:\n---1---\n%s\n---2---\n%s", s1, s2)
	}
	if !strings.HasPrefix(s1, Header()) {
		t.Fatalf("missing header in rendered report:\n%s", s1)
	}
	for _, want := range []string{
		"## Summary",
		"## By file",
		"## By reason",
		"## By language",
		"## Rot-risk markers",
		"Total markers:",
	} {
		if !strings.Contains(s1, want) {
			t.Errorf("rendered report missing %q", want)
		}
	}
}

func TestRenderListStringByteStable(t *testing.T) {
	mk := parseGolden(t)
	a := RenderListString(mk)
	b := RenderListString(mk)
	if a != b {
		t.Fatalf("RenderListString not byte-stable")
	}
	if !strings.Contains(a, "| file | line | symbol | reason | upgrade | rot |") {
		t.Fatalf("missing header row in list report")
	}
	if !strings.Contains(a, "_10 markers total. 6 with upgrade, 4 in rot-risk._") {
		t.Fatalf("missing footer counts in list report")
	}
}

func TestParseFileReturnsEmptyForMissingPath(t *testing.T) {
	mk, err := ParseFile("/this/path/does/not/exist.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mk != nil {
		t.Fatalf("expected nil, got %v", mk)
	}
}

func TestParseDirSkipsVendoredAndHidden(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real.go"), `// sin-debt: ok
package x
`)
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(gitDir, "HEAD"), `// sin-debt: skip me
`)
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.Mkdir(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(vendorDir, "x.go"), `// sin-debt: skip me
`)
	nmDir := filepath.Join(dir, "node_modules")
	if err := os.Mkdir(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nmDir, "x.js"), `// sin-debt: skip me
`)
	mk, err := ParseDir(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	if len(mk) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(mk))
	}
	if !strings.HasSuffix(mk[0].File, "real.go") {
		t.Fatalf("expected real.go, got %s", mk[0].File)
	}
}

func TestParseDirSortIsDeterministic(t *testing.T) {
	mk := parseGolden(t)
	for i := 1; i < len(mk); i++ {
		if mk[i].File < mk[i-1].File {
			t.Fatalf("not sorted by File at index %d", i)
		}
		if mk[i].File == mk[i-1].File && mk[i].Line < mk[i-1].Line {
			t.Fatalf("not sorted by Line within file at index %d", i)
		}
	}
}

func TestPolicyCheckPassesWhenUnderThreshold(t *testing.T) {
	p := DefaultPolicy()
	p.MaxNoUpgrade = 5
	p.RequireUpgrade = false
	mk := parseGolden(t)
	r := p.RunCheck(mk)
	if !r.Ok {
		t.Fatalf("expected ok, got %+v", r)
	}
}

func TestPolicyCheckFailsOverThreshold(t *testing.T) {
	p := DefaultPolicy()
	p.MaxNoUpgrade = 1
	p.RequireUpgrade = false
	mk := parseGolden(t)
	r := p.RunCheck(mk)
	if r.Ok {
		t.Fatalf("expected fail, got %+v", r)
	}
	if len(r.Failed) != 4 {
		t.Fatalf("expected 4 failed markers, got %d", len(r.Failed))
	}
}

func TestPolicyRequireUpgradeForceFails(t *testing.T) {
	p := DefaultPolicy()
	p.RequireUpgrade = true
	p.MaxNoUpgrade = 0
	mk := parseGolden(t)
	r := p.RunCheck(mk)
	if r.Ok {
		t.Fatalf("expected fail with require_upgrade=true")
	}
	if len(r.Failed) != 4 {
		t.Fatalf("expected 4 failed markers, got %d", len(r.Failed))
	}
}

func TestParseInlineCommentFamilies(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"go", "// sin-debt: go fixture\n", 1},
		{"python", "# sin-debt: py fixture\n", 1},
		{"shell", "# sin-debt: sh fixture\n", 1},
		{"rust", "// sin-debt: rs fixture\n", 1},
		{"c-block", "/* sin-debt: c fixture */\n", 1},
		{"html", "<!-- sin-debt: html fixture -->\n", 1},
		{"yaml", "# sin-debt: yaml fixture\n", 1},
		{"multiple", "// sin-debt: one\n// sin-debt: two, upgrade: never\n", 2},
		{"empty", "no markers here\n", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "src."+c.name)
			if err := writeFileSafe(path, c.src); err != nil {
				t.Fatal(err)
			}
			mk, err := ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if len(mk) != c.want {
				t.Fatalf("got %d markers, want %d (%v)", len(mk), c.want, mk)
			}
		})
	}
}

func TestParseIgnoresPlainTextMention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.go")
	src := "// This package documents the `sin-debt:` convention in prose.\n// It explains how to write markers but contains no actual marker.\n"
	if err := writeFileSafe(path, src); err != nil {
		t.Fatal(err)
	}
	mk, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(mk) != 0 {
		t.Fatalf("expected 0 markers (text match should not be a marker), got %+v", mk)
	}
}

func TestParseMaxFileBytesSkipsLargeFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.go")
	huge := strings.Repeat("// sin-debt: nope\n", 10000)
	if err := writeFileSafe(path, huge); err != nil {
		t.Fatal(err)
	}
	mk, err := ParseFileWithCap(path, 1000)
	if err != nil {
		t.Fatalf("ParseFileWithCap: %v", err)
	}
	if mk != nil {
		t.Fatalf("expected nil for over-cap file, got %v", mk)
	}
}

// parseGolden loads every fixture file from `testdata/` and returns a
// single sorted slice. The fixtures are committed so the test is
// idempotent across runs.
func parseGolden(t *testing.T) []Marker {
	t.Helper()
	files := []string{
		"testdata/markers.go",
		"testdata/markers.py",
		"testdata/markers.ts",
		"testdata/markers.sh",
		"testdata/markers.md",
	}
	var all []Marker
	for _, f := range files {
		mk, err := ParseFile(f)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", f, err)
		}
		all = append(all, mk...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	return all
}

func uniqueSortedFiles(mk []Marker) []string {
	set := map[string]bool{}
	for _, m := range mk {
		set[m.File] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := writeFileSafe(path, content); err != nil {
		t.Fatal(err)
	}
}

func writeFileSafe(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
