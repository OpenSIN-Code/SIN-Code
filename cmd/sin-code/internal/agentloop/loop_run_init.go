// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loop is refactored
// Purpose: session-start message preparation extracted from Run().
// Pure file split, same package, no behavioural change.
package agentloop

import (
	"context"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// prepareRunMessages assembles the initial message slice for a run:
// records the user prompt to the ledger, injects session-start context
// (issue #379), the Definition-of-Done preamble, the session-context
// builder block, the user prompt itself, memory prime output, and the
// lessons briefing. Returns the fully prepared message slice.
func (l *Loop) prepareRunMessages(ctx context.Context, sess *session.Session, prompt string) []session.Message {
	l.record(ctx, ledger.TypeUserPrompt, map[string]any{"content": prompt}, "user prompt")
	msgs := sess.History()
	// Session-start context injection (issue #379): on a brand-new session
	// (empty history), build the unified preamble from todos, previous
	// session summary, and auto-memory. Errors are non-fatal; the loop
	// continues with the original prompt if the builder fails.
	if l.SessionContext != nil && len(msgs) == 0 {
		if preamble, err := l.SessionContext.Build(ctx); err == nil && strings.TrimSpace(preamble) != "" {
			msgs = append(msgs, session.Message{Role: "user", Content: preamble})
		}
	}
	// SinCode Loop System: state the Definition-of-Done before the goal so the
	// worker addresses tests/debug/docs/completeness proactively. Enforcement
	// still lives in the stop-gate; this only improves first-pass quality.
	if strings.TrimSpace(l.Preamble) != "" {
		msgs = append(msgs, session.Message{Role: "user", Content: l.Preamble})
	}
	// Issue #379: session-context injection from lessons / memory /
	// autonomy. Fires at session start, OPT-IN ONLY — the builder is
	// nil unless loopbuilder wired an explicit ContextInjector, so
	// legacy sessions keep their exact pre-#379 message stream.
	if l.SessionContextBuilder != nil {
		if blk, berr := l.SessionContextBuilder(ctx, prompt); berr == nil && strings.TrimSpace(blk) != "" {
			msgs = append(msgs, session.Message{Role: "user", Content: blk})
			l.fire(ctx, hooks.SessionStart, "", map[string]any{"bytes": len(blk)})
		}
	}
	msgs = append(msgs, session.Message{Role: "user", Content: prompt})

	if l.Frustration != nil {
		l.Frustration.Track(prompt, time.Now())
	}

	if l.MemoryPrime != nil {
		if primed, err := l.MemoryPrime(ctx, prompt); err == nil && strings.TrimSpace(primed) != "" {
			l.fire(ctx, hooks.MemoryPrime, "", map[string]any{"chars": len(primed)})
			msgs = append(msgs, session.Message{Role: "user", Content: primed})
		}
	}

	// Learning loop closed: inject accumulated workspace lessons before the
	// first turn so the agent never repeats a recorded mistake.
	if l.Lessons != nil {
		briefCtx := map[string]any{"prompt": prompt}
		if briefing, err := l.Lessons.BriefingForContext(ctx, l.Workspace, briefCtx, 10, 2048); err == nil && briefing != "" {
			msgs = append(msgs, session.Message{Role: "user", Content: briefing})
		}
	}

	return msgs
}
