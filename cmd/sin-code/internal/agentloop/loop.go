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

type Loop struct {
	Gate       *verify.Gate
	LocalTool  LocalToolFunc
	LocalSpec  []ToolSpec
	Workspace  string
	MaxTurns   int
	SessionID  string
	Completion func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error)

	Hooks  *hooks.Engine
	Perm   *permission.Engine
	Ask    AskFunc
	Lessons *lessons.Store

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
}

func (l *Loop) tools() []ToolSpec { return l.LocalSpec }

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
		resp, err := l.Completion(ctx, msgs, tools)
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
			return result, nil
		}

		for _, tc := range resp.ToolCalls {
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
	l.fire(ctx, hooks.TaskAbort, "", map[string]any{"reason": "max turns exceeded"})
	return nil, fmt.Errorf("max turns (%d) exceeded without verified completion", maxTurns)
}
