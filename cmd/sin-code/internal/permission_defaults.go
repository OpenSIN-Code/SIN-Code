// SPDX-License-Identifier: MIT
// Purpose: default allow/ask/deny rules for the agent loop (issue #47) and
// the exported bridge so cmd/sin-code (package main) can load agent
// profiles and seed permission rules from ToolsAllow/ToolsDeny.
package internal

import (
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
)

func DefaultPermissionRules() []permission.Rule {
	return []permission.Rule{
		{Tool: "sin_read", Policy: "allow"},
		{Tool: "sin_write", Policy: "allow"},
		{Tool: "sin_edit", Policy: "allow"},
		{Tool: "sckg_*", Policy: "allow"},
		{Tool: "oracle_*", Policy: "allow"},
		{Tool: "poc_*", Policy: "allow"},
		// External MCP servers (qualified "server__tool" names).
		// Read-only / analysis servers run free; action-capable ask.
		{Tool: "websearch__*", Policy: "allow"},
		{Tool: "contextbridge__*", Policy: "allow"},
		{Tool: "simone__*", Policy: "allow"},
		{Tool: "symfonylens__*", Policy: "allow"},
		{Tool: "codocs__*", Policy: "ask"},
		{Tool: "frontend__*", Policy: "ask"},
		{Tool: "goalmode__*", Policy: "ask"},
		{Tool: "grillme__*", Policy: "ask"},
		{Tool: "marketplace__*", Policy: "ask"},
		{Tool: "mcpbuilder__*", Policy: "ask"},
		{Tool: "scheduler__*", Policy: "ask"},
		{Tool: "browser__*", Policy: "ask"},
		{Tool: "honcho__*", Policy: "ask"},
		{Tool: "sin_bootstrap_skill", Policy: "ask"}, // v3.6.0: self-extending meta-tool (issue #51)
		// v3.8.0: stack layer integrations (Bridged-External + stdio MCP).
		{Tool: "vane__*", Policy: "allow"},      // citation-backed research
		{Tool: "superpowers__*", Policy: "allow"}, // already local, just register
		{Tool: "dox__*", Policy: "allow"},        // protocol check
		{Tool: "gh_query", Policy: "allow"},   // v3.9.0: read-only by code-level cross-check
		{Tool: "gh_health", Policy: "allow"},  // v3.9.0: binary presence + auth check
		{Tool: "gh_execute", Policy: "ask"},    // v3.9.0: mutating (issue create, pr merge, ...)
		{Tool: "sin_bash", Policy: "ask"},
		{Tool: "sin_sbom_generate", Policy: "allow"},
		{Tool: "sin_security_scan", Policy: "allow"},
		{Tool: "*", Policy: "ask"},
	}
}

func RulesForAgent(cfg orchestrator.AgentConfig) []permission.Rule {
	rules := make([]permission.Rule, 0, len(cfg.ToolsDeny)+len(cfg.ToolsAllow)+10)
	for _, t := range cfg.ToolsDeny {
		rules = append(rules, permission.Rule{Tool: t, Policy: "deny"})
	}
	for _, t := range cfg.ToolsAllow {
		rules = append(rules, permission.Rule{Tool: t, Policy: "allow"})
	}
	return append(rules, DefaultPermissionRules()...)
}

func LoadEffectiveAgent(name string) (orchestrator.AgentConfig, string, error) {
	return loadEffectiveAgent(name)
}
