// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests to reach 100% statement coverage.
package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadFileError(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, ".sin", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cmdDir, "broken.md")
	if err := os.WriteFile(p, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(p, 0o644)

	old := osReadFileHook
	osReadFileHook = func(name string) ([]byte, error) {
		if name == p {
			return nil, os.ErrPermission
		}
		return os.ReadFile(name)
	}
	defer func() { osReadFileHook = old }()

	cmds, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cmds["broken"]; ok {
		t.Fatal("broken command should have been skipped")
	}
}

func TestParseFrontmatterNoColon(t *testing.T) {
	raw := "---\nno colon here\ndescription: valid\n---\nbody"
	c := parse("x", raw)
	if c.Description != "valid" {
		t.Fatalf("expected description 'valid', got %q", c.Description)
	}
	if c.Template != "body" {
		t.Fatalf("expected template 'body', got %q", c.Template)
	}
}
