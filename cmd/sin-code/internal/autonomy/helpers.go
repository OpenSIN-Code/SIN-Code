// SPDX-License-Identifier: MIT
// Purpose: thin wrappers to avoid pulling crypto+encoding/json imports
// directly into triggers.go when the test surfaces also need them.
package autonomy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
)

func sha256Sum() hash.Hash { return sha256.New() }
func hexEncodeToString(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }
func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
