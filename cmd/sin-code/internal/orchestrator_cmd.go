// SPDX-License-Identifier: MIT
// Purpose: sin-code orchestrate — multi-agent orchestrator CLI (v2).
// SOTA June 2026: Pre-LLM router + planner + parallel specialized agents
// with per-agent model + system prompt. Backed by the orchestrator package.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/plugins"
)

var (
	orch2Prompt       string
	orch2Format       string
	orch2Timeout      time.Duration
	orch2MaxParallel  int
	orch2AgentsDir    string
	orch2PlanOnly     bool
	orch2ShowScratch  bool
	orch2NoPlugins    bool
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
					"name":         c.Name,
					"type":         c.Type,
					"model":        c.Model,
					"tools_allow":  c.ToolsAllow,
					"description":  c.Description,
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
