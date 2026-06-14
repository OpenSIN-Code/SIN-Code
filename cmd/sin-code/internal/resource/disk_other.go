// SPDX-License-Identifier: MIT
//go:build !unix

// Purpose: fallback free-disk probe for non-unix platforms (e.g.
// Windows). Reports "unavailable" so the daemon disk gate is skipped
// rather than guessing (issue #71).
package resource

// DiskFree is unavailable off unix; callers skip the disk gate when
// ok is false.
func DiskFree(_ string) (free int64, ok bool) {
	return 0, false
}
