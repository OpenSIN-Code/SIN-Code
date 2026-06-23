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

type GovernorResult struct {
	Passed      bool
	FinalRung   string
	Verdict     *Verdict
	Escalations []Escalation
	TotalRounds int
}

type AgentFactory func(rung Rung) []Agent

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
			return res, fmt.Errorf("rung %q: %w", rung.Name, err)
		}
		res.TotalRounds += rounds
		res.Verdict = verdict
		lastVerdict = verdict

		if verdict != nil && verdict.Passed {
			res.Passed = true
			return res, nil
		}
	}
	return res, nil
}

func (g *Governor) runRung(ctx context.Context, rung Rung, task *Task, scratch *Scratchpad) (*Verdict, int, error) {
	agents := g.Factory(rung)
	if len(agents) == 0 {
		return nil, 0, fmt.Errorf("factory returned no agents")
	}

	if rung.Agents <= 1 {
		critic := NewCritic(g.Verifier, g.Checks)
		critic.Policy.MaxAttempts = rung.RepairRounds + 1
		cres, err := critic.Drive(ctx, agents[0], task, scratch)
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
	sres, err := spec.Run(ctx, task, agents[:limit], scratch)
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
	cres, err := critic.Drive(ctx, sres.Winner.Agent, task, scratch)
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
