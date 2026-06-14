// SPDX-License-Identifier: MIT
// Purpose: Tests for Golden Dataset Parser
package dataset

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDataset(t *testing.T) {
	// Use the existing critical.json
	ds, err := LoadDataset("../../../evals/critical.json")
	if err != nil {
		t.Fatalf("Failed to load critical.json: %v", err)
	}

	if ds.Name != "critical" {
		t.Errorf("Expected dataset name 'critical', got %q", ds.Name)
	}

	if len(ds.TestCases) != 8 {
		t.Errorf("Expected 8 test cases, got %d", len(ds.TestCases))
	}
}

func TestTestCaseValidation(t *testing.T) {
	ds, _ := LoadDataset("../../../evals/critical.json")

	for i, tc := range ds.TestCases {
		if tc.ID == "" {
			t.Errorf("Test case %d has empty ID", i)
		}
		if tc.Category == "" {
			t.Errorf("Test case %d has empty category", i)
		}
		if tc.Prompt == "" {
			t.Errorf("Test case %d has empty prompt", i)
		}
		if tc.Expected.MustContain == nil || len(tc.Expected.MustContain) == 0 {
			t.Logf("Test case %d has no MustContain constraints (OK)", i)
		}
	}
}

func TestConstraintValidation(t *testing.T) {
	tc := TestCase{
		ID:       "test-constraints",
		Prompt:   "test",
		Category: "testing",
		Constraints: Constraints{
			MaxTurns:      5,
			MaxTokens:     1000,
			TimeoutSeconds: 30,
		},
	}

	if tc.Constraints.MaxTurns != 5 {
		t.Error("MaxTurns constraint not set correctly")
	}
	if tc.Constraints.TimeoutSeconds != 30 {
		t.Error("TimeoutSeconds constraint not set correctly")
	}
}

func TestSaveDataset(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-dataset.json")

	// Create a test dataset
	ds := Dataset{
		Name:     "test",
		Version:  "1.0",
		TestCases: []TestCase{
			{
				ID:       "test-1",
				Category: "basic",
				Prompt:   "hello",
				Expected: Expected{
					MustContain: []string{"world"},
				},
				Constraints: Constraints{
					MaxTurns: 3,
				},
			},
		},
	}

	// Save it
	if err := SaveDataset(testFile, &ds); err != nil {
		t.Fatalf("Failed to save dataset: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); err != nil {
		t.Errorf("Dataset file not created: %v", err)
	}

	// Load it back
	loaded, err := LoadDataset(testFile)
	if err != nil {
		t.Fatalf("Failed to load saved dataset: %v", err)
	}

	if loaded.Name != ds.Name {
		t.Errorf("Loaded dataset name mismatch: %q != %q", loaded.Name, ds.Name)
	}

	if len(loaded.TestCases) != 1 {
		t.Errorf("Expected 1 test case, got %d", len(loaded.TestCases))
	}

	if loaded.TestCases[0].ID != "test-1" {
		t.Errorf("Test case ID mismatch")
	}
}

func TestMustUseToolsConstraint(t *testing.T) {
	tc := TestCase{
		ID: "test-tools",
		Constraints: Constraints{
			MustUseTools: []string{"code_gen", "verify"},
		},
	}

	if len(tc.Constraints.MustUseTools) != 2 {
		t.Error("MustUseTools not set correctly")
	}
}

func TestForbiddenToolsConstraint(t *testing.T) {
	tc := TestCase{
		ID: "test-forbidden",
		Constraints: Constraints{
			ForbiddenTools: []string{"delete_file"},
		},
	}

	if len(tc.Constraints.ForbiddenTools) != 1 {
		t.Error("ForbiddenTools not set correctly")
	}
}

func TestTimeoutConstraint(t *testing.T) {
	tc := TestCase{
		ID: "test-timeout",
		Constraints: Constraints{
			TimeoutSeconds: 60,
		},
	}

	duration := time.Duration(tc.Constraints.TimeoutSeconds) * time.Second
	if duration != 60*time.Second {
		t.Errorf("Timeout conversion failed: %v != 60s", duration)
	}
}

func TestExpectedFields(t *testing.T) {
	tc := TestCase{
		ID: "test-expected",
		Expected: Expected{
			MustContain:  []string{"success", "completed"},
			MustNotContain: []string{"error", "failed"},
			PassThreshold: 0.8,
		},
	}

	if len(tc.Expected.MustContain) != 2 {
		t.Error("MustContain not set correctly")
	}
	if len(tc.Expected.MustNotContain) != 2 {
		t.Error("MustNotContain not set correctly")
	}
	if tc.Expected.PassThreshold != 0.8 {
		t.Error("PassThreshold not set correctly")
	}
}

func BenchmarkLoadDataset(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = LoadDataset("../../../evals/critical.json")
	}
}

func BenchmarkSaveDataset(b *testing.B) {
	ds, _ := LoadDataset("../../../evals/critical.json")
	tmpDir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testFile := filepath.Join(tmpDir, "bench-"+string(rune(i))+".json")
		_ = SaveDataset(testFile, ds)
	}
}
