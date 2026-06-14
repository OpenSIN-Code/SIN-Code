// SPDX-License-Identifier: MIT
// Purpose: the "researcher" — given the objective, recent experiment journal,
// and accumulated lessons, propose the NEXT concrete goal to attempt. This is
// the self-direction core: it removes the need for a human to spell out every
// task. LLM-backed with a deterministic fallback so it always makes progress.
package autopilot

import (
	"context"
	"fmt"
	"strings"
)

// ProposeFunc is an LLM-backed proposer. It receives a fully rendered prompt
// and must return a single concrete, actionable goal for the agent loop.
// Wiring this to a real model is done in auto_cmd.go; tests pass a fake.
type ProposeFunc func(ctx context.Context, prompt string) (string, error)

// Proposer turns the objective + history into the next goal.
type Proposer struct {
	Program *Program
	Propose ProposeFunc // optional; deterministic fallback used when nil
}

// Next renders context and asks for the next goal. On any LLM error it falls
// back to a deterministic proposal so the autonomous loop never stalls.
func (p *Proposer) Next(ctx context.Context, recent []Experiment, lessons []string) (string, error) {
	prompt := p.buildPrompt(recent, lessons)
	if p.Propose != nil {
		if goal, err := p.Propose(ctx, prompt); err == nil {
			if g := strings.TrimSpace(goal); g != "" {
				return g, nil
			}
		}
	}
	return p.fallback(recent), nil
}

// buildPrompt renders the researcher prompt from objective, invariants,
// recent experiments, and lessons.
func (p *Proposer) buildPrompt(recent []Experiment, lessons []string) string {
	var b strings.Builder
	b.WriteString("You are the autonomous research planner for a coding agent.\n")
	b.WriteString("Propose exactly ONE concrete, verifiable next step toward the objective.\n")
	b.WriteString("Return only the step as an imperative instruction, no preamble.\n\n")

	b.WriteString("# OBJECTIVE\n")
	b.WriteString(p.Program.Objective)
	b.WriteString("\n\n")

	if p.Program.MetricName != "" {
		fmt.Fprintf(&b, "# METRIC\nOptimize %q (%s).\n\n", p.Program.MetricName, p.Program.Direction)
	}
	if inv := p.Program.InvariantBriefing(); inv != "" {
		b.WriteString(inv)
		b.WriteByte('\n')
	}

	if len(recent) > 0 {
		b.WriteString("# RECENT EXPERIMENTS (newest first)\n")
		for i, e := range recent {
			if i >= 8 {
				break
			}
			status := string(e.Outcome)
			if e.MetricFound {
				fmt.Fprintf(&b, "- [%s] %s (metric: %.4g)\n", status, oneLine(e.Proposal), e.MetricAfter)
			} else {
				fmt.Fprintf(&b, "- [%s] %s\n", status, oneLine(e.Proposal))
			}
		}
		b.WriteByte('\n')
	}

	if len(lessons) > 0 {
		b.WriteString("# LESSONS (do not repeat these mistakes)\n")
		for i, l := range lessons {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "- %s\n", oneLine(l))
		}
		b.WriteByte('\n')
	}

	b.WriteString("# GUIDANCE\n")
	b.WriteString("- Prefer the smallest change that could improve the metric.\n")
	b.WriteString("- If the last experiment regressed, try a different approach.\n")
	b.WriteString("- Never modify files named in the invariants.\n")
	return b.String()
}

// fallback is a deterministic proposal used when no LLM is wired or it errors.
// It alternates between exploration strategies based on history length.
func (p *Proposer) fallback(recent []Experiment) string {
	base := p.Program.Objective
	switch len(recent) % 4 {
	case 0:
		return base + "\n\nNext step: identify the single hottest code path relevant to the objective and improve it, keeping all tests green."
	case 1:
		return base + "\n\nNext step: the previous attempt is the baseline. Try an alternative implementation strategy for the same target."
	case 2:
		return base + "\n\nNext step: add or tighten a test that captures the metric, then make the smallest change that improves it."
	default:
		return base + "\n\nNext step: refactor for clarity without changing behavior, then re-measure the metric."
	}
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}
