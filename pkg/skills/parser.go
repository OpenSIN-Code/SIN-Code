package skills

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Skill represents a parsed agent skill.
type Skill struct {
	Name        string              // e.g., "spec", "plan"
	Description string              // Short description
	FullText    string              // Raw markdown
	Sections    map[string]string   // Key sections: "Overview", "Steps", "Verification", "Anti-Rationalization"
	Steps       []SkillStep         // Parsed numbered steps
	Metadata    map[string]string   // Frontmatter if any
	Path        string              // Source file path
}

type SkillStep struct {
	Number      int
	Instruction string
	Required    bool
	Tools       []string // Suggested MCP tools
}

// ParseSkillFile reads and parses a SKILL.md file.
func ParseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill file: %w", err)
	}
	return ParseSkill(string(data), path)
}

// ParseSkill parses raw markdown content.
func ParseSkill(content string, sourcePath string) (*Skill, error) {
	skill := &Skill{
		Sections: make(map[string]string),
		Metadata: make(map[string]string),
		Path:     sourcePath,
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty skill content")
	}

	// Extract name from first heading (# Name)
	headingRegex := regexp.MustCompile(`^#\s+(.+)$`)
	if match := headingRegex.FindStringSubmatch(lines[0]); match != nil {
		skill.Name = strings.TrimSpace(match[1])
	} else {
		skill.Name = strings.TrimSuffix(sourcePath, "/SKILL.md")
	}

	// Simple section parser: ## Section Name
	currentSection := ""
	sectionContent := []string{}
	stepRegex := regexp.MustCompile(`^(\d+)\.\s+(.+)$`)

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			// Save previous section
			if currentSection != "" {
				skill.Sections[currentSection] = strings.TrimSpace(strings.Join(sectionContent, "\n"))
			}
			currentSection = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			sectionContent = []string{}
			continue
		}
		sectionContent = append(sectionContent, line)

		// Parse steps inside "Steps" or "Workflow" section
		if currentSection == "Steps" || currentSection == "Workflow" {
			if matches := stepRegex.FindStringSubmatch(line); matches != nil {
				stepNum := 0
				fmt.Sscanf(matches[1], "%d", &stepNum)
				skill.Steps = append(skill.Steps, SkillStep{
					Number:      stepNum,
					Instruction: strings.TrimSpace(matches[2]),
					Required:    true,
				})
			}
		}
	}
	if currentSection != "" {
		skill.Sections[currentSection] = strings.TrimSpace(strings.Join(sectionContent, "\n"))
	}

	// Extract description from first paragraph
	if desc, ok := skill.Sections["Overview"]; ok {
		skill.Description = strings.Split(desc, "\n")[0]
	} else if len(skill.Steps) > 0 {
		skill.Description = skill.Steps[0].Instruction
	}

	return skill, nil
}
