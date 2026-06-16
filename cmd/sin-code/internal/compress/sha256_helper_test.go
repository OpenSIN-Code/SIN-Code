// SPDX-License-Identifier: MIT
// Purpose: helpers for test files. Importing crypto/sha256 directly
// avoids import-cycle issues with the lessons package's helper.
package compress

import "crypto/sha256"

// sha256SumImpl is the actual SHA-256 invocation. Lifted into a
// distinct file so the test helpers can stay small.
func sha256SumImpl(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}
