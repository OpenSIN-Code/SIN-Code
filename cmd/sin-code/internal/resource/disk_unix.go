// SPDX-License-Identifier: MIT
//go:build unix

// Purpose: free-disk probe for the daemon's disk back-pressure on
// unix-like systems (Linux, macOS, *BSD) via statfs (issue #71).
package resource

import "golang.org/x/sys/unix"

// DiskFree returns the number of bytes available to an unprivileged
// process on the filesystem that contains path. ok is false when the
// probe is unavailable (then callers must skip the disk gate).
func DiskFree(path string) (free int64, ok bool) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, false
	}
	// Bavail = blocks free to unprivileged users; Bsize = block size.
	return int64(st.Bavail) * int64(st.Bsize), true // #nosec G115
}
