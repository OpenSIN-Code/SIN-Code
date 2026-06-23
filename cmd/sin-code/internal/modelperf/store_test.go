// SPDX-License-Identifier: MIT
package modelperf

import (
	"context"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "modelperf.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertAndRecommend(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	records := []PerfRecord{
		{Model: "model-a", Category: "code-generation", Dataset: "evals/code.json", PassRate: 0.9, AvgLatencyMs: 1000, AvgCostUSD: 0.01, AvgTokens: 500, SampleCount: 1},
		{Model: "model-b", Category: "code-generation", Dataset: "evals/code.json", PassRate: 0.7, AvgLatencyMs: 800, AvgCostUSD: 0.005, AvgTokens: 400, SampleCount: 1},
		{Model: "model-c", Category: "code-generation", Dataset: "evals/code.json", PassRate: 0.95, AvgLatencyMs: 1500, AvgCostUSD: 0.02, AvgTokens: 600, SampleCount: 1},
	}
	for _, r := range records {
		if err := s.Upsert(ctx, r); err != nil {
			t.Fatalf("Upsert: %v", err)
		}
	}

	recs, err := s.Recommend(ctx, "code-generation", 3, 0)
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected 3 recommendations, got %d", len(recs))
	}
	// model-c has highest pass_rate (0.95) → highest score
	if recs[0].Model != "model-c" {
		t.Errorf("expected model-c first, got %s", recs[0].Model)
	}
}

func TestUpsertIncrementSampleCount(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	r := PerfRecord{Model: "m", Category: "cat", Dataset: "ds", PassRate: 0.8, SampleCount: 1}
	if err := s.Upsert(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(ctx, r); err != nil {
		t.Fatal(err)
	}

	recs, err := s.Recommend(ctx, "cat", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(recs))
	}
	if recs[0].Samples != 2 {
		t.Errorf("expected sample_count=2, got %d", recs[0].Samples)
	}
}

func TestRecommendEmptyStore(t *testing.T) {
	s := testStore(t)
	recs, err := s.Recommend(context.Background(), "nonexistent", 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if recs != nil {
		t.Errorf("expected nil for empty store, got %v", recs)
	}
}

func TestRecommendMinSamples(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	r := PerfRecord{Model: "m", Category: "cat", Dataset: "ds", PassRate: 0.9, SampleCount: 1}
	if err := s.Upsert(ctx, r); err != nil {
		t.Fatal(err)
	}
	// minSamples=2 → should exclude our single-sample model
	recs, err := s.Recommend(ctx, "cat", 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if recs != nil {
		t.Errorf("expected nil with minSamples=2, got %v", recs)
	}
}

func TestRanking(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, r := range []PerfRecord{
		{Model: "a", Category: "cat1", Dataset: "ds", PassRate: 0.5},
		{Model: "b", Category: "cat2", Dataset: "ds", PassRate: 0.9},
		{Model: "c", Category: "cat1", Dataset: "ds", PassRate: 0.8},
	} {
		if err := s.Upsert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := s.Ranking(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recs))
	}
}

func TestCategories(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, r := range []PerfRecord{
		{Model: "a", Category: "cat1", Dataset: "ds", PassRate: 0.5},
		{Model: "b", Category: "cat2", Dataset: "ds", PassRate: 0.9},
	} {
		if err := s.Upsert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	cats, err := s.Categories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}
}

func TestDetectCategory(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"write a unit test for the handler", "test-generation"},
		{"fix this bug in the auth module", "debugging"},
		{"design the API architecture", "planning"},
		{"rename the function", "refactoring"},
		{"audit this repo", "review"},
		{"write a README", "documentation"},
		{"scan for vulnerabilities", "security"},
		{"implement a function", "code-generation"},
	}
	for _, tt := range tests {
		got := DetectCategory(tt.input)
		if got != tt.want {
			t.Errorf("DetectCategory(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUpsertValidation(t *testing.T) {
	s := testStore(t)
	err := s.Upsert(context.Background(), PerfRecord{})
	if err == nil {
		t.Fatal("expected error for empty record")
	}
}
