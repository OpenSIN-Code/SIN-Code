// SPDX-License-Identifier: MIT
// Purpose: sin-code orchestrate — multi-agent orchestrator CLI (v2).
// SOTA June 2026: Pre-LLM router + planner + parallel specialized agents
// with per-agent model + system prompt. Backed by the orchestrator package.
// Also contains: legacy task manager (merged from orchestrate.go) with
// dependencies, parallel execution plans, blocker detection, and rollback.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/plugins"
)

var (
	orch2Prompt      string
	orch2Format      string
	orch2Timeout     time.Duration
	orch2MaxParallel int
	orch2AgentsDir   string
	orch2PlanOnly    bool
	orch2ShowScratch bool
	orch2NoPlugins   bool
)

var OrchestratorRunCmd = &cobra.Command{
	Use:   "orchestrator-run <prompt>",
	Short: "Run a prompt through the multi-agent orchestrator (Pre-LLM router → planner → parallel agents)",
	Long: `orchestrate-run is the v2 SOTA orchestrator. It:
  1. Routes the prompt via cheap keyword-based intent classification (Pre-LLM)
  2. Decomposes it into ordered sub-tasks, each bound to a specialized agent
  3. Dispatches the tasks in parallel (respecting dependencies)
  4. Each agent runs with its own model, system prompt, and tool whitelist
  5. Results merge into a shared scratchpad
  6. Final aggregation produces the response

Default agents: coder, tester, reviewer, docs, security, architect.
User agents can be added to ~/.config/sin-code/agents/{name}/agent.toml
Plugin agents are auto-loaded from ~/.local/share/sin-code/plugins/<name>/

Examples:
  sin-code orchestrate-run "Add user authentication with OAuth2"
  sin-code orchestrate-run "Refactor the billing module" --plan-only
  sin-code orchestrate-run "Write docs for the API" --format json --show-scratch`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		orch2Prompt = args[0]
		return runOrchestrator()
	},
}

var OrchestratorAgentsCmd = &cobra.Command{
	Use:   "orchestrator-agents",
	Short: "List available agents (default + user-defined + plugin)",
	RunE: func(cmd *cobra.Command, args []string) error {
		extra, err := loadAllAgents()
		if err != nil {
			return err
		}
		o := orchestrator.NewWithAgents(extra)
		registry := o.Registry
		all := registry.List()

		// Build plugin name lookup for [plugin X] tagging.
		pluginAgent := map[string]string{}
		if !orch2NoPlugins {
			pr := plugins.NewRegistry()
			_ = pr.LoadFromDir("")
			for _, p := range pr.List() {
				for _, a := range p.Agents {
					pluginAgent["plugin-"+p.Name+"-"+a.Name] = p.Name
				}
			}
		}

		if orch2Format == "json" {
			out := make([]map[string]any, 0, len(all))
			for _, c := range all {
				entry := map[string]any{
					"name":        c.Name,
					"type":        c.Type,
					"model":       c.Model,
					"tools_allow": c.ToolsAllow,
					"description": c.Description,
				}
				if src, ok := pluginAgent[c.Name]; ok {
					entry["source"] = "plugin"
					entry["plugin"] = src
				} else {
					entry["source"] = "default-or-user"
				}
				out = append(out, entry)
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		}
		fmt.Printf("Loaded %d agents:\n\n", len(all))
		for _, c := range all {
			prefix := ""
			if src, ok := pluginAgent[c.Name]; ok {
				prefix = fmt.Sprintf("[plugin %s] ", src)
			}
			fmt.Printf("  %s%-12s type=%-10s model=%-32s tools=%d\n",
				prefix, c.Name, c.Type, c.Model, len(c.ToolsAllow))
			if c.Description != "" {
				fmt.Printf("      %s\n", c.Description)
			}
		}
		return nil
	},
}

var OrchestratorPlanCmd = &cobra.Command{
	Use:   "orchestrator-plan <prompt>",
	Short: "Build a plan from a prompt (no execution)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extra, err := loadAllAgents()
		if err != nil {
			return err
		}
		o := orchestrator.NewWithAgents(extra)
		plan := o.Plan(args[0])
		if orch2Format == "json" {
			return json.NewEncoder(os.Stdout).Encode(plan)
		}
		fmt.Printf("Plan %s (intent=%s, %d tasks):\n\n", plan.ID, plan.Intent, len(plan.Tasks))
		for i, t := range plan.Tasks {
			deps := ""
			if len(t.DependsOn) > 0 {
				deps = fmt.Sprintf(" deps=[%s]", joinIDs(t.DependsOn))
			}
			fmt.Printf("  %d. [%s] agent=%-10s%s\n     %s\n",
				i+1, t.Type, t.AgentName, deps, t.Description)
		}
		return nil
	},
}

