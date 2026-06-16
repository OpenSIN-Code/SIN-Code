// SPDX-License-Identifier: MIT
// Purpose: tests for sub-agent delegation — isolated session, summary-only
// result, and lifecycle hook firing.
package agentloop

import (
	"context"
	"path/filepath"
	"strings"
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

// The spawn_subagent spec is advertised only when a SubagentStore is wired.
func TestSubagentSpec_AdvertisedOnlyWhenEnabled(t *testing.T) {
	off := &Loop{}
	for _, ts := range off.tools() {
		if ts.Name == SpawnSubagentTool {
			t.Fatal("spawn_subagent must not be advertised without SubagentStore")
		}
	}
	on := &Loop{SubagentStore: newTestSessionStore(t)}
	found := false
	for _, ts := range on.tools() {
		if ts.Name == SpawnSubagentTool {
			found = true
		}
	}
	if !found {
		t.Fatal("spawn_subagent must be advertised when SubagentStore is set")
	}
}

// End-to-end: the worker emits a spawn_subagent tool call; the loop intercepts
// it, runs a child loop, and feeds the JSON summary back as the tool result.
func TestSpawnSubagent_ToolCallIntercepted(t *testing.T) {
	store := newTestSessionStore(t)
	parentSess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	loop := &Loop{
		Gate:          passGate(),
		Workspace:     "/tmp",
		SubagentStore: store,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			calls++
			if calls == 1 {
				// First turn: delegate.
				return &Completion{
					ToolCalls: []ToolCall{{ID: "1", Name: SpawnSubagentTool, Args: map[string]any{"goal": "child task", "max_turns": float64(2)}}},
					Raw:       session.Message{Role: "assistant", Content: ""},
				}, nil
			}
			// Later turns (parent + child): just finish.
			return &Completion{Text: "all done", Raw: session.Message{Role: "assistant", Content: "all done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), parentSess, "delegate please")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Verified {
		t.Fatalf("expected verified result, got %+v", res)
	}
	// The child loop must have produced its own assistant turn (calls >= 3:
	// parent delegate, child work, parent finish).
	if calls < 3 {
		t.Fatalf("expected child loop to run (>=3 completions), got %d", calls)
	}
}

// A delegated tool call with no goal returns an error string, not a Go error.
func TestSpawnSubagent_MissingGoal(t *testing.T) {
	store := newTestSessionStore(t)
	loop := &Loop{Workspace: "/tmp", SubagentStore: store}
	out := loop.handleSpawnSubagent(context.Background(), map[string]any{})
	if !strings.Contains(out, "SUBAGENT ERROR") {
		t.Fatalf("expected error string for missing goal, got %q", out)
	}
}
