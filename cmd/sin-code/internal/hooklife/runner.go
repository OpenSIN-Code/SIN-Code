// SPDX-License-Identifier: MIT
// Purpose: dispatch an event to all hooks of its phase. For PreToolUse
// the first Block verdict short-circuits and is returned. For all other
// phases every hook runs and warnings are aggregated. Per-hook timeout
// + panic recovery so a misbehaving hook never breaks a session.
// Docs: runner.doc.md
package hooklife

import (
	"context"
	"fmt"
	"time"
)

// Runner dispatches an event to all hooks of its phase.
type Runner struct {
	reg     *Registry
	timeout time.Duration
	logf    func(format string, args ...any)
}

func NewRunner(reg *Registry) *Runner {
	return &Runner{
		reg:     reg,
		timeout: 10 * time.Second,
		logf:    func(string, ...any) {},
	}
}

func (r *Runner) WithTimeout(d time.Duration) *Runner { r.timeout = d; return r }
func (r *Runner) WithLogger(f func(string, ...any)) *Runner {
	if f != nil {
		r.logf = f
	}
	return r
}

// Dispatch runs all hooks for the event's phase.
//
// For PreToolUse, the first Block verdict short-circuits and is returned
// (the tool must not run). Warnings are collected and merged. For all
// other phases every hook runs and warnings are aggregated.
func (r *Runner) Dispatch(ctx context.Context, ev Event) Decision {
	hooks := r.reg.Hooks(ev.Phase)
	agg := Decision{Verdict: Allow}
	var warnings []string

	for _, h := range hooks {
		d := r.runOne(ctx, h, ev)
		switch d.Verdict {
		case Block:
			if ev.Phase == PreToolUse {
				return d // hard stop
			}
			warnings = append(warnings, fmt.Sprintf("[%s] %s", d.HookID, d.Message))
		case Warn:
			if d.Message != "" {
				warnings = append(warnings, fmt.Sprintf("[%s] %s", d.HookID, d.Message))
			}
		}
	}
	if len(warnings) > 0 {
		agg.Verdict = Warn
		agg.Message = joinLines(warnings)
	}
	return agg
}

func (r *Runner) runOne(ctx context.Context, h Hook, ev Event) Decision {
	cctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	done := make(chan Decision, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				r.logf("hook %s panicked: %v", h.ID(), rec)
				done <- Decision{Verdict: Allow, HookID: h.ID()}
			}
		}()
		d := h.Run(cctx, ev)
		d.HookID = h.ID()
		done <- d
	}()

	select {
	case d := <-done:
		return d
	case <-cctx.Done():
		r.logf("hook %s timed out", h.ID())
		return Decision{Verdict: Allow, HookID: h.ID(), Message: "timeout"}
	}
}

func joinLines(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += "\n"
		}
		out += s
	}
	return out
}
