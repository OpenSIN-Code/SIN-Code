// SPDX-License-Identifier: MIT
// Purpose: integration tests for SessionContext wiring into the agent loop
// start path (issue #379).
package agentloop

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestSessionContextPrependedOnNewSession(t *testing.T) {
	var first []session.Message
	captured := false
	builder := NewSessionContextBuilder(
		nil,
		nil,
		nil,
		&mockTodoReader{items: []string{"todo-1 [P0]: write tests"}},
		&mockSessionSummaryReader{summary: "Previous session: fixed login bug"},
		&mockAutoMemoryReader{data: []byte("## Project Context\n- uses Go")},
	)
	loop := &Loop{
		Gate:           verify.NewGate("off", nil, nil),
		Workspace:      t.TempDir(),
		SessionContext: builder,
		Completion: func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			if !captured {
				first = append([]session.Message(nil), history...)
				captured = true
			}
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), testSession(t), "implement feature X"); err != nil {
		t.Fatal(err)
	}

	if len(first) == 0 {
		t.Fatal("no messages captured")
	}
	if first[0].Role == "system" && len(first) > 1 {
		first = first[1:] // skip system prompt if present
	}
	if first[0].Role != "user" {
		t.Fatalf("expected first message to be user, got %s", first[0].Role)
	}
	for _, want := range []string{"## Open Todos", "todo-1 [P0]: write tests", "## Session Summary", "Previous session: fixed login bug", "## Auto Memory", "uses Go"} {
		if !strings.Contains(first[0].Content, want) {
			t.Fatalf("session context missing %q:\n%s", want, first[0].Content)
		}
	}
}

func TestSessionContextNotPrependedOnResume(t *testing.T) {
	var captured []session.Message
	store, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	// Seed history so the session is not considered new.
	if err := sess.SaveHistory([]session.Message{{Role: "user", Content: "previous turn"}}); err != nil {
		t.Fatal(err)
	}

	builder := NewSessionContextBuilder(
		nil, nil, nil,
		&mockTodoReader{items: []string{"todo-1"}},
		&mockSessionSummaryReader{summary: "summary"},
		&mockAutoMemoryReader{data: []byte("memory")},
	)
	loop := &Loop{
		Gate:           verify.NewGate("off", nil, nil),
		Workspace:      t.TempDir(),
		SessionContext: builder,
		Completion: func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			captured = append([]session.Message(nil), history...)
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), sess, "next turn"); err != nil {
		t.Fatal(err)
	}
	for _, m := range captured {
		if m.Role == "user" && strings.Contains(m.Content, "## Open Todos") {
			t.Fatalf("session context should not be injected on resume, got:\n%s", m.Content)
		}
	}
}

func TestSessionContextNilDoesNotAffectMessages(t *testing.T) {
	var captured []session.Message
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: t.TempDir(),
		Completion: func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			captured = append([]session.Message(nil), history...)
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), testSession(t), "just answer"); err != nil {
		t.Fatal(err)
	}
	users := 0
	for _, m := range captured {
		if m.Role == "user" {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("expected exactly one user message, got %d", users)
	}
}

func TestSessionContextEmptyPreambleNotInjected(t *testing.T) {
	var captured []session.Message
	builder := NewSessionContextBuilder(nil, nil, nil, nil, nil, nil)
	loop := &Loop{
		Gate:           verify.NewGate("off", nil, nil),
		Workspace:      t.TempDir(),
		SessionContext: builder,
		Completion: func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			captured = append([]session.Message(nil), history...)
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), testSession(t), "just answer"); err != nil {
		t.Fatal(err)
	}
	users := 0
	for _, m := range captured {
		if m.Role == "user" {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("expected exactly one user message when preamble is empty, got %d", users)
	}
}
