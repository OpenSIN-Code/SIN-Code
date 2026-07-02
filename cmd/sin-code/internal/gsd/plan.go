// SPDX-License-Identifier: MIT
package gsd

import (
	"fmt"
	"strings"
	"time"
)

func SavePlan(root, phaseID string, tasks []Task) error {
	if err := ensureDir(plansDir(root)); err != nil {
		return fmt.Errorf("gsd: ensure plans dir: %w", err)
	}

	phases, err := ListPhases(root)
	if err != nil {
		return fmt.Errorf("gsd: list phases: %w", err)
	}

	var phaseTitle string
	for _, p := range phases {
		if p.ID == phaseID {
			phaseTitle = p.Title
			break
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Plan: Phase %s — %s\n\n", phaseID, phaseTitle))
	b.WriteString(serializePlanTasks(tasks))
	b.WriteString(fmt.Sprintf("\n_Created: %s_\n", time.Now().UTC().Format(time.RFC3339)))

	return writeFile(planPath(root, phaseID), b.String())
}

func LoadPlan(root, phaseID string) (*Plan, error) {
	content, err := readFile(planPath(root, phaseID))
	if err != nil {
		return nil, fmt.Errorf("gsd: load plan: %w", err)
	}
	if content == "" {
		return nil, fmt.Errorf("gsd: plan for phase %s not found", phaseID)
	}

	plan := &Plan{
		PhaseID: phaseID,
		Tasks:   parsePlanTasks(content),
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "_Created: ") {
			raw := strings.TrimPrefix(line, "_Created: ")
			raw = strings.TrimSuffix(raw, "_")
			if t, err := time.Parse(time.RFC3339, raw); err == nil {
				plan.CreatedAt = t
			}
		}
	}

	return plan, nil
}

func PlanExists(root, phaseID string) bool {
	content, err := readFile(planPath(root, phaseID))
	return err == nil && content != ""
}
