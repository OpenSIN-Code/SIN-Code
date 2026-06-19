package swebench

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDataset(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantErr   bool
		errSubstr string
		wantLen   int
	}{
		{
			name:    "valid JSON array",
			content: `[{"instance_id":"inst-1","repo":"django/django","base_commit":"abc123","problem_statement":"Fix bug","patch":"","test_patch":"","FAIL_TO_PASS":["tests/test_foo.py"],"PASS_TO_PASS":["tests/test_bar.py"],"version":"3.2"}]`,
			wantLen: 1,
		},
		{
			name:    "valid JSON object",
			content: `{"instances":[{"instance_id":"inst-2","repo":"scikit-learn/scikit-learn","base_commit":"def456","problem_statement":"Fix issue","patch":"","test_patch":"","FAIL_TO_PASS":[],"PASS_TO_PASS":[],"version":"1.0"}]}`,
			wantLen: 1,
		},
		{
			name:    "valid JSON array with multiple instances",
			content: `[{"instance_id":"a","repo":"r","base_commit":"c","problem_statement":"p","patch":"","test_patch":"","FAIL_TO_PASS":[],"PASS_TO_PASS":[],"version":"1"},{"instance_id":"b","repo":"r2","base_commit":"c2","problem_statement":"p2","patch":"","test_patch":"","FAIL_TO_PASS":[],"PASS_TO_PASS":[],"version":"2"}]`,
			wantLen: 2,
		},
		{
			name:    "empty JSON array",
			content: `[]`,
			wantLen: 0,
		},
		{
			name:    "empty JSON object",
			content: `{"instances":[]}`,
			wantLen: 0,
		},
		{
			name:      "invalid JSON",
			content:   `{not valid json!!!`,
			wantErr:   true,
			errSubstr: "parse",
		},
		{
			name:      "completely malformed",
			content:   `xyz`,
			wantErr:   true,
			errSubstr: "parse",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "dataset.json")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}

			ds, err := LoadDataset(path)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ds.Instances) != tc.wantLen {
				t.Fatalf("got %d instances, want %d", len(ds.Instances), tc.wantLen)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadDataset("/nonexistent/path/dataset.json")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
		if !strings.Contains(err.Error(), "read") {
			t.Fatalf("error %q should contain 'read'", err.Error())
		}
	})
}

