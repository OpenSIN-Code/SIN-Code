// SPDX-License-Identifier: MIT
// Purpose: SIN-Code core agent loop: PLAN -> ACT -> VERIFY -> DONE
// (mandates C1, C3, AGENTS.md §8). Hook engine (C7) and permission
// engine (M4) are wired at all documented event points (issues #46, #47).
package agentloop

import (
	"context"
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

type Completion struct {
	Text      string
	ToolCalls []ToolCall
	Raw       session.Message
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type LocalToolFunc func(ctx context.Context, name string, args map[string]any) (string, error)

type AskFunc func(tc ToolCall) bool

// StopSnapshot is the read-only view of a run handed to the stop-gate when
// the worker proposes completion (no more tool calls AND the verify-gate
// passed). It carries just enough signal for an independent evaluator to
// decide whether the goal is truly done.
type StopSnapshot struct {
	Prompt       string
	FinalOutput  string
	Turns        int
	ToolsUsed    []string
	VerifyPassed bool
	SessionID    string
}

// StopDecision is the verdict returned by a StopGate. Complete=false forces
// the loop to keep working, re-injecting OpenCriteria as the next instruction.
type StopDecision struct {
	Complete     bool
	OpenCriteria []string
	Report       string
}

// StopGate decouples completion authority from the worker. It is consulted
// only AFTER the verify-gate passes, and may reject the proposed completion
// (Complete=false) to force continued work — the core anti-babysitting hook.
// A nil StopGate preserves the legacy behavior exactly.
type StopGate func(ctx context.Context, snap StopSnapshot) StopDecision

type Loop struct {
	Gate      *verify.Gate
	LocalTool LocalToolFunc
	LocalSpec []ToolSpec
	Workspace string
	MaxTurns  int
	// MaxStopRejects caps how many times the stop-gate can reject
	// completion before the run errors. Zero falls back to the
	// default of 3. Independent of StallThreshold (issue #150):
	// MaxStopRejects is a hard count, StallThreshold is an
	// identical-criteria fingerprint count.
	MaxStopRejects int
	SessionID      string
	Completion     func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error)

	Hooks   *hooks.Engine
	Perm    *permission.Engine
	Ask     AskFunc
	Lessons *lessons.Store

	// StopGate, if set, is consulted when the worker proposes completion and
	// the verify-gate has already passed. If it returns Complete=false the
	// loop re-injects the open criteria and keeps working instead of
	// returning DONE. Optional — nil keeps the legacy single-gate behavior.
	StopGate StopGate

	// StallThreshold escalates early when the stop-gate returns the SAME set
	// of open criteria this many times consecutively (no progress). Zero
	// disables stall detection. Recommended: 3. Independent of MaxStopRejects.
	StallThreshold int

	// AllowContinuation switches the maxTurns outcome from a hard error to a
	// checkpointed, resumable Result (Continuation=true). Daemons set this so
	// a long task is re-enqueued and resumed rather than abandoned; one-shot
	// CLI callers leave it false to preserve the legacy error.
	AllowContinuation bool

	// Preamble, if set, is injected as a user message before the goal prompt.
	// The SinCode Loop System uses it to state the Definition-of-Done up front
	// (write tests, no debug leftovers, finish the job, keep docs in sync) so
	// the worker does that work proactively instead of waiting to be told. It
	// is advisory; the stop-gate independently enforces the same contract.
	Preamble string

	// CompressMessages, if set, is invoked on the message history before
	// every model request to reduce token usage (e.g. via Headroom). It
	// returns a possibly-rewritten history; on error or nil result the
	// original history is used so compression never breaks a run.
	CompressMessages func(ctx context.Context, msgs []session.Message) ([]session.Message, error)

	// Ledger records every prompt, tool call, and verification result for
	// auditability and auto-summaries (issue #43). Optional — loop works
	// without it for backward compatibility.
	Ledger *ledger.Store

	// RunOverride, if set, replaces the default Run. Used by the
	// WebUI v2 chat API (issue #52) so tests can swap in a
	// deterministic result without wiring a real LLM.
	RunOverride func(ctx context.Context, sess *session.Session, prompt string) (*Result, error)
}

type Result struct {
	SessionID string `json:"session_id"`
	Summary   string `json:"summary"`
	Verified  bool   `json:"verified"`
	Turns     int    `json:"turns"`
	// Continuation is true when the run hit maxTurns with AllowContinuation
	// enabled: the work is checkpointed (not failed) and should be resumed.
	Continuation bool `json:"continuation,omitempty"`
	// OpenCriteria carries the unmet acceptance criteria when the run ends
	// without verified completion (stop-gate reject or continuation).
	OpenCriteria []string `json:"open_criteria,omitempty"`
}

func (l *Loop) tools() []ToolSpec { return l.LocalSpec }

func (l *Loop) record(ctx context.Context, typ ledger.EntryType, data map[string]any, summary string) {
	if l.Ledger == nil || l.SessionID == "" {
		return
	}
	_, _ = l.Ledger.Record(ctx, ledger.Entry{
		SessionID: l.SessionID,
		Type:      typ,
		Data:      data,
		Summary:   summary,
	})
}

func (l *Loop) fire(ctx context.Context, event, name string, data map[string]any) hooks.Result {
	if l.Hooks == nil {
		return hooks.Result{}
	}
	return l.Hooks.Fire(ctx, hooks.Payload{
		Event:     event,
		SessionID: l.SessionID,
		Workspace: l.Workspace,
		Name:      name,
		Data:      data,
	})
}

func (l *Loop) execute(ctx context.Context, tc ToolCall) (out string, injects []string) {
	pre := l.fire(ctx, hooks.ToolPre, tc.Name, map[string]any{"args": tc.Args})
	injects = append(injects, pre.PromptInjects...)
	if pre.Blocked {
		return "BLOCKED by hook: " + pre.BlockReason, injects
	}

	if l.Perm != nil {
		switch l.Perm.Check(tc.Name) {
		case permission.Deny:
			l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "deny"})
			return "DENIED by permission policy", injects
		case permission.Ask:
			ask := l.fire(ctx, hooks.PermissionAsk, tc.Name, map[string]any{"args": tc.Args})
			injects = append(injects, ask.PromptInjects...)
			if ask.Blocked {
				l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "ask", "by": "hook"})
				return "DENIED by hook: " + ask.BlockReason, injects
			}
			if l.Ask == nil || !l.Ask(tc) {
				l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "ask", "by": "user"})
				return "DENIED by user", injects
			}
		case permission.Allow:
		}
	}

	if l.LocalTool == nil {
		return "TOOL ERROR: no LocalTool registered", injects
	}
	res, err := l.LocalTool(ctx, tc.Name, tc.Args)
	if err != nil {
		l.fire(ctx, hooks.ToolError, tc.Name, map[string]any{"error": err.Error()})
		l.record(ctx, ledger.TypeToolError, map[string]any{"tool": tc.Name}, "tool error: "+tc.Name)
		if l.Lessons != nil {
			_ = l.Lessons.Record(ctx, lessons.Entry{
				Type:      lessons.TypeToolError,
				Workspace: l.Workspace,
				Context:   map[string]any{"tool": tc.Name},
				Lesson:    "Tool " + tc.Name + " failed: " + err.Error(),
			})
		}
		return "TOOL ERROR: " + err.Error(), injects
	}
	post := l.fire(ctx, hooks.ToolPost, tc.Name, map[string]any{"output_bytes": len(res)})
	injects = append(injects, post.PromptInjects...)
	l.record(ctx, ledger.TypeToolCall, map[string]any{"tool": tc.Name}, "tool call: "+tc.Name)
	return res, injects
}

