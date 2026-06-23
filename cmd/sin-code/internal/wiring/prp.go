// SPDX-License-Identifier: MIT
// Purpose: bridge between prp.Verifier and hooklife.Verifier, plus
// a Deps assembler for the prp CLI.
// Docs: prp.doc.md
package wiring

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/prp"
)

// prpVerifier adapts a hooklife.Verifier (your internal/verify
// engine) to the prp.Verifier interface, so the PRP quality gate
// reuses real verification.
type prpVerifier struct{ v hooklife.Verifier }

func (a prpVerifier) Verify(ctx context.Context, workdir string) (bool, string, error) {
	if a.v == nil {
		return true, "", nil
	}
	return a.v.QualityGate(ctx, workdir)
}

// PRPDeps assembles prp.Deps from existing subsystems. Planner,
// Implementer and PR must be implemented on your agent/orchestrator/
// git layer.
func PRPDeps(verifier hooklife.Verifier, planner prp.Planner, impl prp.Implementer, pr prp.PRController) prp.Deps {
	return prp.Deps{
		Planner:     planner,
		Implementer: impl,
		Verifier:    prpVerifier{v: verifier},
		PR:          pr,
	}
}
