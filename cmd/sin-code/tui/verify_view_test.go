// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"testing"
	"time"
)

func testStyles() Styles { return NewStyles(Themes[0]) }

func TestVerifyStateString(t *testing.T) {
	cases := map[VerifyState]string{
		VerifyIdle:    "idle",
		VerifyPending: "pending",
		VerifyRunning: "running",
		VerifyPassed:  "passed",
		VerifyFailed:  "failed",
		VerifyBlocked: "blocked",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("VerifyState(%d).String() = %q, want %q", state, got, want)
		}
	}
}

func TestVerifyStateIcon(t *testing.T) {
	states := []VerifyState{
		VerifyIdle,
		VerifyPending,
		VerifyRunning,
		VerifyPassed,
		VerifyFailed,
		VerifyBlocked,
	}
	for _, state := range states {
		icon := state.Icon()
		if icon == "" {
			t.Errorf("VerifyState(%d).Icon() returned empty", state)
		}
	}
}

func TestRenderVerifyPanelIdle(t *testing.T) {
	panel := VerifyPanel{State: VerifyIdle}
	out := RenderVerifyPanel(panel, testStyles(), 80)
	if out != "" {
		t.Errorf("expected empty string for idle, got %q", out)
	}
}

func TestRenderVerifyPanelRunning(t *testing.T) {
	panel := VerifyPanel{
		State:    VerifyRunning,
		Mode:     "poc",
		Target:   "./cmd/sin-code/internal/agentloop",
		Evidence: "compiling...",
	}
	out := RenderVerifyPanel(panel, testStyles(), 80)
	if !strings.Contains(out, "Verification Gate") {
		t.Errorf("expected header, got:\n%s", out)
	}
	if !strings.Contains(out, "RUNNING") {
		t.Errorf("expected RUNNING label, got:\n%s", out)
	}
	if !strings.Contains(out, "poc") {
		t.Errorf("expected mode 'poc', got:\n%s", out)
	}
	if !strings.Contains(out, "agentloop") {
		t.Errorf("expected target, got:\n%s", out)
	}
	if !strings.Contains(out, "⟳") {
		t.Errorf("expected running icon, got:\n%s", out)
	}
}

func TestRenderVerifyPanelPassed(t *testing.T) {
	panel := VerifyPanel{
		State:    VerifyPassed,
		Mode:     "poc",
		Target:   "./cmd/sin-code/internal/agentloop",
		Evidence: "go test ./... -race → PASS (42 tests)",
	}
	out := RenderVerifyPanel(panel, testStyles(), 80)
	if !strings.Contains(out, "✅") {
		t.Errorf("expected pass icon, got:\n%s", out)
	}
	if !strings.Contains(out, "PASSED") {
		t.Errorf("expected PASSED label, got:\n%s", out)
	}
	if !strings.Contains(out, "PASS (42 tests)") {
		t.Errorf("expected evidence, got:\n%s", out)
	}
}

func TestRenderVerifyPanelFailed(t *testing.T) {
	panel := VerifyPanel{
		State:    VerifyFailed,
		Mode:     "oracle",
		Target:   "auth flow",
		Evidence: "expected 200, got 500",
	}
	out := RenderVerifyPanel(panel, testStyles(), 80)
	if !strings.Contains(out, "❌") {
		t.Errorf("expected fail icon, got:\n%s", out)
	}
	if !strings.Contains(out, "FAILED") {
		t.Errorf("expected FAILED label, got:\n%s", out)
	}
	if !strings.Contains(out, "expected 200, got 500") {
		t.Errorf("expected evidence, got:\n%s", out)
	}
}

func TestRenderVerifyPanelBlocked(t *testing.T) {
	panel := VerifyPanel{
		State:  VerifyBlocked,
		Mode:   "poc",
		Target: "deterministic check",
	}
	out := RenderVerifyPanel(panel, testStyles(), 80)
	if !strings.Contains(out, "⛔") {
		t.Errorf("expected blocked icon, got:\n%s", out)
	}
	if !strings.Contains(out, "BLOCKED") {
		t.Errorf("expected BLOCKED label, got:\n%s", out)
	}
}

