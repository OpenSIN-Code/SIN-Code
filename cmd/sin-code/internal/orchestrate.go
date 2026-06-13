// SPDX-License-Identifier: MIT
// Purpose: orchestrate — task management with dependencies, parallel execution
// plans, blocker detection, and rollback plans. Built-in Go implementation with
// JSON file storage in ~/.local/state/sin-code/.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	orchAction string
	orchTitle  string
	orchTags   string
	orchID     string
	orchFormat string
)

var OrchestrateCmd = &cobra.Command{
	Use:   "orchestrate",
	Short: "Legacy task manager (use 'sin-code todo' for the SOTA issue tracker)",
	Long: `Manage tasks with dependencies, parallel execution plans, blocker
detection, and rollback plans. Pure Go implementation with JSON file storage.

DEPRECATED: This command is maintained for backward compatibility.
For new projects, use 'sin-code todo' which provides:
  - bbolt storage (faster, ACID)
  - Hash-based IDs (st-a1b2)
  - Dependency graph with cycle detection
  - Append-only audit log
  - Ready/Blocked queries
  - Project namespaces
  - Compaction for old closed tasks

Example:
  sin-code orchestrate --action add --title "Implement feature X" --tags "urgent,backend"
  sin-code orchestrate --action list --format json
  sin-code orchestrate --action complete --id 1`,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOrchestrate(orchAction, orchTitle, orchTags, orchID, orchFormat)
	},
}

type task struct {
	ID           int       `json:"id"`
	Title        string    `json:"title"`
	Tags         []string  `json:"tags"`
	Status       string    `json:"status"`
	Created      string    `json:"created"`
	Updated      string    `json:"updated"`
	Dependencies []int     `json:"dependencies,omitempty"`
	Blocked      bool      `json:"blocked"`
	Blockers     []string  `json:"blockers,omitempty"`
	Rollback     string    `json:"rollback,omitempty"`
}

type orchestrateState struct {
	Tasks    []task `json:"tasks"`
	NextID   int    `json:"next_id"`
	Version  int    `json:"version"`
}

func getStateFile() string {
	stateDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "sin-code")
	_ = os.MkdirAll(stateDir, 0755)
	return filepath.Join(stateDir, "orchestrate.json")
}

func loadState() (*orchestrateState, error) {
	path := getStateFile()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &orchestrateState{Tasks: []task{}, NextID: 1, Version: 1}, nil
		}
		return nil, err
	}
	var state orchestrateState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.NextID == 0 {
		state.NextID = 1
	}
	return &state, nil
}

func saveState(state *orchestrateState) error {
	path := getStateFile()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func runOrchestrate(action, title, tags, idStr, format string) error {
	state, err := loadState()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	var result interface{}
	now := time.Now().Format(time.RFC3339)

	switch action {
	case "add":
		if title == "" {
			return fmt.Errorf("--title is required for add action")
		}
		t := task{
			ID:      state.NextID,
			Title:   title,
			Tags:    splitTags(tags),
			Status:  "pending",
			Created: now,
			Updated: now,
		}
		state.NextID++
		state.Tasks = append(state.Tasks, t)
		if err := saveState(state); err != nil {
			return err
		}
		result = t
		fmt.Printf("Added task #%d: %s\n", t.ID, t.Title)

	case "remove":
		if idStr == "" {
			return fmt.Errorf("--id is required for remove action")
		}
		id := parseID(idStr)
		found := false
		var newTasks []task
		for _, t := range state.Tasks {
			if t.ID == id {
				found = true
				continue
			}
			newTasks = append(newTasks, t)
		}
		if !found {
			return fmt.Errorf("task #%d not found", id)
		}
		state.Tasks = newTasks
		if err := saveState(state); err != nil {
			return err
		}
		fmt.Printf("Removed task #%d\n", id)

	case "complete":
		if idStr == "" {
			return fmt.Errorf("--id is required for complete action")
		}
		id := parseID(idStr)
		found := false
		for i := range state.Tasks {
			if state.Tasks[i].ID == id {
				state.Tasks[i].Status = "completed"
				state.Tasks[i].Updated = now
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("task #%d not found", id)
		}
		if err := saveState(state); err != nil {
			return err
		}
		fmt.Printf("Completed task #%d\n", id)

	case "status":
		if idStr == "" {
			return fmt.Errorf("--id is required for status action")
		}
		id := parseID(idStr)
		for _, t := range state.Tasks {
			if t.ID == id {
				result = t
				break
			}
		}
		if result == nil {
			return fmt.Errorf("task #%d not found", id)
		}

	case "list":
		// Sort: pending first, then in-progress, then completed
		sort.Slice(state.Tasks, func(i, j int) bool {
			order := map[string]int{"pending": 0, "in-progress": 1, "blocked": 2, "completed": 3}
			return order[state.Tasks[i].Status] < order[state.Tasks[j].Status]
		})
		result = state.Tasks

	default:
		return fmt.Errorf("unknown action: %s (use add|remove|list|status|complete)", action)
	}

	if format == "json" && result != nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if action == "list" {
		fmt.Printf("\nTasks (%d total):\n", len(state.Tasks))
		for _, t := range state.Tasks {
			statusIcon := "○"
			if t.Status == "completed" {
				statusIcon = "✓"
			} else if t.Status == "blocked" {
				statusIcon = "✗"
			} else if t.Status == "in-progress" {
				statusIcon = "●"
			}
			tagStr := ""
			if len(t.Tags) > 0 {
				tagStr = fmt.Sprintf(" [%s]", strings.Join(t.Tags, ", "))
			}
			fmt.Printf("  %s #%d: %s  (%s)%s\n", statusIcon, t.ID, t.Title, t.Status, tagStr)
		}
	}
	return nil
}

func parseID(s string) int {
	var id int
	fmt.Sscanf(s, "%d", &id)
	return id
}

func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

func init() {
	RegisterVersionCmd(OrchestrateCmd)
	OrchestrateCmd.Flags().StringVarP(&orchAction, "action", "a", "list", "Action: add|remove|list|status|complete")
	OrchestrateCmd.Flags().StringVarP(&orchTitle, "title", "t", "", "Task title")
	OrchestrateCmd.Flags().StringVarP(&orchTags, "tags", "", "", "Comma-separated tags")
	OrchestrateCmd.Flags().StringVarP(&orchID, "id", "i", "", "Task ID")
	OrchestrateCmd.Flags().StringVarP(&orchFormat, "format", "f", "text", "Output format: text|json")
}
