// SPDX-License-Identifier: MIT
package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestVerifyProducesScoredVerdict(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	checks := []Check{
		{Kind: CheckBuild, Name: "always-pass", Cmd: []string{"true"}},
		{Kind: CheckTest, Name: "always-fail", Cmd: []string{"false"}},
	}
	v := vf.Verify(context.Background(), "t1", "cand-a", checks)
	if v.Passed {
		t.Fatal("verdict must fail when a mandatory check fails")
	}
	if v.Score <= 0 || v.Score >= 1 {
		t.Fatalf("expected partial score in (0,1), got %.2f", v.Score)
	}
	if len(v.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(v.Results))
	}
}

func TestVerifyNeverErrorsOnBrokenCommand(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	checks := []Check{
		{Kind: CheckBuild, Name: "missing-binary", Cmd: []string{"definitely-not-a-real-binary-xyz-123"}},
	}
	v := vf.Verify(context.Background(), "t2", "cand-b", checks)
	if v.Passed {
		t.Fatal("unstartable check must count as failed")
	}
}

func TestBestVerdictPrefersPassedThenScore(t *testing.T) {
	now := time.Now()
	a := &Verdict{Candidate: "a", Passed: false, Score: 0.9, CreatedAt: now}
	b := &Verdict{Candidate: "b", Passed: true, Score: 0.6, CreatedAt: now}
	c := &Verdict{Candidate: "c", Passed: true, Score: 0.8, CreatedAt: now}
	best := BestVerdict([]*Verdict{a, b, c})
	if best.Candidate != "c" {
		t.Fatalf("expected passed+highest-score winner 'c', got %q", best.Candidate)
	}
}

func TestBestVerdictEmpty(t *testing.T) {
	if got := BestVerdict(nil); got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
}

func TestDiagnosisContainsFailedCheckOutput(t *testing.T) {
	v := &Verdict{
		TaskID: "t3", Candidate: "x",
		Results: []CheckResult{
			{Check: Check{Kind: CheckTest, Name: "unit"}, Passed: false, Output: "FAIL: TestFoo"},
		},
	}
	d := v.Diagnosis()
	if !strings.Contains(d, "FAIL: TestFoo") {
		t.Fatalf("diagnosis missing failing output:\n%s", d)
	}
}

func TestDiagnosisAllPass(t *testing.T) {
	v := &Verdict{Results: []CheckResult{{Passed: true}}}
	if d := v.Diagnosis(); d != "all checks passed" {
		t.Fatalf("expected 'all checks passed', got %q", d)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hi", 100); got != "hi" {
		t.Fatalf("short string untouched, got %q", got)
	}
	long := strings.Repeat("x", 2500)
	got := truncate(long, 100)
	if !strings.HasSuffix(got, "...[truncated]") {
		t.Fatalf("expected truncation suffix, got %q", got)
	}
}

func TestEmptyCheckCommandIsFailed(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	v := vf.Verify(context.Background(), "t4", "c", []Check{{Kind: CheckBuild, Name: "x"}})
	if v.Passed {
		t.Fatal("empty cmd must count as failed")
	}
}
