// SPDX-License-Identifier: MIT
// Purpose: agent loop tests (mandates C1, C3, AGENTS.md §8).
package agentloop

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func setupSession(t *testing.T) *session.Session {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRun_DoneAndVerified(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	turns := 0
	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			turns++
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected verified=true")
	}
	if res.Summary != "done" {
		t.Fatalf("summary wrong: %q", res.Summary)
	}
	if res.Turns != 1 {
		t.Fatalf("turns want 1, got %d", res.Turns)
	}
}

func TestRun_VerifyFailsTwiceThenPasses(t *testing.T) {
	s := setupSession(t)
	calls := 0
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) {
			calls++
			if calls < 3 {
				return false, "tests-fail", nil
			}
			return true, "ok", nil
		}, nil)
	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected eventually verified")
	}
	if res.Turns != 3 {
		t.Fatalf("turns want 3, got %d", res.Turns)
	}
}

func TestRun_ExceedsMaxTurns(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return false, "nope", nil },
		nil)
	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		MaxTurns:  3,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "x", Raw: session.Message{Role: "assistant", Content: "x"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "x")
	if err == nil {
		t.Fatal("expected max-turns error")
	}
}

func TestRun_CompletionError(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("off", nil, nil)
	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return nil, errors.New("provider down")
		},
	}
	_, err := loop.Run(context.Background(), s, "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_ToolCallRoundTrip(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)
	toolCalls := 0
	turns := 0
	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			toolCalls++
			return "tool-out", nil
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			turns++
			if turns == 1 {
				return &Completion{
					Text:      "",
					ToolCalls: []ToolCall{{ID: "t1", Name: "sin_read", Args: map[string]any{}}},
					Raw:       session.Message{Role: "assistant", Content: ""},
				}, nil
			}
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "x")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected verified")
	}
	if toolCalls != 1 {
		t.Fatalf("tool calls want 1, got %d", toolCalls)
	}
	if res.Turns != 2 {
		t.Fatalf("turns want 2, got %d", res.Turns)
	}
}
