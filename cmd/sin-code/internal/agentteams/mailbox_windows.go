// SPDX-License-Identifier: MIT
// Platform abstraction: LockFileEx-based file locking for Windows.
// Uses golang.org/x/sys/windows for LockFileEx/UnlockFileEx — the
// standard syscall package only exposes the simpler LockFile/UnlockFile
// which lack the exclusive-lock flag. Cross-process advisory locking
// via a 1-byte exclusive byte-range lock on the file handle.
//go:build windows

package agentteams

import (
	"golang.org/x/sys/windows"
)

// flockLock acquires an exclusive lock via LockFileEx (blocking).
func flockLock(fd int) error {
	var ol windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(fd),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, 0, 1, &ol,
	)
}

// flockUnlock releases a previously acquired lock via UnlockFileEx.
func flockUnlock(fd int) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(fd), 0, 0, 1, &ol)
}
