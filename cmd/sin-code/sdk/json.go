// SPDX-License-Identifier: MIT
// Purpose: standalone helper to bridge `json.RawMessage` into
// typed targets. Trimmed down to what's actually used (a `map`
// and any struct) so the package has no stdlib aliasing.
package sdk

import "encoding/json"

// jsonUnmarshal decodes raw JSON into dst. Errors propagate.
func jsonUnmarshal(raw []byte, dst any) error {
	return json.Unmarshal(raw, dst)
}
