// SPDX-License-Identifier: MIT
// Purpose: execute an EvalSet against a Subject, scoring each case.
// Individual case errors become failing results; the run continues.
// Docs: runner.doc.md
package evalharness

import (
	"context"
	"fmt"
	"time"
)

// Runner executes an EvalSet against a Subject, scoring each case.
type Runner struct {
	Subject     Subject
	Scorer      Scorer
	SubjectName string
	Timeout     time.Duration // per-case; 0 = no timeout
	Progress    func(done, total int, last Result)
}

// Execute runs every case and returns a Run. Individual case errors
// are captured as failing results; the pass continues.
func (r Runner) Execute(ctx context.Context, set EvalSet) (Run, error) {
	if r.Subject == nil {
		return Run{}, fmt.Errorf("evalharness: nil Subject")
	}
	if r.Scorer == nil {
		r.Scorer = SuccessFlag{}
	}
	run := Run{
		ID:        fmt.Sprintf("%s-%d", set.Name, time.Now().Unix()),
		SetName:   set.Name,
		Subject:   r.SubjectName,
		StartedAt: time.Now().UTC(),
	}
	for idx, c := range set.Cases {
		res := r.runCase(ctx, c)
		run.Results = append(run.Results, res)
		if r.Progress != nil {
			r.Progress(idx+1, len(set.Cases), res)
		}
	}
	return run, nil
}

func (r Runner) runCase(ctx context.Context, c EvalCase) Result {
	cctx := ctx
	var cancel context.CancelFunc
	if r.Timeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	start := time.Now()
	out, err := r.Subject.Run(cctx, c)
	dur := time.Since(start)

	w := c.Weight
	if w == 0 {
		w = 1
	}
	if err != nil {
		return Result{CaseID: c.ID, Score: 0, Weight: w, Passed: false, Duration: dur, Err: err.Error()}
	}
	score, passed, detail := r.Scorer.Score(c, out)
	return Result{
		CaseID: c.ID, Score: score, Weight: w, Passed: passed,
		Output: truncate(out.Text, 500), Detail: detail, Duration: dur,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
