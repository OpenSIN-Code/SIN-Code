// SPDX-License-Identifier: MIT
// Purpose: serve — tool definitions for verification, analysis, and security tools.
// sin-debt: shrink, upgrade: consolidate when serve handlers are refactored
package internal

// analysisToolDefs returns the tool definitions for verification, analysis,
// and security tools: orchestrate, ibd, poc, sckg, adw, oracle, efm,
// security_scan, sbom_generate.
func analysisToolDefs() []toolDef {
	return []toolDef{
		{
			name:        "sin_orchestrate",
			description: "Manage tasks with dependencies, parallel execution, and rollback plans",
			handler:     handleOrchestrate,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"add", "list", "status", "complete"}, "default": "list"},
					"title":  map[string]any{"type": "string"},
					"tags":   map[string]any{"type": "string"},
					"id":     map[string]any{"type": "string"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
			},
		},
		{
			name:        "sin_ibd",
			description: "Intent-Based Diffing — compare code changes against stated intent",
			handler:     handleIbd,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"before": map[string]any{"type": "string"},
					"after":  map[string]any{"type": "string"},
					"intent": map[string]any{"type": "string"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"before", "after"},
			},
		},
		{
			name:        "sin_poc",
			description: "Proof-of-Correctness — verify code satisfies its specification",
			handler:     handlePoc,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"spec":   map[string]any{"type": "string"},
					"code":   map[string]any{"type": "string"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"spec", "code"},
			},
		},
		{
			name:        "sin_sckg",
			description: "Semantic Codebase Knowledge Graphs — build & query code graph",
			handler:     handleSckg,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string"},
					"action": map[string]any{"type": "string", "enum": []string{"build", "query", "stats", "export"}, "default": "build"},
					"query":  map[string]any{"type": "string"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
			},
		},
		{
			name:        "sin_adw",
			description: "Architectural Debt Watchdogs — detect god modules, circular deps, high coupling",
			handler:     handleAdw,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string"},
					"strict": map[string]any{"type": "boolean", "default": false},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
			},
		},
		{
			name:        "sin_oracle",
			description: "Verification Oracle — independent verification of claims with evidence",
			handler:     handleOracle,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"claim":    map[string]any{"type": "string"},
					"evidence": map[string]any{"type": "string"},
					"format":   map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"claim"},
			},
		},
		{
			name:        "sin_efm",
			description: "Ephemeral Full-Stack Mocking — spin up disposable test environments",
			handler:     handleEfm,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"up", "down", "list", "status"}, "default": "list"},
					"stack":  map[string]any{"type": "string"},
					"ttl":    map[string]any{"type": "integer", "default": 3600},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
			},
		},
		{
			name:        "sin_security_scan",
			description: "Run the in-tree security subcommand on a project path. Auto-detects Go / Python / Node / generic and runs govulncheck, gosec, go vet, bandit, safety, npm audit, secrets grep, and a file-permission walker. Returns a SecurityResult JSON.",
			handler:     handleSecurity,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Project root to scan (default: .)"},
					"type":    map[string]any{"type": "string", "enum": []string{"auto", "go", "python", "node", "generic"}, "default": "auto"},
					"tools":   map[string]any{"type": "string", "description": "Comma-separated tool whitelist (e.g. govulncheck,gosec)"},
					"format":  map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
					"timeout": map[string]any{"type": "integer", "default": 300, "description": "Per-tool timeout in seconds (max 3600)"},
					"strict":  map[string]any{"type": "boolean", "default": false, "description": "Reserved; not propagated as MCP error"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_sbom_generate",
			description: "Generate a Software Bill of Materials (SPDX 2.3 JSON or CycloneDX 1.5 JSON) for a project. Auto-detects Go / Python / Node / generic.",
			handler:     handleSbom,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Project root to scan (default: .)"},
					"format": map[string]any{"type": "string", "enum": []string{"spdx-json", "cyclonedx-json"}, "default": "spdx-json"},
					"output": map[string]any{"type": "string", "description": "Write to this file path (must be inside <path>). Omit or '-' to return inline."},
				},
				"required": []string{"path"},
			},
		},
	}
}
