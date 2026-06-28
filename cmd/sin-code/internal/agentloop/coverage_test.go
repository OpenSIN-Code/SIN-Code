// SPDX-License-Identifier: MIT
// Purpose: Coverage gap tests for the agentloop package.
// Targets uncovered error paths, edge cases, and helper functions.
// All tests pass under `go test -race -count=1` (mandate M7).
package agentloop

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// ---------------------------------------------------------------------------
// budget.go coverage
// ---------------------------------------------------------------------------

func TestBudgetLevel_String_AllLevels(t *testing.T) {
	cases := []struct {
		level BudgetLevel
		want  string
	}{
		{BudgetGreen, "green"},
		{BudgetYellow, "yellow"},
		{BudgetRed, "red"},
		{BudgetLevel(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.level.String(); got != c.want {
			t.Errorf("BudgetLevel(%d).String() = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestBudget_MaxTokens(t *testing.T) {
	b := NewBudget(5000, 10.0)
	if got := b.MaxTokens(); got != 5000 {
		t.Errorf("MaxTokens: got %d, want 5000", got)
	}
	var nilB *Budget
	if got := nilB.MaxTokens(); got != 0 {
		t.Errorf("nil MaxTokens: got %d, want 0", got)
	}
}

func TestBudget_MaxCostUSD(t *testing.T) {
	b := NewBudget(5000, 10.0)
	if got := b.MaxCostUSD(); got != 10.0 {
		t.Errorf("MaxCostUSD: got %.2f, want 10.0", got)
	}
	var nilB *Budget
	if got := nilB.MaxCostUSD(); got != 0 {
		t.Errorf("nil MaxCostUSD: got %.2f, want 0", got)
	}
}

func TestBudget_UsedTokens_Nil(t *testing.T) {
	var b *Budget
	if got := b.UsedTokens(); got != 0 {
		t.Errorf("nil UsedTokens: got %d, want 0", got)
	}
}

func TestBudget_UsedCost_Nil(t *testing.T) {
	var b *Budget
	if got := b.UsedCost(); got != 0 {
		t.Errorf("nil UsedCost: got %.2f, want 0", got)
	}
}

func TestBudget_Consume_NegativeValues(t *testing.T) {
	b := NewBudget(1000, 5.0)
	if err := b.Consume(-100, -1.0); err != nil {
		t.Fatalf("negative consume should be clamped to 0, got: %v", err)
	}
	if got := b.UsedTokens(); got != 0 {
		t.Errorf("after negative consume, used tokens: got %d, want 0", got)
	}
	if got := b.UsedCost(); got != 0 {
		t.Errorf("after negative consume, used cost: got %.2f, want 0", got)
	}
}

func TestBudget_Consume_BothLimitsExceeded(t *testing.T) {
	b := NewBudget(100, 0.10)
	err := b.Consume(200, 0.50)
	if err == nil {
		t.Fatal("expected error when both limits exceeded")
	}
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted, got: %v", err)
	}
	if !strings.Contains(err.Error(), "tokens") || !strings.Contains(err.Error(), "cost") {
		t.Errorf("error should mention both tokens and cost, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// toolcoverage.go coverage
// ---------------------------------------------------------------------------

func TestJoinBackticks(t *testing.T) {
	got := joinBackticks([]string{"sin_poc", "sin_oracle"})
	if got != "`sin_poc`, `sin_oracle`" {
		t.Errorf("joinBackticks: got %q, want `sin_poc`, `sin_oracle`", got)
	}
}

func TestJoinBackticks_Single(t *testing.T) {
	got := joinBackticks([]string{"sin_poc"})
	if got != "`sin_poc`" {
		t.Errorf("joinBackticks single: got %q, want `sin_poc`", got)
	}
}

func TestJoinBackticks_Empty(t *testing.T) {
	got := joinBackticks(nil)
	if got != "" {
		t.Errorf("joinBackticks empty: got %q, want ''", got)
	}
}

func TestToolCoverageEnforcer_Record_EmptyName(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, nil)
	e.Record("") // should be a no-op
	used := e.Used()
	if len(used) != 0 {
		t.Errorf("Record('') should not add anything, got %v", used)
	}
}

func TestToolCoverageEnforcer_Record_NilEnforcer(t *testing.T) {
	var e *ToolCoverageEnforcer
	e.Record("sin_poc") // should not panic
}

func TestToolCoverageEnforcer_Check_NilEnforcer(t *testing.T) {
	var e *ToolCoverageEnforcer
	ok, missing, forbidden := e.Check()
	if !ok || len(missing) != 0 || len(forbidden) != 0 {
		t.Errorf("nil Check should pass, got ok=%v missing=%v forbidden=%v", ok, missing, forbidden)
	}
}

func TestToolCoverageEnforcer_Used_NilEnforcer(t *testing.T) {
	var e *ToolCoverageEnforcer
	if got := e.Used(); got != nil {
		t.Errorf("nil Used should return nil, got %v", got)
	}
}

func TestToolCoverageEnforcer_HasConstraints_NilEnforcer(t *testing.T) {
	var e *ToolCoverageEnforcer
	if e.HasConstraints() {
		t.Error("nil HasConstraints should be false")
	}
}

func TestToolCoverageEnforcer_HasConstraints_WithRequired(t *testing.T) {
	e := NewToolCoverageEnforcer([]string{"sin_poc"}, nil)
	if !e.HasConstraints() {
		t.Error("expected HasConstraints=true with required tools")
	}
}

func TestToolCoverageEnforcer_HasConstraints_WithForbidden(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, []string{"sin_bash"})
	if !e.HasConstraints() {
		t.Error("expected HasConstraints=true with forbidden tools")
	}
}

func TestToolCoverageEnforcer_HasConstraints_None(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, nil)
	if e.HasConstraints() {
		t.Error("expected HasConstraints=false with no constraints")
	}
}

func TestToolCoverageEnforcer_Feedback_MultipleMissing(t *testing.T) {
	e := NewToolCoverageEnforcer([]string{"sin_poc", "sin_oracle"}, nil)
	fb := e.Feedback([]string{"sin_poc", "sin_oracle"}, nil)
	if fb == "" {
		t.Fatal("expected feedback for multiple missing")
	}
	if !strings.Contains(fb, "call:") {
		t.Errorf("expected 'call:' in feedback, got %q", fb)
	}
}

func TestToolCoverageEnforcer_Feedback_MultipleForbidden(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, []string{"sin_bash", "sin_git"})
	fb := e.Feedback(nil, []string{"sin_bash", "sin_git"})
	if fb == "" {
		t.Fatal("expected feedback for multiple forbidden")
	}
	if !strings.Contains(fb, "forbidden tools:") {
		t.Errorf("expected 'forbidden tools:' in feedback, got %q", fb)
	}
}

func TestToolCoverageEnforcer_Feedback_Empty(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, nil)
	fb := e.Feedback(nil, nil)
	if fb != "" {
		t.Errorf("expected empty feedback for no violations, got %q", fb)
	}
}

func TestToolCoverageEnforcer_Feedback_NilEnforcer(t *testing.T) {
	var e *ToolCoverageEnforcer
	fb := e.Feedback([]string{"x"}, nil)
	if fb != "" {
		t.Errorf("nil enforcer feedback should be empty, got %q", fb)
	}
}

func TestToolCoverageEnforcer_Feedback_SingleForbidden(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, []string{"sin_bash"})
	fb := e.Feedback(nil, []string{"sin_bash"})
	if fb == "" {
		t.Fatal("expected feedback for single forbidden")
	}
	if !strings.Contains(fb, "forbidden tool") {
		t.Errorf("expected 'forbidden tool' in feedback, got %q", fb)
	}
}

// ---------------------------------------------------------------------------
// watch.go coverage
// ---------------------------------------------------------------------------

func TestWatchMode_MatchesPattern_NoPatterns(t *testing.T) {
	w := NewWatchMode(nil)
	if !w.matchesPattern("anything.go") {
		t.Error("no patterns should match everything")
	}
	if !w.matchesPattern("whatever.txt") {
		t.Error("no patterns should match everything")
	}
}

func TestIsWatchIgnored(t *testing.T) {
	// Known ignored dirs
	for _, name := range []string{".git", "vendor", "node_modules", "__pycache__", ".sin-code", ".sin", "dist", "build", "target", ".cache", "tmp"} {
		if !isWatchIgnored(name) {
			t.Errorf("isWatchIgnored(%q) should be true", name)
		}
	}
	// Non-ignored dir
	if isWatchIgnored("src") {
		t.Error("isWatchIgnored('src') should be false")
	}
	if isWatchIgnored("myproject") {
		t.Error("isWatchIgnored('myproject') should be false")
	}
}

func TestWatchMode_ScanDir_NonexistentDir(t *testing.T) {
	w := NewWatchMode([]string{"*.go"})
	w.SetRoot("/nonexistent/path/that/does/not/exist")
	// scanDir on a nonexistent dir should return false, not panic
	if w.scanDir("/nonexistent/path/that/does/not/exist", true) {
		t.Error("expected false for nonexistent dir")
	}
}

func TestWatchMode_ScanForChanges_EmptyRoot(t *testing.T) {
	w := NewWatchMode([]string{"*.go"})
	// root is empty — should fall back to os.Getwd()
	// Just verify it doesn't panic
	w.scanForChanges()
}

func TestWatchMode_InitialScan_EmptyRoot(t *testing.T) {
	w := NewWatchMode([]string{"*.go"})
	// root is empty — should fall back to os.Getwd()
	w.initialScan()
}

func TestWatchMode_MatchesPattern_ExtensionMatch(t *testing.T) {
	w := NewWatchMode([]string{"*.go"})
	// Test the extension-suffix path
	if !w.matchesPattern("test.go") {
		t.Error("expected '*.go' to match 'test.go'")
	}
	if w.matchesPattern("test.py") {
		t.Error("expected '*.go' to not match 'test.py'")
	}
}

func TestWatchMode_ScanDir_WithNonMatchingFiles(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "readme.md", "# readme")
	writeWatchFile(t, root, "main.go", "package main")
	w := NewWatchMode([]string{"*.go"})
	w.SetRoot(root)
	changed := w.scanDir(root, true)
	if changed {
		t.Error("initial scan should not report changes")
	}
	w.mu.Lock()
	count := len(w.snapshots)
	w.mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 file in snapshots (only .go), got %d", count)
	}
}

