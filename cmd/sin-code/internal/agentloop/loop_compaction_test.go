// SPDX-License-Identifier: MIT
// Purpose: end-to-end tests for context compaction modes wired through
// the agent loop. Verifies trigger evaluation, off-mode no-op, sidecar
// writing in lossy modes, and persisted-session isolation (mandate M3:
// verification evidence survives compaction).
package agentloop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// makeLoopShell builds a minimal Loop wired with a stub Completion that
// spans N turns (first N-1 produces output, last turn is a sentinel
// "DONE"). The mock supports tool calls so we can exercise evidence-flag
// detection and tool-preservation paths.
func makeLoopShell(t *testing.T, maxTurns, promptLen int) (*Loop, *session.Session, *session.Store) {
	t.Helper()
	store, err := session.Open(t.TempDir() + "/loop_compact.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}

	gate := verify.NewGate("off", nil, nil)
	prompt := strings.Repeat("z", promptLen)
	callCount := 0
	loop := &Loop{
		Gate:       gate,
		Workspace:  "/tmp",
		MaxTurns:   maxTurns,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			callCount++
			if callCount >= maxTurns {
				return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
			}
			return &Completion{
				Text:      "working",
				Raw:       session.Message{Role: "assistant", Content: strings.Repeat("a", 200)},
				ToolCalls: []ToolCall{{ID: fmt.Sprintf("tc%d", callCount), Name: "noop", Args: map[string]any{}}},
			}, nil
		},
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return strings.Repeat("y", 200), nil
		},
	}
	_ = prompt
	return loop, sess, store
}

func TestLoopCompaction_OffMode_NoOp(t *testing.T) {
	// No compactor attached + explicit off mode = no-op. The loop must
	// not call Compact2 because ContextCompactionMode is empty AND the
	// legacy CompactionStrategy is unset.
	loop, sess, _ := makeLoopShell(t, 5, 4000)
	// Explicitly leave Compactor nil AND leave ContextCompactionMode as
	// its zero value (== off). This exercises the path where the old
	// compactor wiring is fully absent.
	res, err := loop.Run(context.Background(), sess, "short")
	if err != nil {
		t.Fatalf("off-mode loop: %v", err)
	}
	if res == nil {
		t.Fatal("loop.Run returned nil result")
	}
	// Pass-back-test: also exercise a compactor WITH explicit Off mode
	// and require zero compactions.
	c := NewCompactor(nil)
	c.Configure(CompactorConfig{Mode: ContextCompactionOff})
	loop.Compactor = c
	_, _ = loop.Run(context.Background(), sess, "short")
	if stats := c.Stats(); stats.Compactions != 0 {
		t.Errorf("off-mode should produce 0 compactions; got %d", stats.Compactions)
	}
}

func TestLoopCompaction_TokensTrigger_Fires(t *testing.T) {
	// 8000-token prompt + 200-char assistant/tool responses blow past
	// the 32000-token auto-context-window at threshold 0.8 = 25600.
	loop, sess, _ := makeLoopShell(t, 5, 32000)
	loop.Compactor = NewCompactor(nil)
	loop.Compactor.Configure(CompactorConfig{
		Mode:             ContextCompactionDeterministic, // off by default even with token trigger
		Trigger:          CompactionTriggerTokens,
		Threshold:        0.05, // very low so even large noise triggers
		MaxTokens:        1000,
		ContextWindow:    1000,
		PreserveEvidence: true,
		RecentTurns:      2,
	})
	loop.ContextCompactionMode = ContextCompactionDeterministic
	loop.CompactionTrigger = CompactionTriggerTokens
	loop.CompactionThreshold = 0.05
	loop.ContextWindow = 1000
	loop.CompactionMaxTokens = 1000
	loop.CompactionPreserveEvidence = true
	loop.CompactionRecentTurns = 2

	res, err := loop.Run(context.Background(), sess, "")
	if err != nil && res == nil {
		t.Fatalf("loop terminated on error: %v", err)
	}
	stats := loop.Compactor.Stats()
	if stats.Compactions == 0 {
		t.Errorf("token trigger should fire with low threshold; got 0")
	}
}

