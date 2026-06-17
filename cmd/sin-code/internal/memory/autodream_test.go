// SPDX-License-Identifier: MIT
// Purpose: tests for autoDream — background memory consolidation.
// Covers all 5 strategies, lifecycle, context cancellation, stats,
// and race-free concurrent access (mandate M7).
package memory

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func dreamStore(t *testing.T) *Store {
	t.Helper()
	return tempStore(t)
}

func TestAutoDreamDedupeMerges(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	m1 := &Memory{
		Insight:    "always use go modules for dependency management",
		Tags:       []string{"go", "deps"},
		Importance: 0.5,
	}
	m2 := &Memory{
		Insight:    "always use go modules for dependency management too",
		Tags:       []string{"go", "deps"},
		Importance: 0.3,
	}
	if err := s.Add(m1); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(m2); err != nil {
		t.Fatal(err)
	}

	all, _ := s.List(ListFilter{})
	merged := ad.dedupe(context.Background(), all)
	if merged != 1 {
		t.Fatalf("expected 1 dedupe, got %d", merged)
	}

	remaining, _ := s.List(ListFilter{})
	count := 0
	for _, m := range remaining {
		if m.Insight != "[forgotten]" && len(m.Insight) > 0 {
			if !containsStr(m.Tags, "autodream-summary") {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("expected 1 non-summary memory remaining, got %d", count)
	}

	var keeper *Memory
	for _, m := range remaining {
		if m.ID == m1.ID {
			keeper = m
		}
	}
	if keeper == nil {
		for _, m := range remaining {
			if m.ID == m2.ID {
				keeper = m
			}
		}
	}
	if keeper == nil {
		t.Fatal("neither original memory found after dedupe")
	}
	if keeper.Importance < 0.5 {
		t.Errorf("keeper should have max importance, got %f", keeper.Importance)
	}
}

func TestAutoDreamDedupeKeepsDifferent(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	m1 := &Memory{Insight: "use cobra for CLI commands", Tags: []string{"go"}}
	m2 := &Memory{Insight: "prefer gin for HTTP servers", Tags: []string{"go"}}
	_ = s.Add(m1)
	_ = s.Add(m2)

	all, _ := s.List(ListFilter{})
	merged := ad.dedupe(context.Background(), all)
	if merged != 0 {
		t.Fatalf("expected 0 dedupes for different memories, got %d", merged)
	}

	remaining, _ := s.List(ListFilter{})
	if len(remaining) != 2 {
		t.Errorf("expected 2 memories, got %d", len(remaining))
	}
}

func TestAutoDreamContradictionDetection(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	m1 := &Memory{Insight: "use tabs for indentation in go", Tags: []string{"formatting"}}
	m2 := &Memory{Insight: "do not use tabs for indentation in go", Tags: []string{"formatting"}}
	_ = s.Add(m1)
	_ = s.Add(m2)

	all, _ := s.List(ListFilter{})
	found := ad.detectContradictions(context.Background(), all)
	if found != 1 {
		t.Fatalf("expected 1 contradiction, got %d", found)
	}

	links, _ := s.GetLinks(m1.ID)
	hasContradicts := false
	for _, l := range links {
		if l.Rel == string(LinkContradicts) {
			hasContradicts = true
		}
	}
	if !hasContradicts {
		t.Error("expected contradicts link between m1 and m2")
	}
}

func TestAutoDreamSummarization(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	for i := 0; i < summarizeGroupThreshold; i++ {
		_ = s.Add(&Memory{
			Insight: "go tip number " + string(rune('A'+i)),
			Tags:    []string{"go-tips"},
		})
	}

	all, _ := s.List(ListFilter{})
	created := ad.summarize(context.Background(), all)
	if created != 1 {
		t.Fatalf("expected 1 summary, got %d", created)
	}

	summaries, _ := s.List(ListFilter{Tag: "autodream-summary"})
	if len(summaries) != 1 {
		t.Errorf("expected 1 summary memory, got %d", len(summaries))
	}
	if summaries[0].Importance != 0.5 {
		t.Errorf("summary importance should be 0.5, got %f", summaries[0].Importance)
	}
}

func TestAutoDreamSummarizationSkipsSmallGroups(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	for i := 0; i < summarizeGroupThreshold-1; i++ {
		_ = s.Add(&Memory{Insight: "small tip", Tags: []string{"small"}})
	}

	all, _ := s.List(ListFilter{})
	created := ad.summarize(context.Background(), all)
	if created != 0 {
		t.Fatalf("expected 0 summaries for small group, got %d", created)
	}
}

func TestAutoDreamDecayReducesImportance(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	m := &Memory{
		Insight:    "old stale memory about something",
		Tags:       []string{"stale"},
		Importance: 1.0,
		Created:    old,
	}
	_ = s.Add(m)
	m.Created = old
	_ = s.Update(m)

	all, _ := s.List(ListFilter{})
	decayed := ad.decay(context.Background(), all)
	if decayed != 1 {
		t.Fatalf("expected 1 decay, got %d", decayed)
	}

	updated, _ := s.Get(m.ID)
	if updated.Importance >= 1.0 {
		t.Errorf("importance should be reduced, got %f", updated.Importance)
	}
	expected := 1.0 * decayFactor
	if updated.Importance < expected-0.01 || updated.Importance > expected+0.01 {
		t.Errorf("expected ~%f, got %f", expected, updated.Importance)
	}
}

func TestAutoDreamDecaySkipsRecentlyAccessed(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-1 * time.Hour)
	m := &Memory{
		Insight:     "old but recently accessed memory",
		Tags:        []string{"accessed"},
		Importance:  1.0,
		Created:     old,
		LastAccessed: recent,
	}
	_ = s.Add(m)
	m.Created = old
	m.LastAccessed = recent
	_ = s.Update(m)

	all, _ := s.List(ListFilter{})
	decayed := ad.decay(context.Background(), all)
	if decayed != 0 {
		t.Fatalf("expected 0 decays for recently accessed, got %d", decayed)
	}

	updated, _ := s.Get(m.ID)
	if updated.Importance != 1.0 {
		t.Errorf("importance should be unchanged, got %f", updated.Importance)
	}
}

func TestAutoDreamDecaySkipsRecentMemories(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	m := &Memory{
		Insight:    "fresh memory",
		Tags:       []string{"fresh"},
		Importance: 1.0,
	}
	_ = s.Add(m)

	all, _ := s.List(ListFilter{})
	decayed := ad.decay(context.Background(), all)
	if decayed != 0 {
		t.Fatalf("expected 0 decays for fresh memory, got %d", decayed)
	}
}

func TestAutoDreamDecaySkipsZeroImportance(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	m := &Memory{
		Insight:    "old memory with no importance",
		Tags:       []string{"zero"},
		Importance: 0,
		Created:    old,
	}
	_ = s.Add(m)
	m.Created = old
	_ = s.Update(m)

	all, _ := s.List(ListFilter{})
	decayed := ad.decay(context.Background(), all)
	if decayed != 0 {
		t.Fatalf("expected 0 decays for zero importance, got %d", decayed)
	}
}

func TestAutoDreamPromoteFrequent(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	m := &Memory{
		Insight:     "frequently accessed memory",
		Tags:        []string{"popular"},
		Importance:  0.2,
		AccessCount: 5,
	}
	_ = s.Add(m)
	m.AccessCount = 5
	_ = s.Update(m)

	all, _ := s.List(ListFilter{})
	promoted := ad.promote(context.Background(), all)
	if promoted != 1 {
		t.Fatalf("expected 1 promotion, got %d", promoted)
	}

	updated, _ := s.Get(m.ID)
	if updated.Importance <= 0.2 {
		t.Errorf("importance should increase, got %f", updated.Importance)
	}
	expected := 0.2 + promoteBoost
	if updated.Importance < expected-0.01 || updated.Importance > expected+0.01 {
		t.Errorf("expected ~%f, got %f", expected, updated.Importance)
	}
}

func TestAutoDreamPromoteSkipsInfrequent(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	m := &Memory{
		Insight:     "rarely accessed memory",
		Tags:        []string{"rare"},
		Importance:  0.2,
		AccessCount: 1,
	}
	_ = s.Add(m)

	all, _ := s.List(ListFilter{})
	promoted := ad.promote(context.Background(), all)
	if promoted != 0 {
		t.Fatalf("expected 0 promotions for infrequent, got %d", promoted)
	}
}

func TestAutoDreamRunOnceFullPipeline(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	dup1 := &Memory{Insight: "use context for cancellation in go", Tags: []string{"go"}, Importance: 0.5}
	dup2 := &Memory{Insight: "use context for cancellation in go too", Tags: []string{"go"}, Importance: 0.3}
	_ = s.Add(dup1)
	_ = s.Add(dup2)

	contra1 := &Memory{Insight: "tabs are the best for go formatting", Tags: []string{"fmt"}}
	contra2 := &Memory{Insight: "never use tabs for go formatting", Tags: []string{"fmt"}}
	_ = s.Add(contra1)
	_ = s.Add(contra2)

	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	stale := &Memory{Insight: "stale old memory", Tags: []string{"old"}, Importance: 1.0, Created: old}
	_ = s.Add(stale)
	stale.Created = old
	_ = s.Update(stale)

	popular := &Memory{Insight: "popular memory", Tags: []string{"pop"}, Importance: 0.1, AccessCount: 5}
	_ = s.Add(popular)
	popular.AccessCount = 5
	_ = s.Update(popular)

	report, err := ad.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if report.Deduped < 1 {
		t.Errorf("expected >=1 dedupe, got %d", report.Deduped)
	}
	if report.Contradictions < 1 {
		t.Errorf("expected >=1 contradiction, got %d", report.Contradictions)
	}
	if report.Decayed < 1 {
		t.Errorf("expected >=1 decay, got %d", report.Decayed)
	}
	if report.Promoted < 1 {
		t.Errorf("expected >=1 promotion, got %d", report.Promoted)
	}
	if report.Duration <= 0 {
		t.Error("duration should be positive")
	}
}

func TestAutoDreamRunOnceEmptyStore(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	report, err := ad.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce on empty: %v", err)
	}
	if report.Deduped != 0 || report.Contradictions != 0 || report.Summarized != 0 ||
		report.Decayed != 0 || report.Promoted != 0 {
		t.Errorf("expected all zeros on empty store, got %+v", report)
	}
}