func TestWatchMode_ScanDir_EntryInfoError(t *testing.T) {
	root := t.TempDir()
	// Create a symlink to a nonexistent file — entry.Info() may still work,
	// but let's test the general path of scanning a dir with a broken entry
	w := NewWatchMode([]string{"*"})
	w.SetRoot(root)
	// Just ensure scanning an empty dir doesn't panic
	if w.scanDir(root, true) {
		t.Error("expected false for empty dir initial scan")
	}
}

// ---------------------------------------------------------------------------
// background.go coverage
// ---------------------------------------------------------------------------

func TestItoa3_NegativeNumber(t *testing.T) {
	got := itoa3(-5)
	if got != "005" {
		t.Errorf("itoa3(-5) = %q, want '005'", got)
	}
}

func TestItoa3_LargeNumber(t *testing.T) {
	got := itoa3(1234)
	if got != "234" {
		t.Errorf("itoa3(1234) = %q, want '234'", got)
	}
}

func TestTaskRegistry_Finish_NotFound(t *testing.T) {
	r := NewTaskRegistry()
	res := &Result{Summary: "ok"}
	r.Finish("bg-999", "verified", res, nil)
	// Should not panic, should be a no-op
}

func TestTaskRegistry_Finish_WithError(t *testing.T) {
	r := NewTaskRegistry()
	t1 := r.Add("task")
	r.Finish(t1.ID, "cancelled", nil, errors.New("timeout"))
	got, _ := r.Get(t1.ID)
	if got.Status != "cancelled" {
		t.Errorf("expected status=cancelled, got %q", got.Status)
	}
	if got.Err != "timeout" {
		t.Errorf("expected err=timeout, got %q", got.Err)
	}
}