func TestRenderVerifyPanelEvidenceTruncation(t *testing.T) {
	panel := VerifyPanel{
		State:    VerifyPassed,
		Mode:     "poc",
		Evidence: "line1\nline2\nline3\nline4\nline5",
	}
	out := RenderVerifyPanel(panel, testStyles(), 80)
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation marker, got:\n%s", out)
	}
	if strings.Contains(out, "line4") {
		t.Errorf("expected line4 to be truncated, got:\n%s", out)
	}
}

func TestRenderVerifyPanelDuration(t *testing.T) {
	start := time.Now().Add(-2300 * time.Millisecond)
	end := time.Now()
	panel := VerifyPanel{
		State:     VerifyPassed,
		Mode:      "poc",
		StartTime: start,
		EndTime:   end,
	}
	out := RenderVerifyPanel(panel, testStyles(), 80)
	if !strings.Contains(out, "Duration:") {
		t.Errorf("expected duration line, got:\n%s", out)
	}
}

func TestRenderVerifyPanelAttempts(t *testing.T) {
	panel := VerifyPanel{
		State:    VerifyPassed,
		Mode:     "poc",
		Attempts: 3,
	}
	out := RenderVerifyPanel(panel, testStyles(), 80)
	if !strings.Contains(out, "Attempt: #3") {
		t.Errorf("expected attempt count, got:\n%s", out)
	}
}

func TestRenderVerifyHistoryEmpty(t *testing.T) {
	panel := VerifyPanel{}
	out := RenderVerifyHistory(panel, testStyles(), 80)
	if !strings.Contains(out, "No verification history") {
		t.Errorf("expected empty history message, got:\n%s", out)
	}
}

func TestRenderVerifyHistory(t *testing.T) {
	panel := VerifyPanel{
		History: []VerifyAttempt{
			{
				Timestamp: time.Now(),
				State:     VerifyPassed,
				Target:    "./pkg/a",
				Duration:  1200 * time.Millisecond,
			},
			{
				Timestamp: time.Now(),
				State:     VerifyFailed,
				Target:    "./pkg/b",
				Duration:  800 * time.Millisecond,
			},
			{
				Timestamp: time.Now(),
				State:     VerifyBlocked,
				Target:    "./pkg/c",
				Duration:  50 * time.Millisecond,
			},
		},
	}
	out := RenderVerifyHistory(panel, testStyles(), 80)
	if !strings.Contains(out, "Verification History") {
		t.Errorf("expected header, got:\n%s", out)
	}
	if !strings.Contains(out, "./pkg/a") {
		t.Errorf("expected first target, got:\n%s", out)
	}
	if !strings.Contains(out, "./pkg/b") {
		t.Errorf("expected second target, got:\n%s", out)
	}
	if !strings.Contains(out, "./pkg/c") {
		t.Errorf("expected third target, got:\n%s", out)
	}
	if !strings.Contains(out, "✅") {
		t.Errorf("expected pass icon in history, got:\n%s", out)
	}
	if !strings.Contains(out, "❌") {
		t.Errorf("expected fail icon in history, got:\n%s", out)
	}
	if !strings.Contains(out, "⛔") {
		t.Errorf("expected blocked icon in history, got:\n%s", out)
	}
}

func TestRenderVerifyStatusBarIdle(t *testing.T) {
	panel := VerifyPanel{State: VerifyIdle}
	out := RenderVerifyStatusBar(panel, testStyles())
	if out != "" {
		t.Errorf("expected empty for idle, got %q", out)
	}
}

