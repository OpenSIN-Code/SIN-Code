// SPDX-License-Identifier: MIT
// Purpose: tests for issue #151 — token budget with hard cap. The
// loop accumulates prompt+completion tokens and bails out (or
// checkpoints) when MaxTokens is reached.
package agentloop

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// MaxTokens=0 disables the budget guard (legacy behavior).
func TestTokenBudget_DisabledWhenZero(t *testing.T) {
	s := setupSession(t)
	calls := 0
	loop := &Loop{
		Gate:      passGate(),
		Workspace: "/tmp",
		// MaxTokens=0 -> unlimited
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			calls++
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "ok", Usage: Usage{TotalTokens: 999999}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "x")
	if err != nil {
		t.Fatalf("expected success with MaxTokens=0, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 stop-gate call, got %d", calls)
	}
}

// MaxTokens with TotalTokens exceeds the cap -> error (without AllowContinuation).
func TestTokenBudget_Exhausts(t *testing.T) {
	s := setupSession(t)
	calls := 0
	loop := &Loop{
		Gate:          passGate(),
		Workspace:     "/tmp",
		MaxTokens:     100,
		MaxStopRejects: 100, // high — must NOT be the trigger
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			calls++
			return StopDecision{Complete: false, OpenCriteria: []string{"k"}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			calls++
			return &Completion{Text: "ok", Usage: Usage{TotalTokens: 50}}, nil
		},
	}
	// After turn 1, total=50. After turn 2, total=100 (cap). The budget
	// check fires after appending msgs.
	_, err := loop.Run(context.Background(), s, "x")
	if err == nil {
		t.Fatal("expected token budget error")
	}
	if !strings.Contains(err.Error(), "token budget exhausted") {
		t.Errorf("expected 'token budget exhausted' in error, got: %v", err)
	}
}

// MaxTokens + AllowContinuation -> checkpoint, not error.
func TestTokenBudget_AllowContinuation(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:              passGate(),
		Workspace:         "/tmp",
		MaxTokens:         50,
		AllowContinuation: true,
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Usage: Usage{TotalTokens: 50}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "x")
	if err != nil {
		t.Fatalf("expected checkpoint, got error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if !res.Continuation {
		t.Errorf("expected Continuation=true, got false")
	}
}