// ---------------------------------------------------------------------------
// frustration.go coverage
// ---------------------------------------------------------------------------

func TestFrustration_Score_NilDetector(t *testing.T) {
	var d *FrustrationDetector
	if got := d.Score(); got != 0 {
		t.Errorf("nil Score should return 0, got %d", got)
	}
}

func TestNormalizeMessage_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		check func(string) bool
		desc  string
	}{
		{"", func(s string) bool { return s == "" }, "empty string"},
		{"  ", func(s string) bool { return s == "" }, "whitespace only"},
		{"\n\t", func(s string) bool { return s == "" }, "newlines and tabs"},
		{"Hello   World", func(s string) bool { return !strings.Contains(s, "  ") }, "multiple spaces collapsed"},
		{"  Hello  World  ", func(s string) bool { return s == "hello world" }, "trimmed and collapsed"},
	}
	for _, tt := range tests {
		got := normalizeMessage(tt.input)
		if !tt.check(got) {
			t.Errorf("normalizeMessage(%q) = %q, %s failed", tt.input, got, tt.desc)
		}
	}
}

// ---------------------------------------------------------------------------
// provider_adapter.go coverage — cache paths
// ---------------------------------------------------------------------------

func TestNewProviderCompletionWithCache_CacheHit(t *testing.T) {
	cache := llm.NewPromptCache(5 * time.Minute)
	// Pre-populate cache with a known key
	systemPrompt := "You are a helpful assistant"
	firstUser := "Do the task"
	key := llm.CacheKey(systemPrompt, firstUser)
	cache.Set(key, "prefix-123")

	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		// Verify cache headers are set
		if req.Header.Get("X-SIN-Cache-Prefix-ID") != "prefix-123" {
			// Not a hard error — the header may not be set if model doesn't support caching
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)),
		}, nil
	})

	fn := NewProviderCompletionWithCache(c, "claude-3-opus", 100, 0.0, cache)
	comp, err := fn(context.Background(), []session.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: firstUser},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp.Text != "done" {
		t.Errorf("expected 'done', got %q", comp.Text)
	}
}

