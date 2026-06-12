// SPDX-License-Identifier: MIT
// Purpose: slash command tests (mandate C8, AGENTS.md §8).
package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	raw := "---\ndescription: Security review\nagent: security\nverify_mode: oracle\n---\nReview $ARGUMENTS now."
	c := parse("sec-review", raw)
	if c.Description != "Security review" {
		t.Errorf("description: %q", c.Description)
	}
	if c.Agent != "security" {
		t.Errorf("agent: %q", c.Agent)
	}
	if c.VerifyMode != "oracle" {
		t.Errorf("verify_mode: %q", c.VerifyMode)
	}
	if got := c.Expand("the auth module"); got != "Review the auth module now." {
		t.Errorf("expand: %q", got)
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	c := parse("plain", "Just do $ARGUMENTS.")
	if c.Description != "" || c.Agent != "" || c.VerifyMode != "" {
		t.Fatalf("unexpected frontmatter: %+v", c)
	}
	if c.Template != "Just do $ARGUMENTS." {
		t.Errorf("template: %q", c.Template)
	}
}

func TestParseIncompleteFrontmatter(t *testing.T) {
	// missing closing --- — body stays the whole raw string
	raw := "---\ndescription: open\nstill body"
	c := parse("x", raw)
	if c.Description != "" {
		t.Errorf("incomplete frontmatter should be ignored, got %+v", c)
	}
	if !strings.Contains(c.Template, "still body") {
		t.Errorf("body should remain: %q", c.Template)
	}
}

func TestLoadFromProjectDir(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, ".sin", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "test.md"),
		[]byte("---\ndescription: t\n---\nhello $ARGUMENTS"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := cmds["test"]; !ok {
		t.Fatal("command 'test' not loaded")
	} else if c.Description != "t" {
		t.Errorf("description: %q", c.Description)
	}
}

func TestLoadIgnoresNonMd(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, ".sin", "commands")
	os.MkdirAll(cmdDir, 0o755)
	os.WriteFile(filepath.Join(cmdDir, "skip.txt"), []byte("ignored"), 0o644)
	os.WriteFile(filepath.Join(cmdDir, "ok.md"), []byte("body"), 0o644)
	cmds, _ := Load(dir)
	if _, ok := cmds["skip"]; ok {
		t.Error("txt file should be ignored")
	}
	if _, ok := cmds["ok"]; !ok {
		t.Error("md file should be loaded")
	}
}

func TestLoadEmptyDir(t *testing.T) {
	dir := t.TempDir()
	cmds, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 0 {
		t.Errorf("empty dir: want 0, got %d", len(cmds))
	}
}