func TestConvertDataset(t *testing.T) {
	ds := &Dataset{
		Instances: []Instance{
			{
				InstanceID:       "django__django-12345",
				Repo:             "django/django",
				BaseCommit:       "abc123",
				ProblemStatement: "Fix the widget rendering bug",
				Patch:            "diff --git ...",
				TestPatch:        "diff --git ...",
				FailToPass:       []string{"tests.test_widget.TestWidget.test_render"},
				PassToPass:       []string{"tests.test_widget.TestWidget.test_init", "tests.test_form.TestForm.test_valid"},
				Version:          "3.2",
			},
			{
				InstanceID:       "sklearn__sklearn-99",
				Repo:             "scikit-learn/scikit-learn",
				BaseCommit:       "def456",
				ProblemStatement: "Fix classifier crash",
				FailToPass:       []string{"sklearn/tests/test_clf.py::test_fit"},
				PassToPass:       nil,
				Version:          "1.0",
			},
		},
	}

	cases := ConvertDataset(ds)

	if len(cases) != 2 {
		t.Fatalf("got %d cases, want 2", len(cases))
	}

	t.Run("first instance prompt generation", func(t *testing.T) {
		c := cases[0]
		if c.ID != "django__django-12345" {
			t.Errorf("ID = %q, want %q", c.ID, "django__django-12345")
		}
		if c.Description != "SWE-bench: django__django-12345" {
			t.Errorf("Description = %q", c.Description)
		}
		if !strings.Contains(c.Prompt, "django__django-12345") {
			t.Error("Prompt missing instance_id")
		}
		if !strings.Contains(c.Prompt, "django/django") {
			t.Error("Prompt missing repo")
		}
		if !strings.Contains(c.Prompt, "Fix the widget rendering bug") {
			t.Error("Prompt missing problem statement")
		}
		if !strings.Contains(c.Prompt, "tests.test_widget.TestWidget.test_render") {
			t.Error("Prompt missing FailToPass test")
		}
		if !strings.Contains(c.Prompt, "tests.test_widget.TestWidget.test_init") {
			t.Error("Prompt missing PassToPass test")
		}
	})

	t.Run("tags", func(t *testing.T) {
		for i, c := range cases {
			if len(c.Tags) != 1 || c.Tags[0] != "swe-bench" {
				t.Errorf("case %d: Tags = %v, want [swe-bench]", i, c.Tags)
			}
		}
	})

	t.Run("constraints", func(t *testing.T) {
		c := cases[0]
		if v, ok := c.Constraints["require_verify"]; !ok || v != true {
			t.Errorf("require_verify = %v, want true", v)
		}
		if v, ok := c.Constraints["max_turns"]; !ok || v != 60 {
			t.Errorf("max_turns = %v, want 60", v)
		}
	})

	t.Run("metadata", func(t *testing.T) {
		c := cases[0]
		if c.Metadata["repo"] != "django/django" {
			t.Errorf("metadata repo = %q", c.Metadata["repo"])
		}
		if c.Metadata["instance_id"] != "django__django-12345" {
			t.Errorf("metadata instance_id = %q", c.Metadata["instance_id"])
		}
	})

	t.Run("expected", func(t *testing.T) {
		c := cases[0]
		if c.Expected["custom_criteria"] != "All tests pass" {
			t.Errorf("custom_criteria = %v", c.Expected["custom_criteria"])
		}
	})

	t.Run("verify_cmd contains pytest commands", func(t *testing.T) {
		c := cases[0]
		if !strings.Contains(c.VerifyCmd, "python -m pytest 'tests.test_widget.TestWidget.test_render' -x") {
			t.Errorf("VerifyCmd missing FailToPass pytest: %q", c.VerifyCmd)
		}
		if !strings.Contains(c.VerifyCmd, "python -m pytest 'tests.test_widget.TestWidget.test_init' -x") {
			t.Errorf("VerifyCmd missing PassToPass pytest: %q", c.VerifyCmd)
		}
		if !strings.Contains(c.VerifyCmd, "&&") {
			t.Errorf("VerifyCmd should join with &&: %q", c.VerifyCmd)
		}
	})

	t.Run("second instance no PassToPass", func(t *testing.T) {
		c := cases[1]
		if !strings.Contains(c.Prompt, "sklearn__sklearn-99") {
			t.Error("Prompt missing instance_id for second case")
		}
		if strings.Contains(c.VerifyCmd, "&&") {
			t.Errorf("VerifyCmd should not have && with only FailToPass: %q", c.VerifyCmd)
		}
	})

	t.Run("empty dataset", func(t *testing.T) {
		cases := ConvertDataset(&Dataset{})
		if len(cases) != 0 {
			t.Fatalf("got %d cases, want 0", len(cases))
		}
	})
}

