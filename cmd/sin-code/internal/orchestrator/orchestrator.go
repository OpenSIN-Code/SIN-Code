// SPDX-License-Identifier: MIT
// Purpose: top-level Orchestrator — wires together Router, Planner, Dispatcher,
// Registry, Scratchpad, Aggregator. This is the main entry point.
package orchestrator

import (
	"context"
	"fmt"
	"time"
)

type Orchestrator struct {
	Registry   *Registry
	Planner    *Planner
	Dispatcher *Dispatcher
	Aggregator *Aggregator
	Scratchpad *Scratchpad
	MaxParallel int
}

func New() *Orchestrator {
	scratch := NewScratchpad()
	registry := NewRegistryWithDefaults(nil)
	planner := NewPlanner(registry.List())
	dispatcher := NewDispatcher(registry, scratch, 4)
	aggregator := NewAggregator(scratch)
	return &Orchestrator{
		Registry:   registry,
		Planner:    planner,
		Dispatcher: dispatcher,
		Aggregator: aggregator,
		Scratchpad: scratch,
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
		Registry:   registry,
		Planner:    planner,
		Dispatcher: dispatcher,
		Aggregator: aggregator,
		Scratchpad: scratch,
		MaxParallel: 4,
	}
}

func (o *Orchestrator) Run(ctx context.Context, prompt string, opts ...RunOption) (*Result, error) {
	cfg := &runConfig{maxParallel: o.MaxParallel}
	for _, opt := range opts {
		opt(cfg)
	}
	plan := o.Planner.BuildPlan(prompt)
	disp := o.Dispatcher
	if cfg.maxParallel > 0 {
		disp = NewDispatcher(o.Registry, o.Scratchpad, cfg.maxParallel)
	}
	timeout := cfg.timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if err := disp.Dispatch(ctx, plan); err != nil {
		return nil, err
	}
	return o.Aggregator.Aggregate(plan), nil
}

type runConfig struct {
	timeout    time.Duration
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
