// SPDX-License-Identifier: MIT
// Purpose: aggregator — combines sub-task results into a final response
// for the user. Validates success, formats output.
package orchestrator

import (
	"fmt"
	"strings"
)

type Aggregator struct {
	scratch *Scratchpad
}

func NewAggregator(scratch *Scratchpad) *Aggregator {
	return &Aggregator{scratch: scratch}
}

type Result struct {
	Plan       *Plan
	Sections   map[string]string
	TotalTasks int
	OKTasks    int
	FailedTasks int
	Summary    string
}

func (a *Aggregator) Aggregate(plan *Plan) *Result {
	r := &Result{
		Plan:     plan,
		Sections: map[string]string{},
	}
	for _, t := range plan.Tasks {
		r.TotalTasks++
		if t.Status == TaskCompleted {
			r.OKTasks++
		} else {
			r.FailedTasks++
		}
		section := fmt.Sprintf("task:%s:%s", t.Type, t.ID)
		r.Sections[section] = t.Result
		if t.Error != "" {
			r.Sections["error:"+t.ID] = t.Error
		}
	}
	r.Summary = a.summarize(plan)
	return r
}

func (a *Aggregator) summarize(plan *Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan %s (%s):\n", plan.ID, plan.Intent)
	for _, t := range plan.Tasks {
		icon := "✓"
		if t.Status != TaskCompleted {
			icon = "✗"
		}
		fmt.Fprintf(&b, "  %s [%s] %s by %s\n", icon, t.Type, t.ID, t.AgentName)
		if t.Error != "" {
			fmt.Fprintf(&b, "      error: %s\n", t.Error)
		}
	}
	fmt.Fprintf(&b, "\nTotal: %d tasks, %.4f USD, %d tokens, success=%v\n",
		len(plan.Tasks), plan.TotalCost, plan.TokensUsed, plan.Success)
	return b.String()
}
