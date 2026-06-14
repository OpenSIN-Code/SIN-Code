// SPDX-License-Identifier: MIT
// Purpose: dataset parser + validator tests (issue #75).
package dataset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "ds.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const goodJSON = `{
  "name": "Critical",
  "version": "1.0.0",
  "test_cases": [
    {"id": "a", "prompt": "first"},
    {"id": "b", "prompt": "second"}
  ]
}`

func TestLoadDataset_Good(t *testing.T) {
	path := writeTemp(t, goodJSON)
	ds, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ds.Name != "Critical" || ds.Version != "1.0.0" || len(ds.TestCases) != 2 {
		t.Fatalf("unexpected dataset: %+v", ds)
	}
}

func TestLoadDataset_MissingFile(t *testing.T) {
	_, err := LoadDataset("/no/such/file.json")
	if err == nil {
		t.Fatal("expected error on missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Fatalf("expected 'read' in error, got %v", err)
	}
}

func TestDatasetValidate_Errors(t *testing.T) {
	cases := []struct {
		name string
		ds   Dataset
		want string
	}{
		{
			name: "missing name",
			ds:   Dataset{Version: "1.0", TestCases: []TestCase{{ID: "x", Prompt: "y"}}},
			want: "name is required",
		},
		{
			name: "missing version",
			ds:   Dataset{Name: "x", TestCases: []TestCase{{ID: "x", Prompt: "y"}}},
			want: "version is required",
		},
		{
			name: "no test cases",
			ds:   Dataset{Name: "x", Version: "1.0"},
			want: "empty",
		},
		{
			name: "missing prompt",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a"}, // prompt required
			}},
			want: "prompt is required",
		},
		{
			name: "duplicate id",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a", Prompt: "1"},
				{ID: "a", Prompt: "2"},
			}},
			want: "duplicate id",
		},
		{
			name: "require_verify without cmd",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a", Prompt: "p", Constraints: Constraints{RequireVerify: true}},
			}},
			want: "verify_cmd",
		},
		{
			name: "min_quality out of range",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a", Prompt: "p", Expected: Expected{MinQuality: 1.5}},
			}},
			want: "min_quality",
		},
		{
			name: "bad timeout",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a", Prompt: "p", Constraints: Constraints{Timeout: "lol"}},
			}},
			want: "timeout",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.ds.Validate()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q did not contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestFilterByTag(t *testing.T) {
	ds := &Dataset{
		Name: "x", Version: "1.0",
		TestCases: []TestCase{
			{ID: "a", Prompt: "1", Tags: []string{"smoke", "critical"}},
			{ID: "b", Prompt: "2", Tags: []string{"regress"}},
			{ID: "c", Prompt: "3", Tags: []string{"smoke"}},
		},
	}
	got := ds.FilterByTag("smoke")
	if len(got.TestCases) != 2 {
		t.Fatalf("smoke: got %d, want 2", len(got.TestCases))
	}
	got = ds.FilterByTag("UNKNOWN")
	if len(got.TestCases) != 0 {
		t.Fatalf("unknown tag should yield zero cases, got %d", len(got.TestCases))
	}
	got = ds.FilterByTag("")
	if len(got.TestCases) != 3 {
		t.Fatalf("empty filter should yield all 3, got %d", len(got.TestCases))
	}
}

func TestListDatasets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(goodJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(goodJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := ListDatasets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 datasets, got %d (%v)", len(files), files)
	}
}

func TestListDatasets_MissingDir(t *testing.T) {
	_, err := ListDatasets("/no/such/dir/at/all")
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}
