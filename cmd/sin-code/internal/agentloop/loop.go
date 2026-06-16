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

// Usage carries token accounting returned by the model provider. All fields
// optional; zero values mean "unknown" and never trigger the budget guard.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Completion struct {
	Text      string
	ToolCalls []ToolCall
	Raw       session.Message
	Usage     Usage // provider token accounting (optional)
}

// Reflection is the worker's self-critique of a proposed completion. Issues
// non-empty means the agent found problems in its own work and should fix
// them before the stop-gate is consulted.
type Reflection struct {
	Issues []string
	Notes  string
}

// Reflector performs a self-critique pass on a proposed completion. Returning
// a Reflection with non-empty Issues forces one more work turn. A nil
// Reflector disables the reflection step (legacy behavior).
type Reflector func(ctx context.Context, snap StopSnapshot) Reflection

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
	Gate       *verify.Gate
	LocalTool  LocalToolFunc
	LocalSpec  []ToolSpec
	Workspace  string
	MaxTurns   int
	SessionID  string
	Completion func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error)

	Hooks   *hooks.Engine
	Perm    *permission.Engine
	Ask     AskFunc
	Lessons *lessons.Store

	// StopGate, if set, is consulted when the worker proposes completion and
	// the verify-gate has already passed. If it returns Complete=false the
	// loop re-injects the open criteria and keeps working instead of
	// returning DONE. Optional — nil keeps the legacy single-gate behavior.
	StopGate StopGate

	// MaxStopRejects caps how many times the stop-gate may reject a proposed
	// completion before the run escalates (error, or checkpoint when
	// AllowContinuation is set). Zero disables the cap (legacy behavior).
	MaxStopRejects int

	// StallThreshold escalates early when the stop-gate returns the SAME set
	// of open criteria this many times consecutively (no progress). Zero
	// disables stall detection. Recommended: 3. Independent of MaxStopRejects.
	StallThreshold int

	// MaxTokens is a hard cap on cumulative tokens (prompt+completion) across
	// the whole run. Zero means unlimited. When exceeded the run checkpoints
	// (if AllowContinuation) or errors, rather than continuing to spend.
	MaxTokens int

	// BudgetWarnRatio, if set (e.g. 0.8), fires hooks.BudgetWarn once when
	// cumulative token usage crosses that fraction of MaxTokens.
	BudgetWarnRatio float64

	// Reflector, if set, runs a self-critique pass right BEFORE the stop-gate.
	// If it returns issues, the loop injects them and continues working — a
	// cheap quality lift that reduces stop-gate rejections. Runs at most once
	// per proposed completion to avoid infinite self-doubt loops.
	Reflector Reflector

	// SubagentStore, when set, enables the built-in spawn_subagent tool: the
	// worker can delegate isolated subtasks to child loops that share this
	// loop's wiring but get their own session from this store. Nil disables
	// delegation entirely (the tool is neither advertised nor accepted).
	SubagentStore *session.Store

	// AllowContinuation switches the maxTurns outcome from a hard error to a
	// checkpointed, resumable Result (Continuation=true). Daemons set this so
	// a long task is re-enqueued and resumed rather than abandoned; one-shot
	// CLI callers leave it false to preserve the legacy error.
	AllowContinuation bool

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

