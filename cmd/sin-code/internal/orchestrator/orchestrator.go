// SPDX-License-Identifier: MIT
// Purpose: top-level Orchestrator — wires together Router, Planner, Dispatcher,
// Registry, Scratchpad, Aggregator. This is the main entry point.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
)

type Orchestrator struct {
	Registry    *Registry
	Planner     *Planner
	Dispatcher  *Dispatcher
	Aggregator  *Aggregator
	Scratchpad  *Scratchpad
	MaxParallel int
	Episodes    *EpisodeStore

	// Sub-agents (optional, nil-safe). When non-nil they enrich the
	// orchestrator flow: Cartographer maps the repo before planning,
	// Adversary probes for counterexamples after dispatch, Governor
	// runs escalating repair rounds on failed tasks.
	Cartographer *Cartographer
	Adversary    *Adversary
	Governor     *Governor
	Hooks        *hooks.Engine
}

func New() *Orchestrator {
	scratch := NewScratchpad()
	registry := NewRegistryWithDefaults(nil)
	planner := NewPlanner(registry.List())
	dispatcher := NewDispatcher(registry, scratch, 4)
	aggregator := NewAggregator(scratch)
	return &Orchestrator{
		Registry:    registry,
		Planner:     planner,
		Dispatcher:  dispatcher,
		Aggregator:  aggregator,
		Scratchpad:  scratch,
		MaxParallel: 4,
	}
}

func NewWithAgents(extraConfigs []AgentConfig) *Orchestrator {
	scratch := NewScratchpad()
	registry := NewRegistryWithDefaults(extraConfigs)
	planner := NewPlanner(registry.List())
	dispatcher := NewDispatcher(registry, scratch, 4)
	aggregator := NewAggregator(scratch)
	return &Orchestrator{
		Registry:    registry,
		Planner:     planner,
		Dispatcher:  dispatcher,
		Aggregator:  aggregator,
		Scratchpad:  scratch,
		MaxParallel: 4,
	}
}

func (o *Orchestrator) Run(ctx context.Context, prompt string, opts ...RunOption) (*Result, error) {
	cfg := &runConfig{maxParallel: o.MaxParallel}
	for _, opt := range opts {
		opt(cfg)
	}
	// Cartographer: index repo symbols before planning (best-effort,
	// non-fatal — the planner works without it, just with less context).
	if o.Cartographer != nil {
		_ = o.Cartographer.IndexAll(ctx)
	}

	plan := o.Planner.BuildPlan(prompt)
	if o.Episodes != nil && o.Episodes.hasSchema {
		if eps, err := o.Episodes.Similar(ctx, prompt, 3); err == nil && len(eps) > 0 {
			if prior := PlanningPrior(eps); prior != "" {
				if len(plan.Tasks) > 0 {
					plan.Tasks[0].Description = prior + "\n\n" + plan.Tasks[0].Description
				}
			}
		}
	}
	disp := o.Dispatcher
	if cfg.maxParallel > 0 {
		disp = NewDispatcher(o.Registry, o.Scratchpad, cfg.maxParallel)
		disp.PreWarm = o.Dispatcher.PreWarm
	}
	timeout := cfg.timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if err := disp.Dispatch(ctx, plan); err != nil {
		if o.Episodes != nil && o.Episodes.hasSchema {
			_ = o.Episodes.Record(ctx, &Episode{
				Intent:    string(plan.Intent),
				TaskTitle: prompt,
				PlanJSON:  planToJSON(plan),
				Score:     0,
				Passed:    false,
				Rounds:    len(plan.Tasks),
				CreatedAt: timeNow(),
			})
		}
		return nil, err
	}

	// Adversary: probe for counterexamples after dispatch succeeds
	// (best-effort — a nil Adversary or an empty diff is a no-op).
	if o.Adversary != nil {
		diff := scratchDiff(o.Scratchpad, plan, o.Adversary.Workdir)
		if diff != "" {
			advResult, _ := o.Adversary.Review(ctx, diff, string(plan.Intent))
			if advResult != nil && advResult.Landed > 0 {
				for _, atk := range advResult.Attacks {
					if atk.Landed {
						o.Scratchpad.Write("adversary", "adversary-attack:"+atk.Hypothesis, atk.Hypothesis)
					}
				}
			}
		}
	}

	// Governor: escalating repair for failed tasks (best-effort).
	// Only tasks that were dispatched but ended in TaskFailed are
	// eligible — pending/blocked/cancelled tasks are left alone.
	if o.Governor != nil {
		for _, task := range plan.Tasks {
			if task.Status == TaskFailed {
				govResult, _ := o.Governor.Execute(ctx, task, o.Scratchpad)
				if govResult != nil && govResult.Passed {
					task.Status = TaskCompleted
					task.Error = ""
				}
			}
		}
	}

	result := o.Aggregator.Aggregate(plan)
	if o.Episodes != nil && o.Episodes.hasSchema {
		_ = o.Episodes.Record(ctx, &Episode{
			Intent:    string(plan.Intent),
			TaskTitle: prompt,
			PlanJSON:  planToJSON(plan),
			Diff:      result.Summary,
			Score:     float64(result.OKTasks) / float64(maxInt(result.TotalTasks, 1)),
			Passed:    result.FailedTasks == 0 && result.TotalTasks > 0,
			Rounds:    result.TotalTasks,
			CreatedAt: timeNow(),
		})
	}
	return result, nil
}

type runConfig struct {
	timeout     time.Duration
	maxParallel int
}

type RunOption func(*runConfig)

func WithTimeout(d time.Duration) RunOption {
	return func(c *runConfig) { c.timeout = d }
}

func WithMaxParallel(n int) RunOption {
	return func(c *runConfig) { c.maxParallel = n }
}

func (o *Orchestrator) Plan(prompt string) *Plan {
	return o.Planner.BuildPlan(prompt)
}

func (o *Orchestrator) String() string {
	return fmt.Sprintf("Orchestrator{agents=%d, scratchpad=%d entries}",
		len(o.Registry.List()), len(o.Scratchpad.ReadAll()))
}

func planToJSON(plan *Plan) json.RawMessage {
	data, err := json.Marshal(plan)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

// scratchDiff extracts a diff for the Adversary to probe. It checks
// the scratchpad for a pre-recorded diff section first, then falls
// back to `git diff` in the workdir. Returns "" when no diff is
// available — the Adversary call site treats empty as a no-op.
func scratchDiff(scratch *Scratchpad, plan *Plan, workdir string) string {
	if scratch != nil && plan != nil {
		if diff, ok := scratch.Read("diff:" + plan.ID); ok && diff != "" {
			return diff
		}
	}
	if workdir == "" {
		workdir = "."
	}
	out, err := exec.Command("git", "-C", workdir, "diff").CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}
