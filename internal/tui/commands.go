// Purpose: Command catalog for the sin tui. Mirrors `sin --help` subcommands
// so the TUI is a 1:1 discoverable interface for the CLI.
// Docs: commands.doc.md

package tui

import (
	"sort"
	"strings"
)

// Command describes one callable sin subcommand exposed in the TUI menu.
type Command struct {
	// Key is the canonical subcommand name, e.g. "sckg", "code", "doctor".
	Key string
	// Title is shown in the menu, supports emojis and short labels.
	Title string
	// Description is a one-line explanation shown beside the menu item.
	Description string
	// Group classifies the command in the menu (Code, Skills, MCP, ...).
	Group string
	// Args is the raw argument template. If non-empty, the TUI prompts for
	// a value before running. The placeholder is rendered as the prompt hint.
	Args string
	// Danger marks destructive commands (e.g. "policy reset") for red styling.
	Danger bool
}

// Group constants.
const (
	GroupCode    = "Code"
	GroupGo      = "Go Tools"
	GroupPython  = "Python Tools"
	GroupSecurity = "Security"
	GroupSkills  = "Skills"
	GroupMCP     = "MCP & Serve"
	GroupSystem  = "System"
)

// Commands is the full menu catalog. Order within a group = display order.
var Commands = []Command{
	// ── Code hub (sin code) ──────────────────────────────────────
	{Key: "code", Title: "🚀 sin code", Description: "Unified coding workflow hub", Group: GroupCode},
	{Key: "code review", Title: "🔍 review", Description: "Semantic review of a change (IBD)", Group: GroupCode, Args: "<files>"},
	{Key: "code debt", Title: "🏚️  debt", Description: "Show current architectural debt", Group: GroupCode, Args: "<path>"},
	{Key: "code verify", Title: "✅ verify", Description: "Independent execution-based verification", Group: GroupCode, Args: "<target>"},
	{Key: "code preflight", Title: "✈️  preflight", Description: "Fresh GitNexus index for coder agents", Group: GroupCode},
	{Key: "code codocs", Title: "📚 codocs", Description: "Validate co-located .doc.md companions", Group: GroupCode, Args: "<path>"},
	{Key: "code sckg", Title: "🕸️  sckg", Description: "Semantic codebase knowledge graph", Group: GroupCode, Args: "<path>"},
	{Key: "code audit", Title: "🛡️  audit", Description: "47-gate CEO-audit (security/perf/quality)", Group: GroupCode, Args: "<path>"},
	{Key: "code full", Title: "⚡ full pipeline", Description: "preflight → codocs → debt → sckg → ...", Group: GroupCode, Args: "<path>"},

	// ── Go tools (sin sin-code run) ─────────────────────────────
	{Key: "sin-code run discover", Title: "🗂️  discover", Description: "File discovery with relevance scoring", Group: GroupGo, Args: "<path>"},
	{Key: "sin-code run map", Title: "🗺️  map", Description: "Architecture map + dependency graph", Group: GroupGo, Args: "<path>"},
	{Key: "sin-code run grasp", Title: "🤖 grasp", Description: "Deep single-file analysis", Group: GroupGo, Args: "<file>"},
	{Key: "sin-code run scout", Title: "🔭 scout", Description: "Regex/semantic/symbol code search", Group: GroupGo, Args: "<query>"},
	{Key: "sin-code run harvest", Title: "🌾 harvest", Description: "URL fetch + cache + structure extract", Group: GroupGo, Args: "<url>"},
	{Key: "sin-code run execute", Title: "⚙️  execute", Description: "Safe exec with secret redaction", Group: GroupGo, Args: "<cmd>"},
	{Key: "sin-code run orchestrate", Title: "🪄 orchestrate", Description: "Task deps + parallel exec + rollback", Group: GroupGo},

	// ── Python tools ─────────────────────────────────────────────
	{Key: "ibd", Title: "🧬 ibd", Description: "Intent-Based Diffing (semantic AST)", Group: GroupPython, Args: "<files>"},
	{Key: "poc", Title: "📐 poc", Description: "Proof-of-Correctness verification", Group: GroupPython, Args: "<target>"},
	{Key: "adw", Title: "🏛️  adw", Description: "Architectural Debt Watchdog", Group: GroupPython, Args: "<path>"},
	{Key: "oracle", Title: "🔮 oracle", Description: "Independent Verification Oracle", Group: GroupPython, Args: "<target>"},
	{Key: "efm", Title: "🧪 efm", Description: "Ephemeral Full-Stack Mocking", Group: GroupPython, Args: "<spec>"},
	{Key: "sckg", Title: "🕸️  sckg", Description: "SCKG — knowledge graph (11 cmds)", Group: GroupPython, Args: "<cmd> [args]"},

	// ── Security (planned) ───────────────────────────────────────
	{Key: "security", Title: "🔐 security", Description: "8-tool security bundle (Snyk alt.)", Group: GroupSecurity, Args: "scan <path>"},

	// ── Skills ───────────────────────────────────────────────────
	{Key: "brain", Title: "🧠 brain", Description: "Global/project behavioral rules", Group: GroupSkills},
	{Key: "context-bridge", Title: "🌉 context-bridge", Description: "Unified SCKG+brain+gitnexus query", Group: GroupSkills, Args: "<query>"},
	{Key: "websearch", Title: "🔎 websearch", Description: "SerpAPI multi-key pool", Group: GroupSkills, Args: "<query>"},
	{Key: "scheduler", Title: "⏰ scheduler", Description: "Cron/interval job scheduling", Group: GroupSkills},
	{Key: "goal-mode", Title: "🎯 goal-mode", Description: "Goal tracking with checkpoints", Group: GroupSkills},
	{Key: "grill-me", Title: "🔥 grill-me", Description: "Adversarial design-review interview", Group: GroupSkills, Args: "<topic>"},
	{Key: "doc-coauthoring", Title: "📝 doc-coauthoring", Description: "Collaborative README/ADR/SPEC", Group: GroupSkills},
	{Key: "codocs", Title: "📘 codocs", Description: "CoDocs standard + sprint", Group: GroupSkills, Args: "<path>"},
	{Key: "marketplace", Title: "🛍️  marketplace", Description: "Skill catalog search/install", Group: GroupSkills},
	{Key: "browser", Title: "🌐 browser", Description: "106 browser-automation tools", Group: GroupSkills},

	// ── MCP & Serve ──────────────────────────────────────────────
	{Key: "serve", Title: "📡 serve", Description: "Expose tools as unified MCP server", Group: GroupMCP},
	{Key: "serve-mcp", Title: "🛰️  serve-mcp", Description: "Start MCP server (stdio)", Group: GroupMCP},
	{Key: "mcp-config", Title: "📋 mcp-config", Description: "Generate ready-to-use MCP config", Group: GroupMCP},
	{Key: "simone", Title: "🔬 simone", Description: "Simone-MCP code analysis", Group: GroupMCP},

	// ── System ───────────────────────────────────────────────────
	{Key: "status", Title: "📊 status", Description: "Which subsystems are installed", Group: GroupSystem},
	{Key: "doctor", Title: "🩺 doctor", Description: "Diagnose environment + audit chain", Group: GroupSystem},
	{Key: "bootstrap", Title: "🎬 bootstrap", Description: "Initialize subsystems for repo", Group: GroupSystem, Args: "<path>"},
	{Key: "agents-md", Title: "🤝 agents-md", Description: "Create/update AGENTS.md", Group: GroupSystem},
	{Key: "policy", Title: "📜 policy", Description: "Inspect/init SIN policy + audit log", Group: GroupSystem},
	{Key: "skills", Title: "🎓 skills", Description: "Compile portable skills to agent fmt", Group: GroupSystem},
	{Key: "update", Title: "⬆️  update", Description: "Self-update stack (pipx + Go)", Group: GroupSystem},
	{Key: "config", Title: "⚙️  config", Description: "Unified config management", Group: GroupSystem},
}

// Groups returns the menu groups in display order.
func Groups() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range Commands {
		if !seen[c.Group] {
			seen[c.Group] = true
			out = append(out, c.Group)
		}
	}
	return out
}

// Filter returns the commands matching the current query (case-insensitive
// substring match on key, title, or description).
func Filter(query string) []Command {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return Commands
	}
	out := []Command{}
	for _, c := range Commands {
		if strings.Contains(strings.ToLower(c.Key), q) ||
			strings.Contains(strings.ToLower(c.Title), q) ||
			strings.Contains(strings.ToLower(c.Description), q) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ByGroup groups commands by their Group field, preserving catalog order.
func ByGroup(cmds []Command) map[string][]Command {
	out := map[string][]Command{}
	for _, c := range cmds {
		out[c.Group] = append(out[c.Group], c)
	}
	return out
}
