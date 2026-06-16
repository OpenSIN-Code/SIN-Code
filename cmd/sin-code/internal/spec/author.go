// SPDX-License-Identifier: MIT
// Purpose: LLM-backed self-authoring for the Spec-Layer. Implements
// the Planner -> Implementer -> Drift-check loop with up to 3
// retries on mismatch. The Completer interface keeps the package
// dependency-free; wire it to a real model client via internal/wiring.
// Docs: docs/SPEC-LAYER.md §"Self-authoring"
package spec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Completer is the minimal interface a model client must satisfy to
// drive the self-authoring loop. The instinct package has the same
// shape (intentionally) so a single model adapter can serve both.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// AuthorOptions controls one run of Author. Zero value is fully usable
// with safe defaults; production wires a real Completer and a longer
// timeout.
type AuthorOptions struct {
	Completer  Completer     // nil = dry-run (returns a stub spec for testing)
	Timeout    time.Duration // per-call; default 60s
	MaxRetries int           // default 3
	Workdir    string        // for the working tree the implementer sees
}

// AuthorResult is the outcome of one Author call. Either the loop
// converged (Spec != nil, Attempts > 0) or it gave up (Err != nil).
type AuthorResult struct {
	Spec     *Spec
	Attempts int
	Trace    []AuthorStep // per-attempt trace for the audit log
}

// AuthorStep is one iteration of the loop.
type AuthorStep struct {
	Attempt        int
	PlannerOut     string // raw planner output (for debugging)
	ImplementerOut string
	Drift          *CheckReport // nil if the implementer didn't change the tree
	Verdict        string       // "ok" | "drift" | "planner-empty" | "implementer-empty"
}

// Author runs the self-authoring loop for desc. The loop:
//
//  1. Planner LLM call: produce a *.spec.md from desc
//  2. Implementer LLM call: write code that satisfies the spec
//  3. Drift check: run every criterion's verify: command
//  4. On must-priority failure: feed the failing criterion back to
//     the Implementer LLM and retry (up to MaxRetries).
//
// The returned Spec is the version that passed. Trace is non-nil even
// on failure so the operator can see what the loop tried.
func Author(ctx context.Context, desc string, opts AuthorOptions) (*AuthorResult, error) {
	if desc == "" {
		return nil, errors.New("spec: author: description is empty")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	res := &AuthorResult{Trace: make([]AuthorStep, 0, opts.MaxRetries)}

	// Dry-run fallback: no Completer, return a stub spec the operator
	// can hand-edit. Useful for end-to-end testing of the surrounding
	// pipeline without a real LLM.
	if opts.Completer == nil {
		s := stubSpec(desc)
		res.Spec = s
		res.Attempts = 0
		res.Trace = append(res.Trace, AuthorStep{
			Attempt: 0, Verdict: "stub-no-completer",
			PlannerOut: "(no Completer wired; returned stub)",
		})
		return res, nil
	}

	specText := ""
	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		step := AuthorStep{Attempt: attempt}

		// Per-call context with the configured timeout. The Completer
		// must respect ctx.Done(); blockingCompleter in author_test.go
		// does so explicitly.
		cctx, cancel := context.WithTimeout(ctx, opts.Timeout)

		// Step 1: Planner. Produce a *.spec.md from desc.
		plannerOut, err := opts.Completer.Complete(cctx, plannerSystem, plannerUser(desc))
		cancel()
		if err != nil {
			return res, fmt.Errorf("spec: author: planner (attempt %d): %w", attempt, err)
		}
		step.PlannerOut = plannerOut
		specText = strings.TrimSpace(plannerOut)
		if specText == "" {
			step.Verdict = "planner-empty"
			res.Trace = append(res.Trace, step)
			continue
		}

		// Step 2: parse the spec. If the LLM produced invalid markdown,
		// ask the planner to fix it. The error is fed back next attempt.
		s, perr := Parse(specText)
		parseErr := perr
		if parseErr == nil && (len(s.Requirements) == 0 || len(s.Criteria) == 0) {
			parseErr = fmt.Errorf("spec has %d requirements and %d criteria; need at least 1 of each",
				len(s.Requirements), len(s.Criteria))
		}
		if parseErr != nil {
			step.Verdict = "parse-error"
			res.Trace = append(res.Trace, step)
			desc = desc + "\n\nThe previous spec draft was unusable:\n" + parseErr.Error() +
				"\n\nProduce a valid *.spec.md with the exact section structure " +
				"(\"## Objective\", \"## Requirements\", \"## Acceptance Criteria\"), " +
				"at least one requirement and one criterion."
			continue
		}

		// Step 3: Implementer. Ask the LLM to write code that satisfies
		// the spec. (Wired externally in production; here we only
		// capture the output. The actual code-write side-effect is
		// the operator's responsibility via the ghbridge.)
		cctx2, cancel2 := context.WithTimeout(ctx, opts.Timeout)
		implOut, ierr := opts.Completer.Complete(cctx2, implementerSystem, implementerUser(s))
		cancel2()
		if ierr != nil {
			return res, fmt.Errorf("spec: author: implementer (attempt %d): %w", attempt, err)
		}
		step.ImplementerOut = implOut

		// Step 4: Drift check. Run every criterion's verify: command.
		// In a real run this would execute against the workdir; in PR 2
		// we check what we have: a spec with no working tree means
		// the implementer step is the only one that runs.
		rep, cerr := s.Check(ctx, opts.Timeout)
		if cerr != nil {
			step.Verdict = "check-error"
			res.Trace = append(res.Trace, step)
			continue
		}
		step.Drift = rep

		if !rep.HasFailures() {
			step.Verdict = "ok"
			res.Trace = append(res.Trace, step)
			res.Spec = s
			res.Attempts = attempt
			return res, nil
		}
		step.Verdict = "drift"
		res.Trace = append(res.Trace, step)

		// Retry: feed the failing criteria back to the implementer.
		desc = desc + "\n\nFailing criteria:\n" + summarizeFailing(rep)
	}
	return res, fmt.Errorf("spec: author: gave up after %d attempts; see Trace", opts.MaxRetries)
}