func TestScoreInstance(t *testing.T) {
	baseInst := &Instance{
		InstanceID: "test-1",
		FailToPass: []string{"test_a", "test_b"},
		PassToPass: []string{"test_c", "test_d"},
	}

	tests := []struct {
		name          string
		inst          *Instance
		verifyOutput  string
		wantResolved  bool
		wantFTP       int
		wantPTP       int
		wantScore     float64
		wantScoreTol  float64
	}{
		{
			name:         "all pass",
			inst:         baseInst,
			verifyOutput: "test_a passed\ntest_b passed\ntest_c passed\ntest_d passed",
			wantResolved: true,
			wantFTP:      2,
			wantPTP:      2,
			wantScore:    1.0,
			wantScoreTol: 0.001,
		},
		{
			name:         "all pass with ok keyword",
			inst:         baseInst,
			verifyOutput: "test_a ok\ntest_b ok\ntest_c ok\ntest_d ok",
			wantResolved: true,
			wantFTP:      2,
			wantPTP:      2,
			wantScore:    1.0,
			wantScoreTol: 0.001,
		},
		{
			name:         "partial pass - FailToPass only one",
			inst:         baseInst,
			verifyOutput: "test_a passed\ntest_c passed\ntest_d passed",
			wantResolved: false,
			wantFTP:      1,
			wantPTP:      2,
			wantScore:    0.7*0.5 + 0.3*1.0,
			wantScoreTol: 0.001,
		},
		{
			name:         "no pass",
			inst:         baseInst,
			verifyOutput: "test_a FAILED\ntest_b FAILED\ntest_c FAILED\ntest_d FAILED",
			wantResolved: false,
			wantFTP:      0,
			wantPTP:      0,
			wantScore:    0.0,
			wantScoreTol: 0.001,
		},
		{
			name:         "empty verify output",
			inst:         baseInst,
			verifyOutput: "",
			wantResolved: false,
			wantFTP:      0,
			wantPTP:      0,
			wantScore:    0.0,
			wantScoreTol: 0.001,
		},
		{
			name: "no FailToPass tests - PassToPass still counted",
			inst: &Instance{
				InstanceID: "test-2",
				FailToPass: nil,
				PassToPass: []string{"test_x"},
			},
			verifyOutput: "test_x passed",
			wantResolved: false,
			wantFTP:      0,
			wantPTP:      1,
			wantScore:    0.0,
			wantScoreTol: 0.001,
		},
		{
			name: "no PassToPass tests - all FailToPass pass",
			inst: &Instance{
				InstanceID: "test-3",
				FailToPass: []string{"test_y"},
				PassToPass: nil,
			},
			verifyOutput: "test_y passed",
			wantResolved: true,
			wantFTP:      1,
			wantPTP:      0,
			wantScore:    1.0,
			wantScoreTol: 0.001,
		},
		{
			name: "test name in output but no passed/ok keyword",
			inst: &Instance{
				InstanceID: "test-4",
				FailToPass: []string{"test_z"},
				PassToPass: nil,
			},
			verifyOutput: "test_z FAILED",
			wantResolved: false,
			wantFTP:      0,
			wantPTP:      0,
			wantScore:    0.3,
			wantScoreTol: 0.001,
		},
		{
			name: "passed keyword without matching test name",
			inst: &Instance{
				InstanceID: "test-5",
				FailToPass: []string{"test_specific"},
				PassToPass: nil,
			},
			verifyOutput: "test_other passed",
			wantResolved: false,
			wantFTP:      0,
			wantPTP:      0,
			wantScore:    0.3,
			wantScoreTol: 0.001,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := ScoreInstance(tc.inst, tc.verifyOutput)

			if r.InstanceID != tc.inst.InstanceID {
				t.Errorf("InstanceID = %q, want %q", r.InstanceID, tc.inst.InstanceID)
			}
			if r.Resolved != tc.wantResolved {
				t.Errorf("Resolved = %v, want %v", r.Resolved, tc.wantResolved)
			}
			if r.FailToPass != tc.wantFTP {
				t.Errorf("FailToPass = %d, want %d", r.FailToPass, tc.wantFTP)
			}
			if r.FailToPassTotal != len(tc.inst.FailToPass) {
				t.Errorf("FailToPassTotal = %d, want %d", r.FailToPassTotal, len(tc.inst.FailToPass))
			}
			if r.PassToPass != tc.wantPTP {
				t.Errorf("PassToPass = %d, want %d", r.PassToPass, tc.wantPTP)
			}
			if r.PassToPassTotal != len(tc.inst.PassToPass) {
				t.Errorf("PassToPassTotal = %d, want %d", r.PassToPassTotal, len(tc.inst.PassToPass))
			}
			if math.Abs(r.Score-tc.wantScore) > tc.wantScoreTol {
				t.Errorf("Score = %f, want %f (tol %f)", r.Score, tc.wantScore, tc.wantScoreTol)
			}
		})
	}
}

