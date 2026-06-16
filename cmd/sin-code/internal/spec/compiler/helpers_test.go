// SPDX-License-Identifier: MIT
// Purpose: tiny test helper for writing files + YAML marshaling.
package compiler

import (
	"os"

	"gopkg.in/yaml.v3"
)

func writeFileImpl(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// yamlMarshal re-emits a Config to YAML for the round-trip test.
// Uses yaml.v3 (already in go.mod).
func yamlMarshal(c *Config) ([]byte, error) {
	return yaml.Marshal(c)
}
