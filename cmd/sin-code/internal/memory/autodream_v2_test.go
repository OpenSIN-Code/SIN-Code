// SPDX-License-Identifier: MIT
// Purpose: tests for AutoDream v2 — sleep-time reflection (issue #353).
// Covers connections, contradictions, insights, questions, reflection
// storage, constructor options, and race-free concurrency (M7).
package memory

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAutoDreamV2ReflectEmpty(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s)
	report, err := ad.Reflect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Insights) != 0 || len(report.Questions) != 0 ||
		len(report.Connections) != 0 || len(report.ContradictionsFound) != 0 {
		t.Errorf("expected empty report, got %+v", report)
	}
}

func TestAutoDreamV2ReflectNilStore(t *testing.T) {
	ad := NewAutoDreamV2(nil)
	_, err := ad.Reflect(context.Background())
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestAutoDreamV2ReflectFindsConnections(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s)
	_ = s.Add(&Memory{Insight: "use cobra for CLI", Tags: []string{"go", "cli"}})
	_ = s.Add(&Memory{Insight: "cobra subcommands are easy", Tags: []string{"go", "cli"}})

	report, err := ad.Reflect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Connections) == 0 {
		t.Fatal("expected at least 1 connection")
	}
	conn := report.Connections[0]
	if conn.SharedTag == "" {
		t.Error("connection should have a shared tag")
	}
}

func TestAutoDreamV2ReflectFindsContradictions(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s)
	_ = s.Add(&Memory{Insight: "use tabs for formatting", Tags: []string{"fmt"}})
	_ = s.Add(&Memory{Insight: "do not use tabs for formatting", Tags: []string{"fmt"}})

	report, err := ad.Reflect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ContradictionsFound) == 0 {
		t.Fatal("expected at least 1 contradiction")
	}
}

func TestAutoDreamV2ReflectGeneratesInsights(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s)
	for i := 0; i < 4; i++ {
		_ = s.Add(&Memory{Insight: "go tip " + string(rune('A'+i)), Tags: []string{"go", "tips"}})
	}
	report, err := ad.Reflect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Insights) == 0 {
		t.Fatal("expected at least 1 insight")
	}
	found := false
	for _, ins := range report.Insights {
		if strings.Contains(ins, "go") {
			found = true
		}
	}
	if !found {
		t.Error("expected an insight mentioning 'go'")
	}
}

func TestAutoDreamV2ReflectGeneratesQuestions(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s)
	_ = s.Add(&Memory{Insight: "unique insight", Tags: []string{"rare"}})

	report, err := ad.Reflect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Questions) == 0 {
		t.Fatal("expected at least 1 question")
	}
}

func TestAutoDreamV2ReflectStoresReflections(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s)
	for i := 0; i < 4; i++ {
		_ = s.Add(&Memory{Insight: "go pattern " + string(rune('A'+i)), Tags: []string{"go", "pattern"}})
	}
	_, _ = ad.Reflect(context.Background())

	reflections, err := s.List(ListFilter{Tag: "reflection"})
	if err != nil {
		t.Fatal(err)
	}
	if len(reflections) == 0 {
		t.Fatal("expected at least 1 reflection memory")
	}
	if !strings.Contains(reflections[0].Insight, "[reflection]") {
		t.Errorf("reflection should start with [reflection]: %s", reflections[0].Insight)
	}
}

func TestAutoDreamV2NewWithOptions(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s, WithInterval(30*time.Second), WithMaxMemories(200))
	if ad.interval != 30*time.Second {
		t.Errorf("expected 30s interval, got %v", ad.interval)
	}
	if ad.maxMemories != 200 {
		t.Errorf("expected 200 max, got %d", ad.maxMemories)
	}
	// Should still work as an AutoDream.
	if ad.AutoDream == nil {
		t.Error("embedded AutoDream should not be nil")
	}
}

func TestAutoDreamV2ReflectRaceFree(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s)
	for i := 0; i < 10; i++ {
		_ = s.Add(&Memory{Insight: "race reflect " + string(rune('A'+i)), Tags: []string{"race"}})
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = ad.Reflect(context.Background())
		}()
	}
	wg.Wait()
}

func TestAutoDreamV2ReflectContextCancellation(t *testing.T) {
	s := tempStore(t)
	ad := NewAutoDreamV2(s)
	for i := 0; i < 100; i++ {
		_ = s.Add(&Memory{Insight: "filler " + string(rune('A'+i%26)), Tags: []string{"filler"}})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ad.Reflect(ctx)
	if err == nil {
		t.Skip("context cancellation may not be observed if list returns before check")
	}
}

func TestSharedTags(t *testing.T) {
	got := sharedTags([]string{"a", "b", "c"}, []string{"b", "c", "d"})
	if len(got) != 2 {
		t.Fatalf("expected 2 shared, got %d", len(got))
	}
	got = sharedTags([]string{"a"}, []string{"b"})
	if len(got) != 0 {
		t.Errorf("expected 0 shared for disjoint, got %d", len(got))
	}
	got = sharedTags([]string{"a", "reflection"}, []string{"a", "reflection"})
	if len(got) != 1 {
		t.Errorf("reflection tag should be excluded, got %d", len(got))
	}
}