func TestSummarizeResults(t *testing.T) {
	t.Run("mixed results", func(t *testing.T) {
		results := []ScorerResult{
			{InstanceID: "a", Resolved: true, Score: 1.0},
			{InstanceID: "b", Resolved: false, Score: 0.5},
			{InstanceID: "c", Resolved: true, Score: 0.8},
			{InstanceID: "d", Resolved: false, Score: 0.0},
		}

		s := SummarizeResults(results)

		if s.Total != 4 {
			t.Errorf("Total = %d, want 4", s.Total)
		}
		if s.Resolved != 2 {
			t.Errorf("Resolved = %d, want 2", s.Resolved)
		}
		wantRate := 0.5
		if math.Abs(s.ResolveRate-wantRate) > 0.001 {
			t.Errorf("ResolveRate = %f, want %f", s.ResolveRate, wantRate)
		}
		wantMean := (1.0 + 0.5 + 0.8 + 0.0) / 4.0
		if math.Abs(s.MeanScore-wantMean) > 0.001 {
			t.Errorf("MeanScore = %f, want %f", s.MeanScore, wantMean)
		}
		if len(s.Results) != 4 {
			t.Errorf("Results len = %d, want 4", len(s.Results))
		}
	})

	t.Run("empty results", func(t *testing.T) {
		s := SummarizeResults(nil)

		if s.Total != 0 {
			t.Errorf("Total = %d, want 0", s.Total)
		}
		if s.Resolved != 0 {
			t.Errorf("Resolved = %d, want 0", s.Resolved)
		}
		if s.ResolveRate != 0 {
			t.Errorf("ResolveRate = %f, want 0", s.ResolveRate)
		}
		if s.MeanScore != 0 {
			t.Errorf("MeanScore = %f, want 0", s.MeanScore)
		}
	})

	t.Run("all resolved", func(t *testing.T) {
		results := []ScorerResult{
			{InstanceID: "a", Resolved: true, Score: 1.0},
			{InstanceID: "b", Resolved: true, Score: 1.0},
			{InstanceID: "c", Resolved: true, Score: 1.0},
		}

		s := SummarizeResults(results)

		if s.Total != 3 {
			t.Errorf("Total = %d, want 3", s.Total)
		}
		if s.Resolved != 3 {
			t.Errorf("Resolved = %d, want 3", s.Resolved)
		}
		if math.Abs(s.ResolveRate-1.0) > 0.001 {
			t.Errorf("ResolveRate = %f, want 1.0", s.ResolveRate)
		}
		if math.Abs(s.MeanScore-1.0) > 0.001 {
			t.Errorf("MeanScore = %f, want 1.0", s.MeanScore)
		}
	})

	t.Run("none resolved", func(t *testing.T) {
		results := []ScorerResult{
			{InstanceID: "a", Resolved: false, Score: 0.2},
			{InstanceID: "b", Resolved: false, Score: 0.1},
		}

		s := SummarizeResults(results)

		if s.Total != 2 {
			t.Errorf("Total = %d, want 2", s.Total)
		}
		if s.Resolved != 0 {
			t.Errorf("Resolved = %d, want 0", s.Resolved)
		}
		if s.ResolveRate != 0 {
			t.Errorf("ResolveRate = %f, want 0", s.ResolveRate)
		}
		if math.Abs(s.MeanScore-0.15) > 0.001 {
			t.Errorf("MeanScore = %f, want 0.15", s.MeanScore)
		}
	})

	t.Run("single result", func(t *testing.T) {
		results := []ScorerResult{
			{InstanceID: "x", Resolved: true, Score: 0.7},
		}

		s := SummarizeResults(results)

		if s.Total != 1 {
			t.Errorf("Total = %d, want 1", s.Total)
		}
		if s.Resolved != 1 {
			t.Errorf("Resolved = %d, want 1", s.Resolved)
		}
		if math.Abs(s.ResolveRate-1.0) > 0.001 {
			t.Errorf("ResolveRate = %f, want 1.0", s.ResolveRate)
		}
		if math.Abs(s.MeanScore-0.7) > 0.001 {
			t.Errorf("MeanScore = %f, want 0.7", s.MeanScore)
		}
	})
}

