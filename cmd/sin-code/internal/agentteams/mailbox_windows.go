// SPDX-License-Identifier: MIT
// Platform abstraction: no-op file locking for Windows.
// Windows has no syscall.Flock equivalent in the Go syscall package.
// Cross-process serialisation relies on O_APPEND atomicity for small
// writes (POSIX guarantee honoured by the Windows NT kernel for
// FILE_APPEND_DATA) plus the in-process sync.Mutex in mailbox.go.
// A future revision may use LockFileEx for true cross-process locks.
//go:build windows

package agentteams

// flockLock is a no-op on Windows. See file header for rationale.
func flockLock(fd int) error {
	return nil
}

// flockUnlock is a no-op on Windows.
func flockUnlock(fd int) error {
	return nil
}
