// SPDX-License-Identifier: MIT
package gsd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func gsdDir(root string) string {
	return filepath.Join(root, ".gsd")
}

func plansDir(root string) string {
	return filepath.Join(gsdDir(root), "plans")
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func writeFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func projectPath(root string) string {
	return filepath.Join(gsdDir(root), "PROJECT.md")
}

func roadmapPath(root string) string {
	return filepath.Join(gsdDir(root), "ROADMAP.md")
}

func statePath(root string) string {
	return filepath.Join(gsdDir(root), "STATE.md")
}

func planPath(root, phaseID string) string {
	return filepath.Join(plansDir(root), "phase-"+phaseID+".md")
}

func parsePhases(content string) []Phase {
	var phases []Phase
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "## Phase ") {
			continue
		}
		rest := strings.TrimPrefix(line, "## Phase ")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) < 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		remainder := strings.TrimSpace(parts[1])

		phase := Phase{
			ID:        id,
			Status:    StatusPlanning,
			Priority:  PriorityP2,
			CreatedAt: time.Now().UTC(),
		}

		titleEnd := len(remainder)
		for i, ch := range remainder {
			if ch == '[' {
				titleEnd = i
				break
			}
		}
		phase.Title = strings.TrimSpace(remainder[:titleEnd])

		bracketPart := remainder[titleEnd:]
		start := 0
		for start < len(bracketPart) {
			openIdx := strings.Index(bracketPart[start:], "[")
			if openIdx < 0 {
				break
			}
			openIdx += start
			closeIdx := strings.Index(bracketPart[openIdx:], "]")
			if closeIdx < 0 {
				break
			}
			tag := bracketPart[openIdx+1 : openIdx+closeIdx]
			if isPriority(tag) {
				phase.Priority = tag
			} else if isStatus(tag) {
				phase.Status = tag
			}
			start = openIdx + closeIdx + 1
		}

		phases = append(phases, phase)
	}
	return phases
}

func isPriority(s string) bool {
	return s == PriorityP0 || s == PriorityP1 || s == PriorityP2 || s == PriorityP3
}

func isStatus(s string) bool {
	return s == StatusPlanning || s == StatusInProgress || s == StatusCompleted || s == StatusPhaseBlocked
}

func serializePhases(phases []Phase) string {
	var b strings.Builder
	b.WriteString("# Roadmap\n\n")
	for _, p := range phases {
		b.WriteString(fmt.Sprintf("## Phase %s: %s [%s] [%s]\n", p.ID, p.Title, p.Priority, p.Status))
	}
	return b.String()
}

func nextPhaseID(phases []Phase) string {
	maxInt := 0
	for _, p := range phases {
		n, err := strconv.Atoi(p.ID)
		if err == nil && n > maxInt {
			maxInt = n
		}
	}
	return strconv.Itoa(maxInt + 1)
}

func parsePlanTasks(content string) []Task {
	var tasks []Task
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		if len(line) < 6 {
			continue
		}
		marker := line[3]
		taskBody := strings.TrimSpace(line[6:])

		var status string
		if marker == 'x' || marker == 'X' {
			status = TaskStatusDone
		} else {
			status = TaskStatusTodo
		}

		colonIdx := strings.Index(taskBody, ":")
		if colonIdx < 0 {
			continue
		}
		taskID := strings.TrimSpace(taskBody[:colonIdx])
		remainder := strings.TrimSpace(taskBody[colonIdx+1:])

		task := Task{
			ID:     taskID,
			Status: status,
			Effort: EffortMedium,
		}

		parenIdx := strings.LastIndex(remainder, "(")
		if parenIdx >= 0 {
			task.Description = strings.TrimSpace(remainder[:parenIdx])
			meta := strings.TrimSuffix(strings.TrimSpace(remainder[parenIdx:]), ")")
			meta = strings.TrimPrefix(meta, "(")
			task.Effort = extractField(meta, "effort:")
			task.Validation = extractField(meta, "validation:")
			depsStr := extractField(meta, "deps:")
			if depsStr != "" {
				depsStr = strings.TrimPrefix(depsStr, "[")
				depsStr = strings.TrimSuffix(depsStr, "]")
				if depsStr != "" {
					for _, d := range strings.Split(depsStr, ",") {
						d = strings.TrimSpace(d)
						if d != "" {
							task.Dependencies = append(task.Dependencies, d)
						}
					}
				}
			}
		} else {
			task.Description = remainder
		}

		tasks = append(tasks, task)
	}
	return tasks
}

func extractField(meta, prefix string) string {
	idx := strings.Index(meta, prefix)
	if idx < 0 {
		return ""
	}
	rest := meta[idx+len(prefix):]
	rest = strings.TrimSpace(rest)
	if endIdx := strings.Index(rest, ","); endIdx >= 0 {
		return strings.TrimSpace(rest[:endIdx])
	}
	return strings.TrimSpace(rest)
}

func serializePlanTasks(tasks []Task) string {
	var b strings.Builder
	b.WriteString("## Tasks\n")
	for _, t := range tasks {
		marker := " "
		if t.Status == TaskStatusDone {
			marker = "x"
		}
		deps := "[]"
		if len(t.Dependencies) > 0 {
			deps = "[" + strings.Join(t.Dependencies, ", ") + "]"
		}
		effort := t.Effort
		if effort == "" {
			effort = EffortMedium
		}
		validation := t.Validation
		if validation == "" {
			validation = "N/A"
		}
		b.WriteString(fmt.Sprintf("- [%s] %s: %s (effort: %s, deps: %s, validation: %s)\n",
			marker, t.ID, t.Description, effort, deps, validation))
	}
	return b.String()
}
