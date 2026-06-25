// SPDX-License-Identifier: MIT
//go:build linux

// Purpose: Linux Landlock sandbox implementation.
// Also contains: platform command + shim (merged from exec_linux.go).
// Docs: cmd/sin-code/internal/sandbox/landlock_linux.go.doc.md
//
// This file uses raw Linux syscalls. It is intentionally self-contained and
// does not pull github.com/landlock-lsm/go-landlock as a dependency.
// Landlock is best-effort: on kernels < 5.13 or unsupported filesystems the
// syscalls return ENOSYS/EINVAL and the caller degrades to running without
// sandboxing.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Landlock filesystem access rights (ABI v5+).
// See https://www.kernel.org/doc/html/latest/userspace-api/landlock.html#filesystem-flags
const (
	accessFSExecute = 1 << iota
	accessFSWriteFile
	accessFSReadFile
	accessFSReadDir
	accessFSRemoveDir
	accessFSRemoveFile
	accessFSMakeChar
	accessFSMakeDir
	accessFSMakeReg
	accessFSMakeSock
	accessFSMakeFifo
	accessFSMakeBlock
	accessFSMakeSym
	accessFSRefer
	accessFSTruncate
	accessFSIoctlDev
)

// Landlock network access rights (ABI v4+).
const (
	accessNetBindTCP = 1 << iota
	accessNetConnectTCP
)

// rule is a single Landlock filesystem rule: "allow access X on path Y".
type rule struct {
	access uint64
	path   string
}

func roDirs(p string) rule {
	return rule{access: accessFSReadFile | accessFSReadDir | accessFSRefer, path: p}
}
func rwDirs(p string) rule {
	return rule{access: accessFSReadFile | accessFSReadDir | accessFSWriteFile | accessFSRemoveFile | accessFSRemoveDir | accessFSMakeDir | accessFSMakeReg | accessFSMakeSym | accessFSRefer, path: p}
}

// applyRules applies a list of filesystem rules to the current process using
// Linux Landlock. Each rule is applied independently so that a single
// unsupported path does not abort the whole policy.
func applyRules(rules []rule) error {
	if len(rules) == 0 {
		// No rules: still create an empty ruleset that denies everything by default.
		// Use a read-only /dev/null rule to make the semantics explicit.
		rules = []rule{{access: accessFSReadFile | accessFSReadDir, path: "/dev/null"}}
	}

	// Compute the union of all handled access rights.
	var handled uint64
	for _, r := range rules {
		handled |= r.access
	}

	attr := unix.LandlockRulesetAttr{
		Access_fs: handled,
	}
	fd, _, errno := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %v", errno)
	}

	for _, r := range rules {
		parentFD, err := unix.Open(r.path, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			// Best-effort: skip paths that cannot be opened.
			continue
		}
		pathAttr := unix.LandlockPathBeneathAttr{
			Allowed_access: r.access,
			Parent_fd:      int32(parentFD),
		}
		_, _, errno = unix.Syscall6(
			unix.SYS_LANDLOCK_ADD_RULE,
			fd,
			uintptr(unix.LANDLOCK_RULE_PATH_BENEATH),
			uintptr(unsafe.Pointer(&pathAttr)),
			0, 0, 0,
		)
		_ = unix.Close(parentFD)
		if errno != 0 {
			// Rule not supported by this kernel/FS — skip, do not fail.
			continue
		}
	}

	_, _, errno = unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %v", errno)
	}
	return nil
}

// applyNetRules applies a no-TCP policy by creating a network ruleset that
// handles bind/connect but adds no allow rules. Restricting the process with
// such a ruleset denies all TCP bind/connect operations (kernel >= 6.2).
func applyNetRules() error {
	attr := unix.LandlockRulesetAttr{
		Access_net: accessNetBindTCP | accessNetConnectTCP,
	}
	fd, _, errno := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("net_ruleset: %v", errno)
	}
	_, _, errno = unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0)
	if errno != 0 {
		return fmt.Errorf("net restrict: %v", errno)
	}
	return nil
}

// ── Platform command + shim (merged from exec_linux.go) ──────────────────

// applyLandlockImpl applies Landlock filesystem and (optionally) network
// rules to the current process. The ruleset is built from the read-only
// and read-write paths in the policy.
//
// Best-effort: each rule is applied independently so a single bad path
// (e.g. /proc on a kernel that forbids it) does not abort the rest.
func applyLandlockImpl(ro, rw []string, netAllowed bool) error {
	rules := make([]rule, 0, len(ro)+len(rw))
	for _, p := range existing(ro) {
		rules = append(rules, roDirs(p))
	}
	for _, p := range existing(rw) {
		rules = append(rules, rwDirs(p))
	}
	if err := applyRules(rules); err != nil {
		return fmt.Errorf("landlock restrict: %w", err)
	}
	if !netAllowed {
		// Kernel 6.7+ supports LANDLOCK_NET_BIND_TCP/CONNECT. We try
		// gracefully — on older kernels this returns ENOPROTOOPT and
		// we just continue without the net block.
		if err := applyNetRules(); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox: net restrict skipped: %v\n", err)
		}
	}
	return nil
}

// sin-debt: shrink, upgrade: inline when a second platform-specific function is needed, merge into a shared file
func unixExec(path string, args, env []string) error {
	return unix.Exec(path, args, env)
}

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

// sin-debt: shrink, upgrade: when a second platform-specific function is needed, merge into a shared file
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

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func applyLandlock(ro, rw []string, netAllowed bool) error {
	return applyLandlockImpl(ro, rw, netAllowed)
}

func joinPaths(ps []string) string { return strings.Join(ps, "\x00") }
func splitPaths(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}
