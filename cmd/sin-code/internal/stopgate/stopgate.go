// SPDX-License-Identifier: MIT
// Purpose: Stop-Gate — the independent harness that decides whether a goal is
// truly done, decoupling completion authority from the worker (2026 agent-loop
// best practice). The worker only PROPOSES completion (no more tool calls and
// the verify-gate passed); this gate CONFIRMS it against a GoalContract, or
// forces the loop to keep working with the open criteria injected back.
//
// Mode: HYBRID. Deterministic checks run first (build/test/lint/predicate/
// diff-scope, reused from internal/orchestrator) — they are reproducible and
// free. Only if they pass does the strong LLM judge (internal/eval) evaluate
// the non-mechanical semantic criteria. The judge can NEVER turn a
// deterministically-red result green; it can only reject a green one.
//
// Docs: stopgate.doc.md
package stopgate

import (
	"context"
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

// Judge is the subset of *eval.Judge the stop-gate needs. Declared as an
// interface so tests can inject a fake without an HTTP server and so a nil
// judge cleanly disables semantic evaluation.
type Judge interface {
	Evaluate(ctx context.Context, traj eval.Trajectory) (*eval.JudgeResult, error)
}

// Hybrid is the default StopGate: deterministic checks, then (optionally) a
// semantic LLM judge. The zero value is unusable — construct with New.
type Hybrid struct {
	workspace string
	verifier  *orchestrator.Verifier
	judge     Judge
	// failOpenOnJudgeError, when true (default), accepts completion if the
	// judge call itself errors AFTER deterministic checks passed — a flaky
	// evaluator network must never trap the loop forever. Deterministic
	// failures are always fail-closed regardless of this flag.
	failOpenOnJudgeError bool
}

// Option configures a Hybrid.
type Option func(*Hybrid)

// WithJudge attaches a semantic LLM judge. Without it, the gate is purely
// deterministic (still a strict superset of the verify-gate).
func WithJudge(j Judge) Option { return func(h *Hybrid) { h.judge = j } }

// WithFailClosedOnJudgeError makes a judge infrastructure error block
// completion instead of allowing it. Use only when an unreachable evaluator
// should hard-stop the run rather than degrade to deterministic-only.
func WithFailClosedOnJudgeError() Option {
	return func(h *Hybrid) { h.failOpenOnJudgeError = false }
}

// New builds a Hybrid gate rooted at workspace.
func New(workspace string, opts ...Option) *Hybrid {
	h := &Hybrid{
		workspace:            workspace,
		verifier:             orchestrator.NewVerifier(workspace),
		failOpenOnJudgeError: true,
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Evaluate runs the hybrid decision for one contract + run snapshot.
func (h *Hybrid) Evaluate(ctx context.Context, contract goalcontract.GoalContract, snap agentloop.StopSnapshot) agentloop.StopDecision {
	// (1) Deterministic checks — authoritative, fail-closed. For a
	// Definition-of-Done gate EVERY failed check must block completion, not
	// just the orchestrator's mandatory kinds (build/test): a failing
	// predicate like "no-new-todos" or a custom "done-when" is just as
	// disqualifying. We therefore inspect each result rather than trusting
	// the weighted Verdict.Passed.
	if len(contract.DeterministicChecks) > 0 {
		verdict := h.verifier.Verify(ctx, snap.SessionID, "stopgate", contract.DeterministicChecks)
		if open := failedCheckNames(verdict); len(open) > 0 {
			return agentloop.StopDecision{
				Complete:     false,
				OpenCriteria: open,
				Report:       verdict.Diagnosis(),
			}
		}
	}

	// (2) Semantic criteria — only reached when deterministic checks are
	// green. The judge may reject (Complete=false) but cannot resurrect a
	// failed deterministic result, because we already returned above.
	if h.judge != nil && len(contract.SemanticCriteria) > 0 {
		traj := eval.Trajectory{
			Prompt:         snap.Prompt,
			Turns:          snap.Turns,
			ToolsUsed:      snap.ToolsUsed,
			VerifyPassed:   snap.VerifyPassed,
			FinalOutput:    snap.FinalOutput,
			SessionID:      snap.SessionID,
			CustomCriteria: strings.Join(contract.SemanticCriteria, "\n- "),
		}
		result, err := h.judge.Evaluate(ctx, traj)
		if err != nil {
			if h.failOpenOnJudgeError {
				return agentloop.StopDecision{
					Complete: true,
					Report:   "stop-gate: deterministic checks passed; semantic judge unavailable (" + err.Error() + ") — accepted",
				}
			}
			return agentloop.StopDecision{
				Complete:     false,
				OpenCriteria: []string{"semantic evaluation could not be completed: " + err.Error()},
				Report:       "stop-gate: judge error (fail-closed)",
			}
		}
		if !result.Pass {
			return agentloop.StopDecision{
				Complete:     false,
				OpenCriteria: judgeOpenCriteria(result),
				Report:       fmt.Sprintf("stop-gate semantic reject (score %.2f): %s", result.Score, result.Reason),
			}
		}
	}

	return agentloop.StopDecision{
		Complete: true,
		Report:   "stop-gate: all acceptance criteria satisfied",
	}
}

// LoopGate adapts the Hybrid into an agentloop.StopGate bound to one contract,
// ready to assign to Loop.StopGate.
func (h *Hybrid) LoopGate(contract goalcontract.GoalContract) agentloop.StopGate {
	return func(ctx context.Context, snap agentloop.StopSnapshot) agentloop.StopDecision {
		return h.Evaluate(ctx, contract, snap)
	}
}

// failedCheckNames returns one entry per FAILED deterministic check. An empty
// slice means every check passed — the gate proceeds to semantic evaluation.
func failedCheckNames(v *orchestrator.Verdict) []string {
	var out []string
	for _, r := range v.Results {
		if !r.Passed {
			name := r.Check.Name
			if name == "" {
				name = string(r.Check.Kind)
			}
			out = append(out, "deterministic check failed: "+name)
		}
	}
	return out
}

func judgeOpenCriteria(r *eval.JudgeResult) []string {
	var out []string
	if strings.TrimSpace(r.Reason) != "" {
		out = append(out, r.Reason)
	}
	if strings.TrimSpace(r.Feedback) != "" {
		out = append(out, r.Feedback)
	}
	if len(out) == 0 {
		out = []string{"semantic acceptance criteria not met"}
	}
	return out
}