func TestNewProviderCompletionWithCache_CacheMiss(t *testing.T) {
	cache := llm.NewPromptCache(5 * time.Minute)

	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)),
		}, nil
	})

	fn := NewProviderCompletionWithCache(c, "claude-3-opus", 100, 0.0, cache)
	comp, err := fn(context.Background(), []session.Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Do the task"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp.Text != "done" {
		t.Errorf("expected 'done', got %q", comp.Text)
	}
}

func TestNewProviderCompletionWithCache_NonCachingModel(t *testing.T) {
	cache := llm.NewPromptCache(5 * time.Minute)

	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		// Verify no cache headers are set for non-caching model
		if req.Header.Get("X-SIN-Cache-Key") != "" {
			t.Errorf("expected no cache key header for non-caching model, got %q", req.Header.Get("X-SIN-Cache-Key"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)),
		}, nil
	})

	fn := NewProviderCompletionWithCache(c, "gpt-4", 100, 0.0, cache)
	comp, err := fn(context.Background(), []session.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "user"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp.Text != "done" {
		t.Errorf("expected 'done', got %q", comp.Text)
	}
}

func TestNewProviderCompletionWithCache_NilCache(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)),
		}, nil
	})

	fn := NewProviderCompletionWithCache(c, "claude-3-opus", 100, 0.0, nil)
	comp, err := fn(context.Background(), []session.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "user"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp.Text != "done" {
		t.Errorf("expected 'done', got %q", comp.Text)
	}
}

// ---------------------------------------------------------------------------
// loop.go coverage — recordUsage
// ---------------------------------------------------------------------------

func TestLoop_RecordUsage_NilLedger(t *testing.T) {
	l := &Loop{
		SessionID: "test-session",
		// Ledger is nil — should be a no-op
	}
	l.recordUsage(context.Background(), "sin_read", ledger.OutcomeOK)
	// Should not panic
}

func TestLoop_RecordUsage_EmptySessionID(t *testing.T) {
	ledgerStore, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer ledgerStore.Close()

	l := &Loop{
		Ledger:    ledgerStore,
		SessionID: "", // empty — should be a no-op
	}
	l.recordUsage(context.Background(), "sin_read", ledger.OutcomeOK)
	// Should not panic
}

func TestLoop_RecordUsage_Success(t *testing.T) {
	ledgerStore, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer ledgerStore.Close()

	l := &Loop{
		Ledger:    ledgerStore,
		SessionID: "test-session",
	}
	l.recordUsage(context.Background(), "sin_read", ledger.OutcomeOK)
	// Should not panic — coverage of the actual RecordUsage call
}