func (l *Loop) Run(ctx context.Context, sess *session.Session, prompt string) (*Result, error) {
	if l.RunOverride != nil {
		return l.RunOverride(ctx, sess, prompt)
	}
	if l.Completion == nil {
		return nil, fmt.Errorf("agentloop: Completion func not wired")
	}
	if l.SessionID == "" {
		l.SessionID = sess.ID
	}
	l.record(ctx, ledger.TypeUserPrompt, map[string]any{"content": prompt}, "user prompt")
	msgs := sess.History()
	// SinCode Loop System: state the Definition-of-Done before the goal so the
	// worker addresses tests/debug/docs/completeness proactively. Enforcement
	// still lives in the stop-gate; this only improves first-pass quality.
	if strings.TrimSpace(l.Preamble) != "" {
		msgs = append(msgs, session.Message{Role: "user", Content: l.Preamble})
	}
	msgs = append(msgs, session.Message{Role: "user", Content: prompt})

	// Learning loop closed: inject accumulated workspace lessons before the
	// first turn so the agent never repeats a recorded mistake.
	if l.Lessons != nil {
		if entries, err := l.Lessons.Query(ctx, l.Workspace, 25); err == nil {
			if briefing := lessons.Briefing(entries, 10, 2048); briefing != "" {
				msgs = append(msgs, session.Message{Role: "user", Content: briefing})
			}
		}
	}

	maxTurns := l.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 80
	}
	tools := l.tools()

	var pendingInjects []string
	var lastText string
	var lastOpen []string
	stopRejects := 0 // tracks how many times the stop-gate rejected completion
	lastCritFingerprint := ""
	stallCount := 0
	toolsSeen := map[string]bool{}
	var toolsUsed []string

	for turn := 0; turn < maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(pendingInjects) > 0 {
			msgs = append(msgs, session.Message{
				Role:    "user",
				Content: "HOOK INJECT:\n" + strings.Join(pendingInjects, "\n"),
			})
			pendingInjects = nil
		}
		reqMsgs := msgs
		if l.CompressMessages != nil {
			if compressed, cerr := l.CompressMessages(ctx, msgs); cerr == nil && compressed != nil {
				reqMsgs = compressed
			}
		}
		resp, err := l.Completion(ctx, reqMsgs, tools)
		if err != nil {
			return nil, fmt.Errorf("turn %d: %w", turn, err)
		}
		msgs = append(msgs, resp.Raw)

		if len(resp.ToolCalls) == 0 {
			vpre := l.fire(ctx, hooks.VerifyPre, "", nil)
			pendingInjects = append(pendingInjects, vpre.PromptInjects...)
			if vpre.Blocked {
				msgs = append(msgs, session.Message{
					Role:    "user",
					Content: "VERIFICATION BLOCKED by hook — fix before claiming completion:\n" + vpre.BlockReason,
				})
				if err := sess.SaveHistory(msgs); err != nil {
					return nil, err
				}
				continue
			}

			res := l.Gate.Run(ctx, l.Workspace)
			if !res.Passed {
				vf := l.fire(ctx, hooks.VerifyFail, "", map[string]any{
					"mode": string(res.Mode), "report": res.Report,
				})
				l.record(ctx, ledger.TypeVerifyFail, map[string]any{"mode": string(res.Mode)}, "verification failed ("+string(res.Mode)+")")
				pendingInjects = append(pendingInjects, vf.PromptInjects...)
				if l.Lessons != nil {
					_ = l.Lessons.Record(ctx, lessons.Entry{
						Type:      lessons.TypeFailedVerification,
						Workspace: l.Workspace,
						Context:   map[string]any{"mode": string(res.Mode)},
						Lesson:    "Verification failed (" + string(res.Mode) + "): " + res.Report,
					})
				}
				msgs = append(msgs, session.Message{
					Role:    "user",
					Content: "VERIFICATION FAILED (" + string(res.Mode) + ") — fix before claiming completion:\n" + res.Report,
				})
				if err := sess.SaveHistory(msgs); err != nil {
					return nil, err
				}
				continue
			}
			l.fire(ctx, hooks.VerifyPass, "", map[string]any{
				"mode": string(res.Mode), "report": res.Report,
			})
			l.record(ctx, ledger.TypeVerifyPass, map[string]any{"mode": string(res.Mode)}, "verification passed ("+string(res.Mode)+")")

			// Stop-gate: completion authority is decoupled from the worker.
			// The verify-gate passing is necessary but not sufficient — an
			// independent evaluator confirms the goal contract is satisfied
			// before we accept DONE. A reject re-injects the open criteria
			// and keeps the loop working (the core anti-babysitting path).
			lastText = resp.Text
			if l.StopGate != nil {
				dec := l.StopGate(ctx, StopSnapshot{
					Prompt:       prompt,
					FinalOutput:  resp.Text,
					Turns:        turn + 1,
					ToolsUsed:    toolsUsed,
					VerifyPassed: res.Passed,
					SessionID:    sess.ID,
				})
				l.fire(ctx, hooks.StopEval, "", map[string]any{
					"complete": dec.Complete, "open_criteria": dec.OpenCriteria,
				})
				if !dec.Complete {
					lastOpen = dec.OpenCriteria
					stopRejects++
					l.fire(ctx, hooks.StopContinue, "", map[string]any{
						"open_criteria": dec.OpenCriteria, "report": dec.Report,
					})
					l.record(ctx, ledger.TypeStopContinue,
						map[string]any{"open_criteria": dec.OpenCriteria},
						"stop-gate rejected completion; continuing")
					// Stagnation guard: identical open criteria across consecutive
					// rejects means the worker is stuck. Escalate early.
					fp := strings.Join(dec.OpenCriteria, "\x1f")
					if fp != "" && fp == lastCritFingerprint {
						stallCount++
					} else {
						stallCount = 1
						lastCritFingerprint = fp
					}
					if l.StallThreshold > 0 && stallCount >= l.StallThreshold {
						if serr := sess.SaveHistory(msgs); serr != nil {
							return nil, serr
						}
						l.fire(ctx, hooks.StopStalled, "", map[string]any{
							"stall_count": stallCount, "open_criteria": lastOpen,
						})
						l.record(ctx, ledger.TypeStallDetected,
							map[string]any{"stall_count": stallCount, "open_criteria": lastOpen},
							fmt.Sprintf("no progress: identical open criteria %d turns in a row; escalating", stallCount))
						return nil, fmt.Errorf(
							"stop-gate stalled: identical open criteria %d turns in a row "+
								"(StallThreshold=%d); open criteria: %s",
							stallCount, l.StallThreshold, strings.Join(lastOpen, "; "),
						)
					}
					if l.Lessons != nil {
						_ = l.Lessons.Record(ctx, lessons.Entry{
							Type:      lessons.TypeFailedVerification,
							Workspace: l.Workspace,
							Context:   map[string]any{"open_criteria": dec.OpenCriteria},
							Lesson:    "Stop-gate rejected premature completion: " + strings.Join(dec.OpenCriteria, "; "),
						})
					}
					msgs = append(msgs, session.Message{
						Role:    "user",
						Content: formatStopContinue(dec),
					})
					if err := sess.SaveHistory(msgs); err != nil {
						return nil, err
					}
					// Hard cap on stop-gate rejections. Independent of
					// StallThreshold (issue #150): MaxStopRejects is a
					// straight count, stall is a fingerprint match.
					maxRejects := l.MaxStopRejects
					if maxRejects <= 0 {
						maxRejects = 3
					}
					if stopRejects >= maxRejects {
						return nil, fmt.Errorf("stop-gate rejected completion %d times (max %d); open criteria: %s",
							stopRejects, maxRejects, strings.Join(lastOpen, "; "))
					}
					continue
				}
			}

			if err := sess.SaveHistory(msgs); err != nil {
				return nil, err
			}
			result := &Result{
				SessionID: sess.ID, Summary: resp.Text,
				Verified: res.Passed, Turns: turn + 1,
			}
			l.fire(ctx, hooks.TaskComplete, "", map[string]any{
				"summary": result.Summary, "turns": result.Turns, "verified": result.Verified,
			})
			l.record(ctx, ledger.TypeTaskComplete, map[string]any{"summary": result.Summary, "turns": result.Turns, "verified": result.Verified}, "task complete: "+result.Summary)
			return result, nil
		}

		for _, tc := range resp.ToolCalls {
			if !toolsSeen[tc.Name] {
				toolsSeen[tc.Name] = true
				toolsUsed = append(toolsUsed, tc.Name)
			}
			out, injects := l.execute(ctx, tc)
			pendingInjects = append(pendingInjects, injects...)
			msgs = append(msgs, session.Message{
				Role: "tool", ToolCallID: tc.ID, Content: out,
			})
		}
		if err := sess.SaveHistory(msgs); err != nil {
			return nil, err
		}
	}
	// maxTurns reached without verified completion.
	if l.AllowContinuation {
		// Checkpoint instead of abandoning: persist history and hand back a
		// resumable Result so the caller (daemon) can re-enqueue and continue
		// with the same session — a long task never needs a human restart.
		if err := sess.SaveHistory(msgs); err != nil {
			return nil, err
		}
		summary := fmt.Sprintf("checkpoint after %d turns (max reached); resuming", maxTurns)
		l.record(ctx, ledger.TypeTaskCheckpoint, map[string]any{
			"turns": maxTurns, "open_criteria": lastOpen,
		}, summary)
		l.fire(ctx, hooks.TaskAbort, "", map[string]any{
			"reason": "max turns exceeded", "continuation": true,
		})
		if lastText == "" {
			lastText = summary
		}
		return &Result{
			SessionID:    sess.ID,
			Summary:      lastText,
			Verified:     false,
			Turns:        maxTurns,
			Continuation: true,
			OpenCriteria: lastOpen,
		}, nil
	}
	l.fire(ctx, hooks.TaskAbort, "", map[string]any{"reason": "max turns exceeded"})
	l.record(ctx, ledger.TypeTaskAbort, map[string]any{"reason": "max turns exceeded"}, "task aborted: max turns exceeded")
	return nil, fmt.Errorf("max turns (%d) exceeded without verified completion", maxTurns)
}

// formatStopContinue renders the stop-gate rejection into a directive the
// model can act on: explicit, numbered, and unambiguous about NOT being done.
func formatStopContinue(dec StopDecision) string {
	var b strings.Builder
	b.WriteString("NOT DONE — the work is not complete yet. ")
	b.WriteString("An independent evaluator rejected the proposed completion.\n")
	if len(dec.OpenCriteria) > 0 {
		b.WriteString("Open acceptance criteria that MUST be satisfied:\n")
		for i, c := range dec.OpenCriteria {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, c)
		}
	}
	if strings.TrimSpace(dec.Report) != "" {
		b.WriteString("Evaluator notes:\n")
		b.WriteString(dec.Report)
		b.WriteString("\n")
	}
	b.WriteString("Continue working until every criterion is met, then stop.")
	return b.String()
}
