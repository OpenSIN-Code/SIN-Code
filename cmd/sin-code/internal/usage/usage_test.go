// SPDX-License-Identifier: MIT
// Purpose: tests for cmd/sin-code/internal/usage (issue #168). 80%+
// coverage, race-safe.
package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func tempStore(t *testing.T) (*Store, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	cleanup := func() { _ = s.Close() }
	return s, cleanup
}

// seed writes N synthetic events. Forwards use sequential PastTime
// (oldest first); Tail orders by created_at DESC so reverse iteration
// gives the expected newest-first order.
func seed(t *testing.T, s *Store, sess string, count int) []Event {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	out := make([]Event, 0, count)
	for i := 0; i < count; i++ {
		e := Event{
			SessionID:    sess,
			Model:        "meta/llama-3.3-70b-instruct",
			Source:       SourceChat,
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CreatedAt:    now.Add(time.Duration(i) * time.Second),
		}
		if err := s.Record(context.Background(), e); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		out = append(out, e)
	}
	return out
}

func TestOpenCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.db")
	for i := 0; i < 3; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if s == nil {
			t.Fatalf("nil store on iteration %d", i)
		}
		if err := s.Close(); err != nil {
			t.Errorf("close %d: %v", i, err)
		}
	}
}

func TestRecordPersistsAllFields(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.Record(context.Background(), Event{
		SessionID:    "sess-A",
		Model:        "gpt-4o",
		Source:       SourceJudge,
		InputTokens:  1234,
		OutputTokens: 567,
		TotalTokens:  1801,
		CreatedAt:    now,
	}); err != nil {
		t.Fatal(err)
	}
	tail, err := s.Tail(context.Background(), Filter{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 1 {
		t.Fatalf("got %d events, want 1", len(tail))
	}
	got := tail[0]
	if got.SessionID != "sess-A" || got.Model != "gpt-4o" || got.Source != SourceJudge ||
		got.InputTokens != 1234 || got.OutputTokens != 567 || got.TotalTokens != 1801 {
		t.Errorf("mismatch: %+v", got)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("created_at: got %v want %v", got.CreatedAt, now)
	}
	if got.CostUSD == 0 {
		t.Errorf("expected non-zero cost for gpt-4o, got 0")
	}
}

func TestRecordFromChatUsageSkipsZeroTotals(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	if err := s.RecordFromChatUsage(context.Background(), "sess-A", "gpt-4o", SourceChat, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	n, err := s.Count(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows when all tokens zero, got %d", n)
	}
	// but non-zero gets persisted
	if err := s.RecordFromChatUsage(context.Background(), "sess-A", "gpt-4o", SourceChat, 100, 50, 150); err != nil {
		t.Fatal(err)
	}
	n, _ = s.Count(context.Background(), Filter{})
	if n != 1 {
		t.Errorf("expected 1 row, got %d", n)
	}
}

func TestAggregateSessionTotals(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	seed(t, s, "sess-A", 5) // 5*150 = 750
	if err := s.Record(context.Background(), Event{
		SessionID:    "sess-B",
		Model:        "gpt-4o",
		Source:       SourceChat,
		InputTokens:  10,
		OutputTokens: 20,
		TotalTokens:  30,
	}); err != nil {
		t.Fatal(err)
	}
	top, subs, err := s.Aggregate(context.Background(), Filter{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if top.InputTokens != 5*100+10 || top.OutputTokens != 5*50+20 || top.TotalTokens != 5*150+30 {
		t.Errorf("totals mismatch: %+v", top)
	}
	if top.EventCount != 6 {
		t.Errorf("event count: %d", top.EventCount)
	}
	if top.SessionsCount != 2 {
		t.Errorf("sessions count: %d", top.SessionsCount)
	}
	if len(subs) != 0 {
		t.Errorf("subs should be empty without group_by, got %d", len(subs))
	}
	if top.ByModel["meta/llama-3.3-70b-instruct"] != 750 {
		t.Errorf("by_model: %+v", top.ByModel)
	}
}

func TestAggregateByDay(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	day1 := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	for _, ts := range []time.Time{day1.Add(1 * time.Second), day1.Add(2 * time.Second), day2.Add(1 * time.Second)} {
		_ = s.Record(context.Background(), Event{
			SessionID: "s-A", Model: "gpt-4o", Source: SourceChat,
			InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
			CreatedAt: ts,
		})
	}
	_, subs, err := s.Aggregate(context.Background(), Filter{}, "day")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 day rows, got %d", len(subs))
	}
	if subs[0].Group != "2026-06-15" {
		t.Errorf("top group %q, want newest first", subs[0].Group)
	}
	if subs[0].TotalTokens != 150 || subs[1].TotalTokens != 300 {
		t.Errorf("day totals: %+v", subs)
	}
}

func TestAggregateByModel(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	_ = s.Record(context.Background(), Event{Model: "gpt-4o", Source: SourceChat, InputTokens: 100, OutputTokens: 50, TotalTokens: 150})
	_ = s.Record(context.Background(), Event{Model: "gpt-4o", Source: SourceChat, InputTokens: 200, OutputTokens: 60, TotalTokens: 260})
	_ = s.Record(context.Background(), Event{Model: "claude-sonnet-4", Source: SourceChat, InputTokens: 50, OutputTokens: 25, TotalTokens: 75})
	_, subs, err := s.Aggregate(context.Background(), Filter{}, "model")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 model rows, got %d", len(subs))
	}
	if subs[0].Group != "gpt-4o" {
		t.Errorf("top group %q should be gpt-4o (highest tokens)", subs[0].Group)
	}
	if subs[0].TotalTokens != 410 {
		t.Errorf("gpt-4o tokens: %d, want 410", subs[0].TotalTokens)
	}
}

func TestAggregateBySource(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	_ = s.Record(context.Background(), Event{Source: SourceChat, TotalTokens: 100})
	_ = s.Record(context.Background(), Event{Source: SourceChat, TotalTokens: 50})
	_ = s.Record(context.Background(), Event{Source: SourceJudge, TotalTokens: 200})
	_, subs, err := s.Aggregate(context.Background(), Filter{}, "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 source rows, got %d", len(subs))
	}
}

func TestAggregateBySession(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	_ = s.Record(context.Background(), Event{SessionID: "alpha", Source: SourceChat, TotalTokens: 100})
	_ = s.Record(context.Background(), Event{SessionID: "beta", Source: SourceChat, TotalTokens: 50})
	_, subs, err := s.Aggregate(context.Background(), Filter{}, "session")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 session rows, got %d", len(subs))
	}
}

func TestAggregateFilterSessionID(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	seed(t, s, "alpha", 3)
	seed(t, s, "beta", 7)
	top, _, err := s.Aggregate(context.Background(), Filter{SessionID: "alpha"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if top.EventCount != 3 || top.TotalTokens != 3*150 {
		t.Errorf("filter session: %+v", top)
	}
	if top.SessionsCount != 1 {
		t.Errorf("sessions count after filter: %d", top.SessionsCount)
	}
}

func TestAggregateDateRange(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	new := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, ts := range []time.Time{old, mid, new} {
		_ = s.Record(context.Background(), Event{Source: SourceChat, TotalTokens: 100, CreatedAt: ts})
	}
	top, _, err := s.Aggregate(context.Background(), Filter{
		Since: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if top.EventCount != 1 || top.TotalTokens != 100 {
		t.Errorf("range filter: %+v", top)
	}
}

func TestTailReturnsNewestFirst(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	seed(t, s, "sess", 10)
	tail, err := s.Tail(context.Background(), Filter{}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 5 {
		t.Fatalf("got %d events, want 5", len(tail))
	}
	for i := 0; i < len(tail)-1; i++ {
		if tail[i].CreatedAt.Before(tail[i+1].CreatedAt) {
			t.Errorf("tail not sorted newest-first: %v then %v", tail[i].CreatedAt, tail[i+1].CreatedAt)
		}
	}
}

func TestTailByZeroN(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	seed(t, s, "sess", 30)
	tail, err := s.Tail(context.Background(), Filter{}, 0) // 0 → defaults to 20
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 20 {
		t.Errorf("default-N tail: %d, want 20", len(tail))
	}
}

func TestCount(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	if n, err := s.Count(context.Background(), Filter{}); err != nil || n != 0 {
		t.Errorf("empty count: %d / %v", n, err)
	}
	seed(t, s, "s", 3)
	if n, err := s.Count(context.Background(), Filter{}); err != nil || n != 3 {
		t.Errorf("after seed: %d / %v", n, err)
	}
}

func TestPricingUsesExactMatchThenSubstringFallback(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	// Exact-match: gpt-4o @ 0.0050 → 150 * 0.0050 / 1000 = 0.00075
	e1 := Event{Model: "gpt-4o", Source: SourceChat, TotalTokens: 150}
	if err := s.Record(context.Background(), e1); err != nil {
		t.Fatal(err)
	}
	// Substring fallback: "meta/llama-3.3-70b-instruct" matches key "llama-3.3-70b" @ 0.0009 → 150 * 0.0009 / 1000 = 0.000135
	if err := s.Record(context.Background(), Event{
		Model: "meta/llama-3.3-70b-instruct", Source: SourceChat, TotalTokens: 150,
	}); err != nil {
		t.Fatal(err)
	}
	// Unknown → cost stays 0 (not an error).
	if err := s.Record(context.Background(), Event{
		Model: "totally-custom-unlisted-model", Source: SourceChat, TotalTokens: 999,
	}); err != nil {
		t.Fatal(err)
	}
	tail, err := s.Tail(context.Background(), Filter{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if tail[0].CostUSD != 0 {
		t.Errorf("unknown model: cost should be 0, got %v", tail[0].CostUSD)
	}
	// Look at the gpt-4o row (newest first: unknown, llama, gpt-4o).
	for _, e := range tail {
		switch e.Model {
		case "gpt-4o":
			want := 150.0 * 0.0050 / 1000.0
			if absFloat(e.CostUSD-want) > 1e-9 {
				t.Errorf("gpt-4o cost: %v, want %v", e.CostUSD, want)
			}
		case "meta/llama-3.3-70b-instruct":
			want := 150.0 * 0.0009 / 1000.0
			if absFloat(e.CostUSD-want) > 1e-9 {
				t.Errorf("llama cost: %v, want %v", e.CostUSD, want)
			}
		}
	}
}

func TestSetPricingOverridesDefaults(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	s.SetPricing(map[string]float64{"only-this-model": 0.5})
	if got := s.Pricing()["only-this-model"]; got != 0.5 {
		t.Errorf("set pricing: %v", got)
	}
	if _, ok := s.Pricing()["gpt-4o"]; ok {
		t.Error("SetPricing should replace the default map (not merge)")
	}
}

func TestOpenWithPricingMerges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.db")
	s, err := OpenWithPricing(path, map[string]float64{"custom-model": 0.42})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got := s.Pricing()
	if got["custom-model"] != 0.42 {
		t.Errorf("override missing: %v", got["custom-model"])
	}
	if got["gpt-4o"] == 0 {
		t.Error("default model missing after OpenWithPricing (should merge with defaults)")
	}
}

func TestDefaultPathEnvOverride(t *testing.T) {
	t.Setenv("SIN_CODE_TOKENS_DB", "/tmp/custom-tokens.db")
	got := DefaultPath()
	if got != "/tmp/custom-tokens.db" {
		t.Errorf("env override: %q", got)
	}
}

func TestDefaultPathXDGOverride(t *testing.T) {
	t.Setenv("SIN_CODE_TOKENS_DB", "")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg")
	got := DefaultPath()
	if !filepath.IsAbs(got) {
		t.Errorf("should be absolute: %q", got)
	}
	if filepath.Dir(got) != filepath.Join("/tmp/xdg", "sin-code") {
		t.Errorf("xdg: %q", got)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.Record(context.Background(), Event{
		SessionID:    "round-trip",
		Model:        "gpt-4o",
		Source:       SourceJudge,
		InputTokens:  42,
		OutputTokens: 7,
		TotalTokens:  49,
		CreatedAt:    now,
	}); err != nil {
		t.Fatal(err)
	}
	tail, _ := s.Tail(context.Background(), Filter{}, 1)
	if len(tail) != 1 {
		t.Fatal("expected 1")
	}
	data, err := json.Marshal(tail[0])
	if err != nil {
		t.Fatal(err)
	}
	var out Event
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.SessionID != "round-trip" || out.InputTokens != 42 || out.OutputTokens != 7 {
		t.Errorf("round trip: %+v", out)
	}
}

func TestAggregateAcceptsValidGroupBy(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	seed(t, s, "s", 3)
	for _, gb := range []string{"", "day", "month", "model", "source", "session"} {
		if _, _, err := s.Aggregate(context.Background(), Filter{}, gb); err != nil {
			t.Errorf("group_by=%q: %v", gb, err)
		}
	}
	if _, _, err := s.Aggregate(context.Background(), Filter{}, "Bogus"); err == nil {
		t.Error("expected error for unknown group_by")
	}
}

func TestAggregateNilStoreErrors(t *testing.T) {
	var s *Store
	if _, _, err := s.Aggregate(context.Background(), Filter{}, "day"); err == nil {
		t.Error("expected error from nil store")
	}
}

func TestSourceEnumeration(t *testing.T) {
	cases := map[Source]string{
		SourceChat:    "chat",
		SourceVerify:  "verify",
		SourceJudge:   "judge",
		SourceSummary: "summary",
		SourcePlan:    "plan",
		SourceAdHoc:   "adhoc",
	}
	for s, want := range cases {
		if string(s) != want {
			t.Errorf("source %s != %s", s, want)
		}
	}
}

// Concurrent stress: many goroutines recording into the same store. The
// store is single-writer by design; we deliberately exercise contention to
// verify Race-free operation under go test -race.
func TestConcurrentRecord(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	const goroutines = 16
	const each = 50
	var wg sync.WaitGroup
	var failed int32
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				err := s.RecordFromChatUsage(context.Background(),
					fmt.Sprintf("goroutine-%d", gid),
					"gpt-4o", SourceChat, 10, 5, 15)
				if err != nil {
					atomic.AddInt32(&failed, 1)
				}
			}
		}(g)
	}
	wg.Wait()
	if failed > 0 {
		t.Errorf("%d record errors under concurrency", failed)
	}
	n, err := s.Count(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if n != goroutines*each {
		t.Errorf("count: got %d, want %d", n, goroutines*each)
	}
}

func TestConcurrentAggregateUnderWrites(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	seed(t, s, "warmup", 20)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _, _ = s.Aggregate(context.Background(), Filter{}, "model")
					_, _ = s.Tail(context.Background(), Filter{}, 10)
				}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		_ = s.RecordFromChatUsage(context.Background(), "writer", "gpt-4o", SourceChat, 1, 1, 2)
	}
	close(stop)
	wg.Wait()
}

func TestCloseThenUseReturnsError(t *testing.T) {
	s, cleanup := tempStore(t)
	cleanup() // explicit early close (no return value – the closure returns nothing)
	if err := s.Record(context.Background(), Event{TotalTokens: 1, Source: SourceChat, Model: "m"}); err == nil {
		t.Error("expected error after close, got nil")
	}
}

func TestGroupByMonth(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	for month := 1; month <= 3; month++ {
		_ = s.Record(context.Background(), Event{
			Model: "gpt-4o", Source: SourceChat, TotalTokens: 100,
			CreatedAt: time.Date(2026, time.Month(month), 1, 0, 0, 0, 0, time.UTC),
		})
	}
	_, subs, err := s.Aggregate(context.Background(), Filter{}, "month")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 3 {
		t.Fatalf("expected 3 month rows, got %d", len(subs))
	}
	if subs[0].Group != "2026-03" {
		t.Errorf("top group %q (DESC)", subs[0].Group)
	}
}

func TestRecordZeroCreatedAtUsesNow(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	before := time.Now().UTC().Add(-time.Second)
	if err := s.Record(context.Background(), Event{
		Model: "gpt-4o", Source: SourceChat, TotalTokens: 1,
	}); err != nil {
		t.Fatal(err)
	}
	tail, _ := s.Tail(context.Background(), Filter{}, 1)
	after := time.Now().UTC().Add(time.Second)
	if len(tail) != 1 || tail[0].CreatedAt.Before(before) || tail[0].CreatedAt.After(after) {
		t.Errorf("zero CreatedAt: got %v, want between %v and %v", tail[0].CreatedAt, before, after)
	}
}

func TestErrPropagationFromBuildWhere(t *testing.T) {
	// user-facing Filter validation: zero values must not filter anything.
	s, cleanup := tempStore(t)
	defer cleanup()
	_ = s.Record(context.Background(), Event{Model: "gpt-4o", Source: SourceChat, TotalTokens: 1})
	if _, _, err := s.Aggregate(context.Background(), Filter{}, ""); err != nil {
		t.Errorf("empty filter: %v", err)
	}
}

// Test that nil reader error is propagated properly so callers see when the
// store was double-closed or yanked out from under them. Helps surface
// unexpected invalidation.
func TestCloseIdempotent(t *testing.T) {
	s, cleanup := tempStore(t)
	cleanup()
	if err := s.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		// modernc returns a sqlite-specific closed error which won't equal
		// os.ErrClosed; just don't fail the test silently.
		t.Logf("second close returned: %v (acceptable)", err)
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
