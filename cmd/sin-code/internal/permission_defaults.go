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
		{Tool: "share__*", Policy: "ask"},            // v3.17.0: SIN-Code-Share-Skill (registry.go)
		{Tool: "skills__*", Policy: "ask"},           // v3.17.0: SIN-Code-Skills-Skill (registry.go)
		{Tool: "sin_bootstrap_skill", Policy: "ask"}, // v3.6.0: self-extending meta-tool (issue #51)
		// v3.8.0: stack layer integrations (Bridged-External + stdio MCP).
		{Tool: "vane__*", Policy: "allow"},        // citation-backed research
		{Tool: "superpowers__*", Policy: "allow"}, // already local, just register
		{Tool: "dox__*", Policy: "allow"},         // protocol check
		{Tool: "gh_query", Policy: "allow"},       // v3.9.0: read-only by code-level cross-check
		{Tool: "gh_health", Policy: "allow"},      // v3.9.0: binary presence + auth check
		{Tool: "gh_execute", Policy: "ask"},       // v3.9.0: mutating (issue create, pr merge, ...)
		{Tool: "sin_bash", Policy: "ask"},
		{Tool: "sin_sbom_generate", Policy: "allow"},
		{Tool: "sin_security_scan", Policy: "allow"},
		// v3.18+: Browser CDP tools.
		// sin_browser_navigate drives headless Chrome — it can load arbitrary
		// URLs so it requires explicit user confirmation (ask).
		// sin_browser_findings and sin_browser_snapshot only read the already-
		// captured in-memory event slice; no network calls, no side effects.
		{Tool: "sin_browser_navigate", Policy: "ask"},
		{Tool: "sin_browser_findings", Policy: "allow"},
		{Tool: "sin_browser_snapshot", Policy: "allow"},
		{Tool: "sin_browser_vitals_flush", Policy: "allow"},
		{Tool: "sin_browser_diff", Policy: "allow"},

		// v3.16.0: autodev-cli bridge (Bridged-External + autodev-mcp stdio MCP).
		// Qualified name = server-name + "__" + tool-name (registry.go "autodev" + autodev-mcp tools).
		// Split mirrors the gh precedent at lines 40-42: read-only -> allow, mutating -> ask (M4).
		{Tool: "autodev__status", Policy: "allow"},
		{Tool: "autodev__lessons", Policy: "allow"},
		{Tool: "autodev__init", Policy: "ask"},
		{Tool: "autodev__run_experiment", Policy: "ask"},
		{Tool: "autodev__swarm", Policy: "ask"},
		{Tool: "autodev__session_log", Policy: "ask"},
		// v3.18.0: Eval & Observability (issue #75). eval/trace are
		// first-party CLI verbs that orchestrate other tools but do
		// not call them as MCP functions; the policy below covers
		// any future agent-loop surface that exposes them as tools.
		{Tool: "eval__list", Policy: "allow"},
		{Tool: "eval__run", Policy: "ask"}, // may invoke real verification + LLM
		{Tool: "trace__doctor", Policy: "allow"},
		// v3.18.0: issue #170 single-binary installer. The agent
		// loop invokes install via MCP when a fresh machine needs a
		// working sin-code. The install flow downloads + writes under
		// the user's $HOME/.local/bin and is non-privileged — but the
		// permission tier is "ask" so headless mode (incl. the
		// daemon) refuses to run it without explicit approval. Only
		// --verify-only is silent, since it is read-only.
		{Tool: "install__verify_only", Policy: "allow"},
		{Tool: "install__run", Policy: "ask"},
		{Tool: "install__dry_run", Policy: "allow"},
		// v3.18.0: Compress (issue #172). compress is a first-party
		// CLI verb that mutates the on-disk memory stores; the
		// policy below mirrors eval__run (ask): a destructive
		// operation gated on user confirmation.
		{Tool: "compress__plan", Policy: "allow"},     // read-only projection
		{Tool: "compress__apply", Policy: "ask"},      // rewrites + LLM call
		{Tool: "compress__rollback", Policy: "allow"}, // restorative — never destructive
		// v3.18.0: profile renderer (issue #175). Read-only
		// (show/list/verify/dry-run) is allow; the writing `render`
		// surface is `ask` because it touches per-agent dotdirs.
		{Tool: "profile__show", Policy: "allow"},
		{Tool: "profile__list", Policy: "allow"},
		{Tool: "profile__verify", Policy: "allow"},
		{Tool: "profile__render", Policy: "ask"},
		// v3.18.0: sin-debt marker manager (issue #177).
		// Read-only scanners are allow; check is ask because failing
		// the gate is a visible signal; fix/export are ask because they
		// either instruct humans to edit code or write to disk.
		{Tool: "sindept__list", Policy: "allow"},
		{Tool: "sindept__stats", Policy: "allow"},
		{Tool: "sindept__policy", Policy: "allow"},
		{Tool: "sindept__check", Policy: "ask"},
		{Tool: "sindept__fix", Policy: "ask"},
		{Tool: "sindept__export", Policy: "ask"},
		// Backstop catch-all (mirrors sin_bash default at line 44 for unmatched prefixes).
		{Tool: "autodev__*", Policy: "ask"},
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
