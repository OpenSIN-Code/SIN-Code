// SPDX-License-Identifier: MIT
// Purpose: tests for sin-code benchmark — dataset discovery, report
// formatting (text/json/markdown), and pass-rate threshold checking.
// Tests use fixture datasets and do NOT require an LLM.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
)

// writeFixtureDataset writes a minimal valid golden dataset JSON to a
// temp file and returns the path.
func writeFixtureDataset(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
	return path
}

const fixtureDatasetA = `{
  "name": "FixtureA",
  "version": "1.0.0",
  "description": "test fixture A",
  "test_cases": [
    {
      "id": "tc-a1",
      "prompt": "say hello",
      "constraints": {"max_turns": 2, "timeout": "5s"},
      "expected": {"output_contains": ["stub echo"]}
    },
    {
      "id": "tc-a2",
      "prompt": "say world",
      "constraints": {"max_turns": 2, "timeout": "5s"},
      "expected": {"output_contains": ["stub echo"]}
    }
  ]
}`

const fixtureDatasetB = `{
  "name": "FixtureB",
  "version": "2.0.0",
  "description": "test fixture B",
  "test_cases": [
    {
      "id": "tc-b1",
      "prompt": "respond with hello world",
      "constraints": {"max_turns": 2, "timeout": "5s"},
      "expected": {"output_contains": ["hello"]}
    }
  ]
}`

// ── Dataset discovery ─────────────────────────────────────────────

