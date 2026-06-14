// SPDX-License-Identifier: MIT
//go:build linux

// Purpose: Linux Landlock sandbox implementation.
// Docs: cmd/sin-code/internal/sandbox/landlock_linux.go.doc.md
//
// This file uses raw Linux syscalls. It is intentionally self-contained and
// does not pull github.com/landlock-lsm/go-landlock as a dependency.
// Landlock is best-effort: on kernels < 5.13 or unsupported filesystems the
// syscalls return ENOSYS/EINVAL and the caller degrades to running without
// sandboxing.
package sandbox

import (
	"fmt"
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
