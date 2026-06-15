// SPDX-License-Identifier: MIT
// Purpose: bridge between evalharness and hooklife. The verify engine
// is the natural default Subject — it already decides pass/fail for a
// workdir, which is exactly what `Output.Success` means.
// Docs: eval.doc.md
package wiring

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
)

// verifySubject wraps a hooklife.Verifier so an eval case is scored
// by whether the verification gate passes for the workdir named in
// the case prompt.
type verifySubject struct {
	verifier hooklife.Verifier
}

func (s verifySubject) Run(ctx context.Context, c evalharness.EvalCase) (evalharness.Output, error) {
	if s.verifier == nil {
		return evalharness.Output{Success: false, Text: "no verifier wired"}, nil
	}
	workdir := c.Meta["workdir"]
	if workdir == "" {
		workdir = c.Prompt // allow prompt to carry the path
	}
	passed, report, err := s.verifier.QualityGate(ctx, workdir)
	if err != nil {
		return evalharness.Output{}, err
	}
	return evalharness.Output{Text: report, Success: passed}, nil
}

// EvalSubjectFactory returns a factory for `sin eval run --subject <name>`.
// Extend the switch to add agent-run subjects, model-judge subjects, etc.
func EvalSubjectFactory(verifier hooklife.Verifier) func(string) (evalharness.Subject, evalharness.Scorer, error) {
	return func(name string) (evalharness.Subject, evalharness.Scorer, error) {
		switch name {
		case "verify", "gate":
			return verifySubject{verifier: verifier}, evalharness.SuccessFlag{}, nil
		default:
			// default: contains-based scoring against an agent run you wire later
			return verifySubject{verifier: verifier}, evalharness.SuccessFlag{}, nil
		}
	}
}