func TestRenderVerifyStatusBar(t *testing.T) {
	cases := []VerifyState{
		VerifyPending,
		VerifyRunning,
		VerifyPassed,
		VerifyFailed,
		VerifyBlocked,
	}
	styles := testStyles()
	for _, state := range cases {
		panel := VerifyPanel{State: state}
		out := RenderVerifyStatusBar(panel, styles)
		if out == "" {
			t.Errorf("expected non-empty status bar for %s", state)
		}
		if !strings.Contains(out, "verify:") {
			t.Errorf("expected 'verify:' prefix for %s, got %q", state, out)
		}
		if !strings.Contains(out, state.String()) {
			t.Errorf("expected state name %s in bar, got %q", state, out)
		}
		if !strings.Contains(out, state.Icon()) {
			t.Errorf("expected icon %s in bar, got %q", state.Icon(), out)
		}
	}
}

func TestHandleVerifyUpdateRunning(t *testing.T) {
	panel := &VerifyPanel{State: VerifyIdle}
	HandleVerifyUpdate(panel, VerifyUpdateMsg{
		State:  VerifyRunning,
		Mode:   "poc",
		Target: "./pkg/x",
	})
	if panel.State != VerifyRunning {
		t.Errorf("expected state running, got %s", panel.State)
	}
	if panel.Mode != "poc" {
		t.Errorf("expected mode poc, got %q", panel.Mode)
	}
	if panel.StartTime.IsZero() {
		t.Error("expected start time to be set on running")
	}
}

func TestHandleVerifyUpdatePendingDoesNotSetStart(t *testing.T) {
	panel := &VerifyPanel{State: VerifyIdle}
	HandleVerifyUpdate(panel, VerifyUpdateMsg{
		State:  VerifyPending,
		Mode:   "poc",
		Target: "./pkg/x",
	})
	if !panel.StartTime.IsZero() {
		t.Error("expected start time to remain zero on pending")
	}
}

func TestHandleVerifyUpdatePassed(t *testing.T) {
	panel := &VerifyPanel{State: VerifyIdle}
	HandleVerifyUpdate(panel, VerifyUpdateMsg{
		State:    VerifyRunning,
		Mode:     "poc",
		Target:   "./pkg/x",
		Evidence: "all tests pass",
	})
	startBefore := panel.StartTime
	HandleVerifyUpdate(panel, VerifyUpdateMsg{
		State:    VerifyPassed,
		Mode:     "poc",
		Target:   "./pkg/x",
		Evidence: "all tests pass",
	})
	if panel.State != VerifyPassed {
		t.Errorf("expected state passed, got %s", panel.State)
	}
	if panel.EndTime.IsZero() {
		t.Error("expected end time to be set on passed")
	}
	if len(panel.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(panel.History))
	}
	entry := panel.History[0]
	if entry.State != VerifyPassed {
		t.Errorf("expected history state passed, got %s", entry.State)
	}
	if entry.Target != "./pkg/x" {
		t.Errorf("expected history target, got %q", entry.Target)
	}
	if entry.Evidence != "all tests pass" {
		t.Errorf("expected history evidence, got %q", entry.Evidence)
	}
	if entry.Duration <= 0 {
		t.Errorf("expected positive duration, got %v", entry.Duration)
	}
	_ = startBefore
	if !panel.StartTime.IsZero() {
		t.Error("expected start time to be reset after terminal state")
	}
}

func TestHandleVerifyUpdateFailedAddsHistory(t *testing.T) {
	panel := &VerifyPanel{State: VerifyIdle}
	HandleVerifyUpdate(panel, VerifyUpdateMsg{State: VerifyRunning, Mode: "oracle", Target: "auth"})
	HandleVerifyUpdate(panel, VerifyUpdateMsg{State: VerifyFailed, Mode: "oracle", Target: "auth", Evidence: "500"})
	if len(panel.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(panel.History))
	}
	if panel.History[0].State != VerifyFailed {
		t.Errorf("expected failed state in history, got %s", panel.History[0].State)
	}
}

