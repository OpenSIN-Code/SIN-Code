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
		{Tool: "sin_write", Policy: "ask"},
		{Tool: "sin_edit", Policy: "ask"},
		{Tool: "sin_search", Policy: "allow"},     // read-only file search
		{Tool: "sin_replace", Policy: "ask"},       // v3.23.0: naive string replacement (issue #373) — destructive
		{Tool: "sin_apply_diff", Policy: "ask"},    // v3.23.0: unified diff editor (issue #365) — destructive
		{Tool: "sin_generate_diff", Policy: "allow"}, // v3.23.0: diff generator (issue #365) — read-only
		{Tool: "sin_test", Policy: "allow"},
		{Tool: "sin_quality_gate", Policy: "allow"}, // v3.21.0: Test-First Verify-Loop (RFC-test-automation)
		{Tool: "sin_mutation", Policy: "allow"},
		{Tool: "sin_fuzz", Policy: "allow"},
		{Tool: "sin_property", Policy: "allow"},
		{Tool: "sin_http_get", Policy: "allow"},    // read-only HTTP fetch
		{Tool: "sin_web_search", Policy: "allow"},  // read-only web search (DuckDuckGo free + optional Tavily/SerpAPI/Brave)
		{Tool: "sckg_*", Policy: "allow"},
		{Tool: "oracle_*", Policy: "allow"},
		{Tool: "poc_*", Policy: "allow"},
		// External MCP servers (qualified "server__tool" names).
		// Read-only / analysis servers run free; action-capable ask.
		{Tool: "websearch__*", Policy: "allow"},
		{Tool: "youtube__search", Policy: "allow"},           // read-only search
		{Tool: "youtube__get_transcript", Policy: "allow"},    // read-only transcript
		{Tool: "youtube__get_video_info", Policy: "allow"},    // read-only metadata
		{Tool: "youtube__get_channel_videos", Policy: "allow"}, // read-only
		{Tool: "youtube__get_channel_info", Policy: "allow"},   // read-only
		{Tool: "youtube__get_playlist", Policy: "allow"},       // read-only
		{Tool: "youtube__download", Policy: "ask"},             // downloads files (M4)
		{Tool: "youtube__clip", Policy: "ask"},                 // downloads + cuts (M4)
		{Tool: "youtube__highlight_reel", Policy: "ask"},       // merges files (M4)
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
		{Tool: "spawn_subgoal", Policy: "ask"},       // v3.x: spawns sub-agents, costs resources (M4)
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
		// v3.22.0 (issue #382): native_browser facade — split policy mirrors
		// gh_query/gh_execute (lines 47-49). Read-only verbs (no mutation of
		// either the local session or the remote site) run free; mutating
		// verbs are gated on user confirmation (M4). Click / Fill / Submit
		// can change server-side state via form posts in the future driver
		// implementations, so they stay at "ask" even though the current
		// HTTP-direct driver stubs them with ErrUnsupported.
		{Tool: "native_browser__navigate", Policy: "allow"},
		{Tool: "native_browser__snapshot", Policy: "allow"},
		{Tool: "native_browser__screenshot", Policy: "allow"},
		{Tool: "native_browser__wait_for", Policy: "allow"},
		{Tool: "native_browser__click", Policy: "ask"},
		{Tool: "native_browser__fill", Policy: "ask"},
		{Tool: "native_browser__submit", Policy: "ask"},
		{Tool: "native_browser__*", Policy: "ask"}, // fallback for future native_browser tools

		// v3.22.0: sin-analyse-suite — read-only multimodal preprocessing (image, video, PDF, logs, data, audio).
		// All analyse__* tools are read-only — they never modify input files.
		{Tool: "analyse__*", Policy: "allow"},

		// v3.27.0: vibe-notion MCP bridge — full Notion access (Bridged-External).
		// Read tools (notion_read_*) are read-only: workspaces, pages, databases, blocks, comments, users.
		// Write tools (notion_write_*) mutate Notion: create/update/archive pages, append/delete blocks, create comments.
		// Raw CLI escape hatch is ask (can run arbitrary subcommands).
		{Tool: "notion__notion_read_auth_status", Policy: "allow"},
		{Tool: "notion__notion_read_workspaces", Policy: "allow"},
		{Tool: "notion__notion_read_resolve", Policy: "allow"},
		{Tool: "notion__notion_read_search", Policy: "allow"},
		{Tool: "notion__notion_read_page", Policy: "allow"},
		{Tool: "notion__notion_read_database_schema", Policy: "allow"},
		{Tool: "notion__notion_read_database_rows", Policy: "allow"},
		{Tool: "notion__notion_read_block_children", Policy: "allow"},
		{Tool: "notion__notion_read_comments", Policy: "allow"},
		{Tool: "notion__notion_read_me", Policy: "allow"},
		{Tool: "notion__notion_write_create_page", Policy: "ask"},
		{Tool: "notion__notion_write_update_page", Policy: "ask"},
		{Tool: "notion__notion_write_archive_page", Policy: "ask"},
		{Tool: "notion__notion_write_append_block", Policy: "ask"},
		{Tool: "notion__notion_write_delete_block", Policy: "ask"},
		{Tool: "notion__notion_write_upload_block", Policy: "ask"},
		{Tool: "notion__notion_write_move_block", Policy: "ask"},
		{Tool: "notion__notion_write_create_comment", Policy: "ask"},
		{Tool: "notion__notion_raw_cli", Policy: "ask"},
		{Tool: "notion__*", Policy: "ask"},

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
		// v3.21.0: SIN Fusion verify-tournament (issue #290).
		// Tournament spawns N parallel loops = N× cost — needs confirmation.
		// Status/config are read-only.
		{Tool: "fusion__tournament", Policy: "ask"},
		{Tool: "fusion__oracle_tournament", Policy: "ask"},
		{Tool: "fusion__status", Policy: "allow"},
		{Tool: "fusion__config", Policy: "allow"},
		// Read-only todo MCP tools (issue #323). In headless mode (daemon),
		// "ask" resolves to "deny" — so the daemon could not read todos
		// without --yolo. Read-only tools are "allow"; destructive tools
		// (add, complete, claim, dep_add) stay at "ask" (M4).
		{Tool: "sin_todo_list", Policy: "allow"},    // read-only query
		{Tool: "sin_todo_show", Policy: "allow"},    // read-only detail
		{Tool: "sin_todo_ready", Policy: "allow"},   // read-only query
		{Tool: "sin_todo_blocked", Policy: "allow"}, // read-only query
		{Tool: "sin_todo_search", Policy: "allow"},  // read-only search
		{Tool: "sin_todo_prime", Policy: "allow"},   // read-only context
		{Tool: "sin_todo_stats", Policy: "allow"},   // read-only stats
		{Tool: "sin_todo_deps", Policy: "allow"},    // read-only dependency tree
		// Destructive todo tools — stay at "ask" (M4: mutating ops need confirmation).
		{Tool: "sin_todo_add", Policy: "ask"},
		{Tool: "sin_todo_complete", Policy: "ask"},
		{Tool: "sin_todo_claim", Policy: "ask"},
		{Tool: "sin_todo_dep_add", Policy: "ask"},
		// Backstop catch-all (mirrors sin_bash default at line 44 for unmatched prefixes).
		{Tool: "autodev__*", Policy: "ask"},
		// v3.23.0: autonomous research-report generation (issue #384).
		// research__* pays real LLM tokens per call; gate "ask" so the
		// daemon cannot self-escalate (M4). The dry_run / list / show
		// surfaces are "allow" so callers can preview a research plan
		// or inspect stored reports for free.
		{Tool: "research__dry_run", Policy: "allow"},
		{Tool: "research__list", Policy: "allow"},
		{Tool: "research__show", Policy: "allow"},
		{Tool: "research__run", Policy: "ask"},
		{Tool: "research__*", Policy: "ask"},
		{Tool: "sin_run_loop", Policy: "ask"},
		{Tool: "sin_goal_add", Policy: "ask"},      // enqueues autonomous work — operator confirmation (M4)
		{Tool: "sin_goal_list", Policy: "allow"},   // read-only goal listing
		{Tool: "sin_goal_status", Policy: "allow"}, // read-only goal status
		{Tool: "sin_goal_complete", Policy: "ask"}, // marks goal done — operator confirmation (M4)
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

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func LoadEffectiveAgent(name string) (orchestrator.AgentConfig, string, error) {
	return loadEffectiveAgent(name)
}
