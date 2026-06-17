// SPDX-License-Identifier: MIT
// Purpose: deterministic status report rendering and graceful degradation tests.
package status

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

var fixedTime = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

func sampleReport() *Report {
	return &Report{
		GeneratedAt: fixedTime,
		Workspace:   "/workspace",
		Goals: GoalsSection{
			Total: 3,
			ByStatus: map[string]int{
				"pending":  1,
				"running":  1,
				"verified": 1,
			},
			Items: []GoalItem{
				{ID: 1, Status: "running", Priority: 1, Workspace: "/workspace", Prompt: "fix bug", Attempts: 1, MaxRetries: 3},
				{ID: 2, Status: "pending", Priority: 0, Workspace: "/workspace", Prompt: "add test", Attempts: 0, MaxRetries: 3},
			},
			Empty: false,
		},
		Todos: TodosSection{
			Total: 2,
			Open:  1,
			Ready: 1,
			ByStatus: map[string]int{
				"open": 1,
				"done": 1,
			},
			ByPriority: map[string]int{
				"P0": 1,
				"P1": 1,
			},
			Items: []TodoItem{
				{ID: "t1", Title: "first", Status: "open", Priority: "P0", Type: "task", Tags: []string{"a", "b"}, Assignee: "alice"},
			},
			Empty: false,
		},
		Sessions: SessionsSection{
			Total: 1,
			Items: []SessionItem{
				{ID: "sess-1", CreatedAt: "2026-06-17", UpdatedAt: "2026-06-18", Title: "demo", ParentID: ""},
			},
			Empty: false,
		},
		Ledger: LedgerSection{
			DistinctSessions: 1,
			ToolUsage: []ToolUsageItem{
				{Name: "sin_test", Family: "sin_core", Total: 5, OK: 4, Error: 1, Denied: 0},
			},
			FamilyUsage: []FamilyUsageItem{
				{Family: "sin_core", Total: 5, OK: 4, Error: 1, Denied: 0},
			},
			Empty: false,
		},
		Debt: DebtSection{
			Total:   2,
			RotRisk: 1,
			ByReason: []KV{
				{Key: "quick fix", Count: 2},
			},
			Empty: false,
		},
		Skills: SkillsSection{
			Total:     2,
			Installed: 1,
			Runnable:  1,
			Items: []SkillItem{
				{Name: "skill-a", Installed: true, Runnable: true, Detail: "ok"},
				{Name: "skill-b", Installed: false, Runnable: false, Detail: ""},
			},
			Empty: false,
		},
		Warnings: []string{"1 goal(s) pending or running", "1 open todo(s)"},
	}
}

func TestRenderMarkdownDeterministic(t *testing.T) {
	r := sampleReport()
	out1 := RenderMarkdown(r)
	out2 := RenderMarkdown(r)
	if out1 != out2 {
		t.Fatalf("markdown output not deterministic:\n%s\n---\n%s", out1, out2)
	}
	if !strings.Contains(out1, "# SIN-Code Status Snapshot") {
		t.Errorf("missing header")
	}
	if !strings.Contains(out1, "## Readiness") {
		t.Errorf("missing readiness section")
	}
	for _, section := range []string{"## Goals", "## Todos", "## Sessions", "## Ledger / Tool Usage", "## Debt", "## Skills", "## Warnings"} {
		if !strings.Contains(out1, section) {
			t.Errorf("missing section %q", section)
		}
	}
	if idx := strings.Index(out1, "pending"); idx == -1 {
		t.Errorf("missing status row")
	}
}

func TestRenderJSONRoundTrip(t *testing.T) {
	r := sampleReport()
	b, err := RenderJSON(r)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var got Report
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal rendered JSON: %v", err)
	}
	if got.GeneratedAt != r.GeneratedAt {
		t.Errorf("GeneratedAt mismatch: got %v want %v", got.GeneratedAt, r.GeneratedAt)
	}
	if got.Workspace != r.Workspace {
		t.Errorf("Workspace mismatch: got %q want %q", got.Workspace, r.Workspace)
	}
	if got.Goals.Total != r.Goals.Total {
		t.Errorf("Goals.Total mismatch: got %d want %d", got.Goals.Total, r.Goals.Total)
	}
	if got.Todos.Open != r.Todos.Open {
		t.Errorf("Todos.Open mismatch: got %d want %d", got.Todos.Open, r.Todos.Open)
	}
	if got.Ledger.DistinctSessions != r.Ledger.DistinctSessions {
		t.Errorf("Ledger.DistinctSessions mismatch: got %d want %d", got.Ledger.DistinctSessions, r.Ledger.DistinctSessions)
	}
	if len(got.Skills.Items) != len(r.Skills.Items) {
		t.Errorf("Skills.Items length mismatch: got %d want %d", len(got.Skills.Items), len(r.Skills.Items))
	}
}

