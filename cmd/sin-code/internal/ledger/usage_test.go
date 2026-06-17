// SPDX-License-Identifier: MIT
// Purpose: Tests for ledger tool usage aggregation. Run with -race.
package ledger

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecordUsageAndHeatmap(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	records := []UsageRecord{
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_read", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_write", Outcome: OutcomeOK, SessionID: "s1"},
		{ToolName: "sin_bash", Outcome: OutcomeError, SessionID: "s2"},
		{ToolName: "websearch__search", Outcome: OutcomeOK, SessionID: "s2"},
		{ToolName: "websearch__search", Outcome: OutcomeDenied, SessionID: "s2"},
	}
	for _, r := range records {
		if err := s.RecordUsage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	counts, err := s.ToolUsageCounts(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(counts))
	}

	byName := map[string]UsageCount{}
	for _, c := range counts {
		byName[c.ToolName] = c
	}
	if byName["sin_read"].Total != 2 {
		t.Fatalf("sin_read count: %d", byName["sin_read"].Total)
	}
	if byName["sin_bash"].ByOutcome[OutcomeError] != 1 {
		t.Fatalf("sin_bash error count: %v", byName["sin_bash"].ByOutcome)
	}
	if byName["websearch__search"].ByOutcome[OutcomeDenied] != 1 {
		t.Fatalf("websearch__search denied count: %v", byName["websearch__search"].ByOutcome)
	}
	if byName["sin_read"].Family != "sin_core" {
		t.Fatalf("sin_read family: %s", byName["sin_read"].Family)
	}
	if byName["websearch__search"].Family != "websearch" {
		t.Fatalf("websearch__search family: %s", byName["websearch__search"].Family)
	}
}

func TestFamilyUsageCounts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, r := range []UsageRecord{
		{ToolName: "sin_read", SessionID: "s1"},
		{ToolName: "sin_write", SessionID: "s1"},
		{ToolName: "browser__navigate", SessionID: "s1"},
	} {
		if err := s.RecordUsage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	families, err := s.FamilyUsageCounts(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 2 {
		t.Fatalf("expected 2 families, got %d", len(families))
	}
	byFamily := map[string]FamilyCount{}
	for _, f := range families {
		byFamily[f.Family] = f
	}
	if byFamily["sin_core"].Total != 2 {
		t.Fatalf("sin_core total: %d", byFamily["sin_core"].Total)
	}
	if byFamily["browser"].Total != 1 {
		t.Fatalf("browser total: %d", byFamily["browser"].Total)
	}
}

func TestToolCoverageAndUnused(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	known := []string{"sin_read", "sin_write", "sin_edit", "websearch__search"}
	for _, r := range []UsageRecord{
		{ToolName: "sin_read", SessionID: "s1"},
		{ToolName: "sin_write", SessionID: "s1"},
		{ToolName: "sin_read", SessionID: "s2"},
	} {
		if err := s.RecordUsage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	res, err := s.ToolCoverage(ctx, known)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 4 {
		t.Fatalf("expected total 4, got %d", res.Total)
	}
	if len(res.Used) != 2 {
		t.Fatalf("expected 2 used, got %d", len(res.Used))
	}
	if len(res.Unused) != 2 {
		t.Fatalf("expected 2 unused, got %d", len(res.Unused))
	}
	if res.Coverage != 0.5 {
		t.Fatalf("expected coverage 0.5, got %f", res.Coverage)
	}

	unused, err := s.UnusedTools(ctx, known)
	if err != nil {
		t.Fatal(err)
	}
	if len(unused) != 2 {
		t.Fatalf("unused count: %d", len(unused))
	}
}

func TestToolUsageByPeriod(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := s.RecordUsage(ctx, UsageRecord{ToolName: "sin_read", SessionID: "s1", CreatedAt: base}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsage(ctx, UsageRecord{ToolName: "sin_write", SessionID: "s1", CreatedAt: base.Add(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	buckets, err := s.ToolUsageByPeriod(ctx, "day", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 2 {
		t.Fatalf("expected 2 days, got %d", len(buckets))
	}
}

func TestRecordUsageRequiresFields(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.RecordUsage(ctx, UsageRecord{ToolName: "sin_read"}); err == nil {
		t.Fatal("expected error for missing session_id")
	}
	if err := s.RecordUsage(ctx, UsageRecord{SessionID: "s1"}); err == nil {
		t.Fatal("expected error for missing tool_name")
	}
}

func TestUsageRace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tool := "sin_read"
			if i%2 == 0 {
				tool = "sin_write"
			}
			if err := s.RecordUsage(ctx, UsageRecord{ToolName: tool, Outcome: OutcomeOK, SessionID: "race"}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	counts, err := s.ToolUsageCounts(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, c := range counts {
		total += c.Total
	}
	if total != 50 {
		t.Fatalf("expected 50 recorded usages, got %d", total)
	}
}

func TestUsageSinceUntil(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := s.RecordUsage(ctx, UsageRecord{ToolName: "sin_read", SessionID: "s1", CreatedAt: base}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsage(ctx, UsageRecord{ToolName: "sin_read", SessionID: "s1", CreatedAt: base.Add(48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	counts, err := s.ToolUsageCounts(ctx, base, base.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 1 || counts[0].Total != 1 {
		t.Fatalf("expected 1 usage in window, got %v", counts)
	}
}

// _ keeps strings imported used.
var _ = strings.TrimSpace
