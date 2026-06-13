// SPDX-License-Identifier: MIT
// Purpose: Tool catalog hub for the sin-code unified CLI. Lists all
// available tools/commands by category, supports search and pretty printing.
// Docs: hub.doc.md
package hub

import (
	"fmt"
	"strings"
)

// Category groups tools by their primary purpose.
type Category struct {
	Name        string
	Description string
	Tools       []Tool
}

// Tool describes one sin-code command/tool entry in the catalog.
type Tool struct {
	Name        string
	Short       string
	Description string
	Example     string
}

// DefaultCatalog is the canonical hub catalog. It mirrors the 36 subcommands
// plus the most relevant MCP skill surfaces. Keep it alphabetically sorted
// within each category for stable output.
func DefaultCatalog() []Category {
	return []Category{
		{
			Name:        "Core Analysis",
			Description: "Discover, understand, and inspect code",
			Tools: []Tool{
				{Name: "discover", Short: "Find files by relevance", Description: "Smart file discovery with dependency and related-file scoring.", Example: "sin-code discover --pattern '**/*.go' --sort_by relevance"},
				{Name: "grasp", Short: "Single-file analysis", Description: "Structure, dependencies, usage, and context for one file.", Example: "sin-code grasp cmd/sin-code/main.go"},
				{Name: "map", Short: "Architecture map", Description: "Module-level entry points, hot paths, and dependency graph.", Example: "sin-code map --action graph"},
				{Name: "scout", Short: "Pattern search", Description: "Regex, semantic, and symbol search across the codebase.", Example: "sin-code scout 'func.*main' --search_type regex"},
			},
		},
		{
			Name:        "Execution & Orchestration",
			Description: "Run commands, schedule tasks, and orchestrate agents",
			Tools: []Tool{
				{Name: "execute", Short: "Safe command runner", Description: "Execute shell commands with safety checks, timeout, and secret redaction.", Example: "sin-code execute --command 'go test ./...' --timeout 60"},
				{Name: "orchestrate", Short: "Task management", Description: "Persistent task queue with dependencies and rollback plan.", Example: "sin-code orchestrate --action add --title 'Feature X'"},
				{Name: "harvest", Short: "URL/API fetch", Description: "Fetch and structure URLs/APIs with caching and change detection.", Example: "sin-code harvest https://api.example.com/data"},
			},
		},
		{
			Name:        "Verification & Advanced Tools",
			Description: "Proof, oracle, and specialized analysis",
			Tools: []Tool{
				{Name: "poc", Short: "Proof-of-Correctness", Description: "Run verification suites and produce evidence artifacts.", Example: "sin-code poc --target ./..."},
				{Name: "oracle", Short: "Verification Oracle", Description: "Cross-check claims against source and execution flows.", Example: "sin-code oracle --claim 'auth is enforced'"},
				{Name: "ibd", Short: "Intent-Based Diffing", Description: "Review diffs against intent instead of line noise.", Example: "sin-code ibd --from main --to HEAD"},
				{Name: "sckg", Short: "Semantic Code Graph", Description: "Query and navigate the semantic codebase knowledge graph.", Example: "sin-code sckg --query 'auth module'"},
				{Name: "adw", Short: "Architectural Debt Watchdog", Description: "Detect and track architectural debt patterns.", Example: "sin-code adw --path ."},
				{Name: "efm", Short: "Ephemeral Full-Stack Mocking", Description: "Spin up disposable full-stack environments (OrbStack/Docker).", Example: "sin-code efm up --stack docker-compose.yml"},
			},
		},
		{
			Name:        "Security & Compliance",
			Description: "Scan, SBOM, and harden",
			Tools: []Tool{
				{Name: "security", Short: "Security scan", Description: "Run govulncheck, gosec, bandit, npm audit, and secret grep.", Example: "sin-code security --path ."},
				{Name: "sbom", Short: "SBOM generation", Description: "Generate SPDX/CycloneDX SBOMs for Go/Python/Node projects.", Example: "sin-code sbom --path . --format spdx-json"},
			},
		},
		{
			Name:        "Agent & Chat Infrastructure",
			Description: "Interactive and autonomous agent modes",
			Tools: []Tool{
				{Name: "chat", Short: "Chat with LLM", Description: "Single or multi-turn chat with tool access and session management.", Example: "sin-code chat --agent fireworks"},
				{Name: "sessions", Short: "Session manager", Description: "List, resume, and manage chat sessions.", Example: "sin-code sessions list"},
				{Name: "swarm", Short: "Multi-agent swarm", Description: "Spawn parallel agents with a shared workspace.", Example: "sin-code swarm --task 'Refactor auth'"},
				{Name: "goal", Short: "Goal manager", Description: "Persistent goal queue with cron/file triggers.", Example: "sin-code goal create --title 'Add tests'"},
				{Name: "daemon", Short: "Daemon mode", Description: "Run sin-code as a background service.", Example: "sin-code daemon start"},
				{Name: "skill", Short: "Skill manager", Description: "Install, update, and remove ecosystem skills.", Example: "sin-code skill list"},
			},
		},
		{
			Name:        "Methodology Stack (v3.8.0+)",
			Description: "Context, methodology, and research bridges",
			Tools: []Tool{
				{Name: "dox", Short: "AGENTS.md hierarchy", Description: "Self-maintaining AGENTS.md tree protocol (agent0ai/dox).", Example: "sin-code dox check"},
				{Name: "superpowers", Short: "Skill workflows", Description: "obra/superpowers methodology integration.", Example: "sin-code superpowers find 'debug failing test'"},
				{Name: "vane", Short: "Vane research bridge", Description: "Citation-backed AI research via self-hosted Vane.", Example: "sin-code vane search 'Go 1.24 generics'"},
				{Name: "stack", Short: "Stack install/doctor", Description: "One-shot DOX + Superpowers + Vane management.", Example: "sin-code stack install"},
			},
		},
		{
			Name:        "GitHub & Utilities",
			Description: "External bridges and helper commands",
			Tools: []Tool{
				{Name: "gh", Short: "GitHub CLI bridge", Description: "3-tier verb policy bridge to the official gh CLI.", Example: "sin-code gh run issue list --state open"},
				{Name: "config", Short: "Configuration", Description: "View and manage sin-code configuration.", Example: "sin-code config --help"},
				{Name: "self-update", Short: "Self update", Description: "Update the sin-code binary to the latest release.", Example: "sin-code self-update"},
				{Name: "update", Short: "Full-stack update", Description: "Update Go binary, scripts, and skills with rollback.", Example: "sin-code update --check"},
				{Name: "tui", Short: "Interactive TUI", Description: "Terminal UI for browsing and running tools.", Example: "sin-code tui"},
				{Name: "webui", Short: "Web UI", Description: "Start the web interface.", Example: "sin-code webui"},
				{Name: "todo", Short: "Issue tracker", Description: "Local todo/issue tracking with dependencies.", Example: "sin-code todo list"},
				{Name: "notifications", Short: "Notifications", Description: "Manage todo event notifications.", Example: "sin-code notifications list"},
				{Name: "hub", Short: "Tool catalog", Description: "Static, categorized catalog of all sin-code subcommands.", Example: "sin-code hub search security"},
			},
		},
	}
}

