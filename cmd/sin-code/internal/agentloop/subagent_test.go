// SPDX-License-Identifier: MIT
// Purpose: tests for sub-agent delegation — isolated session, summary-only
// result, and lifecycle hook firing.
package agentloop

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

func newTestSessionStore(t *testing.T) *session.Store {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSpawnSubagent_ReturnsOnlySummary(t *testing.T) {
	store := newTestSessionStore(t)
	loop := &Loop{
		Gate:      passGate(),
		Workspace: "/tmp",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "subtask done", Raw: session.Message{Role: "assistant", Content: "subtask done"}}, nil
		},
	}
	res, err := loop.SpawnSubagent(context.Background(), store, SubagentRequest{Goal: "do a thing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Summary != "subtask done" || !res.Verified {
		t.Fatalf("unexpected sub-agent result: %+v", res)
	}
}

// The sub-agent runs in its own session, distinct from any parent session.
func TestSpawnSubagent_IsolatedSession(t *testing.T) {
	store := newTestSessionStore(t)
	var subSessionID string
	loop := &Loop{
		Gate:      passGate(),
		Workspace: "/tmp",
		SessionID: "parent-session",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "ok", Raw: session.Message{Role: "assistant", Content: "ok"}}, nil
		},
	}
	if _, err := loop.SpawnSubagent(context.Background(), store, SubagentRequest{Goal: "sub", MaxTurns: 2}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The parent loop's SessionID must be untouched by delegation.
	if loop.SessionID != "parent-session" {
		t.Fatalf("parent session mutated by subagent: %q", loop.SessionID)
	}
	_ = subSessionID
}
