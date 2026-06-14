// SPDX-License-Identifier: MIT
// Purpose: the Autopilot orchestrator. Wires program.md + proposer + verified
// loop + metric + git keep/revert + journal + budget into one autonomous cycle:
//
//	OBSERVE -> PROPOSE -> ACT -> VERIFY -> MEASURE -> KEEP/REVERT -> LEARN -> repeat
//
// Mandates: M3 (every kept change passes the gate) and M4 (hard budget) hold.
package autopilot

import (
	"context"
	"fmt"
	"io"
	"time"
)

// LoopResult is the minimal contract the autopilot needs from one agent run.
// agentloop.Result satisfies this shape; tests pass a fake.
type LoopResult struct {
	SessionID string
	Verified  bool
	Turns     int
}

// RunGoal executes one goal through the verified agent loop and returns the
// result plus the raw verify output used for metric extraction.
type RunGoal func(ctx context.Context, goal string) (LoopResult, string, error)

// RecordLesson persists a lesson (wired to internal/lessons in auto_cmd.go).
type RecordLesson func(ctx context.Context, workspace, lesson string)

// Config bundles everything the autopilot needs.
type Config struct {
	Workspace string
	Program   *Program
	Proposer  *Proposer
	Journal   *Journal
	Budget    *Budget
	Snap      *Snapshotter
	RunGoal   RunGoal
	Lessons   func(ctx context.Context, workspace string, n int) []string // recent lessons
	Record    RecordLesson
	Out       io.Writer
}

// Autopilot is the autonomous controller.
type Autopilot struct {
	cfg Config
}

// New constructs an Autopilot.
func New(cfg Config) *Autopilot { return &Autopilot{cfg: cfg} }

func (a *Autopilot) logf(format string, args ...any) {
	if a.cfg.Out != nil {
		fmt.Fprintf(a.cfg.Out, format, args...)
	}
}

// Run drives the autonomous loop until the budget is exhausted. It returns the
// number of kept experiments and the best metric value achieved.
func (a *Autopilot) Run(ctx context.Context) (kept int, best float64, err error) {
	c := a.cfg
	best = c.Journal.BestKept(ctx, c.Program.Direction)

	if !c.Snap.IsRepo(ctx) {
		return 0, best, fmt.Errorf("autopilot: workspace is not a git repo (keep/revert requires git)")
	}

	a.logf("autopilot: objective=%q metric=%q dir=%s\n",
		oneLine(c.Program.Objective), c.Program.MetricName, c.Program.Direction)

	for {
		if reason := c.Budget.StopReason(); reason != "" {
			a.logf("autopilot: stopping — %s\n", reason)
			break
		}
		if !c.Budget.Consume() {
			a.logf("autopilot: stopping — experiment cap reached\n")
			break
		}

		// OBSERVE
		recent, _ := c.Journal.Recent(ctx, 8)
		var lessonTexts []string
		if c.Lessons != nil {
			lessonTexts = c.Lessons(ctx, c.Workspace, 10)
		}

		// PROPOSE
		goal, _ := c.Proposer.Next(ctx, recent, lessonTexts)
		exp := Experiment{
			Objective:    c.Program.Objective,
			Proposal:     goal,
			MetricBefore: best,
		}
		n := c.Budget.Used()
		a.logf("\n-- experiment %d -----------------------------------------\n%s\n", n, oneLine(goal))

		// snapshot baseline for potential revert
		baseline, berr := c.Snap.Baseline(ctx)
		if berr != nil {
			return kept, best, fmt.Errorf("baseline: %w", berr)
		}

		// ACT + VERIFY (the existing verified agent loop)
		full := goal
		if inv := c.Program.InvariantBriefing(); inv != "" {
			full = goal + "\n\n" + inv
		}
		res, verifyOut, runErr := c.RunGoal(ctx, full)
		exp.SessionID = res.SessionID

		if runErr != nil || !res.Verified {
			// never passed the gate → revert, learn, continue
			_ = c.Snap.Revert(ctx, baseline)
			exp.Outcome = OutcomeVerifyFail
			exp.MetricAfter = best
			reason := "verification failed"
			if runErr != nil {
				reason = runErr.Error()
			}
			exp.Note = oneLine(reason)
			_, _ = c.Journal.Record(ctx, exp)
			if c.Record != nil {
				c.Record(ctx, c.Workspace, "Autopilot: '"+oneLine(goal)+"' failed verification: "+oneLine(reason))
			}
			a.logf("   x verify failed -> reverted\n")
			continue
		}

		// MEASURE
		m := ExtractMetric(c.Program.ExtractRegex, verifyOut)
		exp.MetricFound = m.Found

		// KEEP / REVERT
		if !m.Found {
			// pass/fail-only mode: a verified change is always kept.
			commit, _ := c.Snap.Keep(ctx, "autopilot: "+oneLine(goal))
			exp.Outcome = OutcomeKept
			exp.Commit = commit
			exp.MetricAfter = best
			_, _ = c.Journal.Record(ctx, exp)
			kept++
			a.logf("   v verified (no metric) -> kept %s\n", short(commit))
			continue
		}

		exp.MetricAfter = m.Value
		if Improved(c.Program.Direction, best, m.Value) {
			commit, _ := c.Snap.Keep(ctx, fmt.Sprintf("autopilot: %s [%s=%.4g]", oneLine(goal), c.Program.MetricName, m.Value))
			exp.Outcome = OutcomeKept
			exp.Commit = commit
			best = BetterOf(c.Program.Direction, best, m.Value)
			_, _ = c.Journal.Record(ctx, exp)
			kept++
			a.logf("   v improved %s=%.4g -> kept %s\n", c.Program.MetricName, m.Value, short(commit))
		} else {
			_ = c.Snap.Revert(ctx, baseline)
			exp.Outcome = OutcomeReverted
			exp.Note = fmt.Sprintf("no improvement (%.4g vs best %.4g)", m.Value, best)
			_, _ = c.Journal.Record(ctx, exp)
			if c.Record != nil {
				c.Record(ctx, c.Workspace, fmt.Sprintf("Autopilot: '%s' regressed %s to %.4g (best %.4g)", oneLine(goal), c.Program.MetricName, m.Value, best))
			}
			a.logf("   <- %s=%.4g did not beat %.4g -> reverted\n", c.Program.MetricName, m.Value, best)
		}
	}

	a.logf("\nautopilot: done — %d kept, %d experiments in %s, best %s=%.4g\n",
		kept, c.Budget.Used(), c.Budget.Elapsed().Round(time.Second), c.Program.MetricName, best)
	return kept, best, nil
}

func short(commit string) string {
	if len(commit) > 8 {
		return commit[:8]
	}
	return commit
}
