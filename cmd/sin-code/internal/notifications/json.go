// SPDX-License-Identifier: MIT
// Purpose: small json wrapper used by dispatch to keep import surface minimal.
package notifications

import "encoding/json"

func jsonEncode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
