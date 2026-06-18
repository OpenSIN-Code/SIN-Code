// SPDX-License-Identifier: MIT
package modelperf

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockProvider struct {
	name    string
	passMod int // pass every Nth case
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Run(ctx context.Context, prompt string) (BenchmarkResult, error) {
	return BenchmarkResult{
		Passed:  true,
		Latency: 100 * time.Millisecond,
		Tokens:  500,
		CostUSD: 0.01,
		Output:  "ok",
	}, nil
}

type failProvider struct{ name string }

func (f *failProvider) Name() string                                   { return f.name }
func (f *failProvider) Run(ctx context.Context, prompt string) (BenchmarkResult, error) {
	return BenchmarkResult{Passed: false, Error: "fail"}, nil
}

func writeTestDataset(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	ds := `{
  "name": "test-dataset",
  "version": "1.0",
  "description": "code generation tasks",
  "test_cases": [
    {"id": "tc1", "prompt": "write a function that adds two numbers"},
    {"id": "tc2", "prompt": "write a function that multiplies two numbers"}
  ]
}`
	if err := os.WriteFile(path, []byte(ds), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunBenchmark_Success(t *testing.T) {
	s := testStore(t)
	path := writeTestDataset(t)

	providers := []BenchmarkProvider{
		&mockProvider{name: "model-a"},
		&mockProvider{name: "model-b"},
	}
	outcome, err := RunBenchmark(context.Background(), s, providers, BenchmarkConfig{
		DatasetPath: path,
		Category:    "code-generation",
		Timeout:     10 * time.Second,
	})
	if err != nil {
		t.Fatalf("RunBenchmark: %v", err)
	}
	if outcome.Providers != 2 {
		t.Errorf("expected 2 providers, got %d", outcome.Providers)
	}
	if outcome.Cases != 2 {
		t.Errorf("expected 2 cases, got %d", outcome.Cases)
	}
	if len(outcome.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(outcome.Results))
	}
	if outcome.Results[0].PassRate != 1.0 {
		t.Errorf("expected 100%% pass rate, got %.2f", outcome.Results[0].PassRate)
	}

	// Verify store has records
	recs, err := s.Recommend(context.Background(), "code-generation", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Errorf("expected 2 recommendations in store, got %d", len(recs))
	}
}

func TestRunBenchmark_FailProvider(t *testing.T) {
	s := testStore(t)
	path := writeTestDataset(t)

	providers := []BenchmarkProvider{
		&mockProvider{name: "good-model"},
		&failProvider{name: "bad-model"},
	}
	outcome, err := RunBenchmark(context.Background(), s, providers, BenchmarkConfig{
		DatasetPath: path,
		Category:    "code-generation",
	})
	if err != nil {
		t.Fatal(err)
	}
	goodResult := findResult(outcome.Results, "good-model")
	badResult := findResult(outcome.Results, "bad-model")
	if goodResult.PassRate != 1.0 {
		t.Errorf("good model should have 100%% pass rate")
	}
	if badResult.PassRate != 0.0 {
		t.Errorf("bad model should have 0%% pass rate")
	}
}

func TestRunBenchmark_AutoDetectCategory(t *testing.T) {
	s := testStore(t)
	path := writeTestDataset(t)

	outcome, err := RunBenchmark(context.Background(), s, []BenchmarkProvider{&mockProvider{name: "m"}}, BenchmarkConfig{
		DatasetPath: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Category == "" {
		t.Error("expected auto-detected category")
	}
}

func TestRunBenchmark_NoProviders(t *testing.T) {
	s := testStore(t)
	_, err := RunBenchmark(context.Background(), s, nil, BenchmarkConfig{DatasetPath: writeTestDataset(t)})
	if err == nil {
		t.Fatal("expected error for no providers")
	}
}

func TestRunBenchmark_NilStore(t *testing.T) {
	_, err := RunBenchmark(context.Background(), nil, []BenchmarkProvider{&mockProvider{name: "m"}}, BenchmarkConfig{DatasetPath: writeTestDataset(t)})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestRunBenchmark_BadDatasetPath(t *testing.T) {
	s := testStore(t)
	_, err := RunBenchmark(context.Background(), s, []BenchmarkProvider{&mockProvider{name: "m"}}, BenchmarkConfig{
		DatasetPath: "/nonexistent/path.json",
	})
	if err == nil {
		t.Fatal("expected error for bad path")
	}
}

func findResult(results []PerProviderResult, model string) *PerProviderResult {
	for i := range results {
		if results[i].Model == model {
			return &results[i]
		}
	}
	return nil
}
