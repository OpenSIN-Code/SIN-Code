// SPDX-License-Identifier: MIT
// Purpose: tests for the eval harness — runner, scorers, regression
// detection. Pure-stdlib (no model mocks) so it runs in the fast suite.
// Docs: evalharness_test.doc.md
package evalharness

import (
	"context"
	"testing"
)

type stubSubject struct{ reply string }

func (s stubSubject) Run(_ context.Context, c EvalCase) (Output, error) {
	return Output{Text: s.reply, Success: true}, nil
}

func TestRunnerAndAggregate(t *testing.T) {
	set := EvalSet{Name: "demo", Cases: []EvalCase{
		{ID: "a", Prompt: "p", Expected: "hello"},
		{ID: "b", Prompt: "p", Expected: "world"},
	}}
	runner := Runner{Subject: stubSubject{reply: "hello"}, Scorer: ExactMatch{}, SubjectName: "stub"}
	run, err := runner.Execute(context.Background(), set)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	score, pass := run.Aggregate()
	if score != 0.5 || pass != 0.5 {
		t.Fatalf("expected 0.5/0.5, got score=%.2f pass=%.2f", score, pass)
	}
}

func TestContainsAllScorer(t *testing.T) {
	s := ContainsAll{PassThreshold: 1.0}
	score, passed, _ := s.Score(EvalCase{Expected: "foo\nbar"}, Output{Text: "x foo y bar z"})
	if score != 1.0 || !passed {
		t.Fatalf("expected full match, got %.2f pass=%v", score, passed)
	}
	score, passed, _ = s.Score(EvalCase{Expected: "foo\nbar"}, Output{Text: "only foo"})
	if score != 0.5 || passed {
		t.Fatalf("expected 0.5 fail, got %.2f pass=%v", score, passed)
	}
}

func TestCompareDetectsRegression(t *testing.T) {
	base := Run{ID: "base", Results: []Result{{CaseID: "a", Score: 1, Weight: 1}, {CaseID: "b", Score: 1, Weight: 1}}}
	cand := Run{ID: "cand", Results: []Result{{CaseID: "a", Score: 1, Weight: 1}, {CaseID: "b", Score: 0, Weight: 1}}}
	cmp := CompareRuns(base, cand, 0.001)
	if cmp.Regressed != 1 {
		t.Fatalf("expected 1 regression, got %d", cmp.Regressed)
	}
	if !cmp.HasRegressions() {
		t.Fatal("expected HasRegressions true")
	}
}

func TestCompareDetectsImprovement(t *testing.T) {
	base := Run{ID: "base", Results: []Result{{CaseID: "a", Score: 0, Weight: 1}}}
	cand := Run{ID: "cand", Results: []Result{{CaseID: "a", Score: 1, Weight: 1}}}
	cmp := CompareRuns(base, cand, 0.001)
	if cmp.Improved != 1 || cmp.HasRegressions() {
		t.Fatalf("expected improvement, got improved=%d regressed=%d", cmp.Improved, cmp.Regressed)
	}
}