// summarizeFailing renders a human-readable list of the failing
// must-priority criteria, suitable for feeding back to the LLM.
func summarizeFailing(rep *CheckReport) string {
	var b strings.Builder
	for _, r := range rep.Results {
		if r.Skipped || r.Passed || r.Priority != Must {
			continue
		}
		fmt.Fprintf(&b, "  - %s: %s\n    command: %s\n    last output: %s\n",
			r.ID, r.Text, r.Command, truncateForLLM(r.Output, 200))
	}
	return b.String()
}

func truncateForLLM(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "... (truncated)"
}

// stubSpec produces a minimal valid spec for the dry-run path. It
// has a single must-priority criterion that just echoes the description
// (which the implementer step can then satisfy trivially).
func stubSpec(desc string) *Spec {
	firstLine := desc
	if i := strings.IndexByte(desc, '\n'); i >= 0 {
		firstLine = strings.TrimSpace(desc[:i])
	}
	if len(firstLine) > 60 {
		firstLine = firstLine[:60] + "..."
	}
	body := fmt.Sprintf(`# %s

## Objective

%s

## Requirements

- [must] R1: The implementation must be functional

## Acceptance Criteria

- A1: stub passes   `+"`verify: true`"+`
`, firstLine, desc)
	return &Spec{
		Title:     firstLine,
		Objective: desc,
		Criteria: []Criterion{
			{ID: "A1", Text: "stub passes", Verify: "true"},
		},
		Raw: body,
	}
}

// --- LLM prompt templates (constants; not configurable in PR 2) ---

const plannerSystem = `You are a Spec-Layer Planner. You write *.spec.md files that capture a contract a code change must satisfy.

Output STRICTLY in this format:

# <Title>

## Objective
<one-screen summary of the change>

## Requirements
- [must] R1: <requirement 1>
- [should] R2: <requirement 2>
- [may] R3: <requirement 3>

## Acceptance Criteria
- A1: <criterion text>  ` + "`verify: <shell command that exits 0 on pass>`" + `
- A2: <criterion text>  ` + "`verify: <shell command>`" + `

Rules:
- max 7 requirements, max 7 criteria
- every criterion must have a backtick-wrapped verify: command
- the verify: commands must be runnable in a shell
- no project-specific names; this spec is the template`

const implementerSystem = `You are a Spec-Layer Implementer. You receive a parsed spec and write the minimal code that satisfies it.

Output a single Go file (or set of files) as a code block. No prose, no markdown. The code must pass every criterion's verify: command.

If a previous attempt failed, you also receive the failing criteria and the last output — fix exactly those.`

func plannerUser(desc string) string {
	return "Write a *.spec.md for the following change:\n\n" + desc
}

func implementerUser(s *Spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Implement the following spec:\n\n# %s\n\n## Objective\n%s\n",
		s.Title, s.Objective)
	b.WriteString("## Requirements\n")
	for _, r := range s.Requirements {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", r.Priority, r.ID, r.Text)
	}
	b.WriteString("\n## Acceptance Criteria\n")
	for _, c := range s.Criteria {
		fmt.Fprintf(&b, "- %s: %s  `verify: %s`\n", c.ID, c.Text, c.Verify)
	}
	if len(s.Invariants) > 0 {
		b.WriteString("\n## Invariants\n")
		for _, inv := range s.Invariants {
			fmt.Fprintf(&b, "- %s\n", inv)
		}
	}
	return b.String()
}
