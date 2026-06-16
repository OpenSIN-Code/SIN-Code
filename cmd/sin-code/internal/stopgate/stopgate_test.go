// SPDX-License-Identifier: MIT
// Purpose: tests for the stop-gate harness — the independent completion
// authority. Covers deterministic fail-closed behavior, semantic judging,
// judge-error fail-open/closed policy, and the precedence rule that a green
// judge can never override a red deterministic check (AGENTS.md §8).
package stopgate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

// fakeJudge is an in-memory Judge for tests — no HTTP server needed.
type fakeJudge struct {
	result *eval.JudgeResult
	err    error
	calls  int
}

func (f *fakeJudge) Evaluate(ctx context.Context, traj eval.Trajectory) (*eval.JudgeResult, error) {
	f.calls++
	return f.result, f.err
}

func passPredicate(name string) orchestrator.Check {
	return orchestrator.Check{Kind: orchestrator.CheckPredicate, Name: name, Cmd: []string{"true"}}
}

func failPredicate(name string) orchestrator.Check {
	return orchestrator.Check{Kind: orchestrator.CheckPredicate, Name: name, Cmd: []string{"false"}}
}

func snap() agentloop.StopSnapshot {
	return agentloop.StopSnapshot{Prompt: "do x", FinalOutput: "done", Turns: 3, VerifyPassed: true, SessionID: "s1"}
}

func TestEmptyContractCompletes(t *testing.T) {
	g := New(t.TempDir())
	dec := g.Evaluate(context.Background(), goalcontract.GoalContract{}, snap())
	if !dec.Complete {
		t.Fatal("empty contract should accept completion")
	}
}

func TestDeterministicPassNoJudge(t *testing.T) {
	g := New(t.TempDir())
	c := goalcontract.GoalContract{DeterministicChecks: []orchestrator.Check{passPredicate("p1")}}
	dec := g.Evaluate(context.Background(), c, snap())
	if !dec.Complete {
		t.Fatalf("all-pass deterministic should complete, got %+v", dec)
	}
}

func TestDeterministicFailBlocks(t *testing.T) {
	g := New(t.TempDir())
	c := goalcontract.GoalContract{DeterministicChecks: []orchestrator.Check{
		passPredicate("ok"), failPredicate("broken"),
	}}
	dec := g.Evaluate(context.Background(), c, snap())
	if dec.Complete {
		t.Fatal("a failing predicate must block completion")
	}
	if len(dec.OpenCriteria) == 0 {
		t.Fatal("expected open criteria naming the failed check")
	}
}

// A failing non-mandatory predicate must block even though the orchestrator's
// weighted Verdict.Passed would be true — this is the DoD fail-closed rule.
func TestFailingPredicateBlocksDespiteWeightedPass(t *testing.T) {
	g := New(t.TempDir())
	c := goalcontract.GoalContract{DeterministicChecks: []orchestrator.Check{failPredicate("done-when")}}
	dec := g.Evaluate(context.Background(), c, snap())
	if dec.Complete {
		t.Fatal("failing predicate must fail the gate (fail-closed)")
	}
}

func TestSemanticJudgeReject(t *testing.T) {
	j := &fakeJudge{result: &eval.JudgeResult{Pass: false, Score: 0.4, Reason: "docs missing"}}
	g := New(t.TempDir(), WithJudge(j))
	c := goalcontract.GoalContract{
		DeterministicChecks: []orchestrator.Check{passPredicate("p")},
		SemanticCriteria:    []string{"docs updated"},
	}
	dec := g.Evaluate(context.Background(), c, snap())
	if dec.Complete {
		t.Fatal("judge reject should block completion")
	}
	if j.calls != 1 {
		t.Fatalf("judge should be called once, got %d", j.calls)
	}
}

func TestSemanticJudgeAccept(t *testing.T) {
	j := &fakeJudge{result: &eval.JudgeResult{Pass: true, Score: 0.9}}
	g := New(t.TempDir(), WithJudge(j))
	c := goalcontract.GoalContract{
		DeterministicChecks: []orchestrator.Check{passPredicate("p")},
		SemanticCriteria:    []string{"docs updated"},
	}
	dec := g.Evaluate(context.Background(), c, snap())
	if !dec.Complete {
		t.Fatalf("judge accept should complete, got %+v", dec)
	}
}

// The judge must never be consulted when a deterministic check already failed:
// a green judge cannot resurrect a red mechanical result.
func TestJudgeNotConsultedWhenDeterministicFails(t *testing.T) {
	j := &fakeJudge{result: &eval.JudgeResult{Pass: true, Score: 1.0}}
	g := New(t.TempDir(), WithJudge(j))
	c := goalcontract.GoalContract{
		DeterministicChecks: []orchestrator.Check{failPredicate("broken")},
		SemanticCriteria:    []string{"looks good"},
	}
	dec := g.Evaluate(context.Background(), c, snap())
	if dec.Complete {
		t.Fatal("deterministic failure must win over a green judge")
	}
	if j.calls != 0 {
		t.Fatalf("judge must not be consulted after deterministic failure, calls=%d", j.calls)
	}
}

func TestJudgeErrorFailOpenByDefault(t *testing.T) {
	j := &fakeJudge{err: errors.New("network down")}
	g := New(t.TempDir(), WithJudge(j))
	c := goalcontract.GoalContract{
		DeterministicChecks: []orchestrator.Check{passPredicate("p")},
		SemanticCriteria:    []string{"x"},
	}
	dec := g.Evaluate(context.Background(), c, snap())
	if !dec.Complete {
		t.Fatal("default policy is fail-open on judge infra error after deterministic pass")
	}
}

func TestJudgeErrorFailClosedOption(t *testing.T) {
	j := &fakeJudge{err: errors.New("network down")}
	g := New(t.TempDir(), WithJudge(j), WithFailClosedOnJudgeError())
	c := goalcontract.GoalContract{
		DeterministicChecks: []orchestrator.Check{passPredicate("p")},
		SemanticCriteria:    []string{"x"},
	}
	dec := g.Evaluate(context.Background(), c, snap())
	if dec.Complete {
		t.Fatal("fail-closed option must block on judge error")
	}
}

func TestLoopGateAdapter(t *testing.T) {
	g := New(t.TempDir())
	gate := g.LoopGate(goalcontract.GoalContract{})
	dec := gate(context.Background(), snap())
	if !dec.Complete {
		t.Fatal("loop gate adapter should pass through Evaluate")
	}
}

// Sanity: a real-ish predicate against the filesystem.
func TestDoneWhenAgainstFile(t *testing.T) {
	ws := t.TempDir()
	marker := filepath.Join(ws, "done.txt")
	g := New(ws)
	check := orchestrator.Check{
		Kind: orchestrator.CheckPredicate, Name: "done-when",
		Cmd: []string{"sh", "-c", "test -f " + marker},
	}
	c := goalcontract.GoalContract{DeterministicChecks: []orchestrator.Check{check}}

	if dec := g.Evaluate(context.Background(), c, snap()); dec.Complete {
		t.Fatal("done-when should fail before the marker exists")
	}
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dec := g.Evaluate(context.Background(), c, snap()); !dec.Complete {
		t.Fatal("done-when should pass once the marker exists")
	}
}
