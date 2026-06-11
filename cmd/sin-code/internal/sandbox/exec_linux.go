// SPDX-License-Identifier: MIT
//go:build linux

package sandbox

import (
	"fmt"

	"golang.org/x/sys/unix"
)

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
			fmt.Fprintf(stderr(), "sandbox: net restrict skipped: %v\n", err)
		}
	}
	return nil
}

func unixExec(path string, args, env []string) error {
	return unix.Exec(path, args, env)
}
