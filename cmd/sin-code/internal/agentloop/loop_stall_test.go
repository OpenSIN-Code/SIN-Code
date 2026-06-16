// SPDX-License-Identifier: MIT
// Purpose: tests for issue #150 — Stagnations-Erkennung + adaptives
// Stop-Budget. The stop-gate returning identical open criteria N
// times in a row is treated as a stall and escalates early.
package agentloop

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

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

// Changing criteria between rejects resets the stall counter.
func TestStopGate_Stall_ChangingCriteriaResets(t *testing.T) {
	s := setupSession(t)
	calls := 0
	loop := &Loop{
		Gate:           passGate(),
		Workspace:      "/tmp",
		MaxStopRejects: 100,
		StallThreshold: 3,
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			calls++
			// alternate between two different criteria sets; never
			// stalls. Eventually MaxStopRejects hits.
			if calls%2 == 1 {
				return StopDecision{Complete: false, OpenCriteria: []string{"CRIT_A"}}
			}
			return StopDecision{Complete: false, OpenCriteria: []string{"CRIT_B"}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "x")
	// Expected outcome: MaxStopRejects (default 3) eventually hits
	// because the criteria keep changing (no stall) and the gate
	// keeps rejecting. The exact error wording depends on the
	// stopRejects vs MaxStopRejects interaction; we just need to
	// verify a non-nil error and that the gate was consulted
	// several times.
	if err == nil {
		t.Fatal("expected error after repeated rejections")
	}
	if calls < 3 {
		t.Fatalf("expected at least 3 gate calls, got %d", calls)
	}
	// Must NOT be a stall error (the criteria changed).
	if strings.Contains(err.Error(), "stalled") {
		t.Fatalf("did not expect stall escalation when criteria change, got: %v", err)
	}
}

// StallThreshold=0 disables the detection (backwards compatible).
func TestStopGate_Stall_Disabled(t *testing.T) {
	s := setupSession(t)
	calls := 0
	loop := &Loop{
		Gate:           passGate(),
		Workspace:      "/tmp",
		MaxStopRejects: 3, // stops via MaxStopRejects
		StallThreshold: 0, // disabled
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			calls++
			return StopDecision{Complete: false, OpenCriteria: []string{"SAME_CRIT"}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "x")
	if err == nil {
		t.Fatal("expected error from MaxStopRejects path")
	}
	if strings.Contains(err.Error(), "stalled") {
		t.Fatalf("did not expect stall error when StallThreshold=0, got: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected exactly 3 gate calls (MaxStopRejects=3), got %d", calls)
	}
}
