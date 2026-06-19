// SPDX-License-Identifier: MIT
// Purpose: Targeted coverage tests for the tool-usage analysis surface in
// usage.go — RecordToolLatency validation paths, time-range filters for
// average latency, period-bucket grouping, error-rate computation, and
// outcome totals round-trip. Run with -race.
package ledger

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestRecordToolLatency_EmptyFields covers the validation branch in
// RecordToolLatency: empty toolName and empty sessionID both reject
// before any DB write, and an empty outcome defaults to OutcomeOK.
func TestRecordToolLatency_EmptyFields(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// empty toolName → error, no row written.
	err := s.RecordToolLatency(ctx, "", 100, OutcomeOK, "s1")
	if err == nil {
		t.Fatal("expected error for empty tool_name")
	}
	if !strings.Contains(err.Error(), "tool latency requires tool_name and session_id") {
		t.Fatalf("unexpected error: %v", err)
	}

	// empty sessionID → error, no row written.
	err = s.RecordToolLatency(ctx, "sin_read", 100, OutcomeOK, "")
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
	if !strings.Contains(err.Error(), "tool latency requires tool_name and session_id") {
		t.Fatalf("unexpected error: %v", err)
	}

	// empty outcome defaults to OutcomeOK and persists successfully.
	if err := s.RecordToolLatency(ctx, "sin_read", 100, "", "s1"); err != nil {
		t.Fatalf("unexpected error with empty outcome: %v", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT outcome FROM tool_latency WHERE session_id = ?`, "s1")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := []string{}
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			t.Fatal(err)
		}
		got = append(got, o)
	}
	if len(got) != 1 || got[0] != string(OutcomeOK) {
		t.Fatalf("expected [ok], got %v", got)
	}
}

// TestToolAvgLatencies_TimeRangeFilter records latencies across two
// windows and verifies that since/until narrow the result correctly
// and that no filter returns every group.
func TestToolAvgLatencies_TimeRangeFilter(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	aStart := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	aEnd := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)
	bStart := aEnd.Add(time.Minute) // strictly after A

	// Window A: three latencies (100, 200, 300) → avg 200, count 3.
	for _, dur := range []int64{100, 200, 300} {
		t0 := aStart.Add(10 * time.Minute)
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO tool_latency (tool_name, duration_ms, outcome, session_id, created_at)
			VALUES (?, ?, ?, ?, ?)`, "sin_read", dur, string(OutcomeOK), "sA", t0.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	// Window B: two latencies for a different tool.
	for _, dur := range []int64{400, 600} {
		ts := bStart.Add(10 * time.Minute)
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO tool_latency (tool_name, duration_ms, outcome, session_id, created_at)
			VALUES (?, ?, ?, ?, ?)`, "sin_write", dur, string(OutcomeOK), "sB", ts.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}

	// Window A only: exactly one group, avg 200, count 3.
	aOnly, err := s.ToolAvgLatencies(ctx, aStart, aEnd)
	if err != nil {
		t.Fatal(err)
	}
	if len(aOnly) != 1 {
		t.Fatalf("expected 1 group in window A, got %d (%v)", len(aOnly), aOnly)
	}
	if aOnly[0].ToolName != "sin_read" || aOnly[0].Count != 3 || aOnly[0].AvgDuration != 200.0 {
		t.Fatalf("window A mismatch: %+v", aOnly[0])
	}

	// No filters: both groups returned.
	all, err := s.ToolAvgLatencies(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 groups with no filter, got %d (%v)", len(all), all)
	}
	byTool := map[string]AvgLatency{}
	for _, al := range all {
		byTool[al.ToolName] = al
	}
	if byTool["sin_write"].Count != 2 {
		t.Fatalf("sin_write count: %d", byTool["sin_write"].Count)
	}
	if got := byTool["sin_write"].AvgDuration; got != 500.0 {
		t.Fatalf("sin_write avg: %v", got)
	}
}

// TestToolLatencyByPeriod_InvalidPeriod validates the unsupported-period
// branch and the supported (day, week, month) bucketings.
func TestToolLatencyByPeriod_InvalidPeriod(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_, err := s.ToolLatencyByPeriod(ctx, "year", time.Time{}, time.Time{})
	if err == nil {
		t.Fatal("expected error for unsupported period")
	}
	if !strings.Contains(err.Error(), "unsupported period") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Seed one row at a known instant.
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_latency (tool_name, duration_ms, outcome, session_id, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		"sin_read", 250, string(OutcomeOK), "s1", base.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	for _, period := range []string{"day", "week", "month"} {
		buckets, err := s.ToolLatencyByPeriod(ctx, period, time.Time{}, time.Time{})
		if err != nil {
			t.Fatalf("period %q: %v", period, err)
		}
		if len(buckets) != 1 {
			t.Fatalf("period %q: expected 1 bucket, got %d (%v)", period, len(buckets), buckets)
		}
		_, ok := buckets[firstKey(buckets)]
		if !ok {
			t.Fatalf("period %q: no entry in bucket map", period)
		}
		if _, present := buckets[firstKey(buckets)]; !present {
			t.Fatalf("period %q: missing key", period)
		}
	}
}

// firstKey is a tiny helper used by the period test to look at the
// single bucket produced by a single-row seed.
func firstKey[T comparable](m map[T]float64) T {
	for k := range m {
		return k
	}
	var zero T
	return zero
}

// TestToolErrorRates_RatioCalculation records 10 tool calls (7 OK,
// 3 error) for one tool and verifies the computed ratio. Empty store
// returns an empty slice.
func TestToolErrorRates_RatioCalculation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// Empty store → empty result, no error.
	empty, err := s.ToolErrorRates(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty store: expected 0 rows, got %v", empty)
	}

	// 7 OK + 3 error for sin_read, 1 OK for sin_write (single-row,
	// different ratio, exercises ORDER BY tool_name).
	records := []UsageRecord{
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeError, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeError, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeError, SessionID: "s1"},
		{ToolName: "sin_write", Outcome: OutcomeOK, SessionID: "s2"},
	}
	for _, r := range records {
		if err := s.RecordUsage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	rates, err := s.ToolErrorRates(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rates) != 2 {
		t.Fatalf("expected 2 rows, got %d (%v)", len(rates), rates)
	}
	// sin_read comes first (ORDER BY tool_name).
	read := rates[0]
	if read.ToolName != "sin_read" {
		t.Fatalf("expected first row sin_read, got %s", read.ToolName)
	}
	if read.TotalCalls != 10 {
		t.Fatalf("sin_read TotalCalls = %d, want 10", read.TotalCalls)
	}
	if read.Errors != 3 {
		t.Fatalf("sin_read Errors = %d, want 3", read.Errors)
	}
	if read.ErrorRate != 0.3 {
		t.Fatalf("sin_read ErrorRate = %v, want 0.3", read.ErrorRate)
	}
	write := rates[1]
	if write.ToolName != "sin_write" || write.TotalCalls != 1 || write.Errors != 0 || write.ErrorRate != 0.0 {
		t.Fatalf("sin_write mismatch: %+v", write)
	}
}

// TestOutcomeTotals_DistinctOutcomes seeds a mix of Outcome values
// and asserts the resulting map's counts sum to the row total. An
// unknown outcome string round-trips via UsageOutcome(o) without panic.
func TestOutcomeTotals_DistinctOutcomes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// Mix: 4 OK, 2 error, 1 denied, plus 1 unknown outcome.
	mix := []UsageRecord{
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeError, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeError, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeDenied, SessionID: "s1"},
	}
	for _, r := range mix {
		if err := s.RecordUsage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	// Insert an unknown outcome string directly to exercise the
	// string → UsageOutcome round-trip through the scan path.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_usage (tool_name, tool_family, outcome, session_id, goal_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"sin_read", "sin_core", "weird_custom_outcome", "s1", "", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	totals, err := s.OutcomeTotals(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, v := range totals {
		sum += v
	}
	if sum != 8 {
		t.Fatalf("outcome totals sum = %d, want 8", sum)
	}
	if totals[OutcomeOK] != 4 {
		t.Fatalf("ok total = %d, want 4", totals[OutcomeOK])
	}
	if totals[OutcomeError] != 2 {
		t.Fatalf("error total = %d, want 2", totals[OutcomeError])
	}
	if totals[OutcomeDenied] != 1 {
		t.Fatalf("denied total = %d, want 1", totals[OutcomeDenied])
	}
	// Unknown outcome must not panic and must surface under the
	// literal UsageOutcome key it was stored under.
	weirdKey := UsageOutcome("weird_custom_outcome")
	if v, ok := totals[weirdKey]; !ok || v != 1 {
		t.Fatalf("unknown outcome missing or wrong count: ok=%v v=%d total=%v", ok, v, totals)
	}
}