func TestRenderMarkdownEmptySections(t *testing.T) {
	r := &Report{
		GeneratedAt: fixedTime,
		Workspace:   "/empty",
	}
	out := RenderMarkdown(r)
	want := []string{"No data yet", "No warnings"}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("expected %q in empty report", w)
		}
	}
}

func TestRenderMarkdownErrorSections(t *testing.T) {
	r := &Report{
		GeneratedAt: fixedTime,
		Workspace:   "/empty",
		Goals:       GoalsSection{Error: "db locked"},
	}
	out := RenderMarkdown(r)
	if !strings.Contains(out, "No data yet — db locked") {
		t.Errorf("expected error message in rendered markdown, got:\n%s", out)
	}
}

func TestRenderMarkdownEscapesPipes(t *testing.T) {
	r := &Report{
		GeneratedAt: fixedTime,
		Workspace:   "/w",
		Goals: GoalsSection{
			Total:    1,
			ByStatus: map[string]int{"running": 1},
			Items:    []GoalItem{{ID: 1, Status: "running", Prompt: "a | b"}},
		},
	}
	out := RenderMarkdown(r)
	if strings.Contains(out, "| a | b |") {
		t.Errorf("pipe not escaped in markdown table")
	}
}

func TestCollectGracefulDegradation(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Workspace: dir}
	rep, err := Collect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if rep.Workspace != dir {
		t.Errorf("Workspace mismatch: got %q want %q", rep.Workspace, dir)
	}
	if rep.Debt.Total != 0 {
		t.Errorf("expected no debt markers in empty temp dir, got %d", rep.Debt.Total)
	}
	_ = RenderMarkdown(rep)
	_, err = RenderJSON(rep)
	if err != nil {
		t.Fatalf("RenderJSON on empty report: %v", err)
	}
}

func TestCollectWithDebt(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	if err := os.WriteFile(f, []byte("package main\n// sin-debt: quick fix, upgrade: refactor\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	cfg := Config{Workspace: dir}
	rep, err := Collect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if rep.Debt.Total != 1 {
		t.Errorf("Debt.Total = %d, want 1", rep.Debt.Total)
	}
	if rep.Debt.RotRisk != 0 {
		t.Errorf("Debt.RotRisk = %d, want 0 (marker has upgrade clause)", rep.Debt.RotRisk)
	}
}

func TestCollectWithLedger(t *testing.T) {
	dir := t.TempDir()
	ledgerDB := filepath.Join(dir, "ledger.db")
	store, err := ledger.Open(ledgerDB)
	if err != nil {
		t.Fatalf("open ledger db: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Record both a ledger entry and a tool usage so DistinctSessions and
	// ToolUsageCounts both have data to read.
	if _, err := store.Record(ctx, ledger.Entry{
		SessionID: "sess-1",
		Type:      ledger.TypeToolCall,
		Data:      map[string]any{"tool": "sin_test"},
		Summary:   "test usage",
	}); err != nil {
		t.Fatalf("record ledger entry: %v", err)
	}
	if err := store.RecordUsage(ctx, ledger.UsageRecord{ToolName: "sin_test", SessionID: "sess-1", Outcome: ledger.OutcomeOK}); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	store.Close()

	t.Setenv("SIN_CODE_LEDGER", ledgerDB)
	cfg := Config{Workspace: dir}
	rep, err := Collect(ctx, cfg)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if rep.Ledger.DistinctSessions != 1 {
		t.Errorf("Ledger.DistinctSessions = %d, want 1", rep.Ledger.DistinctSessions)
	}
	if len(rep.Ledger.ToolUsage) != 1 || rep.Ledger.ToolUsage[0].Name != "sin_test" {
		t.Errorf("Ledger.ToolUsage = %+v, want one sin_test row", rep.Ledger.ToolUsage)
	}
}

func TestSortedHelpers(t *testing.T) {
	in := []ToolUsageItem{
		{Name: "z", Total: 1},
		{Name: "a", Total: 2},
	}
	out := sortedToolUsage(in)
	if out[0].Name != "a" || out[1].Name != "z" {
		t.Errorf("sortedToolUsage failed: %+v", out)
	}
}

func TestBuildWarnings(t *testing.T) {
	r := sampleReport()
	w := buildWarnings(r)
	if len(w) == 0 {
		t.Fatalf("expected warnings")
	}
	found := false
	for _, x := range w {
		if strings.Contains(x, "goal(s) pending or running") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected pending/running goal warning, got %v", w)
	}
}
