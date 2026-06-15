// SPDX-License-Identifier: MIT
// Purpose: phase engine for PRPs. Drives draft -> planned ->
// implementing -> verifying -> ready -> shipped. Persists after
// every step so a run is interruptible/resumable. Delegates the
// hard work to four collaborators (Planner, Implementer, Verifier,
// PRController) wired by the host.
// Docs: engine.doc.md
package prp

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Planner decomposes a goal into tasks. Wire to an agent/model.
type Planner interface {
	Plan(ctx context.Context, goal, context string) (tasks []Task, plan string, err error)
}

// Implementer executes one task and returns notes on what changed.
type Implementer interface {
	Implement(ctx context.Context, p *PRP, t Task) (notes string, err error)
}

// Verifier runs the quality gate for the PRP's working tree.
type Verifier interface {
	Verify(ctx context.Context, workdir string) (passed bool, report string, err error)
}

// PRController opens a pull request for a finished PRP.
type PRController interface {
	OpenPR(ctx context.Context, p *PRP) (url string, err error)
}

// Engine drives a PRP through its phases, persisting after each
// step.
type Engine struct {
	Store       *Store
	Workdir     string
	Planner     Planner
	Implementer Implementer
	Verifier    Verifier
	PR          PRController
	Log         func(format string, args ...any)
}

func (e *Engine) logf(f string, a ...any) {
	if e.Log != nil {
		e.Log(f, a...)
	}
}

// New creates a draft PRP and persists it.
func (e *Engine) New(id, title, goal, contextText string) (*PRP, error) {
	now := time.Now().UTC()
	p := &PRP{
		ID: id, Title: title, Goal: goal, Context: contextText,
		Phase: PhaseDraft, CreatedAt: now, UpdatedAt: now,
	}
	return p, e.save(p)
}

// RunPlan decomposes the goal into tasks (Planner) and advances to
// planned.
func (e *Engine) RunPlan(ctx context.Context, p *PRP) error {
	if e.Planner == nil {
		return fmt.Errorf("no planner wired")
	}
	tasks, plan, err := e.Planner.Plan(ctx, p.Goal, p.Context)
	if err != nil {
		return err
	}
	for i := range tasks {
		if tasks[i].State == "" {
			tasks[i].State = TaskTodo
		}
		if tasks[i].ID == "" {
			tasks[i].ID = fmt.Sprintf("t%d", i+1)
		}
	}
	p.Tasks = tasks
	p.Plan = plan
	p.Phase = PhasePlanned
	e.logf("planned %d tasks", len(tasks))
	return e.save(p)
}

// RunImplement executes tasks until done or the first error/block.
func (e *Engine) RunImplement(ctx context.Context, p *PRP) error {
	if e.Implementer == nil {
		return fmt.Errorf("no implementer wired")
	}
	p.Phase = PhaseImplementing
	for {
		t := p.NextTask()
		if t == nil {
			break
		}
		t.State = TaskDoing
		_ = e.save(p)
		notes, err := e.Implementer.Implement(ctx, p, *t)
		if err != nil {
			t.State = TaskBlocked
			t.Notes = err.Error()
			_ = e.save(p)
			return fmt.Errorf("task %s blocked: %w", t.ID, err)
		}
		t.State = TaskDone
		t.Notes = notes
		e.logf("done task %s: %s", t.ID, t.Title)
		_ = e.save(p)
	}
	if p.AllDone() {
		p.Phase = PhaseVerifying
		_ = e.save(p)
	}
	return nil
}

// RunVerify runs the quality gate; on pass, advances to ready.
func (e *Engine) RunVerify(ctx context.Context, p *PRP) (bool, string, error) {
	if e.Verifier == nil {
		return true, "", nil
	}
	passed, report, err := e.Verifier.Verify(ctx, e.Workdir)
	if err != nil {
		return false, report, err
	}
	if passed {
		p.Phase = PhaseReady
	} else {
		p.Phase = PhaseImplementing // kick back for fixes
	}
	_ = e.save(p)
	return passed, report, nil
}

// RunPR opens a pull request once ready and advances to shipped.
func (e *Engine) RunPR(ctx context.Context, p *PRP) (string, error) {
	if p.Phase != PhaseReady {
		return "", fmt.Errorf("PRP not ready (phase=%s)", p.Phase)
	}
	if e.PR == nil {
		return "", fmt.Errorf("no PR controller wired")
	}
	url, err := e.PR.OpenPR(ctx, p)
	if err != nil {
		return "", err
	}
	p.Phase = PhaseShipped
	_ = e.save(p)
	return url, nil
}

// RunAll executes the full pipeline plan->implement->verify->pr,
// stopping at the first phase that cannot complete (e.g.
// verification failure).
func (e *Engine) RunAll(ctx context.Context, p *PRP) error {
	if p.Phase == PhaseDraft {
		if err := e.RunPlan(ctx, p); err != nil {
			return err
		}
	}
	if err := e.RunImplement(ctx, p); err != nil {
		return err
	}
	passed, report, err := e.RunVerify(ctx, p)
	if err != nil {
		return err
	}
	if !passed {
		return fmt.Errorf("verification failed:\n%s", strings.TrimSpace(report))
	}
	url, err := e.RunPR(ctx, p)
	if err != nil {
		return err
	}
	e.logf("shipped: %s", url)
	return nil
}

func (e *Engine) save(p *PRP) error {
	p.UpdatedAt = time.Now().UTC()
	return e.Store.Save(p)
}