func TestAutoDreamNilStore(t *testing.T) {
	ad := NewAutoDream(nil)
	_, err := ad.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

func TestAutoDreamOptions(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s,
		WithInterval(10*time.Second),
		WithMaxMemories(500),
	)
	if ad.interval != 10*time.Second {
		t.Errorf("expected 10s interval, got %v", ad.interval)
	}
	if ad.maxMemories != 500 {
		t.Errorf("expected 500 max, got %d", ad.maxMemories)
	}
}

func TestAutoDreamOptionsIgnoreInvalid(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s,
		WithInterval(0),
		WithMaxMemories(-1),
	)
	if ad.interval != defaultDreamInterval {
		t.Errorf("expected default interval, got %v", ad.interval)
	}
	if ad.maxMemories != defaultMaxMemories {
		t.Errorf("expected default max, got %d", ad.maxMemories)
	}
}

func TestAutoDreamStartStop(t *testing.T) {
	s := dreamStore(t)
	_ = s.Add(&Memory{Insight: "test", Tags: []string{"t"}, Importance: 1.0})

	ad := NewAutoDream(s, WithInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ad.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	ad.Stop()

	stats := ad.Stats()
	if stats.TotalRuns == 0 {
		t.Error("expected at least 1 run after start+sleep+stop")
	}
}

func TestAutoDreamStartIdempotent(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s, WithInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ad.Start(ctx)
	ad.Start(ctx)
	ad.Stop()
}

func TestAutoDreamStopIdempotent(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)
	ad.Stop()
	ad.Stop()
}

