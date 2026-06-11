// SPDX-License-Identifier: MIT
//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// platformCommand on Linux re-execs the current binary in a hidden
// "__sandbox_exec" mode that applies Landlock then execs the real
// command. The child inherits the prepared *exec.Cmd which carries
// the policy as env vars (NUL-delimited to survive paths with spaces).
func platformCommand(ctx context.Context, policy Policy, name string, args ...string) (*exec.Cmd, Result, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, Result{}, fmt.Errorf("sandbox: resolve self: %w", err)
	}
	shimArgs := append([]string{"__sandbox_exec", name}, args...)
	cmd := exec.CommandContext(ctx, self, shimArgs...)
	cmd.Env = append(os.Environ(),
		"SIN_SANDBOX_ACTIVE=1",
		"SIN_SANDBOX_RO="+joinPaths(policy.ReadOnlyPaths),
		"SIN_SANDBOX_RW="+joinPaths(policy.ReadWritePaths),
		fmt.Sprintf("SIN_SANDBOX_NET=%t", policy.AllowNetwork),
	)
	return cmd, Result{Enforced: true, Mechanism: "landlock"}, nil
}

// ApplyAndExec is called from main() when os.Args[1] == "__sandbox_exec".
// It applies the sandbox to the current process then execs the target.
// On platforms without Landlock support (kernel < 5.13) it degrades
// gracefully: the command still runs but with a warning.
func ApplyAndExec() error {
	if os.Getenv("SIN_SANDBOX_ACTIVE") != "1" {
		return fmt.Errorf("sandbox: shim invoked without SIN_SANDBOX_ACTIVE")
	}
	ro := splitPaths(os.Getenv("SIN_SANDBOX_RO"))
	rw := splitPaths(os.Getenv("SIN_SANDBOX_RW"))
	netAllowed := os.Getenv("SIN_SANDBOX_NET") == "true"

	if err := applyLandlock(ro, rw, netAllowed); err != nil {
		// Landlock unavailable (kernel too old, FS unsupported).
		// Continue without sandboxing so the tool still runs on legacy
		// systems; the caller will see a warning.
		fmt.Fprintf(os.Stderr, "sin-code sandbox: %v (degraded mode)\n", err)
	}

	name := os.Args[2]
	args := os.Args[2:]
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("sandbox: lookpath %q: %w", name, err)
	}
	return unixExec(path, args, os.Environ())
}

func applyLandlock(ro, rw []string, netAllowed bool) error {
	return applyLandlockImpl(ro, rw, netAllowed)
}

func existing(paths []string) []string {
	out := paths[:0]
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func joinPaths(ps []string) string { return strings.Join(ps, "\x00") }
func splitPaths(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}