// loadAllAgents merges user agents (from --agents-dir) with plugin agents
// (from the global plugin dir). Plugin agents are tagged with
// "plugin-<plugin>-<agent>" so the registry can disambiguate from defaults.
func loadAllAgents() ([]orchestrator.AgentConfig, error) {
	user, err := orchestrator.LoadUserAgents(orch2AgentsDir)
	if err != nil {
		return nil, err
	}
	if orch2NoPlugins {
		return user, nil
	}
	pr := plugins.NewRegistry()
	_ = pr.LoadFromDir("")
	pluginAgents := pr.AgentConfigs()
	if len(pluginAgents) == 0 {
		return user, nil
	}
	out := make([]orchestrator.AgentConfig, 0, len(user)+len(pluginAgents))
	out = append(out, user...)
	out = append(out, pluginAgents...)
	return out, nil
}

func init() {
	OrchestratorRunCmd.Flags().StringVar(&orch2Format, "format", "text", "Output format: text|json")
	OrchestratorRunCmd.Flags().DurationVar(&orch2Timeout, "timeout", 2*time.Minute, "Max execution time")
	OrchestratorRunCmd.Flags().IntVar(&orch2MaxParallel, "max-parallel", 4, "Max parallel agents")
	OrchestratorRunCmd.Flags().StringVar(&orch2AgentsDir, "agents-dir", "", "User agents dir (default ~/.config/sin-code/agents)")
	OrchestratorRunCmd.Flags().BoolVar(&orch2PlanOnly, "plan-only", false, "Build plan and exit, no execution")
	OrchestratorRunCmd.Flags().BoolVar(&orch2ShowScratch, "show-scratch", false, "Print shared scratchpad after dispatch")
	OrchestratorRunCmd.Flags().BoolVar(&orch2NoPlugins, "no-plugins", false, "Skip loading plugin agents")

	OrchestratorAgentsCmd.Flags().StringVar(&orch2Format, "format", "text", "Output format: text|json")
	OrchestratorAgentsCmd.Flags().StringVar(&orch2AgentsDir, "agents-dir", "", "User agents dir (default ~/.config/sin-code/agents)")
	OrchestratorAgentsCmd.Flags().BoolVar(&orch2NoPlugins, "no-plugins", false, "Skip loading plugin agents")

	OrchestratorPlanCmd.Flags().StringVar(&orch2Format, "format", "text", "Output format: text|json")
	OrchestratorPlanCmd.Flags().StringVar(&orch2AgentsDir, "agents-dir", "", "User agents dir (default ~/.config/sin-code/agents)")
	OrchestratorPlanCmd.Flags().BoolVar(&orch2NoPlugins, "no-plugins", false, "Skip loading plugin agents")
}

func runOrchestrator() error {
	extra, err := loadAllAgents()
	if err != nil {
		return err
	}
	o := orchestrator.NewWithAgents(extra)
	plan := o.Plan(orch2Prompt)
	if orch2PlanOnly {
		if orch2Format == "json" {
			return json.NewEncoder(os.Stdout).Encode(plan)
		}
		fmt.Printf("Plan %s (intent=%s, %d tasks):\n\n", plan.ID, plan.Intent, len(plan.Tasks))
		for i, t := range plan.Tasks {
			deps := ""
			if len(t.DependsOn) > 0 {
				deps = fmt.Sprintf(" deps=[%s]", joinIDs(t.DependsOn))
			}
			fmt.Printf("  %d. [%s] agent=%-10s%s\n     %s\n",
				i+1, t.Type, t.AgentName, deps, t.Description)
		}
		return nil
	}
	ctx := context.Background()
	opts := []orchestrator.RunOption{
		orchestrator.WithTimeout(orch2Timeout),
		orchestrator.WithMaxParallel(orch2MaxParallel),
	}
	res, err := o.Run(ctx, orch2Prompt, opts...)
	if err != nil {
		return err
	}
	if orch2Format == "json" {
		out := map[string]interface{}{
			"plan":   res.Plan,
			"result": res,
		}
		if orch2ShowScratch {
			out["scratchpad"] = o.Scratchpad.ReadAll()
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	fmt.Println(res.Summary)
	if orch2ShowScratch {
		fmt.Println("\n--- Scratchpad ---")
		for k, v := range o.Scratchpad.ReadAll() {
			fmt.Printf("[%s v%d by %s] %s\n", k, v.Version, v.Agent, v.Content)
		}
	}
	return nil
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += id
	}
	return out
}

// Legacy task manager (merged from orchestrate.go) — task management with
// dependencies, parallel execution plans, blocker detection, and rollback
// plans. Built-in Go implementation with JSON file storage in
// ~/.local/state/sin-code/.

var (
	orchAction string
	orchTitle  string
	orchTags   string
	orchID     string
	orchFormat string

	// jsonMarshalIndent is swapped out in tests to exercise the
	// unreachable JSON-marshal error path without touching real state files.
	jsonMarshalIndent = json.MarshalIndent
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
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	Tags         []string `json:"tags"`
	Status       string   `json:"status"`
	Created      string   `json:"created"`
	Updated      string   `json:"updated"`
	Dependencies []int    `json:"dependencies,omitempty"`
	Blocked      bool     `json:"blocked"`
	Blockers     []string `json:"blockers,omitempty"`
	Rollback     string   `json:"rollback,omitempty"`
}

type orchestrateState struct {
	Tasks   []task `json:"tasks"`
	NextID  int    `json:"next_id"`
	Version int    `json:"version"`
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
	data, err := jsonMarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, filemode.Default())
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
