// SPDX-License-Identifier: MIT
// Purpose: the SinCode Loop System injects a Definition-of-Done preamble before
// the goal prompt so the worker addresses tests/debug/docs/completeness on the
// first pass. These tests verify the preamble is injected before the prompt
// when set, and that an empty preamble leaves the message stream untouched.
package agentloop

import (
	"context"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestPreambleInjectedBeforePrompt(t *testing.T) {
	var first []session.Message
	captured := false
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: t.TempDir(),
		Preamble:  "DEFINITION OF DONE — write tests and update docs",
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

	// Find the preamble and the prompt; preamble must come first.
	preambleIdx, promptIdx := -1, -1
	for i, m := range first {
		if m.Role == "user" && m.Content == "DEFINITION OF DONE — write tests and update docs" {
			preambleIdx = i
		}
		if m.Role == "user" && m.Content == "implement feature X" {
			promptIdx = i
		}
	}
	if preambleIdx == -1 {
		t.Fatal("preamble was not injected into the message history")
	}
	if promptIdx == -1 {
		t.Fatal("prompt missing from message history")
	}
	if preambleIdx > promptIdx {
		t.Fatalf("preamble (%d) must precede prompt (%d)", preambleIdx, promptIdx)
	}
}

func TestNoPreambleLeavesPromptFirst(t *testing.T) {
	var first []session.Message
	captured := false
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: t.TempDir(),
		Completion: func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			if !captured {
				first = append([]session.Message(nil), history...)
				captured = true
			}
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), testSession(t), "just answer"); err != nil {
		t.Fatal(err)
	}
	// With no preamble, the only user message before the model's first reply
	// is the prompt itself.
	users := 0
	for _, m := range first {
		if m.Role == "user" {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("expected exactly one user message without a preamble, got %d", users)
	}
}