func TestWriteEvalDataset(t *testing.T) {
	t.Run("write and verify JSON structure", func(t *testing.T) {
		cases := []TestCase{
			{
				ID:          "test-1",
				Description: "Test case 1",
				Prompt:      "Fix something",
				Tags:        []string{"swe-bench"},
				Constraints: map[string]any{"require_verify": true},
				Expected:    map[string]any{"custom_criteria": "All tests pass"},
				VerifyCmd:   "python -m pytest test_foo.py -x",
				Metadata:    map[string]string{"repo": "django/django"},
			},
			{
				ID:          "test-2",
				Description: "Test case 2",
				Prompt:      "Fix another thing",
				Tags:        []string{"swe-bench"},
				VerifyCmd:   "python -m pytest test_bar.py -x",
				Metadata:    map[string]string{"repo": "flask/flask"},
			},
		}

		dir := t.TempDir()
		outPath := filepath.Join(dir, "subdir", "eval.json")

		if err := WriteEvalDataset(cases, outPath); err != nil {
			t.Fatalf("WriteEvalDataset error: %v", err)
		}

		raw, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("ReadFile error: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("JSON parse error: %v", err)
		}

		if parsed["name"] != "swe-bench" {
			t.Errorf("name = %v, want swe-bench", parsed["name"])
		}
		if parsed["version"] != "1.0" {
			t.Errorf("version = %v, want 1.0", parsed["version"])
		}

		testCases, ok := parsed["test_cases"].([]any)
		if !ok {
			t.Fatal("test_cases is not an array")
		}
		if len(testCases) != 2 {
			t.Fatalf("test_cases len = %d, want 2", len(testCases))
		}

		first := testCases[0].(map[string]any)
		if first["id"] != "test-1" {
			t.Errorf("first case id = %v", first["id"])
		}
		if first["description"] != "Test case 1" {
			t.Errorf("first case description = %v", first["description"])
		}
		if first["prompt"] != "Fix something" {
			t.Errorf("first case prompt = %v", first["prompt"])
		}

		tags, ok := first["tags"].([]any)
		if !ok || len(tags) != 1 || tags[0] != "swe-bench" {
			t.Errorf("first case tags = %v", first["tags"])
		}

		meta, ok := first["metadata"].(map[string]any)
		if !ok {
			t.Fatal("first case metadata is not a map")
		}
		if meta["repo"] != "django/django" {
			t.Errorf("first case metadata repo = %v", meta["repo"])
		}

		constraints, ok := first["constraints"].(map[string]any)
		if !ok {
			t.Fatal("first case constraints is not a map")
		}
		if constraints["require_verify"] != true {
			t.Errorf("first case constraints require_verify = %v", constraints["require_verify"])
		}
	})

	t.Run("empty cases", func(t *testing.T) {
		dir := t.TempDir()
		outPath := filepath.Join(dir, "empty.json")

		if err := WriteEvalDataset(nil, outPath); err != nil {
			t.Fatalf("WriteEvalDataset error: %v", err)
		}

		raw, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("ReadFile error: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("JSON parse error: %v", err)
		}

		if parsed["name"] != "swe-bench" {
			t.Errorf("name = %v, want swe-bench", parsed["name"])
		}
		if parsed["version"] != "1.0" {
			t.Errorf("version = %v, want 1.0", parsed["version"])
		}
		if parsed["test_cases"] != nil {
			t.Errorf("test_cases = %v, want nil (null)", parsed["test_cases"])
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		outPath := filepath.Join(dir, "deep", "nested", "dir", "eval.json")

		if err := WriteEvalDataset([]TestCase{{ID: "x"}}, outPath); err != nil {
			t.Fatalf("WriteEvalDataset error: %v", err)
		}

		if _, err := os.Stat(outPath); err != nil {
			t.Fatalf("output file not created: %v", err)
		}
	})
}

func TestBuildVerifyCmd(t *testing.T) {
	t.Run("both FailToPass and PassToPass", func(t *testing.T) {
		inst := &Instance{
			FailToPass: []string{"test_a.py", "test_b.py"},
			PassToPass: []string{"test_c.py"},
		}
		cmd := buildVerifyCmd(inst)
		if !strings.Contains(cmd, "python -m pytest 'test_a.py' -x") {
			t.Errorf("missing test_a: %q", cmd)
		}
		if !strings.Contains(cmd, "python -m pytest 'test_b.py' -x") {
			t.Errorf("missing test_b: %q", cmd)
		}
		if !strings.Contains(cmd, "python -m pytest 'test_c.py' -x") {
			t.Errorf("missing test_c: %q", cmd)
		}
		parts := strings.Split(cmd, " && ")
		if len(parts) != 3 {
			t.Errorf("expected 3 parts joined by &&, got %d: %q", len(parts), cmd)
		}
	})

	t.Run("only FailToPass", func(t *testing.T) {
		inst := &Instance{
			FailToPass: []string{"test_x.py"},
			PassToPass: nil,
		}
		cmd := buildVerifyCmd(inst)
		if cmd != "python -m pytest 'test_x.py' -x" {
			t.Errorf("cmd = %q", cmd)
		}
	})

	t.Run("only PassToPass", func(t *testing.T) {
		inst := &Instance{
			FailToPass: nil,
			PassToPass: []string{"test_y.py"},
		}
		cmd := buildVerifyCmd(inst)
		if cmd != "python -m pytest 'test_y.py' -x" {
			t.Errorf("cmd = %q", cmd)
		}
	})

	t.Run("empty", func(t *testing.T) {
		inst := &Instance{}
		cmd := buildVerifyCmd(inst)
		if cmd != "" {
			t.Errorf("cmd = %q, want empty", cmd)
		}
	})
}
