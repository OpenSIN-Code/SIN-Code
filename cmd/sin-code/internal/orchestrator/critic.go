// SPDX-License-Identifier: MIT
// Purpose: Critic / Repair loop — bounded verify→diagnose→retry with stall detection.
// A failing Verdict is converted into structured context appended to the
// task description for the next attempt. Stops when score stalls, the
// budget runs out, or verification passes.
package orchestrator

import (
	"context"
	"fmt"
)

type RepairPolicy struct {
	MaxAttempts    int
	MinImprovement float64
}

func DefaultRepairPolicy() RepairPolicy {
	return RepairPolicy{MaxAttempts: 3, MinImprovement: 0.05}
}

type Attempt struct {
	Round    int
	Output   string
	Verdict  *Verdict
	Diagnose string
}

type CriticResult struct {
	Attempts []Attempt
	Final    *Verdict
	Passed   bool
}

type Critic struct {
	Verifier *Verifier
	Checks   []Check
	Policy   RepairPolicy
}

func NewCritic(vf *Verifier, checks []Check) *Critic {
	return &Critic{Verifier: vf, Checks: checks, Policy: DefaultRepairPolicy()}
}

func (c *Critic) Drive(ctx context.Context, ag Agent, task *Task, scratch *Scratchpad) (*CriticResult, error) {
	res := &CriticResult{}
	originalDesc := task.Description
	originalTitle := task.Title
	defer func() {
		task.Description = originalDesc
		task.Title = originalTitle
	}()

	bestScore := -1.0
	diagnosis := ""
	maxAttempts := c.Policy.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for round := 1; round <= maxAttempts; round++ {
		if diagnosis != "" {
			task.Description = fmt.Sprintf(
				"%s\n\n## Previous attempt failed verification\n"+
					"Fix ONLY the failures below. Do not refactor unrelated code.\n\n%s",
				originalDesc, diagnosis,
			)
			if task.Title == "" {
				task.Title = originalTitle
			}
		}

		out, err := ag.Run(ctx, task, scratch)
		if err != nil {
			diagnosis = fmt.Sprintf("agent error: %v", err)
			res.Attempts = append(res.Attempts, Attempt{
				Round: round, Output: out, Diagnose: diagnosis,
				Verdict: &Verdict{TaskID: task.ID, Candidate: ag.Name(), CreatedAt: timeNow()},
			})
			continue
		}

		v := c.Verifier.Verify(ctx, task.ID, fmt.Sprintf("%s-r%d", ag.Name(), round), c.Checks)
		diagnosis = v.Diagnosis()
		res.Attempts = append(res.Attempts, Attempt{Round: round, Output: out, Verdict: v, Diagnose: diagnosis})
		res.Final = v

		if v.Passed {
			res.Passed = true
			return res, nil
		}

		if bestScore >= 0 && v.Score < bestScore+c.Policy.MinImprovement {
			break
		}
		if v.Score > bestScore {
			bestScore = v.Score
		}
	}
	return res, nil
}
