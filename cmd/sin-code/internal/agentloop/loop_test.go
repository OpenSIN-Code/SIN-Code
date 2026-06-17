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

func TestRun_SystemPromptPrepended(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)
	var got []session.Message
	loop := &Loop{
		Gate:         gate,
		Workspace:    "/tmp",
		SystemPrompt: "M6 tool preference block",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			got = msgs
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), s, "hi"); err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("Completion received no messages")
	}
	if got[0].Role != "system" {
		t.Fatalf("first message role want system, got %q", got[0].Role)
	}
	if got[0].Content != "M6 tool preference block" {
		t.Fatalf("first message content wrong: %q", got[0].Content)
	}
}

func TestRun_SystemPromptEmptyNotInjected(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)
	var got []session.Message
	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			got = msgs
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), s, "hi"); err != nil {
		t.Fatal(err)
	}
	for _, m := range got {
		if m.Role == "system" {
			t.Fatalf("unexpected system message when SystemPrompt is empty: %q", m.Content)
		}
	}
}

// Coverage: completion without required tool is rejected, then accepted after
// the tool is called (issue #248).
func TestRun_CoverageRequiredTool_ForcesInvocation(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)
	toolCalls := 0
	turns := 0
	loop := &Loop{
		Gate:                  gate,
		Workspace:             "/tmp",
		CoverageRequiredTools: []string{"sin_poc"},
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			toolCalls++
			return "poc-result", nil
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			turns++
			// On the first turn the model tries to complete without the tool.
			if turns == 1 {
				return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
			}
			// On the second turn it calls the required tool.
			if turns == 2 {
				return &Completion{
					Text:      "",
					ToolCalls: []ToolCall{{ID: "t1", Name: "sin_poc", Args: map[string]any{}}},
					Raw:       session.Message{Role: "assistant", Content: ""},
				}, nil
			}
			// Third turn completes after the tool was used.
			return &Completion{Text: "done after poc", Raw: session.Message{Role: "assistant", Content: "done after poc"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "verify with poc")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected verified after required tool invoked")
	}
	if toolCalls != 1 {
		t.Fatalf("expected sin_poc called once, got %d", toolCalls)
	}
	if turns < 2 {
		t.Fatalf("expected at least 2 turns due to coverage rejection, got %d", turns)
	}
	if res.Summary != "done after poc" {
		t.Fatalf("unexpected summary: %q", res.Summary)
	}
}

// Coverage: a forbidden tool blocks completion even when verification passes.
func TestRun_CoverageForbiddenTool_BlocksCompletion(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)
	loop := &Loop{
		Gate:                   gate,
		Workspace:              "/tmp",
		CoverageForbiddenTools: []string{"sin_bash"},
		MaxTurns:               3,
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "out", nil
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{
				Text: "done",
				ToolCalls: []ToolCall{
					{ID: "t1", Name: "sin_bash", Args: map[string]any{}},
				},
				Raw: session.Message{Role: "assistant", Content: "done", ToolCalls: []byte(`[]`)},
			}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "run bash")
	if err == nil {
		t.Fatal("expected error when forbidden tool is used and coverage never passes")
	}
}

// Coverage: required tool is used on the first turn and completion succeeds.
func TestRun_CoverageRequiredTool_ImmediatePass(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)
	turns := 0
	loop := &Loop{
		Gate:                  gate,
		Workspace:             "/tmp",
		CoverageRequiredTools: []string{"sin_poc"},
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "poc-result", nil
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			turns++
			if turns == 1 {
				return &Completion{
					Text:      "",
					ToolCalls: []ToolCall{{ID: "t1", Name: "sin_poc", Args: map[string]any{}}},
					Raw:       session.Message{Role: "assistant", Content: ""},
				}, nil
			}
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "verify with poc")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected verified")
	}
	if len(loop.Coverage.Used()) != 1 || loop.Coverage.Used()[0] != "sin_poc" {
		t.Fatalf("expected sin_poc recorded, got %v", loop.Coverage.Used())
	}
}
