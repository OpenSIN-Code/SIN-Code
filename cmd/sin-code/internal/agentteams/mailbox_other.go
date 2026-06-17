// SPDX-License-Identifier: MIT
// Platform abstraction: no-op file locking for platforms that lack
// both flock (Unix) and LockFileEx (Windows) — e.g. js/wasm. The
// in-process sync.Mutex in mailbox.go still serialises within a
// single process; cross-process safety is not guaranteed on these
// platforms.
//go:build !unix && !windows

package agentteams

// flockLock is a no-op on unsupported platforms.
func flockLock(fd int) error {
	return nil
}

// flockUnlock is a no-op on unsupported platforms.
func flockUnlock(fd int) error {
	return nil
}
