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

// CriticResult is the bounded verify→diagnose→repair outcome. Findings
// carries the caveman-style one-liners extracted from the final attempt's
// output (the prose layer the orchestrator re-ingests; structured fields
// above are unchanged). ParseErrors is the per-line rejection trace —
// non-empty means the sub-agent emitted prose the Verifier couldn't
// digest; the orchestrator re-injects these verbatim as retry feedback.
type CriticResult struct {
	Attempts    []Attempt
	Final       *Verdict
	Passed      bool
	Findings    []Finding
	ParseErrors []string
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
			collectCriticFindings(res)
			return res, nil
		}

		if bestScore >= 0 && v.Score < bestScore+c.Policy.MinImprovement {
			collectCriticFindings(res)
			break
		}
		if v.Score > bestScore {
			bestScore = v.Score
		}
	}
	collectCriticFindings(res)
	return res, nil
}

// collectCriticFindings parses the LAST attempt's prose through the
// caveman output contract and stores the result on res. The contract is
// the PROSE layer; structured Result fields above are unchanged.
// Empty Findings + nil ParseErrors means "the agent said nothing
// parseable" — the caller decides what to do (typically: re-inject
// ParseErrors as retry feedback). We never silently drop bad lines.
func collectCriticFindings(res *CriticResult) {
	if len(res.Attempts) == 0 {
		return
	}
	if len(res.Findings) > 0 || len(res.ParseErrors) > 0 {
		return
	}
	last := res.Attempts[len(res.Attempts)-1]
	fs, perrs, _ := ParseFindings(last.Output)
	res.Findings = fs
	res.ParseErrors = perrs
}
