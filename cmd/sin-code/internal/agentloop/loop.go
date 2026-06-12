// SPDX-License-Identifier: MIT
// Purpose: SIN-Code core agent loop: PLAN -> ACT -> VERIFY -> DONE
// (mandates C1, C3, AGENTS.md §8).
package agentloop

import (
	"context"
	"fmt"

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
}

type Result struct {
	SessionID string `json:"session_id"`
	Summary   string `json:"summary"`
	Verified  bool   `json:"verified"`
	Turns     int    `json:"turns"`
}

func (l *Loop) tools() []ToolSpec { return l.LocalSpec }

// Run executes one task to completion (or failure) inside a session.
func (l *Loop) Run(ctx context.Context, sess *session.Session, prompt string) (*Result, error) {
	if l.Completion == nil {
		return nil, fmt.Errorf("agentloop: Completion func not wired")
	}
	msgs := sess.History()
	msgs = append(msgs, session.Message{Role: "user", Content: prompt})

	maxTurns := l.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 80
	}
	tools := l.tools()

	for turn := 0; turn < maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := l.Completion(ctx, msgs, tools)
		if err != nil {
			return nil, fmt.Errorf("turn %d: %w", turn, err)
		}
		msgs = append(msgs, resp.Raw)

		// Model claims completion -> enforce the verification gate (M3).
		if len(resp.ToolCalls) == 0 {
			res := l.Gate.Run(ctx, l.Workspace)
			if !res.Passed {
				msgs = append(msgs, session.Message{
					Role:    "user",
					Content: "VERIFICATION FAILED (" + string(res.Mode) + ") — fix before claiming completion:\n" + res.Report,
				})
				if err := sess.SaveHistory(msgs); err != nil {
					return nil, err
				}
				continue
			}
			if err := sess.SaveHistory(msgs); err != nil {
				return nil, err
			}
			return &Result{
				SessionID: sess.ID, Summary: resp.Text,
				Verified: res.Passed, Turns: turn + 1,
			}, nil
		}

		for _, tc := range resp.ToolCalls {
			var out string
			var err error
			if l.LocalTool == nil {
				out = "TOOL ERROR: no LocalTool registered"
			} else {
				out, err = l.LocalTool(ctx, tc.Name, tc.Args)
				if err != nil {
					out = "TOOL ERROR: " + err.Error()
				}
			}
			msgs = append(msgs, session.Message{
				Role: "tool", ToolCallID: tc.ID, Content: out,
			})
		}
		if err := sess.SaveHistory(msgs); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("max turns (%d) exceeded without verified completion", maxTurns)
}
