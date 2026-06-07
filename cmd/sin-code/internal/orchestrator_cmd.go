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

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
)

var (
	orch2Prompt       string
	orch2Format       string
	orch2Timeout      time.Duration
	orch2MaxParallel  int
	orch2AgentsDir    string
	orch2PlanOnly     bool
	orch2ShowScratch  bool
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
	Short: "List available agents (default + user-defined)",
	RunE: func(cmd *cobra.Command, args []string) error {
		extra, err := orchestrator.LoadUserAgents(orch2AgentsDir)
		if err != nil {
			return err
		}
		o := orchestrator.NewWithAgents(extra)
		if orch2Format == "json" {
			return json.NewEncoder(os.Stdout).Encode(o.Registry.List())
		}
		fmt.Printf("Loaded %d agents:\n\n", len(o.Registry.List()))
		for _, c := range o.Registry.List() {
			fmt.Printf("  %-12s type=%-10s model=%-32s tools=%d\n",
				c.Name, c.Type, c.Model, len(c.ToolsAllow))
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
		extra, err := orchestrator.LoadUserAgents(orch2AgentsDir)
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

func init() {
	OrchestratorRunCmd.Flags().StringVar(&orch2Format, "format", "text", "Output format: text|json")
	OrchestratorRunCmd.Flags().DurationVar(&orch2Timeout, "timeout", 5*time.Minute, "Max execution time")
	OrchestratorRunCmd.Flags().IntVar(&orch2MaxParallel, "max-parallel", 4, "Max parallel agents")
	OrchestratorRunCmd.Flags().StringVar(&orch2AgentsDir, "agents-dir", "", "User agents dir (default ~/.config/sin-code/agents)")
	OrchestratorRunCmd.Flags().BoolVar(&orch2PlanOnly, "plan-only", false, "Build plan and exit, no execution")
	OrchestratorRunCmd.Flags().BoolVar(&orch2ShowScratch, "show-scratch", false, "Print shared scratchpad after dispatch")

	OrchestratorAgentsCmd.Flags().StringVar(&orch2Format, "format", "text", "Output format: text|json")
	OrchestratorAgentsCmd.Flags().StringVar(&orch2AgentsDir, "agents-dir", "", "User agents dir (default ~/.config/sin-code/agents)")

	OrchestratorPlanCmd.Flags().StringVar(&orch2Format, "format", "text", "Output format: text|json")
	OrchestratorPlanCmd.Flags().StringVar(&orch2AgentsDir, "agents-dir", "", "User agents dir (default ~/.config/sin-code/agents)")
}

func runOrchestrator() error {
	extra, err := orchestrator.LoadUserAgents(orch2AgentsDir)
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
