// SPDX-License-Identifier: MIT
// Purpose: Extra tests for the token-aware extensions of summary (issue #168).
package summary

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

type fakeTokenSrc struct {
	in, out, total int
	events         int
	cost           float64
	err            error
}

func (f *fakeTokenSrc) SessionTokens(_ context.Context, _ string) (int, int, int, int, float64, error) {
	return f.in, f.out, f.total, f.events, f.cost, f.err
}

func TestBuildWithTokensFillsSummary(t *testing.T) {
	s := &Summary{HasUsage: true, SessionID: "abc123", Turns: 5}
	s.InputTokens, s.OutputTokens, s.TokensUsed, s.TokenCount, s.CostUSD = 100, 50, 150, 1, 0.0009
	out := Format(s)
	if !strings.Contains(out, "Tokens: 150") {
		t.Errorf("expected tokens line, got: %s", out)
	}
	if !strings.Contains(out, "Estimated cost: $0.0009") {
		t.Errorf("expected cost, got: %s", out)
	}
}

func TestFormatNoUsageRendersNoTokensLine(t *testing.T) {
	s := &Summary{SessionID: "x", Turns: 1, HasUsage: false}
	out := Format(s)
	if strings.Contains(out, "Tokens:") || strings.Contains(out, "Estimated cost:") {
		t.Errorf("fake numbers must NEVER render; got: %s", out)
	}
}

func TestOneLineTokenEmptyWithoutUsage(t *testing.T) {
	if got := OneLineToken(&Summary{HasUsage: false}); got != "" {
		t.Errorf("expected empty when no usage; got %q", got)
	}
	if got := OneLineToken(&Summary{HasUsage: true, TokensUsed: 12_345, CostUSD: 0.04}); got == "" {
		t.Errorf("expected non-empty when HasUsage; got empty")
	}
	if !strings.Contains(OneLineToken(&Summary{HasUsage: true, TokensUsed: 12_345, CostUSD: 0.04}), "12.3k") {
		t.Errorf("expected human-formatted tokens; got %q", OneLineToken(&Summary{HasUsage: true, TokensUsed: 12_345, CostUSD: 0.04}))
	}
}

func TestOneLineTokenFormatsMillion(t *testing.T) {
	got := OneLineToken(&Summary{HasUsage: true, TokensUsed: 2_500_000, CostUSD: 2.5})
	if !strings.Contains(got, "2.5M") {
		t.Errorf("expected 2.5M, got %q", got)
	}
}

func TestOneLineTokenNoCost(t *testing.T) {
	got := OneLineToken(&Summary{HasUsage: true, TokensUsed: 12_345, CostUSD: 0})
	if !strings.Contains(got, "12.3k") || strings.Contains(got, "$") {
		t.Errorf("expected tokens-only badge, got %q", got)
	}
}

func TestEvidenceAppendsTokens(t *testing.T) {
	s := &Summary{Verified: true, Verification: "poc", Turns: 5, OneLiner: "did stuff", HasUsage: true, TokensUsed: 1000, CostUSD: 0.005}
	ev := Evidence(s)
	if !strings.Contains(ev, "tokens=1.0k") || !strings.Contains(ev, "cost=$0.0050") {
		t.Errorf("expected tokens/cost in evidence, got %q", ev)
	}
}

func TestEvidenceWithoutUsage(t *testing.T) {
	s := &Summary{Verified: false, Verification: "not verified", Turns: 0, HasUsage: false}
	ev := Evidence(s)
	if strings.Contains(ev, "tokens=") {
		t.Errorf("expected no tokens when HasUsage false; got %q", ev)
	}
}

func TestHumanInt(t *testing.T) {
	cases := map[int]string{
		0:         "0",
		42:        "42",
		999:       "999",
		1_000:     "1.0k",
		12_345:    "12.3k",
		1_500_000: "1.5M",
		-12_345:   "-12.3k",
	}
	for n, want := range cases {
		if got := humanInt(n); got != want {
			t.Errorf("humanInt(%d) = %q, want %q", n, got, want)
		}
	}
}

// BuildWithTokens swallows errors from the token source (best-effort). Make
// sure a broken source still produces a Summary keyed off Ledger entries.
func TestBuildWithTokensSwallowsSrcError(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sid := "tokens-err"
	if _, err := s.Record(ctx, ledger.Entry{SessionID: sid, Type: ledger.TypeUserPrompt, Data: map[string]any{"content": "x"}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	src := &fakeTokenSrc{err: errors.New("disk full")}
	sum, err := BuildWithTokens(ctx, s, sid, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum.HasUsage {
		t.Fatal("expected no usage when token source errors")
	}
}

func TestBuildWithTokensNilSrc(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sid := "tokens-nil"
	if _, err := s.Record(ctx, ledger.Entry{SessionID: sid, Type: ledger.TypeUserPrompt, Data: map[string]any{"content": "x"}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	sum, err := BuildWithTokens(ctx, s, sid, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum.HasUsage {
		t.Fatal("expected no usage with nil token source")
	}
}

func TestBuildWithTokensFillsFromSource(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sid := "tokens-ok"
	if _, err := s.Record(ctx, ledger.Entry{SessionID: sid, Type: ledger.TypeUserPrompt, Data: map[string]any{"content": "x"}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	src := &fakeTokenSrc{in: 10, out: 20, total: 30, events: 1, cost: 0.0012}
	sum, err := BuildWithTokens(ctx, s, sid, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum.InputTokens != 10 || sum.OutputTokens != 20 || sum.TokensUsed != 30 || sum.TokenCount != 1 || sum.CostUSD != 0.0012 {
		t.Fatalf("unexpected token fields: %+v", sum)
	}
	if !sum.HasUsage {
		t.Fatal("expected HasUsage true")
	}
}

func TestBuildWithTokensZeroTotalLeavesHasUsageFalse(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sid := "tokens-zero"
	if _, err := s.Record(ctx, ledger.Entry{SessionID: sid, Type: ledger.TypeUserPrompt, Data: map[string]any{"content": "x"}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	src := &fakeTokenSrc{in: 0, out: 0, total: 0, events: 0, cost: 0}
	sum, err := BuildWithTokens(ctx, s, sid, src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum.HasUsage {
		t.Fatal("expected HasUsage false when total is zero")
	}
}

func TestBuildWithTokensBuildError(t *testing.T) {
	ctx := context.Background()
	s, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, err = BuildWithTokens(ctx, s, "x", &fakeTokenSrc{})
	if err == nil {
		t.Fatal("expected error from Build path")
	}
}
