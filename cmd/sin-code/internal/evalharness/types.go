// SPDX-License-Identifier: MIT
// Purpose: eval-driven development data model — EvalCase, EvalSet, Output,
// Subject, Result, Run + aggregate score. Port of ECC's eval-harness in a
// clean-room Go reimplementation.
// Docs: types.doc.md
package evalharness

import (
	"context"
	"time"
)

// EvalCase is a single test scenario.
type EvalCase struct {
	ID       string            `json:"id"`
	Prompt   string            `json:"prompt"`             // input given to the subject
	Expected string            `json:"expected,omitempty"` // reference answer (optional)
	Tags     []string          `json:"tags,omitempty"`     // e.g. ["go","refactor"]
	Weight   float64           `json:"weight,omitempty"`   // default 1.0
	Meta     map[string]string `json:"meta,omitempty"`
}

// EvalSet is a named collection of cases.
type EvalSet struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Cases       []EvalCase `json:"cases"`
}

// Output is what the subject produced for one case.
type Output struct {
	Text     string            `json:"text"`
	Success  bool              `json:"success"`
	Duration time.Duration     `json:"duration_ns"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// Subject is the thing under evaluation: an agent, a verify gate, a model
// call. Implement this to plug SIN-Code's runtime into the harness.
type Subject interface {
	Run(ctx context.Context, c EvalCase) (Output, error)
}

// Result is the scored outcome for one case.
type Result struct {
	CaseID   string        `json:"case_id"`
	Score    float64       `json:"score"` // 0.0 .. 1.0
	Weight   float64       `json:"weight"`
	Passed   bool          `json:"passed"`
	Output   string        `json:"output,omitempty"`
	Detail   string        `json:"detail,omitempty"`
	Duration time.Duration `json:"duration_ns"`
	Err      string        `json:"error,omitempty"`
}

// Run is a complete evaluation pass over an EvalSet.
type Run struct {
	ID        string    `json:"id"`
	SetName   string    `json:"set_name"`
	Subject   string    `json:"subject"`
	StartedAt time.Time `json:"started_at"`
	Results   []Result  `json:"results"`
}

// Aggregate computes the weighted score and pass-rate of a run.
func (r Run) Aggregate() (weightedScore, passRate float64) {
	if len(r.Results) == 0 {
		return 0, 0
	}
	var sumW, sumWS float64
	passed := 0
	for _, res := range r.Results {
		w := res.Weight
		if w == 0 {
			w = 1
		}
		sumW += w
		sumWS += w * res.Score
		if res.Passed {
			passed++
		}
	}
	if sumW > 0 {
		weightedScore = sumWS / sumW
	}
	passRate = float64(passed) / float64(len(r.Results))
	return weightedScore, passRate
}
