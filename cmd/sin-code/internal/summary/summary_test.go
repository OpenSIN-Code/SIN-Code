// SPDX-License-Identifier: MIT
// Purpose: Tests for the summary builder.
// Docs: summary.doc.md
package summary

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

func testStore(t *testing.T) *ledger.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.db")
	s, err := ledger.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBuildSummary(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sid := "sum-1"

	entries := []ledger.Entry{
		{SessionID: sid, Type: ledger.TypeUserPrompt, Data: map[string]any{"content": "Add tests for the auth module"}, CreatedAt: time.Now().Add(-2 * time.Minute)},
		{SessionID: sid, Type: ledger.TypeToolCall, Data: map[string]any{"tool": "sin_read"}, CreatedAt: time.Now().Add(-90 * time.Second)},
		{SessionID: sid, Type: ledger.TypeToolCall, Data: map[string]any{"tool": "sin_write"}, CreatedAt: time.Now().Add(-60 * time.Second)},
		{SessionID: sid, Type: ledger.TypeVerifyPass, Data: map[string]any{"mode": "poc"}, CreatedAt: time.Now().Add(-30 * time.Second)},
		{SessionID: sid, Type: ledger.TypeTaskComplete, Data: map[string]any{"summary": "Added auth tests and verified with poc gate."}, CreatedAt: time.Now()},
	}
	for _, e := range entries {
		if _, err := s.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	sum, err := Build(ctx, s, sid)
	if err != nil {
		t.Fatal(err)
	}
	if sum.SessionID != sid {
		t.Fatalf("session id mismatch: %s", sum.SessionID)
	}
	if !sum.Verified {
		t.Fatal("expected verified=true")
	}
	if sum.Verification != "poc" {
		t.Fatalf("verification mode mismatch: %s", sum.Verification)
	}
	if sum.Turns != 2 {
		t.Fatalf("expected 2 turns, got %d", sum.Turns)
	}
	if len(sum.ToolsUsed) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(sum.ToolsUsed))
	}
	if sum.OneLiner != "Added auth tests and verified with poc gate." {
		t.Fatalf("unexpected oneliner: %s", sum.OneLiner)
	}
}

func TestBuildSummaryNoEntries(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, err := Build(ctx, s, "missing")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestBuildSummaryStoreError(t *testing.T) {
	ctx := context.Background()
	// Close the ledger immediately so List returns an error.
	s, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, err = Build(ctx, s, "x")
	if err == nil {
		t.Fatal("expected error from closed ledger")
	}
}

func TestBuildFromEntriesEarlierCreatedAt(t *testing.T) {
	base := time.Now()
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeUserPrompt, Data: map[string]any{"content": "x"}, CreatedAt: base.Add(time.Minute)},
		{SessionID: "s1", Type: ledger.TypeToolCall, Data: map[string]any{"tool": "a"}, CreatedAt: base.Add(-time.Minute)},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !s.CreatedAt.Equal(base.Add(-time.Minute)) {
		t.Fatalf("CreatedAt should be the earliest entry, got %s", s.CreatedAt.Format(time.RFC3339))
	}
}

func TestBuildFromEntriesEmptyUserPrompt(t *testing.T) {
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeUserPrompt, Data: map[string]any{"content": ""}, CreatedAt: time.Now()},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.UserPrompts) != 0 {
		t.Fatalf("expected empty user prompts, got %v", s.UserPrompts)
	}
}

func TestBuildFromEntriesNoToolName(t *testing.T) {
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeToolCall, Data: map[string]any{}, CreatedAt: time.Now()},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if s.Turns != 1 {
		t.Fatalf("expected 1 turn, got %d", s.Turns)
	}
	if len(s.ToolsUsed) != 0 {
		t.Fatalf("expected no tool name, got %v", s.ToolsUsed)
	}
}

func TestBuildFromEntriesVerifyFail(t *testing.T) {
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeVerifyFail, Data: map[string]any{"mode": "poc"}, CreatedAt: time.Now()},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if s.Verified {
		t.Fatal("expected not verified")
	}
	if s.Verification != "poc (failed)" {
		t.Fatalf("expected verification failure text, got %q", s.Verification)
	}
}

func TestBuildFromEntriesVerifyFailNoMode(t *testing.T) {
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeVerifyFail, Data: map[string]any{}, CreatedAt: time.Now()},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if s.Verification != "not verified" {
		t.Fatalf("expected not verified, got %q", s.Verification)
	}
}

func TestBuildFromEntriesVerifyPassNoMode(t *testing.T) {
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeVerifyPass, Data: map[string]any{}, CreatedAt: time.Now()},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Verified {
		t.Fatal("expected verified")
	}
	if s.Verification != "unknown mode" {
		t.Fatalf("expected unknown mode, got %q", s.Verification)
	}
}

func TestBuildFromEntriesTaskCompleteNoSummary(t *testing.T) {
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeTaskComplete, Data: map[string]any{}, CreatedAt: time.Now()},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if s.OneLiner != "" {
		t.Fatalf("expected empty one-liner, got %q", s.OneLiner)
	}
}

func TestBuildFromEntriesLongPromptBecomesOneLiner(t *testing.T) {
	long := strings.Repeat("a", 90)
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeUserPrompt, Data: map[string]any{"content": long}, CreatedAt: time.Now()},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	want := long[:80] + "…"
	if s.OneLiner != want {
		t.Fatalf("expected truncated one-liner, got %q", s.OneLiner)
	}
}

func TestBuildFromEntriesLongPromptDoesNotOverwriteSummary(t *testing.T) {
	long := strings.Repeat("b", 90)
	entries := []ledger.Entry{
		{SessionID: "s1", Type: ledger.TypeUserPrompt, Data: map[string]any{"content": long}, CreatedAt: time.Now()},
		{SessionID: "s1", Type: ledger.TypeTaskComplete, Data: map[string]any{"summary": "done"}, CreatedAt: time.Now()},
	}
	s, err := buildFromEntries(entries)
	if err != nil {
		t.Fatal(err)
	}
	if s.OneLiner != "done" {
		t.Fatalf("expected one-liner from task complete, got %q", s.OneLiner)
	}
}

func TestFormat(t *testing.T) {
	s := &Summary{
		SessionID:    "fmt-1",
		Turns:        3,
		Verified:     true,
		Verification: "oracle",
		ToolsUsed:    []string{"sin_read", "sin_edit"},
		UserPrompts:  []string{"fix the bug"},
		OneLiner:     "Bug fixed and verified.",
	}
	out := Format(s)
	for _, want := range []string{"fmt-1", "3", "oracle", "sin_read", "fix the bug"} {
		if !contains(out, want) {
			t.Fatalf("formatted summary missing %q; got %s", want, out)
		}
	}
}

func TestEvidence(t *testing.T) {
	s := &Summary{Verified: true, Verification: "poc", Turns: 2, OneLiner: "Done."}
	ev := Evidence(s)
	if !contains(ev, "VERIFIED") || !contains(ev, "poc") {
		t.Fatalf("unexpected evidence: %s", ev)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && contains_(s, sub))
}
func contains_(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