func TestResolveDatasetPaths_ExplicitArgs(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFixtureDataset(t, dir, "a.json", fixtureDatasetA)
	p2 := writeFixtureDataset(t, dir, "b.json", fixtureDatasetB)

	paths, err := resolveDatasetPaths([]string{p1, p2})
	if err != nil {
		t.Fatalf("resolveDatasetPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
}

func TestResolveDatasetPaths_NoArgsDiscoversEvals(t *testing.T) {
	dir := t.TempDir()
	writeFixtureDataset(t, dir, "alpha.json", fixtureDatasetA)
	writeFixtureDataset(t, dir, "beta.json", fixtureDatasetB)

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(filepath.Dir(dir)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Base(dir)
	if err := os.Rename(filepath.Join(filepath.Dir(dir), target), filepath.Join(filepath.Dir(dir), "evals")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Rename(filepath.Join(filepath.Dir(dir), "evals"), filepath.Join(filepath.Dir(dir), target))
	})

	paths, err := resolveDatasetPaths(nil)
	if err != nil {
		t.Fatalf("resolveDatasetPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 discovered paths, got %d: %v", len(paths), paths)
	}
}

func TestResolveDatasetPaths_NonexistentArg(t *testing.T) {
	_, err := resolveDatasetPaths([]string{"/nonexistent/path/to/file.json"})
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

// ── Dry-run validation ────────────────────────────────────────────

func TestDryRunDatasets_Valid(t *testing.T) {
	dir := t.TempDir()
	p := writeFixtureDataset(t, dir, "ok.json", fixtureDatasetA)
	err := dryRunDatasets([]string{p})
	if err != nil {
		t.Fatalf("dryRunDatasets: %v", err)
	}
}

func TestDryRunDatasets_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := writeFixtureDataset(t, dir, "bad.json", `{not valid json}`)
	err := dryRunDatasets([]string{p})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ── Report formatting ─────────────────────────────────────────────

func makeTestReport() *BenchmarkReport {
	return &BenchmarkReport{
		StartedAt:    time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		FinishedAt:   time.Date(2026, 6, 25, 12, 5, 0, 0, time.UTC),
		MinPassRate:  0.8,
		OverallRate:  0.75,
		TotalCases:   4,
		TotalPassed:  3,
		TotalSkipped: 1,
		Datasets: []DatasetResult{
			{
				Path:         "evals/a.json",
				Name:         "FixtureA",
				Version:      "1.0.0",
				Cases:        2,
				Passed:       2,
				Failed:       0,
				Skipped:      0,
				PassRate:     1.0,
				MedianLatMS:  150,
				MedianTokens: 100,
				MedianLOC:    5,
			},
			{
				Path:         "evals/b.json",
				Name:         "FixtureB",
				Version:      "2.0.0",
				Cases:        2,
				Passed:       1,
				Failed:       1,
				Skipped:      1,
				PassRate:     0.5,
				MedianLatMS:  200,
				MedianTokens: 50,
				MedianLOC:    3,
			},
		},
	}
}

func TestFormatText(t *testing.T) {
	report := makeTestReport()
	out := formatText(report)

	mustContain := []string{
		"SIN-Code Benchmark Report",
		"FixtureA",
		"FixtureB",
		"100.0%",
		"50.0%",
		"OK",
		"BELOW",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("formatText: missing %q in output:\n%s", s, out)
		}
	}
}

func TestFormatMarkdown(t *testing.T) {
	report := makeTestReport()
	out := formatBenchmarkMarkdown(report)

	mustContain := []string{
		"## SIN-Code Benchmark Report",
		"| Dataset |",
		"| FixtureA |",
		"| FixtureB |",
		"100.0%",
		"50.0%",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("formatMarkdown: missing %q in output:\n%s", s, out)
		}
	}
}

func TestFormatJSON(t *testing.T) {
	report := makeTestReport()
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded BenchmarkReport
	if err := json.Unmarshal([]byte(buf.String()), &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if decoded.OverallRate != report.OverallRate {
		t.Errorf("overall rate: got %v, want %v", decoded.OverallRate, report.OverallRate)
	}
	if len(decoded.Datasets) != 2 {
		t.Errorf("datasets: got %d, want 2", len(decoded.Datasets))
	}
	if decoded.Datasets[0].Name != "FixtureA" {
		t.Errorf("dataset[0] name: got %q, want FixtureA", decoded.Datasets[0].Name)
	}
}

// ── Pass-rate threshold ───────────────────────────────────────────

func TestCheckThreshold_AllPass(t *testing.T) {
	report := &BenchmarkReport{
		MinPassRate: 0.8,
		Datasets: []DatasetResult{
			{Path: "a.json", Cases: 10, Passed: 9, Failed: 1, PassRate: 0.9},
			{Path: "b.json", Cases: 10, Passed: 8, Failed: 2, PassRate: 0.8},
		},
	}
	if err := checkThreshold(report, 0.8); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckThreshold_OneBelow(t *testing.T) {
	report := &BenchmarkReport{
		MinPassRate: 0.8,
		Datasets: []DatasetResult{
			{Path: "a.json", Cases: 10, Passed: 9, Failed: 1, PassRate: 0.9},
			{Path: "evals/bad.json", Cases: 10, Passed: 5, Failed: 5, PassRate: 0.5},
		},
	}
	err := checkThreshold(report, 0.8)
	if err == nil {
		t.Fatal("expected error when one dataset is below threshold")
	}
	if !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("error should mention the failing dataset, got: %v", err)
	}
}

func TestCheckThreshold_ErrorDataset(t *testing.T) {
	report := &BenchmarkReport{
		MinPassRate: 0.8,
		Datasets: []DatasetResult{
			{Path: "evals/broken.json", Error: "load failed"},
		},
	}
	err := checkThreshold(report, 0.8)
	if err == nil {
		t.Fatal("expected error for errored dataset")
	}
	if !strings.Contains(err.Error(), "broken.json") {
		t.Errorf("error should mention the broken dataset, got: %v", err)
	}
}

func TestCheckThreshold_EmptyReport(t *testing.T) {
	report := &BenchmarkReport{MinPassRate: 0.8}
	if err := checkThreshold(report, 0.8); err != nil {
		t.Errorf("expected nil for empty report, got %v", err)
	}
}

// ── Helpers ───────────────────────────────────────────────────────

func TestApproxTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"ab", 0},
		{"abcd", 1},
		{"abcdefgh", 2},
	}
	for _, tc := range tests {
		got := approxTokens(tc.input)
		if got != tc.want {
			t.Errorf("approxTokens(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"one line", 1},
		{"line1\nline2", 2},
		{"a\nb\nc\n", 4},
	}
	for _, tc := range tests {
		got := countLines(tc.input)
		if got != tc.want {
			t.Errorf("countLines(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestMedianInt64(t *testing.T) {
	tests := []struct {
		input []int64
		want  int64
	}{
		{nil, 0},
		{[]int64{5}, 5},
		{[]int64{1, 3, 5}, 3},
		{[]int64{1, 2, 3, 4}, 3},
		{[]int64{10, 20, 30, 40, 50}, 30},
	}
	for _, tc := range tests {
		got := medianInt64(tc.input)
		if got != tc.want {
			t.Errorf("medianInt64(%v) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCountModelRequired(t *testing.T) {
	ds := &dataset.Dataset{
		Name:    "test",
		Version: "1.0",
		TestCases: []dataset.TestCase{
			{ID: "a", Prompt: "x"},
			{ID: "b", Prompt: "x", Scorer: dataset.ScorerConfig{Type: "compile_and_run", Language: "python", RequiresModel: true}},
			{ID: "c", Prompt: "x", Scorer: dataset.ScorerConfig{Type: "exact"}},
			{ID: "d", Prompt: "x", Scorer: dataset.ScorerConfig{Type: "contains", RequiresModel: true}},
		},
	}
	if err := ds.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := countModelRequired(ds)
	if got != 2 {
		t.Errorf("countModelRequired: got %d, want 2", got)
	}
}
