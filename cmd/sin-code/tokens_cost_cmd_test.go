// SPDX-License-Identifier: MIT
// Purpose: tests for `sin-code tokens cost` (cost projection and budget
// alerts). Uses an isolated SQLite store (SIN_CODE_TOKENS_DB override)
// so no real user data is touched.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/usage"
)

// seedCostEvents writes events across two sessions and two models so the
// cost report has meaningful data to aggregate.
func seedCostEvents(t *testing.T, storePath string) {
	t.Helper()
	store, err := usage.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	events := []usage.Event{
		{SessionID: "sess-expensive", Model: "gpt-4o", Source: usage.SourceChat,
			InputTokens: 10000, OutputTokens: 5000, TotalTokens: 15000, CreatedAt: now},
		{SessionID: "sess-expensive", Model: "gpt-4o", Source: usage.SourceChat,
			InputTokens: 8000, OutputTokens: 4000, TotalTokens: 12000, CreatedAt: now.Add(-1 * time.Hour)},
		{SessionID: "sess-cheap", Model: "meta/llama-3.3-70b-instruct", Source: usage.SourceChat,
			InputTokens: 1000, OutputTokens: 500, TotalTokens: 1500, CreatedAt: now.Add(-2 * time.Hour)},
		{SessionID: "sess-cheap", Model: "meta/llama-3.3-70b-instruct", Source: usage.SourceJudge,
			InputTokens: 500, OutputTokens: 200, TotalTokens: 700, CreatedAt: now.Add(-3 * time.Hour)},
		{SessionID: "sess-old", Model: "gpt-4o", Source: usage.SourceChat,
			InputTokens: 2000, OutputTokens: 1000, TotalTokens: 3000,
			CreatedAt: now.AddDate(0, 0, -10)},
	}
	for _, e := range events {
		if err := store.Record(context.Background(), e); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	_ = store.Close()
}

func TestTokensCostRegistersAsSubcommand(t *testing.T) {
	cmd := NewTokensCmd()
	subs := map[string]bool{}
	for _, s := range cmd.Commands() {
		subs[s.Name()] = true
	}
	if !subs["cost"] {
		t.Errorf("missing 'cost' subcommand (got %v)", subs)
	}
}

func TestTokensCostClassifyBudget(t *testing.T) {
	cases := []struct {
		spend  float64
		budget float64
		want   budgetAlertLevel
	}{
		{1.0, 10.0, budgetGreen},      // 10%
		{5.0, 10.0, budgetYellow},     // 50%
		{6.0, 10.0, budgetYellow},     // 60%
		{8.0, 10.0, budgetYellow},     // 80% → yellow (50–80% inclusive)
		{8.01, 10.0, budgetRed},       // just over 80% → red
		{10.0, 10.0, budgetRed},       // 100% → red (over 80%, not over budget)
		{10.01, 10.0, budgetCritical}, // just over 100% → critical
		{15.0, 10.0, budgetCritical},  // 150% → critical
		{1.0, 0, budgetNone},          // no budget
	}
	for _, c := range cases {
		got := classifyBudget(c.spend, c.budget)
		if got != c.want {
			t.Errorf("classifyBudget(spend=%.2f, budget=%.2f) = %q, want %q", c.spend, c.budget, got, c.want)
		}
	}
}

func TestTokensCostDaysInMonth(t *testing.T) {
	cases := []struct {
		year  int
		month time.Month
		want  int
	}{
		{2026, time.January, 31},
		{2026, time.February, 28}, // 2026 is not a leap year
		{2024, time.February, 29}, // 2024 is a leap year
		{2026, time.April, 30},
		{2026, time.December, 31},
	}
	for _, c := range cases {
		tt := time.Date(c.year, c.month, 15, 0, 0, 0, 0, time.UTC)
		got := daysInMonth(tt)
		if got != c.want {
			t.Errorf("daysInMonth(%d-%02d) = %d, want %d", c.year, c.month, got, c.want)
		}
	}
}

func TestTokensCostEndToEndText(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)
	seedCostEvents(t, storePath)

	cmd := NewTokensCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"cost", "--budget", "1.00"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost: %v\noutput: %s", err, out.String())
	}
	body := out.String()
	for _, want := range []string{"COST PROJECTION", "Total (lifetime)", "Today:", "This month:", "Budget Alert", "Cost by Model", "Top 5"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in output, got:\n%s", want, body)
		}
	}
}

func TestTokensCostEndToEndJSON(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)
	seedCostEvents(t, storePath)

	cmd := NewTokensCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cost", "--json", "--budget", "1.00"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost --json: %v\noutput: %s", err, out.String())
	}
	var report costReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json unmarshal: %v\nbody: %s", err, out.String())
	}
	if report.TotalSpendUSD <= 0 {
		t.Errorf("total spend should be positive, got %f", report.TotalSpendUSD)
	}
	if report.BudgetUSD != 1.00 {
		t.Errorf("budget should be 1.00, got %f", report.BudgetUSD)
	}
	if report.BudgetAlert == budgetNone {
		t.Error("expected a budget alert level, got none")
	}
	if len(report.ByModel) == 0 {
		t.Error("expected at least one model row")
	}
	if len(report.TopSessions) == 0 {
		t.Error("expected at least one session row")
	}
	if len(report.TopSessions) > 5 {
		t.Errorf("expected at most 5 sessions, got %d", len(report.TopSessions))
	}
}

