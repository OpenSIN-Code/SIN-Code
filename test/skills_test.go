package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/pkg/skills"
)

func TestParseSkill(t *testing.T) {
	content := `# test-skill
## Overview
A test skill.

## Steps
1. First step.
2. Second step.
## Verification
- [ ] Verified.
`
	skill, err := skills.ParseSkill(content, "test.md")
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "test-skill" {
		t.Errorf("expected name test-skill, got %s", skill.Name)
	}
	if len(skill.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(skill.Steps))
	}
	if skill.Steps[0].Instruction != "First step." {
		t.Errorf("bad instruction: %s", skill.Steps[0].Instruction)
	}
}

func TestRegistryInstallAndRun(t *testing.T) {
	tmpDir := t.TempDir()
	reg, err := skills.NewRegistry(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	// Create a dummy skill directory
	skillDir := filepath.Join(tmpDir, "dummy")
	os.MkdirAll(skillDir, 0755)
	skillContent := `# dummy
## Steps
1. Do nothing.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)
	if err := reg.Install(skillDir); err != nil {
		t.Fatal(err)
	}
	list := reg.List()
	if len(list) != 1 || list[0] != "dummy" {
		t.Errorf("registry list mismatch: %v", list)
	}
}
