// SPDX-License-Identifier: MIT
package gsd

import (
	"fmt"
	"strings"
)

func AnalyzeWaves(plan *Plan) [][]Task {
	if plan == nil || len(plan.Tasks) == 0 {
		return nil
	}

	taskMap := make(map[string]*Task)
	for i := range plan.Tasks {
		taskMap[plan.Tasks[i].ID] = &plan.Tasks[i]
	}

	resolved := make(map[string]bool)
	var waves [][]Task

	remaining := make([]Task, len(plan.Tasks))
	copy(remaining, plan.Tasks)

	for len(remaining) > 0 {
		var wave []Task
		var next []Task

		for _, t := range remaining {
			ready := true
			for _, dep := range t.Dependencies {
				if !resolved[dep] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, t)
			} else {
				next = append(next, t)
			}
		}

		if len(wave) == 0 {
			wave = remaining
			next = nil
		}

		for _, t := range wave {
			resolved[t.ID] = true
		}

		waves = append(waves, wave)
		remaining = next
	}

	return waves
}

func UpdateTaskStatus(root, phaseID, taskID, status string) error {
	plan, err := LoadPlan(root, phaseID)
	if err != nil {
		return err
	}

	found := false
	for i, t := range plan.Tasks {
		if t.ID == taskID {
			found = true
			plan.Tasks[i].Status = status
			break
		}
	}
	if !found {
		return fmt.Errorf("gsd: task %q not found in phase %s", taskID, phaseID)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Plan: Phase %s\n\n", phaseID))
	b.WriteString(serializePlanTasks(plan.Tasks))

	return writeFile(planPath(root, phaseID), b.String())
}

func ExecuteState(root, phaseID string) (*ExecuteReport, error) {
	plan, err := LoadPlan(root, phaseID)
	if err != nil {
		return nil, err
	}

	waves := AnalyzeWaves(plan)

	completed := 0
	for _, t := range plan.Tasks {
		if t.Status == TaskStatusDone {
			completed++
		}
	}

	nextWave := -1
	for i, wave := range waves {
		hasPending := false
		for _, t := range wave {
			if t.Status != TaskStatusDone {
				hasPending = true
				break
			}
		}
		if hasPending {
			nextWave = i
			break
		}
	}

	return &ExecuteReport{
		PhaseID:       phaseID,
		Waves:         waves,
		CompletedCount: completed,
		TotalCount:    len(plan.Tasks),
		NextWave:      nextWave,
	}, nil
}

type ExecuteReport struct {
	PhaseID       string
	Waves         [][]Task
	CompletedCount int
	TotalCount    int
	NextWave      int
}
