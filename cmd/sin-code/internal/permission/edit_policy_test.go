// SPDX-License-Identifier: MIT
// Purpose: Tests for the unified edit policy (issue #373).
package permission

import (
	"testing"
)

func TestEditPolicy_AllowSmallEdit(t *testing.T) {
	p := NewEditPolicy("/workspace")
	op := EditOperation{
		Type:      "string_replace",
		FilePath:  "/workspace/src/main.go",
		OldString: "old",
		NewString: "new",
	}
	if got := p.CheckPermission(op); got != "allow" {
		t.Errorf("got %q, want %q", got, "allow")
	}
}

func TestEditPolicy_DenyEnvFile(t *testing.T) {
	p := NewEditPolicy("/workspace")
	op := EditOperation{
		Type:      "string_replace",
		FilePath:  "/workspace/.env",
		OldString: "KEY=old",
		NewString: "KEY=new",
	}
	if got := p.CheckPermission(op); got != "deny" {
		t.Errorf("got %q, want %q", got, "deny")
	}
}

func TestEditPolicy_DenyEnvLocalFile(t *testing.T) {
	p := NewEditPolicy("/workspace")
	op := EditOperation{
		Type:      "string_replace",
		FilePath:  "/workspace/.env.local",
		OldString: "a",
		NewString: "b",
	}
	if got := p.CheckPermission(op); got != "deny" {
		t.Errorf("got %q, want %q", got, "deny")
	}
}

func TestEditPolicy_DenyOutsideWorkspace(t *testing.T) {
	p := NewEditPolicy("/workspace")
	op := EditOperation{
		Type:      "string_replace",
		FilePath:  "/etc/passwd",
		OldString: "x",
		NewString: "y",
	}
	if got := p.CheckPermission(op); got != "deny" {
		t.Errorf("got %q, want %q", got, "deny")
	}
}

func TestEditPolicy_AskLargeDiff(t *testing.T) {
	p := NewEditPolicy("/workspace")
	// Build old and new strings with > 100 lines combined.
	oldLines := ""
	newLines := ""
	for i := 0; i < 60; i++ {
		oldLines += "old line\n"
		newLines += "new line\n"
	}
	op := EditOperation{
		Type:      "string_replace",
		FilePath:  "/workspace/src/big.go",
		OldString: oldLines,
		NewString: newLines,
	}
	if got := p.CheckPermission(op); got != "ask" {
		t.Errorf("got %q, want %q", got, "ask")
	}
}

func TestEditPolicy_ValidateMissingFilePath(t *testing.T) {
	p := NewEditPolicy("/workspace")
	op := EditOperation{
		Type:      "string_replace",
		FilePath:  "",
		OldString: "x",
		NewString: "y",
	}
	if err := p.Validate(op); err == nil {
		t.Error("Validate should fail for empty FilePath")
	}
	if got := p.CheckPermission(op); got != "deny" {
		t.Errorf("CheckPermission got %q, want %q", got, "deny")
	}
}

func TestEditPolicy_ValidateInvalidType(t *testing.T) {
	p := NewEditPolicy("/workspace")
	op := EditOperation{
		Type:      "bogus",
		FilePath:  "/workspace/src/main.go",
		OldString: "x",
		NewString: "y",
	}
	if err := p.Validate(op); err == nil {
		t.Error("Validate should fail for unknown type")
	}
}

func TestEditPolicy_RelativePathInWorkspace(t *testing.T) {
	p := NewEditPolicy("/workspace")
	op := EditOperation{
		Type:      "string_replace",
		FilePath:  "src/main.go",
		OldString: "old",
		NewString: "new",
	}
	if got := p.CheckPermission(op); got != "allow" {
		t.Errorf("got %q, want %q", got, "allow")
	}
}

func TestEditPolicy_RelativePathOutsideWorkspace(t *testing.T) {
	p := NewEditPolicy("/workspace")
	op := EditOperation{
		Type:      "string_replace",
		FilePath:  "../other/main.go",
		OldString: "old",
		NewString: "new",
	}
	if got := p.CheckPermission(op); got != "deny" {
		t.Errorf("got %q, want %q", got, "deny")
	}
}
