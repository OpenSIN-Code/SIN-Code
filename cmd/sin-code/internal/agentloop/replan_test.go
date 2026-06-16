// SPDX-License-Identifier: MIT
// Purpose: tests for adaptive re-planning (#010) and diff-based progress
// stall detection (#011). Verifies that a stall first attempts a Replanner
// recovery (resetting stall tracking), that the replan budget is enforced,
// and that an unchanging workspace diff escalates independently of the
// stop-gate's criteria text.
package agentloop

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// A stall first triggers the Replanner; once it injects a fresh strategy the
// stall counter resets and the run can complete.
func TestReplan_RecoversFromStall(t *testing.T) {
	s := setupSession(t)
	gateCalls := 0
	replanCalls := 0
	loop := &Loop{
		Gate:           passGate(),
		Workspace:      "/tmp",
		StallThreshold: 2,
		ReplanBudget:   1,
		Replanner: func(ctx context.Context, snap StallSnapshot) string {
			replanCalls++
			return "decompose the task differently: start with the parser"
		},
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			gateCalls++
			// Stall on the first two evaluations (same criteria), then accept
			// once the replan has been injected.
			if gateCalls <= 2 {
				return StopDecision{Complete: false, OpenCriteria: []string{"SAME"}}
			}
			return StopDecision{Complete: true}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "build x")
	if err != nil {
		t.Fatalf("expected recovery, got error: %v", err)
	}
	if !res.Verified {
		t.Fatal("expected verified completion after replan recovery")
	}
	if replanCalls != 1 {
		t.Fatalf("expected exactly 1 replan, got %d", replanCalls)
	}
}

// When the Replanner keeps the run alive but progress never resumes, the
// replan budget caps recovery attempts and the run finally escalates.
func TestReplan_BudgetExhausted(t *testing.T) {
	s := setupSession(t)
	replanCalls := 0
	loop := &Loop{
		Gate:           passGate(),
		Workspace:      "/tmp",
		StallThreshold: 2,
		ReplanBudget:   2,
		Replanner: func(ctx context.Context, snap StallSnapshot) string {
			replanCalls++
			return "try yet another angle"
		},
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			// Never satisfied, always the same criteria -> perpetual stall.
			return StopDecision{Complete: false, OpenCriteria: []string{"SAME"}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "build x")
	if err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("expected stall escalation after budget, got: %v", err)
	}
	if replanCalls != 2 {
		t.Fatalf("expected exactly ReplanBudget=2 replans, got %d", replanCalls)
	}
}

// A nil Replanner preserves the legacy abort-on-stall behavior.
func TestReplan_NilPreservesAbort(t *testing.T) {
	s := setupSession(t)
	loop := &Loop{
		Gate:           passGate(),
		Workspace:      "/tmp",
		StallThreshold: 2,
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			return StopDecision{Complete: false, OpenCriteria: []string{"SAME"}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "build x")
	if err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("expected legacy abort-on-stall, got: %v", err)
	}
}

// The diff-based probe escalates when the workspace stops changing, even if
// the stop-gate keeps rephrasing its criteria (so the textual fingerprint
// never trips). With no Replanner, this aborts.
func TestProgressProbe_NoChangeEscalates(t *testing.T) {
	s := setupSession(t)
	gateCalls := 0
	loop := &Loop{
		Gate:                passGate(),
		Workspace:           "/tmp",
		NoProgressThreshold: 3,
		// Constant signal => identical diff hash every reject.
		ProgressProbe: func(ctx context.Context, ws string) ProgressSignal {
			return ProgressSignal{DiffHash: "constant", LinesChanged: 0}
		},
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			gateCalls++
			// Different criteria text every turn -> textual fingerprint never
			// stalls; only the diff probe can catch this.
			return StopDecision{Complete: false, OpenCriteria: []string{string(rune('a' + gateCalls))}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "build x")
	if err == nil || !strings.Contains(err.Error(), "no workspace change") {
		t.Fatalf("expected no-progress escalation, got: %v", err)
	}
	// First reject sets the baseline hash; next 3 identical rejects trip the
	// threshold => escalate on the 4th stop-gate call.
	if gateCalls != 4 {
		t.Fatalf("expected escalation on 4th reject, got %d", gateCalls)
	}
}

// A changing diff hash keeps resetting the no-progress counter, so the probe
// never escalates while real edits are happening.
func TestProgressProbe_ChangingDiffResets(t *testing.T) {
	s := setupSession(t)
	gateCalls := 0
	probeCalls := 0
	loop := &Loop{
		Gate:                passGate(),
		Workspace:           "/tmp",
		NoProgressThreshold: 3,
		ProgressProbe: func(ctx context.Context, ws string) ProgressSignal {
			probeCalls++
			// Unique hash each call => counter always resets.
			return ProgressSignal{DiffHash: string(rune('A' + probeCalls)), LinesChanged: probeCalls}
		},
		StopGate: func(ctx context.Context, snap StopSnapshot) StopDecision {
			gateCalls++
			if gateCalls >= 5 {
				return StopDecision{Complete: true} // eventually accept
			}
			return StopDecision{Complete: false, OpenCriteria: []string{"keep going"}}
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "build x")
	if err != nil {
		t.Fatalf("changing diff should not escalate, got: %v", err)
	}
	if !res.Verified {
		t.Fatal("expected eventual verified completion")
	}
}
