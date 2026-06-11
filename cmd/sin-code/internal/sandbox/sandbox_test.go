// SPDX-License-Identifier: MIT
package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDefaultPolicy_SaneValues verifies that DefaultPolicy returns reasonable
// defaults: contains the workdir + tmp in RW paths, no network by default,
// and a non-zero timeout.
func TestDefaultPolicy_SaneValues(t *testing.T) {
	workdir := "/work"
	tmpDir := "/tmp"
	p := DefaultPolicy(workdir, tmpDir)
	if len(p.ReadWritePaths) != 2 {
		t.Errorf("expected 2 RW paths, got %d", len(p.ReadWritePaths))
	}
	if p.ReadWritePaths[0] != workdir || p.ReadWritePaths[1] != tmpDir {
		t.Errorf("unexpected RW paths: %v", p.ReadWritePaths)
	}
	if p.AllowNetwork {
		t.Error("expected AllowNetwork=false by default")
	}
	if p.Timeout <= 0 {
		t.Errorf("expected positive Timeout, got %v", p.Timeout)
	}
	if len(p.ReadOnlyPaths) == 0 {
		t.Error("expected non-empty ReadOnlyPaths")
	}
}

// TestCommand_NonLinuxReturnsDegraded verifica that on non-Linux we always
// return a Result with Enforced=false and a warning, but still produce a
// working *exec.Cmd. Sandbox is a soft-fail: it never breaks the caller.
func TestCommand_NonLinuxReturnsDegraded(t *testing.T) {
	policy := DefaultPolicy(t.TempDir(), t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, res, err := Command(ctx, policy, "echo", "hello")
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd == nil {
		t.Fatal("Command returned nil *exec.Cmd")
	}
	// On non-Linux we expect the no-op fallback.
	if res.Enforced {
		t.Skip("landlock available; this assertion only runs on non-Linux")
	}
	if res.Mechanism == "" {
		t.Error("expected non-empty Mechanism on non-Linux fallback")
	}
	if res.Warning == "" {
		t.Error("expected non-empty Warning when sandbox is degraded")
	}
}

// TestExisting_FiltersMissingPaths verifies that existing() drops entries
// that don't exist on disk (e.g. /opt on a stripped container).
func TestExisting_FiltersMissingPaths(t *testing.T) {
	tmp := t.TempDir()
	present := filepath.Join(tmp, "present")
	if err := os.MkdirAll(present, 0755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(tmp, "missing")
	in := []string{present, missing, "/definitely/does/not/exist/zzz"}
	got := existing(in)
	if len(got) != 1 {
		t.Errorf("expected 1 existing path, got %d: %v", len(got), got)
	}
	if got[0] != present {
		t.Errorf("expected %q, got %q", present, got[0])
	}
}
