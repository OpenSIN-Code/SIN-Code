// SPDX-License-Identifier: MIT
// Purpose: tests for issue #152 — self-reflection pass before the
// stop-gate. When the Reflector returns issues, the loop continues
// working instead of consulting the gate.
package agentloop

import (
	"context"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// Reflector=nil preserves legacy behavior (no reflection).
func TestReflector_DisabledWhenNil(t *testing.T) {
	s := setupSession(t)
	gateCalls := 0
	loop := &Loop{
		Gate: passGate(),
		// Reflector: nil (default)
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			gateCalls++
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "ok", Raw: session.Message{Role: "assistant", Content: "ok"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "x")
	if err != nil {
		t.Fatal(err)
	}
	if gateCalls != 1 {
		t.Errorf("expected 1 stop-gate call, got %d", gateCalls)
	}
}

// Reflector returns issues -> loop injects them and forces another turn.
func TestReflector_ForcesAnotherTurn(t *testing.T) {
	s := setupSession(t)
	reflections := 0
	completionCalls := 0
	gateCalls := 0
	loop := &Loop{
		Gate: passGate(),
		Reflector: func(ctx context.Context, snap StopSnapshot) Reflection {
			reflections++
			// Return issues once, then accept. The Reflector runs at
			// most once per proposal — the second time the worker
			// proposes completion, reflectedThisProposal is true and
			// the Reflector is skipped.
			if reflections == 1 {
				return Reflection{Issues: []string{"missing test"}, Notes: "add a unit test"}
			}
			return Reflection{}
		},
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			gateCalls++
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			completionCalls++
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "x")
	if err != nil {
		t.Fatal(err)
	}
	// Reflector runs ONCE (returns issues, the loop continues).
	// On the second turn, reflectedThisProposal is still true, so
	// the Reflector is skipped — straight to stop-gate.
	if reflections != 1 {
		t.Errorf("expected 1 reflection call (one per proposal), got %d", reflections)
	}
	if completionCalls != 2 {
		t.Errorf("expected 2 completion calls (one per turn), got %d", completionCalls)
	}
	if gateCalls != 1 {
		t.Errorf("expected 1 stop-gate call (after second completion), got %d", gateCalls)
	}
}

// Reflector with no issues on first call -> straight to stop-gate.
func TestReflector_NoIssuesProceeds(t *testing.T) {
	s := setupSession(t)
	reflections := 0
	gateCalls := 0
	loop := &Loop{
		Gate: passGate(),
		Reflector: func(ctx context.Context, snap StopSnapshot) Reflection {
			reflections++
			return Reflection{} // no issues
		},
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			gateCalls++
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "ok", Raw: session.Message{Role: "assistant", Content: "ok"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "x")
	if err != nil {
		t.Fatal(err)
	}
	if reflections != 1 {
		t.Errorf("expected 1 reflection call, got %d", reflections)
	}
	if gateCalls != 1 {
		t.Errorf("expected 1 stop-gate call, got %d", gateCalls)
	}
}
