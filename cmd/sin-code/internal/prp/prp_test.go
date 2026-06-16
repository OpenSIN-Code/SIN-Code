// SPDX-License-Identifier: MIT
// Purpose: tests for the PRP engine — full pipeline, verification
// failure kick-back, blocked tasks, persistence round-trip.
// Docs: prp_test.doc.md
package prp

import (
	"context"
	"testing"
)

type fakePlanner struct{}

func (fakePlanner) Plan(_ context.Context, goal, _ string) ([]Task, string, error) {
	return []Task{{Title: "step one"}, {Title: "step two"}}, "plan for " + goal, nil
}

type fakeImpl struct{ fail string }

func (f fakeImpl) Implement(_ context.Context, _ *PRP, t Task) (string, error) {
	if t.Title == f.fail {
		return "", context.DeadlineExceeded
	}
	return "implemented " + t.Title, nil
}

type fakeVerifier struct{ pass bool }

func (f fakeVerifier) Verify(_ context.Context, _ string) (bool, string, error) {
	return f.pass, "report", nil
}

type fakePR struct{}

func (fakePR) OpenPR(_ context.Context, _ *PRP) (string, error) { return "http://pr/1", nil }

func newEngine(t *testing.T, impl Implementer, v Verifier) *Engine {
	t.Helper()
	return &Engine{
		Store: NewStore(t.TempDir()), Workdir: t.TempDir(),
		Planner: fakePlanner{}, Implementer: impl, Verifier: v, PR: fakePR{},
	}
}

func TestFullPipelineSucceeds(t *testing.T) {
	e := newEngine(t, fakeImpl{}, fakeVerifier{pass: true})
	p, _ := e.New("x", "X", "do the thing", "")
	if err := e.RunAll(context.Background(), p); err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if p.Phase != PhaseShipped {
		t.Fatalf("expected shipped, got %s", p.Phase)
	}
	if !p.AllDone() {
		t.Fatal("expected all tasks done")
	}
}

func TestVerificationFailureStops(t *testing.T) {
	e := newEngine(t, fakeImpl{}, fakeVerifier{pass: false})
	p, _ := e.New("x", "X", "goal", "")
	err := e.RunAll(context.Background(), p)
	if err == nil {
		t.Fatal("expected verification failure error")
	}
	if p.Phase != PhaseImplementing {
		t.Fatalf("expected kicked back to implementing, got %s", p.Phase)
	}
}

func TestBlockedTask(t *testing.T) {
	e := newEngine(t, fakeImpl{fail: "step two"}, fakeVerifier{pass: true})
	p, _ := e.New("x", "X", "goal", "")
	_ = e.RunPlan(context.Background(), p)
	err := e.RunImplement(context.Background(), p)
	if err == nil {
		t.Fatal("expected blocked task error")
	}
	var blocked bool
	for _, task := range p.Tasks {
		if task.State == TaskBlocked {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("expected a blocked task")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	e := newEngine(t, fakeImpl{}, fakeVerifier{pass: true})
	p, _ := e.New("roundtrip", "Round Trip", "goal", "some context")
	_ = e.RunPlan(context.Background(), p)

	loaded, err := e.Store.Load(p.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Title != p.Title || len(loaded.Tasks) != len(p.Tasks) || loaded.Phase != p.Phase {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", loaded, p)
	}
}
