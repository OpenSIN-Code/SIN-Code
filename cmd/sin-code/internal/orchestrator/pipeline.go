// SPDX-License-Identifier: MIT
// Purpose: typed, composable tool chains with trace-on-fail semantics.
// One fused call replaces 6-10 single-shot LLM round-trips; typechecks
// at construction; every stage may fail and the trace is the agent's
// resume context.
package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Artifact struct {
	Type string
	Data any
}

type Stage interface {
	Name() string
	InType() string
	OutType() string
	Run(ctx context.Context, in Artifact) (Artifact, error)
}

type StageResult struct {
	Stage    string
	Duration time.Duration
	Output   Artifact
	Err      error
}

type Trace struct {
	Chain    string
	Results  []StageResult
	Halted   bool
	HaltedAt string
}

func (t *Trace) Diagnosis() string {
	var b strings.Builder
	fmt.Fprintf(&b, "PIPELINE %s: %d/%d stages completed\n", t.Chain, t.completed(), len(t.Results))
	for _, r := range t.Results {
		status := "ok"
		if r.Err != nil {
			status = "FAILED: " + r.Err.Error()
		}
		fmt.Fprintf(&b, "- [%s] %s (%s)\n", r.Stage, status, r.Duration.Round(time.Millisecond))
	}
	if t.Halted {
		fmt.Fprintf(&b, "Chain halted at %q. Earlier stage outputs remain valid and are reusable.\n", t.HaltedAt)
	}
	return b.String()
}

func (t *Trace) completed() int {
	n := 0
	for _, r := range t.Results {
		if r.Err == nil {
			n++
		}
	}
	return n
}

type Chain struct {
	name   string
	stages []Stage
}

func NewChain(name string, stages ...Stage) (*Chain, error) {
	if len(stages) == 0 {
		return nil, fmt.Errorf("chain %q: empty", name)
	}
	for i := 1; i < len(stages); i++ {
		out, in := stages[i-1].OutType(), stages[i].InType()
		if out != in && in != "any" {
			return nil, fmt.Errorf("chain %q: stage %q outputs %q but stage %q expects %q",
				name, stages[i-1].Name(), out, stages[i].Name(), in)
		}
	}
	return &Chain{name: name, stages: stages}, nil
}

func (c *Chain) Run(ctx context.Context, in Artifact) (Artifact, *Trace) {
	trace := &Trace{Chain: c.name}
	cur := in
	for _, s := range c.stages {
		start := timeNow()
		out, err := s.Run(ctx, cur)
		trace.Results = append(trace.Results, StageResult{
			Stage: s.Name(), Duration: timeNow().Sub(start), Output: out, Err: err,
		})
		if err != nil {
			trace.Halted = true
			trace.HaltedAt = s.Name()
			return cur, trace
		}
		cur = out
	}
	return cur, trace
}

type parallelStage struct {
	name     string
	branches []Stage
	merge    func([]Artifact) (Artifact, error)
	outType  string
}

func (p *parallelStage) Name() string    { return p.name }
func (p *parallelStage) InType() string  { return p.branches[0].InType() }
func (p *parallelStage) OutType() string { return p.outType }

func (p *parallelStage) Run(ctx context.Context, in Artifact) (Artifact, error) {
	type res struct {
		i   int
		art Artifact
		err error
	}
	ch := make(chan res, len(p.branches))
	for i, b := range p.branches {
		i, b := i, b
		go func() {
			a, err := b.Run(ctx, in)
			ch <- res{i, a, err}
		}()
	}
	outs := make([]Artifact, len(p.branches))
	for range p.branches {
		r := <-ch
		if r.err != nil {
			return Artifact{}, fmt.Errorf("parallel branch %q: %w", p.branches[r.i].Name(), r.err)
		}
		outs[r.i] = r.art
	}
	return p.merge(outs)
}

func Parallel(name, outType string, merge func([]Artifact) (Artifact, error), branches ...Stage) Stage {
	return &parallelStage{name: name, branches: branches, merge: merge, outType: outType}
}

type guardStage struct {
	inner Stage
	check func(Artifact) error
}

func (g *guardStage) Name() string    { return g.inner.Name() + "+guard" }
func (g *guardStage) InType() string  { return g.inner.InType() }
func (g *guardStage) OutType() string { return g.inner.OutType() }

func (g *guardStage) Run(ctx context.Context, in Artifact) (Artifact, error) {
	if err := g.check(in); err != nil {
		return Artifact{}, fmt.Errorf("guard rejected input: %w", err)
	}
	return g.inner.Run(ctx, in)
}

func Guard(inner Stage, check func(Artifact) error) Stage {
	return &guardStage{inner: inner, check: check}
}

type FuncStage struct {
	StageName string
	In, Out   string
	Fn        func(ctx context.Context, in Artifact) (Artifact, error)
}

func (f *FuncStage) Name() string    { return f.StageName }
func (f *FuncStage) InType() string  { return f.In }
func (f *FuncStage) OutType() string { return f.Out }
func (f *FuncStage) Run(ctx context.Context, in Artifact) (Artifact, error) {
	return f.Fn(ctx, in)
}