func (l *Loop) tools() []ToolSpec {
	if !l.subagentEnabled() {
		return l.LocalSpec
	}
	// Advertise the built-in delegation tool alongside the wired tools.
	return append(append([]ToolSpec(nil), l.LocalSpec...), subagentSpec)
}

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

	// Built-in delegation tool: handled by the loop itself (spawns a child
	// loop) rather than the user-supplied LocalTool dispatcher.
	if tc.Name == SpawnSubagentTool && l.subagentEnabled() {
		out := l.handleSpawnSubagent(ctx, tc.Args)
		l.fire(ctx, hooks.ToolPost, tc.Name, map[string]any{"output_bytes": len(out)})
		l.record(ctx, ledger.TypeToolCall, map[string]any{"tool": tc.Name}, "tool call: "+tc.Name)
		return out, injects
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
	toolsSeen := map[string]bool{}
	var toolsUsed []string

	stopRejects := 0 // how many times the stop-gate rejected completion
	lastCritFingerprint := ""
	stallCount := 0
	totalTokens := 0
	warnedBudget := false
	reflectedThisProposal := false

	for turn := 0; turn < maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Each turn yields a fresh completion (a new proposal), so it gets
		// exactly one self-reflection pass — the flag guards against
		// re-reflecting within a single proposal, never across proposals.
		reflectedThisProposal = false
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

		// Token budget accounting. Provider usage is optional; if zero we
		// simply skip the guard for that turn (backward compatible).
		if u := resp.Usage.TotalTokens; u > 0 {
			totalTokens += u
		} else {
			totalTokens += resp.Usage.PromptTokens + resp.Usage.CompletionTokens
		}
		if l.MaxTokens > 0 {
			if !warnedBudget && l.BudgetWarnRatio > 0 &&
				float64(totalTokens) >= l.BudgetWarnRatio*float64(l.MaxTokens) {
				warnedBudget = true
				l.fire(ctx, hooks.BudgetWarn, "", map[string]any{
					"total_tokens": totalTokens, "max_tokens": l.MaxTokens,
				})
			}
			if totalTokens >= l.MaxTokens {
				if serr := sess.SaveHistory(msgs); serr != nil {
					return nil, serr
				}
				l.fire(ctx, hooks.BudgetExhausted, "", map[string]any{
					"total_tokens": totalTokens, "max_tokens": l.MaxTokens,
				})
				l.record(ctx, ledger.TypeTokenBudgetExhausted,
					map[string]any{"total_tokens": totalTokens, "max_tokens": l.MaxTokens},
					fmt.Sprintf("token budget exhausted: %d/%d", totalTokens, l.MaxTokens))
				if l.AllowContinuation {
					return &Result{
						SessionID: sess.ID, Summary: lastText, Verified: false,
						Turns: turn + 1, Continuation: true, OpenCriteria: lastOpen,
					}, nil
				}
				return nil, fmt.Errorf("token budget exhausted: %d/%d tokens used", totalTokens, l.MaxTokens)
			}
		}

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

			// Self-reflection: one cheap self-critique pass before the
			// independent stop-gate. The flag is reset whenever the worker
			// did real work (tool calls) in between, so each fresh proposal
			// gets exactly one reflection (no infinite self-doubt loop).
			if l.Reflector != nil && !reflectedThisProposal {
				reflectedThisProposal = true
				ref := l.Reflector(ctx, StopSnapshot{
					Prompt: prompt, FinalOutput: resp.Text, Turns: turn + 1,
					ToolsUsed: toolsUsed, VerifyPassed: res.Passed, SessionID: sess.ID,
				})
				if len(ref.Issues) > 0 {
					l.fire(ctx, hooks.ReflectIssues, "", map[string]any{"issues": ref.Issues})
					l.record(ctx, ledger.TypeReflection,
						map[string]any{"issues": ref.Issues},
						"self-reflection found issues; continuing")
					var b strings.Builder
					b.WriteString("SELF-REVIEW found issues to fix before completing:\n")
					for i, is := range ref.Issues {
						fmt.Fprintf(&b, "  %d. %s\n", i+1, is)
					}
					if strings.TrimSpace(ref.Notes) != "" {
						b.WriteString("Notes: " + ref.Notes + "\n")
					}
					msgs = append(msgs, session.Message{Role: "user", Content: b.String()})
					if err := sess.SaveHistory(msgs); err != nil {
						return nil, err
					}
					continue
				}
			}

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

					// Stagnation guard: identical open criteria across
					// consecutive rejects means the worker is stuck. Escalate
					// early instead of burning the full reject budget.
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

					// Reject budget guard: cap how many times the stop-gate
					// may reject before escalating (checkpoint or error).
					if l.MaxStopRejects > 0 && stopRejects >= l.MaxStopRejects {
						if serr := sess.SaveHistory(msgs); serr != nil {
							return nil, serr
						}
						l.record(ctx, ledger.TypeTaskAbort,
							map[string]any{"reason": "max stop rejects exceeded", "stop_rejects": stopRejects, "open_criteria": lastOpen},
							fmt.Sprintf("stop-gate rejected %d times; escalating", stopRejects))
						l.fire(ctx, hooks.TaskAbort, "", map[string]any{
							"reason": "max stop rejects exceeded", "continuation": l.AllowContinuation,
						})
						if l.AllowContinuation {
							return &Result{
								SessionID: sess.ID, Summary: lastText, Verified: false,
								Turns: turn + 1, Continuation: true, OpenCriteria: lastOpen,
							}, nil
						}
						return nil, fmt.Errorf(
							"stop-gate rejected completion %d times (MaxStopRejects=%d); open criteria: %s",
							stopRejects, l.MaxStopRejects, strings.Join(lastOpen, "; "),
						)
					}

					l.fire(ctx, hooks.StopContinue, "", map[string]any{
						"open_criteria": dec.OpenCriteria, "report": dec.Report,
					})
					l.record(ctx, ledger.TypeStopContinue,
						map[string]any{"open_criteria": dec.OpenCriteria},
						"stop-gate rejected completion; continuing")
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
