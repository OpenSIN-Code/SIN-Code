// SPDX-License-Identifier: MIT
// Purpose: tests for the self-authoring loop. Uses a stub Completer
// to drive the Planner->Implementer->Drift-check cycle without a
// real LLM. Covers the dry-run path, the parse-error retry, the
// drift retry, and the convergence path.
// Docs: docs/SPEC-LAYER.md §"Self-authoring"
package spec

import (
	"context"
	"strings"
	"testing"
	"time"
)

// scriptCompleter returns a fixed script of LLM outputs in order.
// Each call to Complete() consumes the next entry. Lets us drive the
// loop deterministically through Planner/Implementer failures and
// successes without a real model.
type scriptCompleter struct {
	outs []string
	i    int
}

func (s *scriptCompleter) Complete(_ context.Context, _, _ string) (string, error) {
	if s.i >= len(s.outs) {
		return "", nil
	}
	out := s.outs[s.i]
	s.i++
	return out, nil
}

func TestAuthor_DryRun_NoCompleter(t *testing.T) {
	res, err := Author(context.Background(), "add a button", AuthorOptions{})
	if err != nil {
		t.Fatalf("Author: %v", err)
	}
	if res.Spec == nil {
		t.Fatal("expected non-nil Spec on dry-run")
	}
	if res.Attempts != 0 {
		t.Fatalf("dry-run should have 0 attempts, got %d", res.Attempts)
	}
	if !strings.Contains(res.Spec.Title, "add a button") {
		t.Fatalf("title: %q", res.Spec.Title)
	}
}

func TestAuthor_ConvergeFirstTry(t *testing.T) {
	plannerOut := `# Demo

## Objective
demo

## Requirements
- [must] R1: x

## Acceptance Criteria
- A1: ok   ` + "`verify: true`" + `
`
	implOut := "package x\n" // empty package, no code needed; verify: true exits 0

	c := &scriptCompleter{outs: []string{plannerOut, implOut}}
	res, err := Author(context.Background(), "demo", AuthorOptions{Completer: c})
	if err != nil {
		t.Fatalf("Author: %v", err)
	}
	if res.Spec == nil {
		t.Fatal("expected spec on convergence")
	}
	if res.Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", res.Attempts)
	}
}

func TestAuthor_RetryOnDrift(t *testing.T) {
	// First attempt: planner OK, but the spec has a failing criterion
	// (verify: false exits 1). The loop should retry.
	plannerOut := `# Demo

## Objective
demo

## Requirements
- [must] R1: x

## Acceptance Criteria
- A1: fails   ` + "`verify: false`" + `
`
	// Second attempt: planner produces a spec that passes.
	plannerOut2 := `# Demo

## Objective
demo

## Requirements
- [must] R1: x

## Acceptance Criteria
- A1: ok   ` + "`verify: true`" + `
`
	implOut := "package x"

	c := &scriptCompleter{outs: []string{plannerOut, implOut, plannerOut2, implOut}}
	res, err := Author(context.Background(), "demo", AuthorOptions{Completer: c, MaxRetries: 3})
	if err != nil {
		t.Fatalf("Author: %v", err)
	}
	if res.Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d (trace: %+v)", res.Attempts, res.Trace)
	}
}

func TestAuthor_GiveUpAfterMaxRetries(t *testing.T) {
	plannerOut := `# Demo

## Objective
demo

## Requirements
- [must] R1: x

## Acceptance Criteria
- A1: always fails   ` + "`verify: false`" + `
`
	// Same failing spec on every retry.
	c := &scriptCompleter{outs: []string{
		plannerOut, "x",
		plannerOut, "x",
		plannerOut, "x",
	}}
	res, err := Author(context.Background(), "demo", AuthorOptions{Completer: c, MaxRetries: 3})
	if err == nil {
		t.Fatal("expected give-up error")
	}
	if res.Spec != nil {
		t.Fatal("expected nil spec on give-up")
	}
	if len(res.Trace) != 3 {
		t.Fatalf("expected 3 trace steps, got %d", len(res.Trace))
	}
}

func TestAuthor_RetryOnParseError(t *testing.T) {
	// First attempt: invalid spec (no sections). The loop's
	// `continue` after parse-error means the Implementer is NOT
	// called on this attempt, so the second completer call is
	// Attempt 2's Planner.
	bad := "this is not a valid spec at all"
	good := `# Demo

## Objective
demo

## Requirements
- [must] R1: x

## Acceptance Criteria
- A1: ok   ` + "`verify: true`" + `
`
	// Order of completions:
	//   attempt 1: Planner (bad), parse-error -> continue
	//   attempt 2: Planner (good), Implementer (impl)
	c := &scriptCompleter{outs: []string{bad, good, "impl"}}
	res, err := Author(context.Background(), "demo", AuthorOptions{Completer: c, MaxRetries: 3})
	if err != nil {
		t.Fatalf("Author: %v", err)
	}
	if res.Attempts != 2 {
		t.Fatalf("expected 2 attempts (parse-error retry), got %d", res.Attempts)
	}
}

func TestAuthor_EmptyDescription(t *testing.T) {
	_, err := Author(context.Background(), "", AuthorOptions{})
	if err == nil {
		t.Fatal("expected error on empty description")
	}
}

func TestAuthor_TimeoutEnforced(t *testing.T) {
	// Completer that respects context cancellation but never returns
	// a value — simulates a hung LLM call. The Author loop should
	// time out via the per-call timeout in opts.
	slow := &blockingCompleter{ch: make(chan string)} // unbuffered: never delivers
	res, err := Author(context.Background(), "demo",
		AuthorOptions{Completer: slow, Timeout: 50 * time.Millisecond, MaxRetries: 1})
	if err == nil {
		t.Fatalf("expected timeout error, got result: %+v", res)
	}
	if !strings.Contains(err.Error(), "planner") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected timeout-related error, got: %v", err)
	}
}

type blockingCompleter struct{ ch chan string }

func (b *blockingCompleter) Complete(ctx context.Context, _, _ string) (string, error) {
	select {
	case out := <-b.ch:
		return out, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
