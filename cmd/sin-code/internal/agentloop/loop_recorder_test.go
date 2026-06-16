// SPDX-License-Identifier: MIT
// Purpose: tests for issue #168 — the agent loop threads the SessionID
// through context so the LLM client recorder sees the right session.
package agentloop

import (
	"context"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// TestRun_ThreadsSessionIDInContext: every completion call receives a
// context whose SessionIDFromContext == the run's session ID. This is the
// load-bearing contract for the LLM-usage recorder (#168).
func TestRun_ThreadsSessionIDInContext(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	var got SessionIDsByCall
	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			got = append(got, llm.SessionIDFromContext(ctx))
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), s, "hi"); err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("never called Completion")
	}
	for i, sid := range got {
		if sid != s.ID {
			t.Errorf("call %d session id: got %q want %q", i, sid, s.ID)
		}
	}
}

// TestWithSessionID_IsContextIdempotent: empty session ID returns the
// parent unchanged (no context value added).
func TestWithSessionID_IsContextIdempotent(t *testing.T) {
	parent := context.Background()
	if !isSameCtx(llm.WithSessionID(parent, ""), parent) {
		t.Error("empty SessionID should not wrap parent ctx")
	}
	wrapped := llm.WithSessionID(parent, "sess-1")
	if llm.SessionIDFromContext(wrapped) != "sess-1" {
		t.Error("with-session-id round-trip failed")
	}
}

type SessionIDsByCall []string

func isSameCtx(a, b context.Context) bool {
	if a == b {
		return true
	}
	type keyer struct{}
	aVal, _ := a.Value(keyer{}).(string)
	bVal, _ := b.Value(keyer{}).(string)
	_ = aVal
	_ = bVal
	// Different pointer, equal values? Compare both nil.
	return a == b
}