func TestAutoDreamContextCancellation(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	for i := 0; i < 100; i++ {
		_ = s.Add(&Memory{Insight: "filler memory number " + string(rune('A'+i%26)) + string(rune('A'+i/26)), Tags: []string{"filler"}})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ad.RunOnce(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestAutoDreamStatsTracking(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	_ = s.Add(&Memory{Insight: "use context for cancellation in go", Tags: []string{"a"}, Importance: 0.5})
	_ = s.Add(&Memory{Insight: "use context for cancellation in go too", Tags: []string{"a"}, Importance: 0.3})

	r1, _ := ad.RunOnce(context.Background())
	r2, _ := ad.RunOnce(context.Background())

	stats := ad.Stats()
	if stats.TotalRuns != 2 {
		t.Errorf("expected 2 total runs, got %d", stats.TotalRuns)
	}
	if stats.TotalDeduped != r1.Deduped+r2.Deduped {
		t.Errorf("total deduped mismatch: stats=%d, r1+r2=%d", stats.TotalDeduped, r1.Deduped+r2.Deduped)
	}
	if stats.LastRun.IsZero() {
		t.Error("last run should be set")
	}
}

func TestAutoDreamRaceFree(t *testing.T) {
	s := dreamStore(t)
	ad := NewAutoDream(s)

	for i := 0; i < 20; i++ {
		_ = s.Add(&Memory{
			Insight:    "race test memory " + string(rune('A'+i)),
			Tags:       []string{"race"},
			Importance: 0.5,
		})
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = ad.RunOnce(context.Background())
		}()
		go func() {
			defer wg.Done()
			_ = ad.Stats()
		}()
	}
	wg.Wait()
}

func TestJaccardSimilarity(t *testing.T) {
	if s := jaccardSimilarity("hello world", "hello world"); s != 1.0 {
		t.Errorf("identical: got %f", s)
	}
	if s := jaccardSimilarity("hello world", "goodbye world"); s <= 0 || s >= 1.0 {
		t.Errorf("partial: got %f", s)
	}
	if s := jaccardSimilarity("hello", "world"); s != 0 {
		t.Errorf("disjoint: got %f", s)
	}
	if s := jaccardSimilarity("", ""); s != 1.0 {
		t.Errorf("both empty: got %f", s)
	}
	if s := jaccardSimilarity("hello", ""); s != 0 {
		t.Errorf("one empty: got %f", s)
	}
}

func TestIsContradiction(t *testing.T) {
	if !isContradiction("use tabs for formatting", "do not use tabs for formatting") {
		t.Error("expected contradiction with negation")
	}
	if isContradiction("use tabs for formatting", "use spaces for formatting") {
		t.Error("expected no contradiction without negation asymmetry")
	}
	if isContradiction("hello world", "goodbye universe") {
		t.Error("expected no contradiction for disjoint content")
	}
}

func TestSameTags(t *testing.T) {
	if !sameTags([]string{"a", "b"}, []string{"a", "b"}) {
		t.Error("equal tags should match")
	}
	if sameTags([]string{"a"}, []string{"a", "b"}) {
		t.Error("different length should not match")
	}
	if sameTags([]string{"a", "b"}, []string{"a", "c"}) {
		t.Error("different tags should not match")
	}
	if !sameTags(nil, nil) {
		t.Error("both nil should match")
	}
}

func TestPickKeeper(t *testing.T) {
	a := &Memory{ID: "a", Importance: 0.5, Updated: time.Now()}
	b := &Memory{ID: "b", Importance: 0.3, Updated: time.Now().Add(-time.Hour)}
	keeper, dupe := pickKeeper(a, b)
	if keeper.ID != "a" {
		t.Errorf("higher importance should be keeper, got %s", keeper.ID)
	}
	if dupe.ID != "b" {
		t.Errorf("lower importance should be dupe, got %s", dupe.ID)
	}

	c := &Memory{ID: "c", Importance: 0.3, Updated: time.Now()}
	d := &Memory{ID: "d", Importance: 0.3, Updated: time.Now().Add(-time.Hour)}
	keeper, dupe = pickKeeper(c, d)
	if keeper.ID != "c" {
		t.Errorf("newer should be keeper when importance equal, got %s", keeper.ID)
	}
}

func TestStoreUpdate(t *testing.T) {
	s := dreamStore(t)
	m := &Memory{Insight: "original", Tags: []string{"test"}, Importance: 0.5}
	_ = s.Add(m)

	m.Importance = 0.9
	m.AccessCount = 3
	if err := s.Update(m); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(m.ID)
	if got.Importance != 0.9 {
		t.Errorf("importance not updated: %f", got.Importance)
	}
	if got.AccessCount != 3 {
		t.Errorf("access count not updated: %d", got.AccessCount)
	}
}

func TestStoreUpdateValidation(t *testing.T) {
	s := dreamStore(t)
	if err := s.Update(nil); err == nil {
		t.Error("expected error for nil")
	}
	if err := s.Update(&Memory{}); err == nil {
		t.Error("expected error for empty insight")
	}
	if err := s.Update(&Memory{Insight: "x"}); err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestStoreTouch(t *testing.T) {
	s := dreamStore(t)
	m := &Memory{Insight: "touchable", Tags: []string{"test"}}
	_ = s.Add(m)

	if err := s.Touch(m.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Touch(m.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(m.ID)
	if got.AccessCount != 2 {
		t.Errorf("expected access count 2, got %d", got.AccessCount)
	}
	if got.LastAccessed.IsZero() {
		t.Error("last accessed should be set")
	}
}

func TestStoreTouchNotFound(t *testing.T) {
	s := dreamStore(t)
	if err := s.Touch("nonexistent"); err == nil {
		t.Error("expected error for nonexistent ID")
	}
}

func TestDeterministicSummary(t *testing.T) {
	mems := []*Memory{
		{Insight: "first insight"},
		{Insight: "second insight"},
		{Insight: "third insight"},
	}
	summary := deterministicSummary("test", mems)
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if !strings.Contains(summary, "[autodream summary: test]") {
		t.Errorf("summary missing tag prefix: %s", summary)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 10) != "short" {
		t.Error("short string should be unchanged")
	}
	long := "this is a very long string that exceeds the limit"
	trunc := truncate(long, 10)
	if len(trunc) != 13 {
		t.Errorf("expected 13 chars (10+...), got %d", len(trunc))
	}
}

func TestWordSet(t *testing.T) {
	set := wordSet("Hello, World! Hello.")
	if len(set) != 2 {
		t.Errorf("expected 2 unique words, got %d", len(set))
	}
	if !set["hello"] {
		t.Error("expected 'hello' in set")
	}
	if !set["world"] {
		t.Error("expected 'world' in set")
	}
}

func TestGroupByTag(t *testing.T) {
	all := []*Memory{
		{ID: "1", Insight: "a", Tags: []string{"x"}},
		{ID: "2", Insight: "b", Tags: []string{"x"}},
		{ID: "3", Insight: "c", Tags: []string{"y"}},
		{ID: "4", Insight: "d", Tags: []string{"x", "autodream-summary"}},
	}
	groups := groupByTag(all)
	if len(groups["x"]) != 3 {
		t.Errorf("expected 3 in x, got %d", len(groups["x"]))
	}
	if len(groups["y"]) != 1 {
		t.Errorf("expected 1 in y, got %d", len(groups["y"]))
	}
	if _, ok := groups["autodream-summary"]; ok {
		t.Error("autodream-summary tag should be excluded")
	}
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
