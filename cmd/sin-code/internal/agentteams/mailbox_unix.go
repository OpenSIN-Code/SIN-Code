// SPDX-License-Identifier: MIT
// Platform abstraction: flock-based file locking for Unix-like systems
// (darwin, linux, freebsd, …). Windows has no syscall.Flock; see
// mailbox_windows.go for the no-op counterpart.
//go:build unix

package agentteams

import "syscall"

// flockLock acquires an exclusive advisory lock on the file descriptor.
// The lock is released by flockUnlock or by closing the fd.
func flockLock(fd int) error {
	return syscall.Flock(fd, syscall.LOCK_EX)
}

// flockUnlock releases a previously acquired advisory lock.
func flockUnlock(fd int) error {
	return syscall.Flock(fd, syscall.LOCK_UN)
}