func TestTokensCostModelFilter(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)
	seedCostEvents(t, storePath)

	cmd := NewTokensCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cost", "--json", "--model", "gpt-4o"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost --model: %v", err)
	}
	var report costReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	for _, m := range report.ByModel {
		if m.Model != "gpt-4o" {
			t.Errorf("model filter failed: found %q", m.Model)
		}
	}
}

func TestTokensCostNoBudgetConfigured(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)
	seedCostEvents(t, storePath)

	cmd := NewTokensCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cost"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost: %v", err)
	}
	body := out.String()
	if !strings.Contains(body, "no budget configured") {
		t.Errorf("expected 'no budget configured' message, got:\n%s", body)
	}
}

func TestTokensCostEmptyStore(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)

	store, err := usage.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	cmd := NewTokensCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cost", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute cost on empty store: %v", err)
	}
	var report costReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if report.TotalSpendUSD != 0 {
		t.Errorf("empty store total should be 0, got %f", report.TotalSpendUSD)
	}
	if len(report.ByModel) != 0 {
		t.Errorf("empty store should have 0 models, got %d", len(report.ByModel))
	}
}

func TestTokensCostBudgetAlertLabel(t *testing.T) {
	cases := map[budgetAlertLevel]string{
		budgetGreen:    "GREEN",
		budgetYellow:   "YELLOW",
		budgetRed:      "RED",
		budgetCritical: "CRITICAL",
		budgetNone:     "no budget",
	}
	for level, want := range cases {
		got := budgetAlertLabel(level)
		if !strings.Contains(got, want) {
			t.Errorf("budgetAlertLabel(%q) = %q, want to contain %q", level, got, want)
		}
	}
}

func TestTokensCostLoadBudgetConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	orig := readConfigFile
	defer func() { readConfigFile = orig }()

	readConfigFile = func(path string) ([]byte, error) {
		return []byte(`tokens.budget_monthly_usd = 42.50
# comment
llm.model = "gpt-4o"
`), nil
	}
	got := loadBudgetConfig()
	if got != 42.50 {
		t.Errorf("loadBudgetConfig: got %f, want 42.50", got)
	}
}

func TestTokensCostLoadBudgetConfigUnset(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	orig := readConfigFile
	defer func() { readConfigFile = orig }()

	readConfigFile = func(path string) ([]byte, error) {
		return []byte(`llm.model = "gpt-4o"
# no budget line
`), nil
	}
	got := loadBudgetConfig()
	if got != 0 {
		t.Errorf("loadBudgetConfig with no budget: got %f, want 0", got)
	}
}

func TestTokensCostBudgetOverrideFlag(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)
	seedCostEvents(t, storePath)

	cmd := NewTokensCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cost", "--json", "--budget", "0.01"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var report costReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json: %v", err)
	}
	if report.BudgetUSD != 0.01 {
		t.Errorf("budget override: got %f, want 0.01", report.BudgetUSD)
	}
	if report.BudgetAlert != budgetCritical {
		t.Errorf("expected critical with tiny budget, got %q", report.BudgetAlert)
	}
}

func TestTokensCostComputeReportProjection(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)

	store, err := usage.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	// Seed events across the last 3 days (within the 7-day window).
	for i := 0; i < 3; i++ {
		ts := now.AddDate(0, 0, -i)
		if err := store.Record(context.Background(), usage.Event{
			SessionID: "s", Model: "gpt-4o", Source: usage.SourceChat,
			InputTokens: 1000, OutputTokens: 500, TotalTokens: 1500,
			CreatedAt: ts,
		}); err != nil {
			t.Fatal(err)
		}
	}
	_ = store.Close()

	store2, err := usage.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	report, err := computeCostReport(context.Background(), store2, 0, "")
	if err != nil {
		t.Fatalf("computeCostReport: %v", err)
	}
	if !report.HasProjection {
		t.Error("expected projection to be computed")
	}
	if report.ProjectionUSD <= 0 {
		t.Errorf("projection should be positive, got %f", report.ProjectionUSD)
	}
	if report.DaysRemaining <= 0 {
		t.Errorf("days remaining should be positive, got %d", report.DaysRemaining)
	}
}

func TestTokensCostComputeReportModelFilter(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)

	store, err := usage.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Record(context.Background(), usage.Event{
		SessionID: "s1", Model: "gpt-4o", Source: usage.SourceChat,
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
	})
	_ = store.Record(context.Background(), usage.Event{
		SessionID: "s2", Model: "claude-sonnet-4", Source: usage.SourceChat,
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
	})
	_ = store.Close()

	store2, _ := usage.Open(storePath)
	defer store2.Close()

	report, err := computeCostReport(context.Background(), store2, 0, "gpt-4o")
	if err != nil {
		t.Fatalf("computeCostReport: %v", err)
	}
	for _, m := range report.ByModel {
		if m.Model != "gpt-4o" {
			t.Errorf("model filter leaked: found %q", m.Model)
		}
	}
	if len(report.TopSessions) != 1 {
		t.Errorf("expected 1 session after model filter, got %d", len(report.TopSessions))
	}
}
