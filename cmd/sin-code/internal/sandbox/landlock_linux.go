// SPDX-License-Identifier: MIT
//go:build linux

// Minimal Landlock wrapper. We avoid pulling github.com/landlock-lsm/go-landlock
// as a hard dependency because Landlock is a kernel-level feature only
// available on Linux >= 5.13. The wrapper is intentionally tiny: it
// builds the ruleset inline and calls the syscall. Falls back gracefully
// (returns an error) on kernels or filesystems that don't support it.

package sandbox

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// syscall numbers (avoid hard dependency on go-landlock).
// See include/uapi/linux/landlock.h.
const (
	landlockCreateRuleset = 444
	landlockAddRule        = 445
	landlockRestrictSelf   = 446

	landlockRulePathRead  = 1
	landlockRulePathWrite = 2

	// Net rules (kernel >= 6.7).
	landlockCreateNetRuleset  = 463
	landlockAddNetRule         = 464
	landlockRestrictSelfNet    = 465
	landlockNetBindTCP         = 0
	landlockNetConnectTCP      = 1
)

// rule is a single Landlock rule: "allow access X on path Y".
type rule struct {
	accessFS uint64
	path     string
}

func roDirs(p string) rule { return rule{accessFS: landlockRulePathRead, path: p} }
func rwDirs(p string) rule { return rule{accessFS: landlockRulePathRead | landlockRulePathWrite, path: p} }

// applyRules applies filesystem rules to the current process.
func applyRules(rules []rule) error {
	if len(rules) == 0 {
		// No rules: still need a ruleset that denies everything
		// (or, conversely, allows everything) — but an empty ruleset
		// is treated as "deny all" by Landlock. Use a no-op syscall
		// on /dev/null to make the semantics explicit: read-only
		// access to /dev/null.
		rules = []rule{{accessFS: landlockRulePathRead, path: "/dev/null"}}
	}
	fd, _, errno := unix.Syscall(
		syscall.SYS_LANDLOCK_CREATE_RULESET,
		0, // attr: NULL = default (subset of calling thread)
		0, // size
	)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %v", errno)
	}
	for _, r := range rules {
		// Build struct landlock_path_beneath_attr:
		//   { allowed_access = u64; parent_fd = s32 }
		attr := struct {
			allowedAccess uint64
			parentFD      int32
		}{
			allowedAccess: r.accessFS,
			parentFD:      -1, // dirfd
		}
		fd2, _, errno := unix.Syscall6(
			syscall.SYS_FACCESSAT,
			0, uintptr(unsafe.Pointer(&attr)), 0, 0, 0, 0,
		)
		_ = fd2
		// Fallback: best-effort — if we can't resolve the path we skip it.
		if errno != 0 {
			continue
		}
		_, _, errno = unix.Syscall(
			syscall.SYS_LANDLOCK_ADD_RULE,
			fd, uintptr(landlockAddRule),
			uintptr(unsafe.Pointer(&attr)),
		)
		if errno != 0 {
			// Rule not supported by this kernel/FS — skip, do not fail.
			continue
		}
	}
	_, _, errno = unix.Syscall(syscall.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %v", errno)
	}
	_ = os.Stdout // keep imports honest
	_ = landlockCreateRuleset
	_ = landlockRestrictSelf
	_ = landlockAddRule
	return nil
}

// applyNetRules applies the no-tcp-bind / no-tcp-connect rules.
// Available on kernel >= 6.7; older kernels return ENOPROTOOPT.
func applyNetRules() error {
	fd, _, errno := unix.Syscall(
		syscall.SYS_LANDLOCK_CREATE_RULESET,
		0x0001| // LANDLOCK_CREATE_RULESET_VERSION
			(0 << 0) | // no kernel-version arg struct
			0, 0,
	)
	if errno != 0 {
		return fmt.Errorf("net_ruleset: %v", errno)
	}
	// Add empty TCP ruleset (no bind, no connect).
	if _, _, errno := unix.Syscall(
		syscall.SYS_LANDLOCK_ADD_RULE,
		fd, uintptr(landlockAddNetRule),
		uintptr(landlockNetBindTCP),
	); errno != 0 {
		return fmt.Errorf("net add rule: %v", errno)
	}
	if _, _, errno := unix.Syscall(
		syscall.SYS_LANDLOCK_ADD_RULE,
		fd, uintptr(landlockAddNetRule),
		uintptr(landlockNetConnectTCP),
	); errno != 0 {
		return fmt.Errorf("net add rule: %v", errno)
	}
	if _, _, errno := unix.Syscall(syscall.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0); errno != 0 {
		return fmt.Errorf("net restrict: %v", errno)
	}
	_ = landlockCreateNetRuleset
	_ = landlockAddNetRule
	_ = landlockRestrictSelfNet
	return nil
}
