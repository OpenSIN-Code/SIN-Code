// SPDX-License-Identifier: MIT
// Purpose: Targeted Verification — run only the affected test packages.
// Inner loop: fast targeted suite. Final gate: full suite. Soundness is
// preserved; only the inner loop iterates 10-50x faster.
package orchestrator

import (
	"context"
	"fmt"
	"time"
)

type TargetedVerifier struct {
	Inner *Verifier
	Graph *ImpactGraph
}

func NewTargetedVerifier(inner *Verifier, graph *ImpactGraph) *TargetedVerifier {
	return &TargetedVerifier{Inner: inner, Graph: graph}
}

func (tv *TargetedVerifier) FastChecks(changedFiles []string) []Check {
	imp := tv.Graph.Predict(changedFiles)

	checks := []Check{}
	if len(imp.AffectedPkgs) > 0 {
		checks = append(checks, Check{
			Kind:    CheckBuild,
			Name:    "build-affected",
			Cmd:     append([]string{"go", "build"}, imp.AffectedPkgs...),
			Timeout: 90 * time.Second,
		})
	} else {
		checks = append(checks, Check{
			Kind:    CheckBuild,
			Name:    "build-all",
			Cmd:     []string{"go", "build", "./..."},
			Timeout: 3 * time.Minute,
		})
	}
	if len(imp.AffectedTestPkgs) > 0 {
		checks = append(checks, Check{
			Kind:    CheckTest,
			Name:    fmt.Sprintf("test-affected(%d pkgs)", len(imp.AffectedTestPkgs)),
			Cmd:     append([]string{"go", "test", "-count=1", "-timeout=90s"}, imp.AffectedTestPkgs...),
			Timeout: 2 * time.Minute,
		})
	}
	return checks
}

func (tv *TargetedVerifier) FinalChecks() []Check {
	return DefaultGoChecks()
}

func (tv *TargetedVerifier) VerifyStaged(ctx context.Context, taskID, candidate string, changedFiles []string) *Verdict {
	fast := tv.Inner.Verify(ctx, taskID, candidate+"@fast", tv.FastChecks(changedFiles))
	if !fast.Passed {
		return fast
	}
	return tv.Inner.Verify(ctx, taskID, candidate+"@full", tv.FinalChecks())
}

func (tv *TargetedVerifier) Speedup(changedFiles []string) string {
	imp := tv.Graph.Predict(changedFiles)
	total := len(tv.Graph.nodes)
	if total == 0 || len(imp.AffectedPkgs) == 0 {
		return "targeted verification: no prediction available, using full suite"
	}
	return fmt.Sprintf("targeted verification: %d/%d packages (%.0fx reduction)",
		len(imp.AffectedPkgs), total,
		float64(total)/float64(len(imp.AffectedPkgs)))
}
