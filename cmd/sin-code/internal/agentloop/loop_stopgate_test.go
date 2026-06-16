// SPDX-License-Identifier: MIT
// Purpose: tests for the loop's stop-gate integration and continuation path —
// the anti-babysitting mechanics. Verifies the gate can force continued work,
// that open criteria are injected, and that maxTurns checkpoints instead of
// erroring when continuation is allowed (AGENTS.md §8).
package agentloop

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func passGate() *verify.Gate {
	return verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil }, nil)
}

// The stop-gate rejects the first proposed completion, forcing another turn;
// the second proposal is accepted.
func TestStopGate_ForcesContinuation(t *testing.T) {
	s := setupSession(t)
	gateCalls := 0
	loop := &Loop{
		Gate:      passGate(),
		Workspace: "/tmp",
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			gateCalls++
			if gateCalls == 1 {
				return StopDecision{Complete: false, OpenCriteria: []string{"write the tests"}}
			}
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "build x")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected verified after gate finally accepts")
	}
	if gateCalls != 2 {
		t.Fatalf("stop-gate should be consulted twice, got %d", gateCalls)
	}
	if res.Turns < 2 {
		t.Fatalf("expected at least 2 turns due to forced continuation, got %d", res.Turns)
	}
}

// The open criteria from a rejection must be injected into the conversation so
// the model knows precisely what remains.
func TestStopGate_InjectsOpenCriteria(t *testing.T) {
	s := setupSession(t)
	var sawInjection bool
	turns := 0
	loop := &Loop{
		Gate:      passGate(),
		Workspace: "/tmp",
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			if turns == 1 {
				return StopDecision{Complete: false, OpenCriteria: []string{"UNIQUE_CRITERION_42"}}
			}
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			turns++
			for _, m := range msgs {
				if strings.Contains(m.Content, "UNIQUE_CRITERION_42") {
					sawInjection = true
				}
			}
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), s, "go"); err != nil {
		t.Fatal(err)
	}
	if !sawInjection {
		t.Fatal("open criteria should be injected into the next turn's messages")
	}
}

// With AllowContinuation, hitting maxTurns yields a resumable checkpoint
// Result rather than an error.
func TestContinuation_CheckpointsAtMaxTurns(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:              verify.NewGate("poc", func(ctx context.Context, ws string) (bool, string, error) { return false, "nope", nil }, nil),
		Workspace:         "/tmp",
		MaxTurns:          2,
		AllowContinuation: true,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "still working", Raw: session.Message{Role: "assistant", Content: "still working"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "long task")
	if err != nil {
		t.Fatalf("continuation should not error, got %v", err)
	}
	if res == nil || !res.Continuation {
		t.Fatalf("expected continuation result, got %+v", res)
	}
	if res.Verified {
		t.Fatal("checkpoint is not verified")
	}
}

// Without AllowContinuation, maxTurns still errors (legacy behavior preserved).
func TestNoContinuation_StillErrorsAtMaxTurns(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:      verify.NewGate("poc", func(ctx context.Context, ws string) (bool, string, error) { return false, "nope", nil }, nil),
		Workspace: "/tmp",
		MaxTurns:  2,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "x", Raw: session.Message{Role: "assistant", Content: "x"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), s, "x"); err == nil {
		t.Fatal("expected max-turns error without continuation")
	}
}

// A nil stop-gate must preserve exact legacy behavior: verify-pass = done.
func TestNilStopGate_LegacyBehavior(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:      passGate(),
		Workspace: "/tmp",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "x")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified || res.Turns != 1 {
		t.Fatalf("nil stop-gate should behave like legacy, got %+v", res)
	}
}

// Stall detection fires before MaxStopRejects when criteria never change.
func TestStopGate_Stall_EscalatesEarly(t *testing.T) {
	s := setupSession(t)
	calls := 0
	loop := &Loop{
		Gate:           passGate(),
		Workspace:      "/tmp",
		MaxStopRejects: 100, // high — must NOT be the trigger
		StallThreshold: 3,
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			calls++
			return StopDecision{Complete: false, OpenCriteria: []string{"SAME_CRIT"}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "stall")
	if err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("expected stall escalation, got: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected escalation at StallThreshold=3, got %d calls", calls)
	}
}

// MaxStopRejects caps total rejects even when the criteria keep changing
// (so the stall guard never trips).
func TestStopGate_MaxRejects_Escalates(t *testing.T) {
	s := setupSession(t)
	calls := 0
	loop := &Loop{
		Gate:           passGate(),
		Workspace:      "/tmp",
		MaxStopRejects: 3,
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			calls++
			return StopDecision{Complete: false, OpenCriteria: []string{fmt.Sprintf("crit-%d", calls)}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "reject")
	if err == nil || !strings.Contains(err.Error(), "rejected completion") {
		t.Fatalf("expected max-rejects escalation, got: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected escalation at MaxStopRejects=3, got %d calls", calls)
	}
}

// Reflector with issues forces one corrective turn, then completes.
func TestReflector_ForcesCorrectionThenCompletes(t *testing.T) {
	s := setupSession(t)
	reflectCalls := 0
	loop := &Loop{
		Gate:      passGate(),
		Workspace: "/tmp",
		Reflector: func(ctx context.Context, snap StopSnapshot) Reflection {
			reflectCalls++
			if reflectCalls == 1 {
				return Reflection{Issues: []string{"missing error handling"}}
			}
			return Reflection{} // clean on 2nd pass
		},
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "reflect")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Verified {
		t.Fatal("expected verified completion after reflection cleared")
	}
	if reflectCalls < 2 {
		t.Fatalf("expected at least 2 reflection passes, got %d", reflectCalls)
	}
}
