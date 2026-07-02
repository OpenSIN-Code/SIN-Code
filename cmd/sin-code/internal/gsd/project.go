// SPDX-License-Identifier: MIT
package gsd

import (
	"fmt"
	"strings"
	"time"
)

func InitProject(root, name, description string) error {
	if err := ensureDir(gsdDir(root)); err != nil {
		return fmt.Errorf("gsd: init project: %w", err)
	}
	if err := ensureDir(plansDir(root)); err != nil {
		return fmt.Errorf("gsd: init plans dir: %w", err)
	}

	now := time.Now().UTC()
	proj := Project{
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	projContent := serializeProject(proj)
	if err := writeFile(projectPath(root), projContent); err != nil {
		return fmt.Errorf("gsd: write PROJECT.md: %w", err)
	}

	if err := writeFile(roadmapPath(root), "# Roadmap\n\n"); err != nil {
		return fmt.Errorf("gsd: write ROADMAP.md: %w", err)
	}

	stateContent := fmt.Sprintf("# State\n\nCurrent Phase: \n\n## History\n\nUpdated At: %s\n", now.Format(time.RFC3339))
	if err := writeFile(statePath(root), stateContent); err != nil {
		return fmt.Errorf("gsd: write STATE.md: %w", err)
	}

	return nil
}

func LoadProject(root string) (*Project, error) {
	content, err := readFile(projectPath(root))
	if err != nil {
		return nil, fmt.Errorf("gsd: load project: %w", err)
	}
	if content == "" {
		return nil, fmt.Errorf("gsd: project not initialized in %s", root)
	}
	return parseProject(content), nil
}

func ProjectStatus(root string) (*StatusReport, error) {
	proj, err := LoadProject(root)
	if err != nil {
		return nil, err
	}

	phases, err := ListPhases(root)
	if err != nil {
		return nil, err
	}

	completed := 0
	for _, p := range phases {
		if p.Status == StatusCompleted {
			completed++
		}
	}

	var currentPhase string
	stateContent, _ := readFile(statePath(root))
	if sc := parseStateCurrent(stateContent); sc != "" {
		currentPhase = sc
	} else {
		for _, p := range phases {
			if p.Status == StatusInProgress {
				currentPhase = p.ID
				break
			}
		}
	}

	var pct float64
	if len(phases) > 0 {
		pct = float64(completed) / float64(len(phases)) * 100
	}

	return &StatusReport{
		Project:        *proj,
		PhaseCount:     len(phases),
		CompletedCount: completed,
		CurrentPhase:   currentPhase,
		CompletionPct:  pct,
	}, nil
}

type StatusReport struct {
	Project        Project
	PhaseCount     int
	CompletedCount int
	CurrentPhase   string
	CompletionPct  float64
}

func serializeProject(p Project) string {
	var b strings.Builder
	b.WriteString("# Project\n\n")
	b.WriteString(fmt.Sprintf("Name: %s\n", p.Name))
	b.WriteString(fmt.Sprintf("Description: %s\n", p.Description))
	if len(p.TechStack) > 0 {
		b.WriteString(fmt.Sprintf("Tech Stack: %s\n", strings.Join(p.TechStack, ", ")))
	}
	b.WriteString(fmt.Sprintf("Created At: %s\n", p.CreatedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Updated At: %s\n", p.UpdatedAt.Format(time.RFC3339)))
	return b.String()
}

func parseProject(content string) *Project {
	p := &Project{}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Name: ") {
			p.Name = strings.TrimPrefix(line, "Name: ")
		} else if strings.HasPrefix(line, "Description: ") {
			p.Description = strings.TrimPrefix(line, "Description: ")
		} else if strings.HasPrefix(line, "Tech Stack: ") {
			raw := strings.TrimPrefix(line, "Tech Stack: ")
			for _, s := range strings.Split(raw, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					p.TechStack = append(p.TechStack, s)
				}
			}
		} else if strings.HasPrefix(line, "Created At: ") {
			if t, err := time.Parse(time.RFC3339, strings.TrimPrefix(line, "Created At: ")); err == nil {
				p.CreatedAt = t
			}
		} else if strings.HasPrefix(line, "Updated At: ") {
			if t, err := time.Parse(time.RFC3339, strings.TrimPrefix(line, "Updated At: ")); err == nil {
				p.UpdatedAt = t
			}
		}
	}
	return p
}

func parseStateCurrent(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Current Phase: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Current Phase: "))
		}
	}
	return ""
}
