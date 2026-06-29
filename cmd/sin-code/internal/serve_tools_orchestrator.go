// SPDX-License-Identifier: MIT
// Purpose: serve — tool definitions for orchestrator, agent, and LSP tools.
// sin-debt: shrink, upgrade: consolidate when serve handlers are refactored
package internal

// orchestratorToolDefs returns the tool definitions for orchestrator_*,
// agent_*, and lsp_servers tools: orchestrator_run, orchestrator_plan,
// orchestrator_agents, agent_show, agent_set, agent_doctor, lsp_servers.
func orchestratorToolDefs() []toolDef {
	return []toolDef{
		{
			name:        "sin_orchestrator_run",
			description: "Run a prompt through the multi-agent orchestrator (Pre-LLM router → planner → parallel agents)",
			handler:     handleOrchestratorRun,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"prompt"},
				"properties": map[string]any{
					"prompt":       map[string]any{"type": "string"},
					"timeout":      map[string]any{"type": "string", "default": "2m"},
					"max_parallel": map[string]any{"type": "integer", "default": 4},
				},
			},
		},
		{
			name:        "sin_orchestrator_plan",
			description: "Build a plan from a prompt (no execution) — previews sub-tasks and agents",
			handler:     handleOrchestratorPlan,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"prompt"},
				"properties": map[string]any{
					"prompt": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_orchestrator_agents",
			description: "List all available agents (default + user-defined) with their config",
			handler:     handleOrchestratorAgents,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_agent_show",
			description: "Show effective config for a single agent (merged defaults + user overrides)",
			handler:     handleAgentShow,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"name"},
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_agent_set",
			description: "Set fields on a user agent (programmatic edit of agent.toml)",
			handler:     handleAgentSet,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"name", "kvs"},
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"kvs":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
		},
		{
			name:        "sin_agent_doctor",
			description: "Validate agents (model exists on provider, API key present, base URL reachable)",
			handler:     handleAgentDoctor,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string"},
					"offline": map[string]any{"type": "boolean", "default": false},
				},
			},
		},
		{
			name:        "sin_lsp_servers",
			description: "List detected LSP servers on PATH (gopls, pyright, tsserver, rust-analyzer)",
			handler:     handleLspServers,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}
