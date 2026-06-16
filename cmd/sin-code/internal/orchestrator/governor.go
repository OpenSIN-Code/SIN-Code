// SPDX-License-Identifier: MIT
// Purpose: Budget Governor — escalating ladder of compute strategies.
// Rung 1 (cheap single-shot) → Rung 2 (repair) → Rung 3 (best-of-N).
// Each climb is logged with its justification (the failing verdict).
package orchestrator

import (
	"context"
	"fmt"
	"time"
)

type Rung struct {
	Name         string
	Agents       int
	RepairRounds int
	Timeout      time.Duration
}

func DefaultLadder() []Rung {
	return []Rung{
		{Name: "single-shot", Agents: 1, RepairRounds: 1, Timeout: 5 * time.Minute},
		{Name: "single+repair", Agents: 1, RepairRounds: 3, Timeout: 10 * time.Minute},
		{Name: "best-of-3+repair", Agents: 3, RepairRounds: 2, Timeout: 20 * time.Minute},
	}
}

type Escalation struct {
	FromRung string
	ToRung   string
	Reason   string
	Verdict  *Verdict
	At       time.Time
}

// GovernorResult is the bounded ladder outcome. Findings carries the
// caveman-style one-liners derived from each Escalation; the
// prose Reason strings remain on Escalation (free-form audit trail)
// and Findings is the structured prose layer the orchestrator
// re-ingests. Each escalation becomes ONE Finding tagged `risk` (the
// blast radius of staying at the lower rung).
type GovernorResult struct {
	Passed      bool
	FinalRung   string
	Verdict     *Verdict
	Escalations []Escalation
	TotalRounds int
	Findings    []Finding
}

type AgentFactory func(rung Rung) []Agent

var criticDriveHook = func(c *Critic, ctx context.Context, ag Agent, task *Task, scratch *Scratchpad) (*CriticResult, error) {
	return c.Drive(ctx, ag, task, scratch)
}

var specRunHook = func(s *SpeculativeRunner, ctx context.Context, task *Task, agents []Agent, scratch *Scratchpad) (*SpecResult, error) {
	return s.Run(ctx, task, agents, scratch)
}

type Governor struct {
	Ladder   []Rung
	Verifier *Verifier
	Checks   []Check
	RepoRoot string
	Factory  AgentFactory
	Router   *StrategyRouter
}

func (g *Governor) Execute(ctx context.Context, task *Task, scratch *Scratchpad) (*GovernorResult, error) {
	res := &GovernorResult{}
	var lastVerdict *Verdict

	for i, rung := range g.Ladder {
		if i > 0 {
			res.Escalations = append(res.Escalations, Escalation{
				FromRung: g.Ladder[i-1].Name,
				ToRung:   rung.Name,
				Reason:   escalationReason(lastVerdict),
				Verdict:  lastVerdict,
				At:       timeNow(),
			})
		}
		res.FinalRung = rung.Name

		rctx, cancel := context.WithTimeout(ctx, rung.Timeout)
		verdict, rounds, err := g.runRung(rctx, rung, task, scratch)
		cancel()
		if err != nil {
			collectGovernorFindings(res, task)
			return res, fmt.Errorf("rung %q: %w", rung.Name, err)
		}
		res.TotalRounds += rounds
		res.Verdict = verdict
		lastVerdict = verdict

		if verdict != nil && verdict.Passed {
			res.Passed = true
			collectGovernorFindings(res, task)
			return res, nil
		}
	}
	collectGovernorFindings(res, task)
	return res, nil
}

// collectGovernorFindings derives caveman-style Findings from each
// Escalation. Every climb is a single `risk` Finding — the lower rung
// ended red, so the orchestrator's blast radius is "task needs another
// rung". The Governor is the only sub-agent whose Findings are
// file-level (Path = task.ID, Line=0) because the rung ladder doesn't
// own a file location.
func collectGovernorFindings(res *GovernorResult, task *Task) {
	if res == nil {
		return
	}
	res.Findings = make([]Finding, 0, len(res.Escalations))
	taskPath := "task://orphan"
	if task != nil && task.ID != "" {
		taskPath = "task://" + task.ID
	}
	for _, e := range res.Escalations {
		res.Findings = append(res.Findings, Finding{
			Tag:        TagRisk,
			Symbol:     e.FromRung + "->" + e.ToRung,
			Path:       taskPath,
			Line:       0,
			Confidence: 1.0,
			Hint:       truncateGovernorHint(e.Reason),
		})
	}
}

// truncateGovernorHint keeps the Finding Hint under the 240-char
// caveman one-liner ceiling. The Escalation.Reason is the audit-trail
// free-form string and may be longer; the Finding only carries the
// first 200 chars + a tail marker.
func truncateGovernorHint(reason string) string {
	const max = 200
	if len(reason) <= max {
		return reason
	}
	return reason[:max] + "... [truncated]"
}

func (g *Governor) runRung(ctx context.Context, rung Rung, task *Task, scratch *Scratchpad) (*Verdict, int, error) {
	agents := g.Factory(rung)
	if len(agents) == 0 {
		return nil, 0, fmt.Errorf("factory returned no agents")
	}

	if rung.Agents <= 1 {
		critic := NewCritic(g.Verifier, g.Checks)
		critic.Policy.MaxAttempts = rung.RepairRounds + 1
		cres, err := criticDriveHook(critic, ctx, agents[0], task, scratch)
		if err != nil {
			return nil, 0, err
		}
		return cres.Final, len(cres.Attempts), nil
	}

	spec := NewSpeculativeRunner(g.RepoRoot, g.Checks)
	spec.MaxParallel = rung.Agents
	limit := rung.Agents
	if limit > len(agents) {
		limit = len(agents)
	}
	sres, err := specRunHook(spec, ctx, task, agents[:limit], scratch)
	if err != nil {
		return nil, 0, err
	}
	rounds := len(sres.Candidates)

	if sres.Winner == nil {
		return nil, rounds, nil
	}
	if sres.Winner.Verdict != nil && sres.Winner.Verdict.Passed {
		if g.RepoRoot != "" {
			if _, err := spec.MergeWinner(ctx, sres.Winner); err != nil {
				return sres.Winner.Verdict, rounds, fmt.Errorf("merge winner: %w", err)
			}
		}
		return sres.Winner.Verdict, rounds, nil
	}

	wvf := NewVerifier(sres.Winner.Worktree)
	critic := NewCritic(wvf, g.Checks)
	critic.Policy.MaxAttempts = rung.RepairRounds
	cres, err := criticDriveHook(critic, ctx, sres.Winner.Agent, task, scratch)
	if err != nil {
		return sres.Winner.Verdict, rounds, err
	}
	rounds += len(cres.Attempts)
	if cres.Passed && g.RepoRoot != "" {
		if _, err := spec.MergeWinner(ctx, sres.Winner); err != nil {
			return cres.Final, rounds, fmt.Errorf("merge repaired winner: %w", err)
		}
	}
	return cres.Final, rounds, nil
}

func escalationReason(v *Verdict) string {
	if v == nil {
		return "no verdict produced at lower rung"
	}
	failed := []string{}
	for _, r := range v.Results {
		if !r.Passed {
			failed = append(failed, string(r.Check.Kind)+":"+r.Check.Name)
		}
	}
	return fmt.Sprintf("lower rung ended red (score %.2f), failing: %v", v.Score, failed)
}
