// SPDX-License-Identifier: MIT
// Purpose: Tool catalog hub for the sin-code unified CLI. Lists all
// available subcommands by category, supports search and pretty printing.
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
	Namespace   string   // fully-qualified namespaced name (e.g. sin_discover, browser__navigate)
	Short       string   // one-line description
	Description string   // longer description
	Example     string   // example snippet
	Category    string   // category tag (e.g. core, mcp, chat, external)
	Tags        []string // read-only, destructive, mutating, etc.
	ReadOnly    bool     // hint: does not mutate the workspace
	Destructive bool     // hint: may mutate the workspace
}

// DefaultCatalog is the canonical hub catalog. It mirrors all current
// subcommands. Keep it alphabetically sorted within each category for stable
// output.
func DefaultCatalog() []Category {
	return []Category{
		{
			Name:        "Core Analysis",
			Description: "Discover, understand, and inspect code",
			Tools: []Tool{
				{Name: "codegraph", Namespace: "codegraph", Short: "Multi-language code graph", Description: "Build and query a multi-language code graph across Go, Python, Rust, and TypeScript.", Example: "sin-code codegraph --path .", Category: "core", Tags: []string{"read-only", "analysis"}, ReadOnly: true},
				{Name: "discover", Namespace: "discover", Short: "Find files by relevance", Description: "Smart file discovery with dependency and related-file scoring.", Example: "sin-code discover --pattern '**/*.go' --sort_by relevance", Category: "core", Tags: []string{"read-only", "analysis"}, ReadOnly: true},
				{Name: "grasp", Namespace: "grasp", Short: "Single-file analysis", Description: "Structure, dependencies, usage, and context for one file.", Example: "sin-code grasp cmd/sin-code/main.go", Category: "core", Tags: []string{"read-only", "analysis"}, ReadOnly: true},
				{Name: "map", Namespace: "map", Short: "Architecture map", Description: "Module-level entry points, hot paths, and dependency graph.", Example: "sin-code map --action graph", Category: "core", Tags: []string{"read-only", "analysis"}, ReadOnly: true},
				{Name: "scout", Namespace: "scout", Short: "Pattern search", Description: "Regex, semantic, and symbol search across the codebase.", Example: "sin-code scout 'func.*main' --search_type regex", Category: "core", Tags: []string{"read-only", "analysis"}, ReadOnly: true},
			},
		},
		{
			Name:        "Execution & Orchestration",
			Description: "Run commands, schedule tasks, and orchestrate agents",
			Tools: []Tool{
				{Name: "execute", Namespace: "execute", Short: "Safe command runner", Description: "Execute shell commands with safety checks, timeout, and secret redaction.", Example: "sin-code execute --command 'go test ./...' --timeout 60", Category: "execution", Tags: []string{"destructive", "shell"}, Destructive: true},
				{Name: "harvest", Namespace: "harvest", Short: "URL/API fetch", Description: "Fetch and structure URLs/APIs with caching and change detection.", Example: "sin-code harvest https://api.example.com/data", Category: "execution", Tags: []string{"read-only", "network"}, ReadOnly: true},
				{Name: "orchestrate", Namespace: "orchestrate", Short: "Task management", Description: "Persistent task queue with dependencies and rollback plan.", Example: "sin-code orchestrate --action add --title 'Feature X'", Category: "execution", Tags: []string{"destructive", "tasks"}, Destructive: true},
				{Name: "orchestrator-agents", Namespace: "orchestrator-agents", Short: "List agents", Description: "List all available agents (default + user-defined) with their config.", Example: "sin-code orchestrator-agents", Category: "execution", Tags: []string{"read-only", "agents"}, ReadOnly: true},
				{Name: "orchestrator-plan", Namespace: "orchestrator-plan", Short: "Plan from prompt", Description: "Build a plan from a prompt (no execution) — previews sub-tasks and agents.", Example: "sin-code orchestrator-plan --prompt 'Add tests'", Category: "execution", Tags: []string{"read-only", "agents"}, ReadOnly: true},
				{Name: "orchestrator-run", Namespace: "orchestrator-run", Short: "Run orchestrator", Description: "Run a prompt through the multi-agent orchestrator (Pre-LLM router → planner → parallel agents).", Example: "sin-code orchestrator-run --prompt 'Refactor auth'", Category: "execution", Tags: []string{"destructive", "agents"}, Destructive: true},
				{Name: "subagent", Namespace: "subagent", Short: "Isolated sub-agent", Description: "Spawn an isolated-context sub-agent for a specific task.", Example: "sin-code subagent --prompt 'Review this file'", Category: "execution", Tags: []string{"destructive", "agents"}, Destructive: true},
			},
		},
		{
			Name:        "Verification & Advanced Tools",
			Description: "Proof, oracle, and specialized analysis",
			Tools: []Tool{
				{Name: "adw", Namespace: "adw", Short: "Architectural Debt Watchdog", Description: "Detect and track architectural debt patterns.", Example: "sin-code adw --path .", Category: "verification", Tags: []string{"read-only", "audit"}, ReadOnly: true},
				{Name: "debt", Namespace: "debt", Short: "sin-debt marker manager", Description: "Scan, report, and policy-check // sin-debt: markers.", Example: "sin-code debt stats", Category: "verification", Tags: []string{"read-only", "audit"}, ReadOnly: true},
				{Name: "efm", Namespace: "efm", Short: "Ephemeral Full-Stack Mocking", Description: "Spin up disposable full-stack environments (OrbStack/Docker).", Example: "sin-code efm up --stack docker-compose.yml", Category: "verification", Tags: []string{"destructive", "infrastructure"}, Destructive: true},
				{Name: "grill", Namespace: "grill", Short: "Adversarial design review", Description: "Native adversarial design-review interview.", Example: "sin-code grill --topic 'API design'", Category: "verification", Tags: []string{"read-only", "review"}, ReadOnly: true},
				{Name: "ibd", Namespace: "ibd", Short: "Intent-Based Diffing", Description: "Review diffs against intent instead of line noise.", Example: "sin-code ibd --from main --to HEAD", Category: "verification", Tags: []string{"read-only", "diff"}, ReadOnly: true},
				{Name: "oracle", Namespace: "oracle", Short: "Verification Oracle", Description: "Cross-check claims against source and execution flows.", Example: "sin-code oracle --claim 'auth is enforced'", Category: "verification", Tags: []string{"read-only", "verify"}, ReadOnly: true},
				{Name: "poc", Namespace: "poc", Short: "Proof-of-Correctness", Description: "Run verification suites and produce evidence artifacts.", Example: "sin-code poc --target ./...", Category: "verification", Tags: []string{"read-only", "verify"}, ReadOnly: true},
				{Name: "review", Namespace: "review", Short: "Ponytail complexity review", Description: "Static complexity analyzer with ponytail 5-tag format.", Example: "sin-code review --complexity", Category: "verification", Tags: []string{"read-only", "audit"}, ReadOnly: true},
				{Name: "sckg", Namespace: "sckg", Short: "Semantic Code Graph", Description: "Query and navigate the semantic codebase knowledge graph.", Example: "sin-code sckg --query 'auth module'", Category: "verification", Tags: []string{"read-only", "graph"}, ReadOnly: true},
			},
		},
		{
			Name:        "Security & Compliance",
			Description: "Scan, SBOM, and harden",
			Tools: []Tool{
				{Name: "audit", Namespace: "audit", Short: "Repo-wide complexity audit", Description: "Run a repo-wide complexity audit.", Example: "sin-code audit", Category: "security", Tags: []string{"read-only", "audit"}, ReadOnly: true},
				{Name: "ceo-audit", Namespace: "ceo-audit", Short: "CEO-grade audit", Description: "Run a 48-gate CEO-grade repository audit.", Example: "sin-code ceo-audit", Category: "security", Tags: []string{"read-only", "audit"}, ReadOnly: true},
				{Name: "sbom", Namespace: "sbom", Short: "SBOM generation", Description: "Generate SPDX/CycloneDX SBOMs for Go/Python/Node projects.", Example: "sin-code sbom --path . --format spdx-json", Category: "security", Tags: []string{"read-only", "compliance"}, ReadOnly: true},
				{Name: "security", Namespace: "security", Short: "Security scan", Description: "Run govulncheck, gosec, bandit, npm audit, and secret grep.", Example: "sin-code security --path .", Category: "security", Tags: []string{"read-only", "scan"}, ReadOnly: true},
			},
		},
		{
			Name:        "Agent & Chat Infrastructure",
			Description: "Interactive and autonomous agent modes",
			Tools: []Tool{
				{Name: "chat", Namespace: "chat", Short: "Chat with LLM", Description: "Single or multi-turn chat with tool access and session management.", Example: "sin-code chat --agent fireworks", Category: "agent", Tags: []string{"destructive", "interactive"}, Destructive: true},
				{Name: "daemon", Namespace: "daemon", Short: "Daemon mode", Description: "Run sin-code as a background autonomous service.", Example: "sin-code daemon start --verify-cmd 'go test ./...'", Category: "agent", Tags: []string{"destructive", "autonomy"}, Destructive: true},
				{Name: "goal", Namespace: "goal", Short: "Goal manager", Description: "Persistent goal queue with cron/file triggers.", Example: "sin-code goal create --title 'Add tests'", Category: "agent", Tags: []string{"destructive", "autonomy"}, Destructive: true},
				{Name: "ledger", Namespace: "ledger", Short: "Session ledger", Description: "Semantic session ledger query.", Example: "sin-code ledger list", Category: "agent", Tags: []string{"read-only", "sessions"}, ReadOnly: true},
				{Name: "memory", Namespace: "memory", Short: "Memory manager", Description: "Long-term project memory store, search, and prime.", Example: "sin-code memory search 'auth'", Category: "agent", Tags: []string{"destructive", "memory"}, Destructive: true},
				{Name: "notifications", Namespace: "notifications", Short: "Notifications", Description: "Manage todo event notifications.", Example: "sin-code notifications list", Category: "agent", Tags: []string{"destructive", "events"}, Destructive: true},
				{Name: "sessions", Namespace: "sessions", Short: "Session manager", Description: "List, resume, fork, and manage chat sessions.", Example: "sin-code sessions list", Category: "agent", Tags: []string{"read-only", "sessions"}, ReadOnly: true},
				{Name: "skill", Namespace: "skill", Short: "Skill manager", Description: "Install, update, and remove ecosystem skills.", Example: "sin-code skill list", Category: "agent", Tags: []string{"destructive", "skills"}, Destructive: true},
				{Name: "skills", Namespace: "skills", Short: "Bundled skills", Description: "List bundled project-local agent skills.", Example: "sin-code skills list", Category: "agent", Tags: []string{"read-only", "skills"}, ReadOnly: true},
				{Name: "summary", Namespace: "summary", Short: "Session summary", Description: "Deterministic session summary builder.", Example: "sin-code summary --session <id>", Category: "agent", Tags: []string{"read-only", "sessions"}, ReadOnly: true},
				{Name: "swarm", Namespace: "swarm", Short: "Multi-agent swarm", Description: "Spawn parallel agents with a shared workspace.", Example: "sin-code swarm --task 'Refactor auth'", Category: "agent", Tags: []string{"destructive", "agents"}, Destructive: true},
			},
		},
		{
			Name:        "Methodology Stack",
			Description: "Context, methodology, and research bridges",
			Tools: []Tool{
				{Name: "autodev", Namespace: "autodev", Short: "Autodev bridge", Description: "Bridged-External autodev MCP server.", Example: "sin-code autodev --help", Category: "methodology", Tags: []string{"destructive", "external"}, Destructive: true},
				{Name: "dox", Namespace: "dox", Short: "AGENTS.md hierarchy", Description: "Self-maintaining AGENTS.md tree protocol (agent0ai/dox).", Example: "sin-code dox check", Category: "methodology", Tags: []string{"read-only", "docs"}, ReadOnly: true},
				{Name: "gh", Namespace: "gh", Short: "GitHub CLI bridge", Description: "3-tier verb policy bridge to the official gh CLI.", Example: "sin-code gh run issue list --state open", Category: "methodology", Tags: []string{"destructive", "github"}, Destructive: true},
				{Name: "stack", Namespace: "stack", Short: "Stack install/doctor", Description: "One-shot DOX + Superpowers + Vane management.", Example: "sin-code stack install", Category: "methodology", Tags: []string{"destructive", "install"}, Destructive: true},
				{Name: "superpowers", Namespace: "superpowers", Short: "Skill workflows", Description: "obra/superpowers methodology integration.", Example: "sin-code superpowers find 'debug failing test'", Category: "methodology", Tags: []string{"read-only", "workflows"}, ReadOnly: true},
				{Name: "vane", Namespace: "vane", Short: "Vane research bridge", Description: "Citation-backed AI research via self-hosted Vane.", Example: "sin-code vane search 'Go 1.24 generics'", Category: "methodology", Tags: []string{"read-only", "network", "research"}, ReadOnly: true},
			},
		},
		{
			Name:        "File & Editor Utilities",
			Description: "Read, write, and manipulate files",
			Tools: []Tool{
				{Name: "edit", Namespace: "edit", Short: "Surgical file edit", Description: "LSP-grade structural file edit with symbol, anchor, and string modes.", Example: "sin-code edit --path main.go --symbol handleScout --new_text '...'", Category: "utility", Tags: []string{"destructive", "filesystem"}, Destructive: true},
				{Name: "index", Namespace: "index", Short: "Code index", Description: "Manage persistent incremental code index (build, refresh, status, clear).", Example: "sin-code index --action build", Category: "utility", Tags: []string{"destructive", "index"}, Destructive: true},
				{Name: "lsp", Namespace: "lsp", Short: "LSP tools", Description: "List detected LSP servers and run LSP-based queries.", Example: "sin-code lsp servers", Category: "utility", Tags: []string{"read-only", "lsp"}, ReadOnly: true},
				{Name: "plugin", Namespace: "plugin", Short: "Plugin manager", Description: "Manage external plugins and their MCP tools.", Example: "sin-code plugin list", Category: "utility", Tags: []string{"destructive", "plugins"}, Destructive: true},
				{Name: "read", Namespace: "read", Short: "Read files", Description: "Read files token-efficiently with hashline/raw/outline modes.", Example: "sin-code read --path main.go --mode hashline", Category: "utility", Tags: []string{"read-only", "filesystem"}, ReadOnly: true},
				{Name: "write", Namespace: "write", Short: "Write files", Description: "Write a file atomically with syntax pre-validation.", Example: "sin-code write --path out.txt --content 'hello'", Category: "utility", Tags: []string{"destructive", "filesystem"}, Destructive: true},
			},
		},
		{
			Name:        "Lifecycle & Meta Utilities",
			Description: "Update, install, and manage sin-code itself",
			Tools: []Tool{
				{Name: "assets", Namespace: "assets", Short: "Asset catalog", Description: "List and show loaded Markdown frontmatter assets.", Example: "sin-code assets list", Category: "meta", Tags: []string{"read-only", "catalog"}, ReadOnly: true},
				{Name: "catalog", Namespace: "catalog", Short: "Unified catalog", Description: "Unified tool catalog (hub + assets + MCP + chat + external).", Example: "sin-code catalog list", Category: "meta", Tags: []string{"read-only", "catalog"}, ReadOnly: true},
				{Name: "checkpoint", Namespace: "checkpoint", Short: "Workspace checkpoint", Description: "Create a workspace checkpoint snapshot.", Example: "sin-code checkpoint --name pre-refactor", Category: "meta", Tags: []string{"destructive", "workspace"}, Destructive: true},
				{Name: "compile-spec", Namespace: "compile-spec", Short: "Compile spec layer", Description: "Compile declarative .sin-code.yml contracts.", Example: "sin-code compile-spec --path .sin-code.yml", Category: "meta", Tags: []string{"read-only", "spec"}, ReadOnly: true},
				{Name: "config", Namespace: "config", Short: "Configuration", Description: "View and manage sin-code configuration.", Example: "sin-code config show", Category: "meta", Tags: []string{"read-only", "config"}, ReadOnly: true},
				{Name: "cover", Namespace: "cover", Short: "Coverage Drohne", Description: "Scan, check, generate, and hook coverage reports.", Example: "sin-code cover scan", Category: "meta", Tags: []string{"read-only", "coverage"}, ReadOnly: true},
				{Name: "eval", Namespace: "eval", Short: "Eval harness", Description: "Run golden datasets and compare arms.", Example: "sin-code eval run --dataset evals/critical.json", Category: "meta", Tags: []string{"read-only", "eval"}, ReadOnly: true},
				{Name: "evalset", Namespace: "evalset", Short: "Evalset manager", Description: "Manage eval datasets.", Example: "sin-code evalset list", Category: "meta", Tags: []string{"read-only", "eval"}, ReadOnly: true},
				{Name: "hooks", Namespace: "hooks", Short: "Lifecycle hooks", Description: "Manage lifecycle hooks.", Example: "sin-code hooks list", Category: "meta", Tags: []string{"read-only", "hooks"}, ReadOnly: true},
				{Name: "hub", Namespace: "hub", Short: "Tool catalog", Description: "Static, categorized catalog of all sin-code subcommands.", Example: "sin-code hub search security", Category: "meta", Tags: []string{"read-only", "catalog"}, ReadOnly: true},
				{Name: "install", Namespace: "install", Short: "Install sin-code", Description: "Single-binary installer with SHA256-verified release downloads.", Example: "sin-code install", Category: "meta", Tags: []string{"destructive", "install"}, Destructive: true},
				{Name: "instinct", Namespace: "instinct", Short: "Instinct manager", Description: "Manage learned instincts.", Example: "sin-code instinct list", Category: "meta", Tags: []string{"read-only", "learning"}, ReadOnly: true},
				{Name: "prp", Namespace: "prp", Short: "PRP workflow", Description: "Pull-request preparation workflow.", Example: "sin-code prp create", Category: "meta", Tags: []string{"destructive", "github"}, Destructive: true},
				{Name: "profile", Namespace: "profile", Short: "Profile renderer", Description: "Render the single-source-of-truth per-agent profile.", Example: "sin-code profile render", Category: "meta", Tags: []string{"destructive", "install"}, Destructive: true},
				{Name: "rewind", Namespace: "rewind", Short: "Workspace rewind", Description: "Rewind the workspace to a checkpoint.", Example: "sin-code rewind --to pre-refactor", Category: "meta", Tags: []string{"destructive", "workspace"}, Destructive: true},
				{Name: "rtk", Namespace: "rtk", Short: "Rust Token Killer", Description: "Rust token killer bridge.", Example: "sin-code rtk --help", Category: "meta", Tags: []string{"destructive", "security"}, Destructive: true},
				{Name: "self-update", Namespace: "self-update", Short: "Self update", Description: "Update the sin-code binary to the latest release.", Example: "sin-code self-update", Category: "meta", Tags: []string{"destructive", "install"}, Destructive: true},
				{Name: "spec", Namespace: "spec", Short: "Spec layer", Description: "Spec-Layer contract manager for *.spec.md files.", Example: "sin-code spec list", Category: "meta", Tags: []string{"read-only", "spec"}, ReadOnly: true},
				{Name: "todo", Namespace: "todo", Short: "Issue tracker", Description: "Local todo/issue tracking with dependencies.", Example: "sin-code todo list", Category: "meta", Tags: []string{"destructive", "tasks"}, Destructive: true},
				{Name: "trace", Namespace: "trace", Short: "Trace doctor", Description: "Sanity-check the OTel exporter setup.", Example: "sin-code trace doctor --exporter stdout", Category: "meta", Tags: []string{"read-only", "observability"}, ReadOnly: true},
				{Name: "triage", Namespace: "triage", Short: "Backlog triage", Description: "Backlog auto-prioritizer via gh.", Example: "sin-code triage --repo owner/repo", Category: "meta", Tags: []string{"read-only", "github"}, ReadOnly: true},
				{Name: "tui", Namespace: "tui", Short: "Interactive TUI", Description: "Terminal UI for browsing and running tools.", Example: "sin-code tui", Category: "meta", Tags: []string{"read-only", "interactive"}, ReadOnly: true},
				{Name: "update", Namespace: "update", Short: "Full-stack update", Description: "Update Go binary, scripts, and skills with rollback.", Example: "sin-code update --check", Category: "meta", Tags: []string{"destructive", "install"}, Destructive: true},
				{Name: "webui", Namespace: "webui", Short: "Web UI", Description: "Start the web interface.", Example: "sin-code webui", Category: "meta", Tags: []string{"destructive", "server"}, Destructive: true},
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

// Search returns tools whose name, short, namespace, or description contains the query.
func Search(query string) []Tool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return AllTools()
	}
	var out []Tool
	for _, t := range AllTools() {
		if strings.Contains(strings.ToLower(t.Name), q) ||
			strings.Contains(strings.ToLower(t.Namespace), q) ||
			strings.Contains(strings.ToLower(t.Short), q) ||
			strings.Contains(strings.ToLower(t.Description), q) ||
			strings.Contains(strings.ToLower(t.Category), q) ||
			containsTag(t.Tags, q) {
			out = append(out, t)
		}
	}
	return out
}

func containsTag(tags []string, q string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

// FormatList renders a flat list of tools with aligned columns.
func FormatList(tools []Tool) string {
	if len(tools) == 0 {
		return "No tools found."
	}
	maxName := 0
	for _, t := range tools {
		name := t.Name
		if t.Namespace != "" && t.Namespace != t.Name {
			name = t.Namespace
		}
		if len(name) > maxName {
			maxName = len(name)
		}
	}
	var b strings.Builder
	for _, t := range tools {
		name := t.Name
		if t.Namespace != "" && t.Namespace != t.Name {
			name = t.Namespace
		}
		pad := strings.Repeat(" ", maxName-len(name)+2)
		fmt.Fprintf(&b, "%s%s%s", name, pad, t.Short)
		if t.ReadOnly {
			fmt.Fprintf(&b, " [read-only]")
		} else if t.Destructive {
			fmt.Fprintf(&b, " [destructive]")
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

// FormatDetail returns a single tool's detailed description.
func FormatDetail(t Tool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Name:        %s\n", t.Name)
	if t.Namespace != "" && t.Namespace != t.Name {
		fmt.Fprintf(&b, "Namespace:   %s\n", t.Namespace)
	}
	fmt.Fprintf(&b, "Short:       %s\n", t.Short)
	fmt.Fprintf(&b, "Description: %s\n", t.Description)
	fmt.Fprintf(&b, "Category:    %s\n", t.Category)
	if len(t.Tags) > 0 {
		fmt.Fprintf(&b, "Tags:        %s\n", strings.Join(t.Tags, ", "))
	}
	if t.ReadOnly {
		fmt.Fprintf(&b, "Hint:        read-only\n")
	} else if t.Destructive {
		fmt.Fprintf(&b, "Hint:        destructive\n")
	}
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
			name := t.Name
			if t.Namespace != "" && t.Namespace != t.Name {
				name = t.Namespace
			}
			hint := ""
			if t.ReadOnly {
				hint = " [read-only]"
			} else if t.Destructive {
				hint = " [destructive]"
			}
			fmt.Fprintf(&b, "  %-20s %s%s\n", name, t.Short, hint)
		}
	}
	return b.String()
}
