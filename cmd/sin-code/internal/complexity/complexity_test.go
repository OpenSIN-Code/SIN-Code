package complexity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindAll(t *testing.T) {
	root := testdataRoot(t)
	markers, err := ParseMarkers(root)
	if err != nil {
		t.Fatalf("parse markers: %v", err)
	}
	findings, err := Find(Options{Root: root, MarkerMap: markers})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(findings) != 8 {
		t.Fatalf("expected 8 findings, got %d", len(findings))
	}

	got, err := Report(Rank(findings), "text")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	want, err := os.ReadFile(filepath.Join(root, "sample.golden.txt"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("text output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	if findings[2].ApprovedBy == "" {
		t.Fatalf("expected sin-debt marker on interface finding")
	}
}

func TestFindAllJSON(t *testing.T) {
	root := testdataRoot(t)
	findings, err := Find(Options{Root: root})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	out, err := Report(findings, "json")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	var envelope struct {
		Findings []Finding `json:"findings"`
		NetLines int       `json:"net_lines"`
		NetDeps  int       `json:"net_deps"`
		Status   string    `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if envelope.Status != "cuts-available" {
		t.Fatalf("expected status cuts-available, got %s", envelope.Status)
	}
	if envelope.NetLines != 23 {
		t.Fatalf("expected net_lines 23, got %d", envelope.NetLines)
	}
	if envelope.NetDeps != 1 {
		t.Fatalf("expected net_deps 1, got %d", envelope.NetDeps)
	}
	if len(envelope.Findings) != 8 {
		t.Fatalf("expected 8 findings in json, got %d", len(envelope.Findings))
	}
}

func TestFindAllMarkdown(t *testing.T) {
	root := testdataRoot(t)
	findings, err := Find(Options{Root: root})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	out, err := Report(findings, "markdown")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	for _, s := range []string{"## Complexity review", "| Tag |", "net: -23 lines, -1 deps possible"} {
		if !strings.Contains(out, s) {
			t.Fatalf("markdown output missing %q", s)
		}
	}
}

func TestTagFilter(t *testing.T) {
	root := testdataRoot(t)
	findings, err := Find(Options{Root: root, Tags: []string{TagYagni}})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(findings) != 4 {
		t.Fatalf("expected 4 yagni findings, got %d", len(findings))
	}
	for _, f := range findings {
		if f.Tag != TagYagni {
			t.Fatalf("expected yagni, got %s", f.Tag)
		}
	}
}

func TestSinDebtMarkers(t *testing.T) {
	root := testdataRoot(t)
	markers, err := ParseMarkers(root)
	if err != nil {
		t.Fatalf("parse markers: %v", err)
	}
	ms, ok := markers["iface.go"]
	if !ok {
		t.Fatalf("expected markers in iface.go")
	}
	if len(ms) != 1 || ms[0].Line != 4 || ms[0].Reason != "needed for future UDP impl" {
		t.Fatalf("unexpected marker: %+v", ms)
	}
}

func TestRank(t *testing.T) {
	input := []Finding{
		{Tag: TagShrink, Path: "a.go", Line: 1, LineCount: 3},
		{Tag: TagStdlib, Path: "b.go", Line: 1, LineCount: 10},
		{Tag: TagNative, Path: "c.go", Line: 1, LineCount: 3, DepsRemoved: []string{"x"}},
	}
	ranked := Rank(input)
	if ranked[0].LineCount != 10 {
		t.Fatalf("expected first line count 10, got %d", ranked[0].LineCount)
	}
	if ranked[1].Tag != TagNative {
		t.Fatalf("expected native second, got %s", ranked[1].Tag)
	}
}

func TestEmptyReport(t *testing.T) {
	out, err := Report(nil, "text")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if out != "Lean already. Ship." {
		t.Fatalf("expected empty report, got %q", out)
	}
}

func TestUnknownFormat(t *testing.T) {
	_, err := Report(nil, "xml")
	if err == nil {
		t.Fatalf("expected error for unknown format")
	}
}

func testdataRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("testdata/sample")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
