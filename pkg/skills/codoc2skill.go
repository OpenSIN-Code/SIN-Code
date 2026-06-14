package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CoDocToSkill converts a CoDoc (.doc.md) into a SKILL.md skill.
// It extracts verification steps, anti-patterns, and usage examples.
func CoDocToSkill(codocPath string, outputDir string) error {
	content, err := os.ReadFile(codocPath)
	if err != nil {
		return err
	}
	text := string(content)

	// Extract base name from filename (e.g., "auth.doc.md" -> "auth")
	base := strings.TrimSuffix(filepath.Base(codocPath), ".doc.md")
	skillName := "codoc-" + base

	// Parse sections
	var overview, steps, verification, anti strings.Builder

	overview.WriteString(fmt.Sprintf("Auto-generated from CoDoc: %s\n", base))
	overview.WriteString("This skill encapsulates the verified invariants and patterns from the documentation.\n")

	// Look for typical CoDoc markers: "## Verification", "## Anti-Patterns", "## Steps"
	stepRegex := regexp.MustCompile(`(?m)^###?\s+(\d+)[\.\)]\s+(.+)$`)
	verifRegex := regexp.MustCompile(`(?m)^-\s+\[ \]\s+(.+)$`)
	antiRegex := regexp.MustCompile(`(?m)^\|\s*(.+?)\s*\|\s*(.+?)\s*\|`)

	steps.WriteString("## Steps\n")
	matches := stepRegex.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		steps.WriteString(fmt.Sprintf("%s. %s\n", m[1], m[2]))
	}
	if len(matches) == 0 {
		steps.WriteString("1. Follow the guidelines defined in the CoDoc.\n")
	}

	verification.WriteString("## Verification\n")
	verifs := verifRegex.FindAllStringSubmatch(text, -1)
	for _, v := range verifs {
		verification.WriteString(fmt.Sprintf("- [ ] %s\n", v[1]))
	}
	if len(verifs) == 0 {
		verification.WriteString("- [ ] All invariants from CoDoc are satisfied.\n")
	}

	anti.WriteString("## Anti-Rationalization\n| Excuse | Rebuttal |\n|--------|----------|\n")
	antis := antiRegex.FindAllStringSubmatch(text, -1)
	for _, a := range antis {
		anti.WriteString(fmt.Sprintf("| %s | %s |\n", strings.TrimSpace(a[1]), strings.TrimSpace(a[2])))
	}
	if len(antis) == 0 {
		anti.WriteString("| \"I can skip reading the CoDoc\" | CoDoc contains critical safety invariants. |\n")
	}

	// Assemble final SKILL.md
	skillContent := fmt.Sprintf(`# %s

## Overview
%s

%s

%s

%s

## Quality Gates
- This skill must not violate any invariant defined in the original CoDoc.
- All generated code must be verified by the Critic agent.

## Source CoDoc
%s
`, skillName, overview.String(), steps.String(), verification.String(), anti.String(), codocPath)

	outputFile := filepath.Join(outputDir, skillName, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return err
	}
	return os.WriteFile(outputFile, []byte(skillContent), 0644)
}

// BatchConvertCoDocs scans a directory for .doc.md files and converts all.
func BatchConvertCoDocs(srcDir, dstSkillsDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".doc.md") {
			fmt.Printf("Converting CoDoc: %s\n", path)
			if err := CoDocToSkill(path, dstSkillsDir); err != nil {
				return fmt.Errorf("convert %s: %w", path, err)
			}
		}
		return nil
	})
}