// AllTools flattens the catalog into a single slice.
func AllTools() []Tool {
	var out []Tool
	for _, c := range DefaultCatalog() {
		out = append(out, c.Tools...)
	}
	return out
}

// Search returns tools whose name, short, or description contains the query.
func Search(query string) []Tool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return AllTools()
	}
	var out []Tool
	for _, t := range AllTools() {
		if strings.Contains(strings.ToLower(t.Name), q) ||
			strings.Contains(strings.ToLower(t.Short), q) ||
			strings.Contains(strings.ToLower(t.Description), q) {
			out = append(out, t)
		}
	}
	return out
}

// FormatList renders a flat list of tools with aligned columns.
func FormatList(tools []Tool) string {
	if len(tools) == 0 {
		return "No tools found."
	}
	maxName := 0
	for _, t := range tools {
		if len(t.Name) > maxName {
			maxName = len(t.Name)
		}
	}
	var b strings.Builder
	for _, t := range tools {
		pad := strings.Repeat(" ", maxName-len(t.Name)+2)
		fmt.Fprintf(&b, "%s%s%s\n", t.Name, pad, t.Short)
	}
	return b.String()
}

// FormatDetail returns a single tool's detailed description.
func FormatDetail(t Tool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Name:        %s\n", t.Name)
	fmt.Fprintf(&b, "Short:       %s\n", t.Short)
	fmt.Fprintf(&b, "Description: %s\n", t.Description)
	fmt.Fprintf(&b, "Example:     %s\n", t.Example)
	return b.String()
}

// FormatCategories renders the full categorized catalog.
func FormatCategories(cats []Category) string {
	if len(cats) == 0 {
		return "Catalog is empty."
	}
	var b strings.Builder
	for _, c := range cats {
		fmt.Fprintf(&b, "\n── %s ──\n%s\n\n", c.Name, c.Description)
		for _, t := range c.Tools {
			fmt.Fprintf(&b, "  %-12s %s\n", t.Name, t.Short)
		}
	}
	return b.String()
}
