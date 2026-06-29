// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loop is refactored
// Purpose: per-turn, token, and thinking budget enforcement extracted
// from Run(). Pure file split, same package, no behavioural change.
package agentloop

import (
	"context"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// budgetState holds mutable budget-tracking state shared across turns
// in Run(). It is passed by pointer to checkBudgets so modifications
// propagate back to the caller.
type budgetState struct {
	totalTokens   int
	warnedBudget  bool
	verifiedOnly  bool
}

// checkBudgets enforces per-turn, token, and thinking budget caps after
// a model response. Returns a non-nil Result when the run should return
// early (continuation or verified-after-thinking-exhaustion), a non-nil
// error when the run should fail, or nil/nil to continue the turn loop.
// Mandate M3: the verify gate is always honoured before surfacing a
// budget-exhaustion outcome.
func (l *Loop) checkBudgets(
	ctx context.Context,
	sess *session.Session,
	msgs []session.Message,
	resp *Completion,
	turn int,
	bs *budgetState,
	lastText string,
	lastOpen []string,
) (*Result, error) {
	// Issue #375: post-response per-turn charge. Charges the fresh response's
	// reasoning + total tokens into the per-turn tracker; increments BEFORE
	// checking cap so subsequent turns see accurate usage. On breach: emit
	// hooks.BudgetExceeded and toggle verifiedOnly so per-run caps
	// (MaxTokens, ThinkingBudgetPerRequest) are skipped. Mandate M3 keeps
	// the verify gate authoritative.
	if l.perTurnBudget != nil {
		perr := l.perTurnBudget.Charge(resp.Usage.ThinkingTokens, resp.Usage.TotalTokens)
		if perr != nil {
			l.fire(ctx, hooks.BudgetExhausted, "", map[string]any{
				"turn":          turn,
				"dimension":     "per-turn",
				"thinking_used": l.perTurnBudget.ThinkingUsed(),
				"thinking_cap":  l.PerTurnThinkingBudget,
				"tokens_used":   l.perTurnBudget.TokensUsed(),
				"tokens_cap":    l.PerTurnBudget,
			})
			// Per-turn cap breached. The response has already been
			// appended to msgs above so the verifier could grade it
			// (mandate M3: budget must never bypass verify). We
			// surface the cap breach to the caller as a hard error
			// so the post-mortem makes the failed gate visible.
			if serr := l.saveHistory(ctx, sess, msgs); serr != nil {
				return nil, serr
			}
			return nil, perr
		}
	}
	// Token budget accounting (issue #151). Provider usage is optional;
	// if zero we simply skip the guard for that turn.
	if !bs.verifiedOnly {
		if u := resp.Usage.TotalTokens; u > 0 {
			bs.totalTokens += u
		} else {
			bs.totalTokens += resp.Usage.PromptTokens + resp.Usage.CompletionTokens
		}
	}
	if !bs.verifiedOnly && l.MaxTokens > 0 {
		if !bs.warnedBudget && l.BudgetWarnRatio > 0 &&
			float64(bs.totalTokens) >= l.BudgetWarnRatio*float64(l.MaxTokens) {
			bs.warnedBudget = true
			l.fire(ctx, hooks.BudgetWarn, "", map[string]any{
				"total_tokens": bs.totalTokens, "max_tokens": l.MaxTokens,
			})
		}
		if bs.totalTokens >= l.MaxTokens {
			if serr := l.saveHistory(ctx, sess, msgs); serr != nil {
				return nil, serr
			}
			l.fire(ctx, hooks.BudgetExhausted, "", map[string]any{
				"dimension": "tokens", "total_tokens": bs.totalTokens, "max_tokens": l.MaxTokens,
			})
			l.record(ctx, ledger.TypeTokenBudgetExhausted,
				map[string]any{"total_tokens": bs.totalTokens, "max_tokens": l.MaxTokens},
				fmt.Sprintf("token budget exhausted: %d/%d", bs.totalTokens, l.MaxTokens))
			if l.AllowContinuation {
				return &Result{
					SessionID: sess.ID, Summary: lastText, Verified: false,
					Turns: turn + 1, Continuation: true, OpenCriteria: lastOpen,
				}, nil
			}
			return nil, fmt.Errorf("token budget exhausted: %d/%d tokens used", bs.totalTokens, l.MaxTokens)
		}
	}

	// Issue: Thinking Budget Enforcement (first PR). Accumulate the
	// provider's reported reasoning-token usage and stop the run
	// when the per-run cap is exceeded (ThinkingBudgetPerRequest > 0).
	// Zero values from providers that do not surface the field are
	// safe — they never trigger the guard.
	if !bs.verifiedOnly && resp.Usage.ThinkingTokens > 0 {
		l.thinkingUsed += resp.Usage.ThinkingTokens
	}
	if !bs.verifiedOnly && l.ThinkingBudgetPerRequest > 0 && l.thinkingUsed > l.ThinkingBudgetPerRequest {
		if serr := l.saveHistory(ctx, sess, msgs); serr != nil {
			return nil, serr
		}
		l.fire(ctx, hooks.BudgetExhausted, "", map[string]any{
			"dimension":           "thinking",
			"thinking_tokens":     l.thinkingUsed,
			"max_thinking_tokens": l.ThinkingBudgetPerRequest,
		})
		l.record(ctx, ledger.TypeTokenBudgetExhausted,
			map[string]any{
				"dimension":           "thinking",
				"thinking_tokens":     l.thinkingUsed,
				"max_thinking_tokens": l.ThinkingBudgetPerRequest,
			},
			fmt.Sprintf("thinking budget exhausted: %d > %d", l.thinkingUsed, l.ThinkingBudgetPerRequest))
		// Mandate M3: never skip verification when stopping early. Run
		// the gate on the current workspace first; if it passes the
		// work IS done and we hand back a Verified=true result
		// regardless of the budget. Only when verification FAILS do
		// we surface the budget outcome (Continuation or error).
		if l.Gate != nil {
			vr := l.Gate.Run(ctx, l.Workspace)
			if vr.Passed {
				l.fire(ctx, hooks.VerifyPass, "", map[string]any{
					"mode":                     string(vr.Mode),
					"report":                   vr.Report,
					"after_thinking_exhausted": true,
				})
				l.record(ctx, ledger.TypeVerifyPass,
					map[string]any{"mode": string(vr.Mode), "after_thinking_exhausted": true},
					"verification passed after thinking budget exhausted")
				result := &Result{
					SessionID: sess.ID, Summary: resp.Text,
					Verified: true, Turns: turn + 1,
					Tokens: bs.totalTokens,
				}
				l.fire(ctx, hooks.TaskComplete, "", map[string]any{
					"summary":                         result.Summary,
					"turns":                           result.Turns,
					"verified":                        true,
					"thinking_exhausted_but_verified": true,
				})
				return result, nil
			}
			l.fire(ctx, hooks.VerifyFail, "", map[string]any{
				"mode":                     string(vr.Mode),
				"report":                   vr.Report,
				"after_thinking_exhausted": true,
			})
		}
		if l.AllowContinuation {
			return &Result{
				SessionID: sess.ID, Summary: lastText, Verified: false,
				Turns: turn + 1, Continuation: true, OpenCriteria: lastOpen,
			}, nil
		}
		return nil, fmt.Errorf("thinking budget exhausted (%d > %d)", l.thinkingUsed, l.ThinkingBudgetPerRequest)
	}

	return nil, nil
}
