// SPDX-License-Identifier: MIT
// Purpose: OS-level isolation for shell command execution. On Linux it
// uses Landlock (kernel >= 5.13) to restrict filesystem and network
// access. On other platforms it falls back to a no-op with an explicit
// warning so callers can surface degraded isolation to the user.
package sandbox

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// Policy describes what a sandboxed command may access.
type Policy struct {
	// ReadOnlyPaths are directories the command may read but not modify.
	ReadOnlyPaths []string
	// ReadWritePaths are directories the command may read and modify.
	// Typically the project workdir and a scratch temp dir — nothing else.
	ReadWritePaths []string
	// AllowNetwork controls outbound network access. Default false.
	AllowNetwork bool
	// Timeout is the hard wall-clock limit for the command.
	Timeout time.Duration
}

// DefaultPolicy returns the recommended policy for agent bash execution:
// read/write only inside workdir and tmpDir, read-only system paths,
// no network.
func DefaultPolicy(workdir, tmpDir string) Policy {
	return Policy{
		ReadOnlyPaths: []string{
			"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc",
			"/opt", "/proc/self", "/dev/null", "/dev/urandom",
		},
		ReadWritePaths: []string{workdir, tmpDir},
		AllowNetwork:   false,
		Timeout:        2 * time.Minute,
	}
}

// Result reports how the command was isolated.
type Result struct {
	// Enforced is true when OS-level sandboxing was active.
	Enforced bool
	// Mechanism names the isolation backend ("landlock", "none").
	Mechanism string
	// Warning is non-empty when isolation is degraded.
	Warning string
}

// Command builds an *exec.Cmd whose process is confined by the given
// policy where the platform supports it. The returned Result tells the
// caller whether enforcement is active so it can warn the user.
func Command(ctx context.Context, policy Policy, name string, args ...string) (*exec.Cmd, Result, error) {
	if policy.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, policy.Timeout)
		_ = cancel
	}
	return platformCommand(ctx, policy, name, args...)
}

// existing filters out paths that don't exist on disk. Used by
// platformCommand to skip rules that would never match anything anyway.
func existing(paths []string) []string {
	out := paths[:0]
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}
