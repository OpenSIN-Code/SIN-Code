// SPDX-License-Identifier: MIT
package gsd

import (
	"fmt"
	"strconv"
	"time"
)

func AddPhase(root, title, priority string) (*Phase, error) {
	if priority == "" {
		priority = PriorityP2
	}
	if !isPriority(priority) {
		return nil, fmt.Errorf("gsd: invalid priority %q", priority)
	}

	content, err := readFile(roadmapPath(root))
	if err != nil {
		return nil, fmt.Errorf("gsd: read roadmap: %w", err)
	}

	phases := parsePhases(content)
	id := nextPhaseID(phases)

	phase := Phase{
		ID:        id,
		Title:     title,
		Priority:  priority,
		Status:    StatusPlanning,
		CreatedAt: time.Now().UTC(),
	}

	phases = append(phases, phase)

	if err := writeFile(roadmapPath(root), serializePhases(phases)); err != nil {
		return nil, fmt.Errorf("gsd: write roadmap: %w", err)
	}

	return &phase, nil
}

func InsertPhase(root, afterID, title, priority string) (*Phase, error) {
	if priority == "" {
		priority = PriorityP2
	}
	if !isPriority(priority) {
		return nil, fmt.Errorf("gsd: invalid priority %q", priority)
	}

	content, err := readFile(roadmapPath(root))
	if err != nil {
		return nil, fmt.Errorf("gsd: read roadmap: %w", err)
	}

	phases := parsePhases(content)

	insertIdx := -1
	for i, p := range phases {
		if p.ID == afterID {
			insertIdx = i + 1
			break
		}
	}
	if insertIdx < 0 {
		return nil, fmt.Errorf("gsd: phase %q not found", afterID)
	}

	decimalID := computeInsertID(afterID, phases)

	phase := Phase{
		ID:        decimalID,
		Title:     title,
		Priority:  priority,
		Status:    StatusPlanning,
		CreatedAt: time.Now().UTC(),
	}

	phases = append(phases[:insertIdx], append([]Phase{phase}, phases[insertIdx:]...)...)

	if err := writeFile(roadmapPath(root), serializePhases(phases)); err != nil {
		return nil, fmt.Errorf("gsd: write roadmap: %w", err)
	}

	return &phase, nil
}

func computeInsertID(afterID string, phases []Phase) string {
	base, err := strconv.ParseFloat(afterID, 64)
	if err != nil {
		return afterID + ".5"
	}

	maxFrac := base
	for _, p := range phases {
		f, err := strconv.ParseFloat(p.ID, 64)
		if err != nil {
			continue
		}
		intPart := float64(int(f))
		if intPart == float64(int(base)) && f > maxFrac {
			maxFrac = f
		}
	}

	increment := 0.5
	newID := base + increment
	for {
		_, err := strconv.ParseFloat(fmt.Sprintf("%g", newID), 64)
		if err != nil {
			break
		}
		exists := false
		for _, p := range phases {
			if p.ID == fmt.Sprintf("%g", newID) {
				exists = true
				break
			}
		}
		if !exists {
			break
		}
		newID += increment
		increment /= 2
	}

	return fmt.Sprintf("%g", newID)
}

func RemovePhase(root, id string) error {
	content, err := readFile(roadmapPath(root))
	if err != nil {
		return fmt.Errorf("gsd: read roadmap: %w", err)
	}

	phases := parsePhases(content)

	found := false
	var filtered []Phase
	for _, p := range phases {
		if p.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, p)
	}
	if !found {
		return fmt.Errorf("gsd: phase %q not found", id)
	}

	renumbered := renumberPhases(filtered)

	return writeFile(roadmapPath(root), serializePhases(renumbered))
}

func renumberPhases(phases []Phase) []Phase {
	counter := 1
	for i := range phases {
		if _, err := strconv.Atoi(phases[i].ID); err == nil {
			phases[i].ID = strconv.Itoa(counter)
			counter++
		}
	}
	return phases
}

func EditPhase(root, id string, opts EditOpts) error {
	content, err := readFile(roadmapPath(root))
	if err != nil {
		return fmt.Errorf("gsd: read roadmap: %w", err)
	}

	phases := parsePhases(content)

	found := false
	for i, p := range phases {
		if p.ID == id {
			found = true
			if opts.Title != "" {
				phases[i].Title = opts.Title
			}
			if opts.Priority != "" {
				if !isPriority(opts.Priority) {
					return fmt.Errorf("gsd: invalid priority %q", opts.Priority)
				}
				phases[i].Priority = opts.Priority
			}
			if opts.Status != "" {
				if !isStatus(opts.Status) {
					return fmt.Errorf("gsd: invalid status %q", opts.Status)
				}
				phases[i].Status = opts.Status
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("gsd: phase %q not found", id)
	}

	return writeFile(roadmapPath(root), serializePhases(phases))
}

type EditOpts struct {
	Title    string
	Priority string
	Status   string
}

func ListPhases(root string) ([]Phase, error) {
	content, err := readFile(roadmapPath(root))
	if err != nil {
		return nil, fmt.Errorf("gsd: read roadmap: %w", err)
	}
	return parsePhases(content), nil
}

func GetPhase(root, id string) (*Phase, error) {
	phases, err := ListPhases(root)
	if err != nil {
		return nil, err
	}
	for i := range phases {
		if phases[i].ID == id {
			return &phases[i], nil
		}
	}
	return nil, fmt.Errorf("gsd: phase %q not found", id)
}
