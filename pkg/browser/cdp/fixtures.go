// SPDX-License-Identifier: MIT
// Purpose: static test assets used by the fault-injection test server.
package cdp

import "encoding/base64"

// onePxPNG is a 1x1 transparent PNG used by the slow-LCP fixture in
// testserver.go. It is served with an artificial delay to guarantee that
// LCP is reported above the "needs improvement" threshold.
var onePxPNG = mustDecode(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")

func mustDecode(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
