// SPDX-License-Identifier: MIT
// Purpose: tiny test helper for opening files.
package instinct

import "os"

// osOpenHelper is a thin wrapper around os.Open so test code does
// not need to import os directly. Returns an interface so callers
// only need Close() (test code never reads the body).
func osOpenHelper(p string) (interface{ Close() error }, error) {
	return os.Open(p)
}
