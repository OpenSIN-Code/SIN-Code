// SPDX-License-Identifier: MIT
// Purpose: tests for per-repo .sin-code.yml configuration loading.
package repoconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileIsZero(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if c.MaxTurns != 0 || c.VerifyMode != "" {
		t.Fatalf("expected zero config, got %+v", c)
	}
}

func TestLoad_ParsesAndDisablesChecks(t *testing.T) {
	dir := t.TempDir()
	yml := "max_turns: 99\nverify_mode: oracle\ndisable_checks: [\"go vet\"]\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxTurns != 99 || c.VerifyMode != "oracle" {
		t.Fatalf("unexpected config: %+v", c)
	}
	if !c.IsCheckDisabled("go vet") || c.IsCheckDisabled("go test") {
		t.Fatal("disable_checks not honored correctly")
	}
}
