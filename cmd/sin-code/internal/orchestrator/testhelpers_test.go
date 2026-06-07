// SPDX-License-Identifier: MIT
// Purpose: test helpers for the orchestrator package.
package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func filepath_join(parts ...string) string {
	return filepath.Join(parts...)
}

func must_mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func must_write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
