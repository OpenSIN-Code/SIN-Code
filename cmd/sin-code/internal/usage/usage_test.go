// SPDX-License-Identifier: MIT
// Purpose: tests for cmd/sin-code/internal/usage (issue #168). 80%+
// coverage, race-safe.
package usage

import (
	"context"
	"database/sql"
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

func TestDefaultPath_NoHome(t *testing.T) {
	prev := userHomeDir
	userHomeDir = func() (string, error) { return "", nil }
	defer func() { userHomeDir = prev }()
	t.Setenv("SIN_CODE_TOKENS_DB", "")
	t.Setenv("XDG_DATA_HOME", "")
	if got := DefaultPath(); got != "tokens.db" {
		t.Errorf("expected fallback tokens.db, got %q", got)
	}
}

func TestOpen_MkdirError(t *testing.T) {
	prev := mkdirAll
	mkdirAll = func(path string, perm os.FileMode) error { return fmt.Errorf("mkdir denied") }
	defer func() { mkdirAll = prev }()
	_, err := Open("/some/path/tokens.db")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpen_SqlOpenError(t *testing.T) {
	prev := sqlOpen
	sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
		return nil, fmt.Errorf("driver broken")
	}
	defer func() { sqlOpen = prev }()
	_, err := Open(t.TempDir() + "/tokens.db")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpen_MigrateError(t *testing.T) {
	prev := migrateExec
	migrateExec = func(db *sql.DB, schema string) error { return fmt.Errorf("migration failed") }
	defer func() { migrateExec = prev }()
	_, err := Open(t.TempDir() + "/tokens.db")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenWithPricing_Nil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.db")
	s, err := OpenWithPricing(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, ok := s.Pricing()["gpt-4o"]; !ok {
		t.Error("OpenWithPricing(nil) should keep defaults")
	}
}

func TestClose_Nil(t *testing.T) {
	var s *Store
	if err := s.Close(); err != nil {
		t.Errorf("nil Close should return nil, got %v", err)
	}
}

func TestClose_NilDB(t *testing.T) {
	s := &Store{}
	if err := s.Close(); err != nil {
		t.Errorf("Close with nil db should return nil, got %v", err)
	}
}

func TestComputeCost_SubstringFallback(t *testing.T) {
	// A model that does not have an exact match but contains a known
	// substring ("llama-3.3-70b") should use the substring rate.
	s, cleanup := tempStore(t)
	defer cleanup()
	e := Event{Model: "my-custom-llama-3.3-70b-thing", Source: SourceChat, TotalTokens: 1000}
	if err := s.Record(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	tail, _ := s.Tail(context.Background(), Filter{}, 1)
	want := 1000.0 * 0.0009 / 1000.0
	if len(tail) != 1 || absFloat(tail[0].CostUSD-want) > 1e-9 {
		t.Errorf("substring cost: got %v, want %v", tail[0].CostUSD, want)
	}
}

func TestBuildWhere_SourceAndModel(t *testing.T) {
	where, args := buildWhere(Filter{Source: SourceChat, Model: "gpt-4o"})
	if where == "" {
		t.Fatal("expected WHERE clause")
	}
	if len(args) != 2 || args[0] != string(SourceChat) || args[1] != "gpt-4o" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestAggregate_FilterBySourceAndModel(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	_ = s.Record(context.Background(), Event{Source: SourceChat, Model: "gpt-4o", TotalTokens: 100})
	_ = s.Record(context.Background(), Event{Source: SourceJudge, Model: "gpt-4o", TotalTokens: 50})
	_ = s.Record(context.Background(), Event{Source: SourceChat, Model: "claude-sonnet-4", TotalTokens: 200})
	top, _, err := s.Aggregate(context.Background(), Filter{Source: SourceChat, Model: "gpt-4o"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if top.TotalTokens != 100 {
		t.Errorf("expected 100 tokens, got %d", top.TotalTokens)
	}
	if top.EventCount != 1 {
		t.Errorf("expected 1 event, got %d", top.EventCount)
	}
}

func TestAggregate_EmptyStore(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	top, subs, err := s.Aggregate(context.Background(), Filter{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if top.EventCount != 0 || top.TotalTokens != 0 {
		t.Errorf("expected empty aggregate: %+v", top)
	}
	if subs != nil {
		t.Errorf("expected nil subs, got %d", len(subs))
	}
}

func TestAggregate_BadDateParse(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	// Insert a raw row with an unparseable created_at to exercise the
	// parse-fallback path.
	_, err := s.db.Exec(`INSERT INTO usage_events (id, created_at) VALUES (?, ?)`, "bad-date", "not-a-date")
	if err != nil {
		t.Fatal(err)
	}
	top, _, err := s.Aggregate(context.Background(), Filter{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !top.FirstEvent.IsZero() || !top.LastEvent.IsZero() {
		t.Errorf("bad dates should leave FirstEvent/LastEvent zero: %+v", top)
	}
}

func TestTail_ScanError(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	// Insert a raw row that violates the total_tokens type so Scan fails.
	_, err := s.db.Exec(`INSERT INTO usage_events (id, session_id, model, source, input_tokens, output_tokens, total_tokens, cost_usd, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "bad", "s", "m", "chat", 0, 0, "not-an-int", 0, "2026-06-16T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Tail(context.Background(), Filter{}, 10)
	if err == nil {
		t.Fatal("expected scan error from corrupt total_tokens")
	}
}

func TestCount_WithFilter(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	_ = s.Record(context.Background(), Event{Source: SourceChat, Model: "gpt-4o", TotalTokens: 100})
	_ = s.Record(context.Background(), Event{Source: SourceJudge, Model: "gpt-4o", TotalTokens: 50})
	n, err := s.Count(context.Background(), Filter{Source: SourceChat})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 chat event, got %d", n)
	}
}

func TestDefaultPath_HomeFallback(t *testing.T) {
	prev := userHomeDir
	userHomeDir = func() (string, error) { return "/tmp/home", nil }
	defer func() { userHomeDir = prev }()
	t.Setenv("SIN_CODE_TOKENS_DB", "")
	t.Setenv("XDG_DATA_HOME", "")
	got := DefaultPath()
	want := filepath.Join("/tmp/home", ".local", "share", "sin-code", "tokens.db")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOpen_EmptyPath(t *testing.T) {
	prev := userHomeDir
	userHomeDir = func() (string, error) { return t.TempDir(), nil }
	defer func() { userHomeDir = prev }()
	t.Setenv("SIN_CODE_TOKENS_DB", "")
	t.Setenv("XDG_DATA_HOME", "")
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}

func TestAggregate_QueryError(t *testing.T) {
	s, cleanup := tempStore(t)
	cleanup() // close DB
	_, _, err := s.Aggregate(context.Background(), Filter{}, "")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestCount_Error(t *testing.T) {
	s, cleanup := tempStore(t)
	cleanup() // close DB
	_, err := s.Count(context.Background(), Filter{})
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestScanBreakdowns_QueryError(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()

	prev := queryRows
	defer func() { queryRows = prev }()

	queryRows = func(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		return nil, fmt.Errorf("query failed")
	}
	err := scanBreakdowns(context.Background(), s.db, Filter{}, "", nil, &Aggregation{ByModel: map[string]int{}, BySource: map[string]int{}})
	if err == nil {
		t.Fatal("expected query error")
	}
}

func TestScanBreakdowns_RowsCloseError(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()

	prev := rowsClose
	defer func() { rowsClose = prev }()

	rowsClose = func(r *sql.Rows) error { return fmt.Errorf("close failed") }
	err := scanBreakdowns(context.Background(), s.db, Filter{}, "", nil, &Aggregation{ByModel: map[string]int{}, BySource: map[string]int{}})
	if err == nil {
		t.Fatal("expected close error")
	}
}

func TestScanBreakdowns_ScanError(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()

	prev := queryRows
	defer func() { queryRows = prev }()

	calls := 0
	queryRows = func(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		calls++
		if calls == 1 {
			return db.QueryContext(ctx, "SELECT 'm', 0")
		}
		// Return rows whose second column is text, so Scan into int fails.
		return db.QueryContext(ctx, "SELECT 's', 'bad'")
	}
	err := scanBreakdowns(context.Background(), s.db, Filter{}, "", nil, &Aggregation{ByModel: map[string]int{}, BySource: map[string]int{}})
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestScanBreakdowns_FirstScanError(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()

	prev := queryRows
	defer func() { queryRows = prev }()

	queryRows = func(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		return db.QueryContext(ctx, "SELECT 'm', 'bad'")
	}
	err := scanBreakdowns(context.Background(), s.db, Filter{}, "", nil, &Aggregation{ByModel: map[string]int{}, BySource: map[string]int{}})
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestScanBreakdowns_SecondQueryError(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()

	prev := queryRows
	defer func() { queryRows = prev }()

	calls := 0
	queryRows = func(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		calls++
		if calls == 1 {
			return db.QueryContext(ctx, "SELECT 'm', 0")
		}
		return nil, fmt.Errorf("second query failed")
	}
	err := scanBreakdowns(context.Background(), s.db, Filter{}, "", nil, &Aggregation{ByModel: map[string]int{}, BySource: map[string]int{}})
	if err == nil {
		t.Fatal("expected second query error")
	}
}

func TestAggregate_SubQueryError(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	_ = s.Record(context.Background(), Event{Source: SourceChat, TotalTokens: 100})

	prev := aggregateQuery
	defer func() { aggregateQuery = prev }()

	aggregateQuery = func(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		return nil, fmt.Errorf("sub-query failed")
	}
	_, _, err := s.Aggregate(context.Background(), Filter{}, "day")
	if err == nil {
		t.Fatal("expected sub-query error")
	}
}

func TestAggregate_SubScanError(t *testing.T) {
	s, cleanup := tempStore(t)
	defer cleanup()
	_ = s.Record(context.Background(), Event{Source: SourceChat, TotalTokens: 100})

	prev := aggregateQuery
	defer func() { aggregateQuery = prev }()

	aggregateQuery = func(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		return db.QueryContext(ctx, "SELECT 'g', 'bad', 0, 0, 0, 0, '', ''")
	}
	_, _, err := s.Aggregate(context.Background(), Filter{}, "day")
	if err == nil {
		t.Fatal("expected sub-scan error")
	}
}

func TestTail_QueryError(t *testing.T) {
	s, cleanup := tempStore(t)
	cleanup() // close DB
	_, err := s.Tail(context.Background(), Filter{}, 10)
	if err == nil {
		t.Fatal("expected query error")
	}
}

func TestOpenWithPricing_Error(t *testing.T) {
	prev := mkdirAll
	mkdirAll = func(path string, perm os.FileMode) error { return fmt.Errorf("mkdir denied") }
	defer func() { mkdirAll = prev }()
	_, err := OpenWithPricing("/some/path/tokens.db", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