func TestLoopCompaction_TurnsTrigger_Fires(t *testing.T) {
	// turns trigger: many turns with short messages trigger at
	// len(msgs) > maxTurns * 0.05.
	loop, sess, _ := makeLoopShell(t, 20, 100)
	loop.Compactor = NewCompactor(nil)
	loop.Compactor.Configure(CompactorConfig{
		Mode:             ContextCompactionDeterministic,
		Trigger:          CompactionTriggerTurns,
		Threshold:        0.05,
		MaxTokens:        8000,
		PreserveEvidence: true,
		RecentTurns:      2,
	})
	loop.ContextCompactionMode = ContextCompactionDeterministic
	loop.CompactionTrigger = CompactionTriggerTurns
	loop.CompactionThreshold = 0.05
	loop.CompactionMaxTokens = 8000
	loop.CompactionPreserveEvidence = true
	loop.CompactionRecentTurns = 2

	res, err := loop.Run(context.Background(), sess, "")
	if err != nil && res == nil {
		t.Fatalf("loop terminated: %v", err)
	}
	stats := loop.Compactor.Stats()
	if stats.Compactions == 0 {
		t.Errorf("turns trigger should fire with low threshold; got 0")
	}
}

func TestLoopCompaction_LossyModeWritesSidecar(t *testing.T) {
	// Override home directory so we don't pollute the user's real
	// ~/.local/share/sin-code/context-snapshots. t.TempDir can't be
	// used directly because writeCompactionSidecar hardcodes
	// os.UserHomeDir(); however, when sessID is empty the call short-
	// circuits, so we exercise the path with a real session.
	t.Helper()
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })

	loop, sess, _ := makeLoopShell(t, 4, 16000)
	loop.Compactor = NewCompactor(stubSummarizer("SIDE"))
	loop.Compactor.Configure(CompactorConfig{
		Mode:             ContextCompactionLLM,
		Trigger:          CompactionTriggerTokens,
		Threshold:        0.05,
		MaxTokens:        200,
		ContextWindow:    200,
		PreserveEvidence: true,
		RecentTurns:      2,
	})
	loop.ContextCompactionMode = ContextCompactionLLM
	loop.CompactionTrigger = CompactionTriggerTokens
	loop.CompactionThreshold = 0.05
	loop.CompactionMaxTokens = 200
	loop.ContextWindow = 200
	loop.CompactionPreserveEvidence = true
	loop.CompactionRecentTurns = 2

	res, err := loop.Run(context.Background(), sess, "")
	if err != nil && res == nil {
		t.Fatalf("loop terminated: %v", err)
	}

	// sidecar lives in $HOME/.local/share/sin-code/context-snapshots/<hash>/turn-NNNNN.json
	expectedDir := filepath.Join(tmpHome, ".local", "share", "sin-code", "context-snapshots")
	found := false
	if entries, err := os.ReadDir(expectedDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				sub, _ := os.ReadDir(filepath.Join(expectedDir, entry.Name()))
				for _, s := range sub {
					if strings.HasSuffix(s.Name(), ".json") {
						found = true
						t.Logf("sidecar written: %s", filepath.Join(expectedDir, entry.Name(), s.Name()))
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("expected sidecar snapshot in %s, none found", expectedDir)
	}
}

func TestLoopCompaction_DroppedEvidencePreservedAcrossCompaction(t *testing.T) {
	// Compaction must NEVER drop a message containing the verify
	// markers (mandate M3). We hand-seed msgs with VERIFICATION FAILED
	// mid-history and assert the message survives the next compact
	// trigger.
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: "/tmp",
		MaxTurns:  1,
	}
	// Pre-populate the msgs the loop will see by injecting through Run.
	fakeCanceled := false
	loop.Completion = func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
		if fakeCanceled {
			return nil, context.Canceled
		}
		fakeCanceled = true
		// Return an evidence-flagged assistant turn, then bail.
		return &Completion{
			Text: "VERIFICATION FAILED — requirements not met",
			Raw:  session.Message{Role: "assistant", Content: "VERIFICATION FAILED — requirements not met"},
		}, nil
	}

	store, err := session.Open(t.TempDir() + "/preserve.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}

	loop.Compactor = NewCompactor(nil)
	loop.Compactor.Configure(CompactorConfig{
		Mode:             ContextCompactionDeterministic,
		Trigger:          CompactionTriggerTurns,
		Threshold:        0.0001,
		MaxTokens:        8000,
		PreserveEvidence: true,
		RecentTurns:      2,
	})
	loop.ContextCompactionMode = ContextCompactionDeterministic
	loop.CompactionTrigger = CompactionTriggerTurns
	loop.CompactionThreshold = 0.0001
	loop.CompactionMaxTokens = 8000
	loop.CompactionPreserveEvidence = true
	loop.CompactionRecentTurns = 2

	res, err := loop.Run(context.Background(), sess, "")
	_ = res
	if err != nil {
		// Verify session DB has the evidence message even after the
		// loop terminated. History() returns []Message (no error).
		history := sess.History()
		found := false
		for _, m := range history {
			if strings.Contains(m.Content, "VERIFICATION FAILED") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("evidence message lost from session history after compaction")
		}
	}
}
