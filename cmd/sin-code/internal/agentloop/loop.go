// SPDX-License-Identifier: MIT
// Purpose: SIN-Code core agent loop: PLAN -> ACT -> VERIFY -> DONE
// (mandates C1, C3, AGENTS.md §8). Hook engine (C7) and permission
// engine (M4) are wired at all documented event points (issues #46, #47).
//
// Type definitions live in types.go; the Loop struct lives in
// loop_struct.go; budget enforcement lives in loop_budget.go;
// session-start message preparation lives in loop_run_init.go;
// max-turns handling lives in loop_maxturns.go.
package agentloop

import (
	"context"
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

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
	// Ensure the coverage enforcer exists when constraints are configured.
	// It is recreated per-Run so REPL/dataset reuse of the same Loop gets
	// fresh state for each prompt/test case (issue #248).
	if len(l.CoverageRequiredTools) > 0 || len(l.CoverageForbiddenTools) > 0 {
		l.Coverage = NewToolCoverageEnforcer(l.CoverageRequiredTools, l.CoverageForbiddenTools)
	}
	msgs := l.prepareRunMessages(ctx, sess, prompt)

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
	bs := &budgetState{} // issue #151: cumulative tokens, budget warn, verifiedOnly

	// Issue: Thinking Budget Enforcement (first PR). Reset the per-run
	// thinking accumulator so a second Run() on the same Loop instance
	// starts at zero. The Loop itself is documented as one-Run-at-a-time
	// (mandate M7), so we do not need a mutex on this field.
	l.thinkingUsed = 0
	// Issue #375: lazy-construct per-turn tracker only when at least
	// one per-turn cap is wired. No-cap path stays zero-cost.
	if l.PerTurnBudget > 0 || l.PerTurnThinkingBudget > 0 {
		l.perTurnBudget = NewPerTurnBudget(l.PerTurnThinkingBudget, l.PerTurnBudget)
	} else {
		l.perTurnBudget = nil
	}
	if l.perTurnBudget != nil {
		l.perTurnBudget.Reset()
	}
	reflectedThisProposal := false
	toolsSeen := map[string]bool{}
	var toolsUsed []string

	var runTurns int
	defer func() {
		l.fire(ctx, hooks.SessionEnd, "", map[string]any{"turns": runTurns})
	}()

	for turn := 0; turn < maxTurns; turn++ {
		runTurns = turn + 1
		l.fire(ctx, hooks.TurnStart, "", map[string]any{"turn": turn})
		l.emitProgress(ProgressEvent{Event: "turn.start", Turn: turn})
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
		if l.Compactor != nil {
			if l.shouldFireCompaction(maxTurns, msgs) {
				// Mirror any config drift the compactor holds (e.g. CLI
				// override after NewCompactor) so the loop honours the
				// freshest settings on every turn.
				l.Compactor.Configure(l.buildCompactorConfig())
				maxTkns := l.CompactionMaxTokens
				if maxTkns <= 0 {
					maxTkns = 8000
				}
				ctxWin := l.effectiveContextWindow(maxTkns)
				threshold := l.CompactionThreshold
				if threshold <= 0 {
					threshold = DefaultCompactionThreshold
				}
				cpre := l.fire(ctx, hooks.CompactionPre, "", map[string]any{
					"messages_before": len(msgs),
					"strategy":        l.CompactionStrategy.String(),
					"mode":            l.ContextCompactionMode.String(),
					"trigger":         l.CompactionTrigger.String(),
					"max_tokens":      maxTkns,
					"context_window":  ctxWin,
					"threshold":       threshold,
				})
				// Issue: Context Compaction Modes (first PR). When a
				// non-off Mode is configured we keep the unbounded history
				// in `msgs` (so the persisted session DB stays complete,
				// mandate M3 verification audit) and produce a
				// deterministically compacted view for the model via
				// Compact2. The legacy Compact() path remains unchanged
				// when Mode is off.
				ctxSnapshot := l.compactionSnapshot(ctx, sess, msgs)
				if ctxSnapshot.mode != "" && ctxSnapshot.mode != ContextCompactionOff {
					if path := l.writeCompactionSidecar(sess, turn, ctxSnapshot.result); path != "" {
						cpre.PromptInjects = append(cpre.PromptInjects,
							"[CONTEXT-COMPACTION] prior turns were summarised — see "+path)
					}
				} else if len(ctxSnapshot.result.Kept) != len(msgs) {
					// Legacy in-place compaction: persist the compacted
					// history so future --resume reads the trimmed view.
					msgs = ctxSnapshot.result.Kept
				}
				for _, inj := range cpre.PromptInjects {
					msgs = append(msgs, session.Message{Role: "user", Content: inj})
				}
				if err := l.saveHistory(ctx, sess, msgs); err != nil {
					return nil, err
				}
				l.fire(ctx, hooks.MemoryCompact, "", map[string]any{
					"messages_before": len(msgs),
					"messages_after":  len(ctxSnapshot.result.Kept),
					"mode":            ctxSnapshot.mode.String(),
					"turn":            turn,
				})
			}
		}
		reqMsgs := msgs
		if l.CompressMessages != nil {
			if compressed, cerr := l.CompressMessages(ctx, msgs); cerr == nil && compressed != nil {
				reqMsgs = compressed
			}
		}
		// Mandate M6: the tool-preference block is prepended fresh each
		// turn so the model cannot compress it away.
		if l.SystemPrompt != "" {
			sysContent := l.SystemPrompt
			if l.Frustration != nil {
				sysContent += l.Frustration.SystemPromptSuffix()
			}
			reqMsgs = append([]session.Message{{Role: "system", Content: sysContent}}, reqMsgs...)
		}
		resp, err := l.getCompletion()(ctx, reqMsgs, tools)
		if err != nil {
			return nil, fmt.Errorf("turn %d: %w", turn, err)
		}
		msgs = append(msgs, resp.Raw)
		// Budget enforcement: per-turn, token, and thinking caps.
		// Extracted to loop_budget.go — returns non-nil to stop the run.
		if bres, berr := l.checkBudgets(ctx, sess, msgs, resp, turn, bs, lastText, lastOpen); bres != nil || berr != nil {
			return bres, berr
		}

		if len(resp.ToolCalls) == 0 {
			vpre := l.fire(ctx, hooks.VerifyPre, "", nil)
			pendingInjects = append(pendingInjects, vpre.PromptInjects...)
			if vpre.Blocked {
				msgs = append(msgs, session.Message{
					Role:    "user",
					Content: "VERIFICATION BLOCKED by hook — fix before claiming completion:\n" + vpre.BlockReason,
				})
				if err := l.saveHistory(ctx, sess, msgs); err != nil {
					return nil, err
				}
				continue
			}

			res := l.Gate.Run(ctx, l.Workspace)
			if !res.Passed {
				vf := l.fire(ctx, hooks.VerifyFail, "", map[string]any{
					"mode": string(res.Mode), "report": res.Report,
				})
				l.emitProgress(ProgressEvent{
					Event: "verify.fail",
					Turn:  turn,
					Data:  map[string]any{"mode": string(res.Mode)},
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

				// SIN Fusion v1: if a TournamentRunner is wired and the
				// failure is structural, fan out to N providers instead
				// of retrying with the same model. First PoC-pass wins.
				// Oracle mode is also supported when the tournament is
				// explicitly configured for oracle (issue #344); the
				// tournament judge selects the winner, not first-pass-wins.
				if l.TournamentRunner != nil &&
					(l.Gate.Mode() == verify.ModePoC || l.Gate.Mode() == verify.ModeOracle) &&
					l.TournamentRunner.ShouldRun(res) {
					output, tokens, terr := l.TournamentRunner.Run(ctx, prompt)
					if terr == nil && output != "" {
						l.fire(ctx, hooks.VerifyPass, "", map[string]any{
							"mode": "poc", "report": "fusion tournament: winner passed verify-gate",
						})
						l.record(ctx, ledger.TypeVerifyPass,
							map[string]any{"mode": "poc", "fusion": true},
							"fusion tournament winner passed verify-gate")
						bs.totalTokens += tokens
						result := &Result{
							SessionID: sess.ID, Summary: output,
							Verified: true, Turns: turn + 1,
							Tokens: bs.totalTokens,
						}
						l.fire(ctx, hooks.TaskComplete, "", map[string]any{
							"summary": result.Summary, "turns": result.Turns,
							"verified": true, "fusion": true,
						})
						l.record(ctx, ledger.TypeTaskComplete,
							map[string]any{"summary": result.Summary, "fusion": true},
							"fusion tournament task complete")
						return result, nil
					}
				}

				msgs = append(msgs, session.Message{
					Role:    "user",
					Content: "VERIFICATION FAILED (" + string(res.Mode) + ") — fix before claiming completion:\n" + res.Report,
				})
				if err := l.saveHistory(ctx, sess, msgs); err != nil {
					return nil, err
				}
				continue
			}
			l.fire(ctx, hooks.VerifyPass, "", map[string]any{
				"mode": string(res.Mode), "report": res.Report,
			})
			l.emitProgress(ProgressEvent{
				Event: "verify.pass",
				Turn:  turn,
				Data:  map[string]any{"mode": string(res.Mode)},
			})
			l.record(ctx, ledger.TypeVerifyPass, map[string]any{"mode": string(res.Mode)}, "verification passed ("+string(res.Mode)+")")

			// Self-reflection: one cheap self-critique pass before the
			// independent stop-gate. Reset the flag whenever the worker did
			// real work (tool calls) in between, so each fresh proposal gets
			// exactly one reflection. Issue #152.
			if l.Reflector != nil && !reflectedThisProposal {
				reflectedThisProposal = true
				reflectTools := toolsUsed
				if l.Coverage != nil {
					reflectTools = l.Coverage.Used()
				}
				ref := l.Reflector(ctx, StopSnapshot{
					Prompt: prompt, FinalOutput: resp.Text, Turns: turn + 1,
					ToolsUsed: reflectTools, VerifyPassed: res.Passed, SessionID: sess.ID,
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
					if err := l.saveHistory(ctx, sess, msgs); err != nil {
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
			effectiveStopGate := l.wrapStopGate()
			if effectiveStopGate != nil {
				snapTools := toolsUsed
				if l.Coverage != nil {
					snapTools = l.Coverage.Used()
				}
				dec := effectiveStopGate(ctx, StopSnapshot{
					Prompt:       prompt,
					FinalOutput:  resp.Text,
					Turns:        turn + 1,
					ToolsUsed:    snapTools,
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
						if serr := l.saveHistory(ctx, sess, msgs); serr != nil {
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
					if err := l.saveHistory(ctx, sess, msgs); err != nil {
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

			if err := l.saveHistory(ctx, sess, msgs); err != nil {
				return nil, err
			}
			result := &Result{
				SessionID: sess.ID, Summary: resp.Text,
				Verified: res.Passed, Turns: turn + 1,
				Tokens: bs.totalTokens,
			}
			l.fire(ctx, hooks.TaskComplete, "", map[string]any{
				"summary": result.Summary, "turns": result.Turns, "verified": result.Verified,
			})
			l.emitProgress(ProgressEvent{
				Event: "task.complete",
				Turn:  turn,
				Data: map[string]any{
					"verified": result.Verified,
					"turns":    result.Turns,
				},
			})
			l.record(ctx, ledger.TypeTaskComplete, map[string]any{"summary": result.Summary, "turns": result.Turns, "verified": result.Verified}, "task complete: "+result.Summary)
			return result, nil
		}

		for _, tc := range resp.ToolCalls {
			// Real work happened in this turn — reset the reflection
			// flag so a fresh proposal can be re-evaluated.
			reflectedThisProposal = false
			if l.Coverage != nil {
				l.Coverage.Record(tc.Name)
			}
			if l.LoopDetector != nil && l.LoopDetector.Record(tc.Name) {
				l.fire(ctx, hooks.LoopDetected, "", map[string]any{"tool": tc.Name})
				return nil, fmt.Errorf("observer loop detected: repeated tool calls (last tool %s)", tc.Name)
			}
			if !toolsSeen[tc.Name] {
				toolsSeen[tc.Name] = true
				toolsUsed = append(toolsUsed, tc.Name)
			}
			// Observer-loop detection (issue #377): any tool call
			// whose fingerprint closes a repeated-sequence cycle
			// returns ErrLoopDetected. Fail-closed: surface as a
			// TOOL REFUSED message and skip execute() so the model
			// gets feedback AND the dispatch site never reaches a
			// destructive mutator while the worker is thrashing.
			if l.LoopDetector != nil && l.LoopDetector.Enabled() {
				if oerr := l.LoopDetector.Observe(tc, ""); oerr != nil {
					trip := l.LoopDetector.LastTrip()
					data := map[string]any{"reason": "loop.detected"}
					if trip != nil {
						data["pattern_length"] = trip.Length
						data["repeats"] = trip.Repeats
						data["tool"] = trip.ToolName
						data["key"] = trip.Key
						data["history_len"] = trip.HistoryLen
					}
					l.fire(ctx, hooks.LoopDetected, tc.Name, data)
					msgs = append(msgs, session.Message{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content: "TOOL REFUSED: " + oerr.Error() +
							" — refusing dispatch of " + tc.Name +
							"; the model should break the cycle.",
					})
					continue
				}
			}
			out, injects := l.execute(ctx, tc)
			pendingInjects = append(pendingInjects, injects...)
			msgs = append(msgs, session.Message{
				Role: "tool", ToolCallID: tc.ID, Content: out,
			})
		}
		if err := l.saveHistory(ctx, sess, msgs); err != nil {
			return nil, err
		}
		l.fire(ctx, hooks.TurnEnd, "", map[string]any{"turn": turn})
	}
	// maxTurns reached without verified completion.
	return l.handleMaxTurns(ctx, sess, msgs, maxTurns, bs.totalTokens, lastText, lastOpen)
}
