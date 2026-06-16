// SPDX-License-Identifier: MIT
// Purpose: thin wrappers to avoid pulling crypto+encoding/json imports
// directly into triggers.go when the test surfaces also need them.
package autonomy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"os/exec"
)

func sha256Sum() hash.Hash                   { return sha256.New() }
func hexEncodeToString(h hash.Hash) string   { return hex.EncodeToString(h.Sum(nil)) }
func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// runCmd runs a command in workspace and returns its combined output. Used by
// the GitHub/CI scanners to read the git remote and current branch.
func runCmd(workspace string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workspace
	out, err := cmd.Output()
	return string(out), err
}

// firstNonEmpty returns the first non-empty string in vals, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
