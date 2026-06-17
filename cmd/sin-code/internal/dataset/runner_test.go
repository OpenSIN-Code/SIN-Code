// SPDX-License-Identifier: MIT
// Purpose: dataset runner tests (issue #75).
package dataset

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

func newTestStore(t *testing.T) *session.Store {
	t.Helper()
	s, err := session.Open(filepath.Join(t.TempDir(), "sess.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestLoop(result *agentloop.Result, err error) *agentloop.Loop {
	return &agentloop.Loop{
		RunOverride: func(context.Context, *session.Session, string) (*agentloop.Result, error) {
			return result, err
		},
	}
}

func TestNewRunner_NilLoop(t *testing.T) {
	_, err := NewRunner(RunnerConfig{}, nil, newTestStore(t))
	if err == nil || !strings.Contains(err.Error(), "loop is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRunner_NilStore(t *testing.T) {
	_, err := NewRunner(RunnerConfig{}, newTestLoop(&agentloop.Result{}, nil), nil)
	if err == nil || !strings.Contains(err.Error(), "session store is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRunner_Defaults(t *testing.T) {
	r, err := NewRunner(RunnerConfig{}, newTestLoop(&agentloop.Result{}, nil), newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.cfg.MaxConcurrency != 1 {
		t.Fatalf("MaxConcurrency: got %d, want 1", r.cfg.MaxConcurrency)
	}
	if r.cfg.TimeoutPerCase != 5*time.Minute {
		t.Fatalf("TimeoutPerCase: got %v, want 5m", r.cfg.TimeoutPerCase)
	}
	if !r.cfg.HeadlessMode {
		t.Fatal("HeadlessMode should be true")
	}
	if r.cfg.VerifyMode != "poc" {
		t.Fatalf("VerifyMode: got %q, want poc", r.cfg.VerifyMode)
	}
}

func TestRunDataset_NilDataset(t *testing.T) {
	r, err := NewRunner(RunnerConfig{}, newTestLoop(&agentloop.Result{}, nil), newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.RunDataset(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "nil dataset") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDataset_Serial(t *testing.T) {
	ds := &Dataset{
		Name: "x", Version: "1.0",
		TestCases: []TestCase{
			{ID: "a", Prompt: "p1"},
			{ID: "b", Prompt: "p2"},
		},
	}
	r, err := NewRunner(RunnerConfig{}, newTestLoop(&agentloop.Result{Summary: "ok", Verified: true}, nil), newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	results, err := r.RunDataset(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Success || !results[1].Success {
		t.Fatalf("expected both successes: %+v", results)
	}
}

func TestRunCase_Success(t *testing.T) {
	r, err := NewRunner(RunnerConfig{}, newTestLoop(&agentloop.Result{Summary: "ok", Verified: true, Turns: 3}, nil), newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p"})
	if !res.Success {
		t.Fatalf("expected success: %+v", res)
	}
	if res.Turns != 3 || res.FinalOutput != "ok" || !res.VerifyPassed {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunCase_Error(t *testing.T) {
	loop := newTestLoop(&agentloop.Result{Summary: "partial", Verified: false, Turns: 2}, errors.New("loop failed"))
	r, err := NewRunner(RunnerConfig{}, loop, newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p"})
	if res.Success {
		t.Fatalf("expected failure: %+v", res)
	}
	if !strings.Contains(res.Error, "loop failed") {
		t.Fatalf("expected loop error, got %q", res.Error)
	}
	if res.Turns != 2 || res.FinalOutput != "partial" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunCase_SessionCreateError(t *testing.T) {
	store := newTestStore(t)
	store.Close() // force StartOrResume to fail
	r, err := NewRunner(RunnerConfig{}, newTestLoop(&agentloop.Result{}, nil), store)
	if err != nil {
		t.Fatal(err)
	}
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p"})
	if res.Success || !strings.Contains(res.Error, "session create") {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunCase_TimeoutParsing(t *testing.T) {
	loop := &agentloop.Loop{
		RunOverride: func(ctx context.Context, _ *session.Session, _ string) (*agentloop.Result, error) {
			// The context should have a 1-second timeout.
			deadline, ok := ctx.Deadline()
			if !ok {
				return nil, errors.New("expected deadline")
			}
			if time.Until(deadline) > 2*time.Second {
				return nil, errors.New("deadline too far")
			}
			return &agentloop.Result{Summary: "ok", Verified: true}, nil
		},
	}
	r, err := NewRunner(RunnerConfig{}, loop, newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p", Constraints: Constraints{Timeout: "1s"}})
	if !res.Success {
		t.Fatalf("expected success: %+v", res)
	}
}

func TestRunCase_InvalidTimeoutUsesFallback(t *testing.T) {
	loop := &agentloop.Loop{
		RunOverride: func(ctx context.Context, _ *session.Session, _ string) (*agentloop.Result, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				return nil, errors.New("expected deadline")
			}
			if time.Until(deadline) > 35*time.Second || time.Until(deadline) < 25*time.Second {
				return nil, errors.New("expected 30s fallback")
			}
			return &agentloop.Result{Summary: "ok", Verified: true}, nil
		},
	}
	r, err := NewRunner(RunnerConfig{TimeoutPerCase: -1}, loop, newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p", Constraints: Constraints{Timeout: "not-a-duration"}})
	if !res.Success {
		t.Fatalf("expected success: %+v", res)
	}
}

func TestRunCase_TimedOut(t *testing.T) {
	loop := &agentloop.Loop{
		RunOverride: func(ctx context.Context, _ *session.Session, _ string) (*agentloop.Result, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return &agentloop.Result{Summary: "ok", Verified: true}, nil
			}
		},
	}
	r, err := NewRunner(RunnerConfig{TimeoutPerCase: 1 * time.Millisecond}, loop, newTestStore(t))
	if err != nil {
		t.Fatal(err)
	}
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p"})
	if !res.TimedOut {
		t.Fatalf("expected timed out: %+v", res)
	}
	if res.Success {
		t.Fatal("expected failure on timeout")
	}
}

func TestApplyRules_MaxTurns(t *testing.T) {
	r := &Runner{}
	res := RunResult{Turns: 5, FinalOutput: "out", Success: true}
	tc := TestCase{Constraints: Constraints{MaxTurns: 3}}
	out := r.applyRules(&tc, &res)
	if out.Success {
		t.Fatal("expected failure due to max turns")
	}
	if !strings.Contains(out.Error, "max_turns") {
		t.Fatalf("unexpected error: %q", out.Error)
	}
}

func TestApplyRules_RequireVerifyNotPassed(t *testing.T) {
	r := &Runner{}
	res := RunResult{VerifyPassed: false, Success: true}
	tc := TestCase{Constraints: Constraints{RequireVerify: true}}
	out := r.applyRules(&tc, &res)
	if out.Success || !strings.Contains(out.Error, "verify not passed") {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestApplyRules_MustUseTools(t *testing.T) {
	r := &Runner{}
	res := RunResult{ToolsUsed: []string{"tool_a"}, Success: true}
	tc := TestCase{Constraints: Constraints{MustUseTools: []string{"tool_a", "tool_b"}}}
	out := r.applyRules(&tc, &res)
	if out.Success || !strings.Contains(out.Error, "missing required tool: tool_b") {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestApplyRules_ForbiddenTools(t *testing.T) {
	r := &Runner{}
	res := RunResult{ToolsUsed: []string{"tool_x"}, Success: true}
	tc := TestCase{Constraints: Constraints{ForbiddenTools: []string{"tool_x"}}}
	out := r.applyRules(&tc, &res)
	if out.Success || !strings.Contains(out.Error, "used forbidden tool: tool_x") {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestApplyRules_OutputContains(t *testing.T) {
	r := &Runner{}
	res := RunResult{FinalOutput: "hello world", Success: true}
	tc := TestCase{Expected: Expected{OutputContains: []string{"missing"}}}
	out := r.applyRules(&tc, &res)
	if out.Success || !strings.Contains(out.Error, "missing output keyword") {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestApplyRules_OutputAvoids(t *testing.T) {
	r := &Runner{}
	res := RunResult{FinalOutput: "hello world", Success: true}
	tc := TestCase{Expected: Expected{OutputAvoids: []string{"world"}}}
	out := r.applyRules(&tc, &res)
	if out.Success || !strings.Contains(out.Error, "contains forbidden output keyword") {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestApplyRules_PreservesExistingError(t *testing.T) {
	r := &Runner{}
	res := RunResult{FinalOutput: "x", Success: true, Error: "first error"}
	tc := TestCase{Expected: Expected{OutputContains: []string{"missing"}}}
	out := r.applyRules(&tc, &res)
	if out.Success || out.Error != "first error" {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestApplyRules_MultipleViolations(t *testing.T) {
	r := &Runner{}
	res := RunResult{Turns: 10, FinalOutput: "bad output", Success: true}
	tc := TestCase{
		Constraints: Constraints{MaxTurns: 5},
		Expected:    Expected{OutputAvoids: []string{"bad"}},
	}
	out := r.applyRules(&tc, &res)
	if out.Success {
		t.Fatal("expected failure")
	}
	if !strings.Contains(out.Error, "turns=10 > max_turns=5") || !strings.Contains(out.Error, "contains forbidden output keyword") {
		t.Fatalf("expected both violations, got %q", out.Error)
	}
}

func TestContains_False(t *testing.T) {
	if contains([]string{"a", "b"}, "c") {
		t.Fatal("expected false")
	}
}