func TestHandleVerifyUpdateBlockedAddsHistory(t *testing.T) {
	panel := &VerifyPanel{State: VerifyIdle}
	HandleVerifyUpdate(panel, VerifyUpdateMsg{State: VerifyRunning, Mode: "poc", Target: "det"})
	HandleVerifyUpdate(panel, VerifyUpdateMsg{State: VerifyBlocked, Mode: "poc", Target: "det", Evidence: "frozen glob"})
	if len(panel.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(panel.History))
	}
	if panel.History[0].State != VerifyBlocked {
		t.Errorf("expected blocked state in history, got %s", panel.History[0].State)
	}
}

func TestResetVerifyPanel(t *testing.T) {
	panel := &VerifyPanel{
		State:     VerifyPassed,
		Mode:      "poc",
		Target:    "./pkg/x",
		Evidence:  "evidence text",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Attempts:  5,
		History:   []VerifyAttempt{{Target: "old"}},
	}
	ResetVerifyPanel(panel)
	if panel.State != VerifyIdle {
		t.Errorf("expected idle, got %s", panel.State)
	}
	if panel.Mode != "" {
		t.Errorf("expected empty mode, got %q", panel.Mode)
	}
	if panel.Target != "" {
		t.Errorf("expected empty target, got %q", panel.Target)
	}
	if panel.Evidence != "" {
		t.Errorf("expected empty evidence, got %q", panel.Evidence)
	}
	if !panel.StartTime.IsZero() {
		t.Error("expected zero start time")
	}
	if !panel.EndTime.IsZero() {
		t.Error("expected zero end time")
	}
	if panel.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", panel.Attempts)
	}
}

func TestResetVerifyPanelPreservesHistory(t *testing.T) {
	panel := &VerifyPanel{
		State:   VerifyPassed,
		History: []VerifyAttempt{{Target: "persisted"}},
	}
	ResetVerifyPanel(panel)
	if len(panel.History) != 1 {
		t.Fatalf("expected history to persist, got %d", len(panel.History))
	}
	if panel.History[0].Target != "persisted" {
		t.Errorf("expected persisted target, got %q", panel.History[0].Target)
	}
}

func TestVerifyPanelHistoryCap(t *testing.T) {
	panel := &VerifyPanel{State: VerifyIdle}
	for i := 0; i < 25; i++ {
		HandleVerifyUpdate(panel, VerifyUpdateMsg{
			State:    VerifyRunning,
			Mode:     "poc",
			Target:   "./pkg/" + string(rune('a'+i%26)),
			Evidence: "run",
		})
		HandleVerifyUpdate(panel, VerifyUpdateMsg{
			State:    VerifyPassed,
			Mode:     "poc",
			Target:   "./pkg/" + string(rune('a'+i%26)),
			Evidence: "pass",
		})
	}
	if len(panel.History) > 20 {
		t.Errorf("expected history capped at 20, got %d", len(panel.History))
	}
	if len(panel.History) != 20 {
		t.Errorf("expected exactly 20 entries, got %d", len(panel.History))
	}
}

func TestStyleForVerifyState(t *testing.T) {
	styles := testStyles()
	cases := []VerifyState{
		VerifyPassed,
		VerifyFailed,
		VerifyBlocked,
		VerifyRunning,
		VerifyPending,
		VerifyIdle,
	}
	for _, state := range cases {
		_ = styleForVerifyState(state, styles)
	}
}

func TestVerifyUpdateMsgFields(t *testing.T) {
	msg := VerifyUpdateMsg{
		State:    VerifyPassed,
		Mode:     "poc",
		Target:   "./pkg/y",
		Evidence: "ok",
	}
	panel := &VerifyPanel{}
	HandleVerifyUpdate(panel, msg)
	if panel.Mode != "poc" || panel.Target != "./pkg/y" || panel.Evidence != "ok" {
		t.Errorf("fields not propagated: %+v", panel)
	}
}
