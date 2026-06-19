// SPDX-License-Identifier: MIT
// Purpose: tests for tui.ToolBash sandbox routing. Closes the M3/M4
// parity gap with cmd/sin-code/chat_tools.go:toolBash by asserting the
// TUI agent surface routes through sandbox.Command when enabled.
package tui

import (
	"context"
	"strings"
	"testing"
)

// snapshotSandboxConfig is a test helper that captures the current
// sandbox configuration so a test can restore it via t.Cleanup, keeping
// the package-global state from leaking between cases (mandate M7).
func snapshotSandboxConfig(t *testing.T) {
	t.Helper()
	tuiSandboxConfigMu.Lock()
	snap := tuiSandboxConfig
	tuiSandboxConfigMu.Unlock()
	t.Cleanup(func() {
		tuiSandboxConfigMu.Lock()
		tuiSandboxConfig = snap
		tuiSandboxConfigMu.Unlock()
	})
	tuiBashPathMu.Lock()
	pathSnap := tuiBashPathTag
	tuiBashPathMu.Unlock()
	t.Cleanup(func() {
		tuiBashPathMu.Lock()
		tuiBashPathTag = pathSnap
		tuiBashPathMu.Unlock()
	})
}

// TestTuiToolBash_SandboxEnabled_RunsThroughSandbox verifies that when
// tuiSetSandbox is called with a non-empty workspace, tuiToolBash
// routes exec through sandbox.Command and produces the expected output.
//
// Routing is verified structurally via tuiReadBashPath — the function
// records which branch it took, so the test can assert that the
// sandbox branch was selected (not the raw exec.CommandContext path).
// Without this guard the TUI agent runner can silently bypass M3/M4.
func TestTuiToolBash_SandboxEnabled_RunsThroughSandbox(t *testing.T) {
	snapshotSandboxConfig(t)

	ws := t.TempDir()
	tuiSetSandbox(ws)

	tuiSandboxConfigMu.RLock()
	enabled := tuiSandboxConfig.enabled
	storedWS := tuiSandboxConfig.workspace
	tuiSandboxConfigMu.RUnlock()
	if !enabled {
		t.Fatalf("tuiSetSandbox did not enable sandbox")
	}
	if storedWS != ws {
		t.Fatalf("tuiSetSandbox stored workspace %q, want %q", storedWS, ws)
	}

	out, err := tuiToolBash(context.Background(), "echo sandbox-routed-marker")
	if err != nil {
		t.Fatalf("tuiToolBash returned error: %v / out=%q", err, out)
	}
	if !strings.Contains(out, "sandbox-routed-marker") {
		t.Fatalf("expected command output to contain marker, got: %q", out)
	}
	if got := tuiReadBashPath(); got != "sandbox" {
		t.Fatalf("execution path = %q, want %q (sandbox routing invariant violated)", got, "sandbox")
	}
}

// TestTuiToolBash_SandboxDisabled_RunsBareExec is the parity counterpart
// — when tuiSetSandbox("") is called the function must take the legacy
// exec.CommandContext path. If this ever stops being true, the fallback
// branch is silently broken and operators lose the option to disable
// the sandbox explicitly.
func TestTuiToolBash_SandboxDisabled_RunsBareExec(t *testing.T) {
	snapshotSandboxConfig(t)

	tuiSetSandbox("") // disable

	tuiSandboxConfigMu.RLock()
	enabled := tuiSandboxConfig.enabled
	ws := tuiSandboxConfig.workspace
	tuiSandboxConfigMu.RUnlock()
	if enabled {
		t.Fatalf("tuiSetSandbox(\"\") did not disable sandbox")
	}
	if ws != "" {
		t.Fatalf("tuiSetSandbox(\"\") did not clear workspace, got %q", ws)
	}

	out, err := tuiToolBash(context.Background(), "echo legacy-exec-marker")
	if err != nil {
		t.Fatalf("tuiToolBash returned error: %v / out=%q", err, out)
	}
	if !strings.Contains(out, "legacy-exec-marker") {
		t.Fatalf("expected command output to contain marker, got: %q", out)
	}
	if got := tuiReadBashPath(); got != "exec" {
		t.Fatalf("execution path = %q, want %q (legacy-exec routing invariant violated)", got, "exec")
	}
}

// TestTuiToolBash_TruncatesOutput checks the output cap
// (tuiMaxToolOutput) is honoured on the sandbox branch too, matching
// chat_tools.go:toolBash's [... truncated] convention.
func TestTuiToolBash_TruncatesOutput(t *testing.T) {
	snapshotSandboxConfig(t)
	tuiSetSandbox(t.TempDir())

	// Generate ~2x the cap with a deterministic marker on the head.
	marker := "TRUNC-MARKER-HEAD"
	overflow := strings.Repeat("X", tuiMaxToolOutput)
	cmd := "printf '%s' '" + marker + overflow + "'"

	out, err := tuiToolBash(context.Background(), cmd)
	if err != nil {
		t.Fatalf("tuiToolBash returned error: %v / out=%q", err, out)
	}
	if !strings.HasPrefix(out, marker) {
		t.Fatalf("expected output to start with marker, got prefix: %q", out[:min(len(out), len(marker)+8)])
	}
	if !strings.Contains(out, "[... truncated]") {
		t.Fatalf("expected output to contain truncation marker, got tail: %q", out[len(out)-64:])
	}
	if len(out) <= tuiMaxToolOutput {
		t.Fatalf("expected truncated output (>%d bytes), got %d", tuiMaxToolOutput, len(out))
	}
}
