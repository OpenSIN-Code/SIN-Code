package skills

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type SkillGenerator struct {
	agent interface{} // agent.System
}

func NewSkillGenerator(agentSys interface{}) *SkillGenerator {
	return &SkillGenerator{agent: agentSys}
}

// GenerateSkillFromPrompt asks the agent to create a SKILL.md content.
func (g *SkillGenerator) GenerateSkillFromPrompt(ctx context.Context, prompt string) (string, error) {
	systemPrompt := `You are a skill architect. Generate a valid SKILL.md file following the agent-skills standard.
The skill must have:
- A name (first heading)
- ## Overview
- ## Steps (numbered list)
- ## Verification (checkbox list)
- ## Anti-Rationalization (markdown table)

Return ONLY the SKILL.md content.`
	userPrompt := fmt.Sprintf("Create a skill that does: %s", prompt)

	// For now, return a template. In real implementation, call
	// agent.Generate(ctx, systemPrompt, userPrompt). The prompts are
	// retained so the wiring is ready once the agent backend is attached.
	_ = systemPrompt
	_ = userPrompt
	response := generateSkillTemplate(prompt)
	
	// Basic validation
	if !strings.Contains(response, "# ") || !strings.Contains(response, "## Steps") {
		return "", fmt.Errorf("generated skill is malformed")
	}
	return response, nil
}

// SaveGeneratedSkill writes the generated skill to disk and registers it.
func (g *SkillGenerator) SaveGeneratedSkill(content, outputDir string) (string, error) {
	// Extract name from first line
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "# ") {
		return "", fmt.Errorf("no skill name found")
	}
	name := strings.TrimPrefix(lines[0], "# ")
	name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	skillDir := fmt.Sprintf("%s/%s", outputDir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", err
	}
	skillPath := fmt.Sprintf("%s/SKILL.md", skillDir)
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		return "", err
	}
	return name, nil
}

func generateSkillTemplate(prompt string) string {
	return fmt.Sprintf(`# generated-skill

## Overview
Generated skill based on: %s

## Steps
1. Analyze requirement
2. Design solution
3. Implement
4. Test
5. Verify

## Verification
- [ ] All steps completed
- [ ] Output meets requirements

## Anti-Rationalization
| Excuse | Rebuttal |
|--------|----------|
| "This is good enough" | Quality matters always |
`, prompt)
}
