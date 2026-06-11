// SPDX-License-Identifier: MIT
//go:build !linux

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// platformCommand on non-Linux platforms cannot enforce kernel-level
// isolation. It returns the plain command with an explicit warning so
// the caller (sin_bash tool) can surface degraded isolation and apply
// its existing safeguards (timeouts, secret redaction, deny-lists).
func platformCommand(ctx context.Context, _ Policy, name string, args ...string) (*exec.Cmd, Result, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd, Result{
		Enforced:  false,
		Mechanism: "none",
		Warning:   "OS-level sandboxing unavailable on this platform; relying on redaction, timeouts and deny-lists only",
	}, nil
}

// ApplyAndExec is the no-op fallback on non-Linux platforms. The
// sandbox re-exec shim in main.go calls this only when SIN_SANDBOX_ACTIVE
// is set (set by platformCommand on Linux), so on non-Linux it should
// never be reached. We return an error to surface a programming mistake.
func ApplyAndExec() error {
	return fmt.Errorf("sandbox: ApplyAndExec called on non-Linux platform (not supported)")
}

func joinPaths(ps []string) string { return strings.Join(ps, "\x00") }
func splitPaths(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}
