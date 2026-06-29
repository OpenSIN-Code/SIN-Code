// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package agentloop

import (
	"context"
	"fmt"
	"strings"
)

// wrapStopGate returns a StopGate that first evaluates tool-coverage
// constraints (issue #248), then delegates to the configured StopGate. If no
// coverage constraints and no StopGate are configured, it returns nil so the
// loop preserves exact legacy behavior.
func (l *Loop) wrapStopGate() StopGate {
	hasCoverage := l.Coverage != nil && l.Coverage.HasConstraints()
	if !hasCoverage && l.StopGate == nil {
		return nil
	}
	return func(ctx context.Context, snap StopSnapshot) StopDecision {
		if l.Coverage != nil {
			if ok, missing, forbidden := l.Coverage.Check(); !ok {
				return StopDecision{
					Complete:     false,
					OpenCriteria: l.Coverage.OpenCriteria(missing, forbidden),
					Report:       l.Coverage.Feedback(missing, forbidden),
				}
			}
		}
		if l.StopGate != nil {
			return l.StopGate(ctx, snap)
		}
		return StopDecision{Complete: true}
	}
}

// formatStopContinue renders the stop-gate rejection into a directive the
// model can act on: explicit, numbered, and unambiguous about NOT being done.
func formatStopContinue(dec StopDecision) string {
	var b strings.Builder
	b.WriteString("NOT DONE — the work is not complete yet. ")
	b.WriteString("An independent evaluator rejected the proposed completion.\n")
	if len(dec.OpenCriteria) > 0 {
		b.WriteString("Open acceptance criteria that MUST be satisfied:\n")
		for i, c := range dec.OpenCriteria {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, c)
		}
	}
	if strings.TrimSpace(dec.Report) != "" {
		b.WriteString("Evaluator notes:\n")
		b.WriteString(dec.Report)
		b.WriteString("\n")
	}
	b.WriteString("Continue working until every criterion is met, then stop.")
	return b.String()
}
