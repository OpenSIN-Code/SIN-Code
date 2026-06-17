// SPDX-License-Identifier: MIT
// Purpose: Source adapter for the in-process MCP tools registered by
// `serve.go`. These tools mirror the `sin_*` CLI subcommands but are
// exposed over the Model Context Protocol for external agents.
package catalog

import (
	"context"
	"strings"
)

// MCPSource is a Source backed by the static in-process MCP tool list.
type MCPSource struct{}

// Name implements Source.
func (MCPSource) Name() string { return "mcp" }

// List implements Source.
func (MCPSource) List(_ context.Context, kind Kind) ([]*Asset, error) {
	if kind != "" && kind != KindMCP {
		return nil, nil
	}
	out := make([]*Asset, 0, len(mcpTools))
	for _, t := range mcpTools {
		a := &Asset{
			Kind:        KindMCP,
			Name:        t.name,
			Namespace:   t.name,
			Short:       firstSentence(t.description),
			Description: t.description,
			Example:     t.example,
			Source:      "mcp",
			Tags:        t.tags,
			ReadOnly:    t.readOnly,
			Destructive: t.destructive,
		}
		out = append(out, a)
	}
	return out, nil
}

// Get implements Source.
func (MCPSource) Get(_ context.Context, kind Kind, name string) (*Asset, bool, error) {
	if kind != "" && kind != KindMCP {
		return nil, false, nil
	}
	for _, t := range mcpTools {
		if t.name == name {
			return &Asset{
				Kind:        KindMCP,
				Name:        t.name,
				Namespace:   t.name,
				Short:       firstSentence(t.description),
				Description: t.description,
				Example:     t.example,
				Source:      "mcp",
				Tags:        t.tags,
				ReadOnly:    t.readOnly,
				Destructive: t.destructive,
			}, true, nil
		}
	}
	return nil, false, nil
}

type mcpTool struct {
	name        string
	description string
	example     string
	tags        []string
	readOnly    bool
	destructive bool
}

// mcpTools is the static mirror of the tools registered in
// cmd/sin-code/internal/serve.go. Keep it sorted alphabetically.
var mcpTools = []mcpTool{
	{name: "sin_adw", description: "Architectural Debt Watchdogs — detect god modules, circular deps, high coupling", example: `{\"path\": \".\", \"strict\": false}`, tags: []string{"read-only", "analysis"}, readOnly: true},
	{name: "sin_agent_doctor", description: "Validate agents (model exists on provider, API key present, base URL reachable)", example: `{\"name\": \"fireworks\", \"offline\": false}`, tags: []string{"read-only", "agents"}, readOnly: true},
	{name: "sin_agent_set", description: "Set fields on a user agent (programmatic edit of agent.toml)", example: `{\"name\": \"fireworks\", \"kvs\": [\"model=gpt-4o\"]}`, tags: []string{"destructive", "agents"}, destructive: true},
	{name: "sin_agent_show", description: "Show effective config for a single agent (merged defaults + user overrides)", example: `{\"name\": \"fireworks\"}`, tags: []string{"read-only", "agents"}, readOnly: true},
	{name: "sin_discover", description: "Discover files with relevance scoring, pattern matching, and dependency analysis", example: `{\"path\": \".\", \"pattern\": \"**/*.go\"}`, tags: []string{"read-only", "analysis"}, readOnly: true},
	{name: "sin_edit", description: "Surgical file edit, three addressing modes. Symbol mode (preferred for whole definitions): pass symbol=NAME to replace/delete/insert around an entire function/class/struct located via AST. Anchor mode: LINE:HASH anchors from sin_read, drift-tolerant. String mode: old_string/new_string with ambiguity detection. Result is syntax-validated and written atomically.", example: `{\"path\": \"main.go\", \"symbol\": \"handleScout\", \"new_text\": \"...\"}`, tags: []string{"destructive", "filesystem"}, destructive: true},
	{name: "sin_efm", description: "Ephemeral Full-Stack Mocking — spin up disposable test environments", example: `{\"action\": \"up\", \"stack\": \"docker-compose.yml\"}`, tags: []string{"destructive", "infrastructure"}, destructive: true},
	{name: "sin_execute", description: "Execute shell commands safely with secret redaction, timeout, and error analysis", example: `{\"command\": \"go test ./...\", \"timeout\": 60}`, tags: []string{"destructive", "shell"}, destructive: true},
	{name: "sin_grasp", description: "Deep code understanding for a single file — structure, dependencies, usage", example: `{\"path\": \"cmd/sin-code/main.go\"}`, tags: []string{"read-only", "analysis"}, readOnly: true},
	{name: "sin_harvest", description: "Fetch URLs with caching, structure extraction, and change detection", example: `{\"url\": \"https://api.example.com/data\"}`, tags: []string{"read-only", "network"}, readOnly: true},
	{name: "sin_ibd", description: "Intent-Based Diffing — compare code changes against stated intent", example: `{\"before\": \"main\", \"after\": \"HEAD\", \"intent\": \"refactor auth\"}`, tags: []string{"read-only", "diff"}, readOnly: true},
	{name: "sin_index", description: "Manage persistent incremental code index (build, refresh, status, clear)", example: `{\"action\": \"build\", \"root\": \".\"}`, tags: []string{"destructive", "index"}, destructive: true},
	{name: "sin_lsp_servers", description: "List detected LSP servers on PATH (gopls, pyright, tsserver, rust-analyzer)", example: `{}`, tags: []string{"read-only", "lsp"}, readOnly: true},
	{name: "sin_map", description: "Map code architecture with dependency graphs, entry points, and hot paths", example: `{\"path\": \".\", \"action\": \"map\"}`, tags: []string{"read-only", "analysis"}, readOnly: true},
	{name: "sin_memory_add", description: "Add a long-term project memory (insight, project, tags). Used by orchestrator agents via prime context.", example: `{\"insight\": \"auth uses JWT\", \"tags\": \"auth,jwt\"}`, tags: []string{"destructive", "memory"}, destructive: true},
	{name: "sin_memory_list", description: "List project memories (filter by project/tag)", example: `{\"project\": \"sin-code\"}`, tags: []string{"read-only", "memory"}, readOnly: true},
	{name: "sin_memory_prime", description: "Print top-K relevant memories for an LLM prompt (markdown formatted, ready to inject)", example: `{\"query\": \"auth module\", \"top\": 10}`, tags: []string{"read-only", "memory"}, readOnly: true},
	{name: "sin_memory_search", description: "Semantic search (uses NIM embeddings if SIN_NIM_API_KEY is set; substring fallback otherwise)", example: `{\"query\": \"auth module\", \"top\": 10}`, tags: []string{"read-only", "memory"}, readOnly: true},
	{name: "sin_memory_stats", description: "Memory DB statistics (total, links, embeddings, embedder status)", example: `{}`, tags: []string{"read-only", "memory"}, readOnly: true},
	{name: "sin_notifications_list", description: "List recent non-dismissed notifications (JSON, top 50)", example: `{\"limit\": 50}`, tags: []string{"read-only", "notifications"}, readOnly: true},
	{name: "sin_notifications_mark_read", description: "Mark a notification as read by ID", example: `{\"id\": \"abc123\"}`, tags: []string{"destructive", "notifications"}, destructive: true},
	{name: "sin_notifications_stats", description: "Notification statistics (total, unread, by type)", example: `{}`, tags: []string{"read-only", "notifications"}, readOnly: true},
	{name: "sin_oracle", description: "Verification Oracle — independent verification of claims with evidence", example: `{\"claim\": \"auth is enforced\"}`, tags: []string{"read-only", "verify"}, readOnly: true},
	{name: "sin_orchestrator_agents", description: "List all available agents (default + user-defined) with their config", example: `{}`, tags: []string{"read-only", "agents"}, readOnly: true},
	{name: "sin_orchestrator_plan", description: "Build a plan from a prompt (no execution) — previews sub-tasks and agents", example: `{\"prompt\": \"Add tests\"}`, tags: []string{"read-only", "agents"}, readOnly: true},
	{name: "sin_orchestrator_run", description: "Run a prompt through the multi-agent orchestrator (Pre-LLM router → planner → parallel agents)", example: `{\"prompt\": \"Refactor auth\", \"max_parallel\": 4}`, tags: []string{"destructive", "agents"}, destructive: true},
	{name: "sin_orchestrate", description: "Manage tasks with dependencies, parallel execution, and rollback plans", example: `{\"action\": \"add\", \"title\": \"Feature X\"}`, tags: []string{"destructive", "tasks"}, destructive: true},
	{name: "sin_poc", description: "Proof-of-Correctness — verify code satisfies its specification", example: `{\"spec\": \"fizzbuzz returns correct values\", \"code\": \"func fizzbuzz(n int)...\"}`, tags: []string{"read-only", "verify"}, readOnly: true},
	{name: "sin_read", description: "Read files token-efficiently: hashline mode emits LINE:HASH anchors for sin_edit, outline mode returns structure only for large files, raw mode is offset/limit guarded. Always prefer over native read.", example: `{\"path\": \"main.go\", \"mode\": \"hashline\"}`, tags: []string{"read-only", "filesystem"}, readOnly: true},
	{name: "sin_sbom_generate", description: "Generate a Software Bill of Materials (SPDX 2.3 JSON or CycloneDX 1.5 JSON) for a project", example: `{\"path\": \".\", \"format\": \"spdx-json\"}`, tags: []string{"read-only", "compliance"}, readOnly: true},
	{name: "sin_sckg", description: "Semantic Codebase Knowledge Graphs — build & query code graph", example: `{\"path\": \".\", \"action\": \"build\"}`, tags: []string{"read-only", "graph"}, readOnly: true},
	{name: "sin_scout", description: "Search code with regex, semantic, symbol, and usage search", example: `{\"query\": \"func.*main\", \"search_type\": \"regex\"}`, tags: []string{"read-only", "search"}, readOnly: true},
	{name: "sin_security_scan", description: "Run the in-tree security subcommand on a project path. Auto-detects Go / Python / Node / generic and runs govulncheck, gosec, go vet, bandit, safety, npm audit, secrets grep, and a file-permission walker.", example: `{\"path\": \".\", \"type\": \"auto\"}`, tags: []string{"read-only", "security"}, readOnly: true},
	{name: "sin_todo_add", description: "Add a todo (v2 bbolt store, hash ID, supports priority/type/tags/project/assignee)", example: `{\"title\": \"Add tests\", \"priority\": \"P2\"}`, tags: []string{"destructive", "tasks"}, destructive: true},
	{name: "sin_todo_blocked", description: "List blocked todos (have open dependencies)", example: `{}`, tags: []string{"read-only", "tasks"}, readOnly: true},
	{name: "sin_todo_claim", description: "Atomically claim a todo (assigns to --as, sets status=in_progress)", example: `{\"id\": \"abc123\"}`, tags: []string{"destructive", "tasks"}, destructive: true},
	{name: "sin_todo_complete", description: "Mark a todo as done (status=done, sets closed_at, fires hooks+notifications)", example: `{\"id\": \"abc123\"}`, tags: []string{"destructive", "tasks"}, destructive: true},
	{name: "sin_todo_dep_add", description: "Add a dependency between two todos (child depends on parent)", example: `{\"child\": \"a\", \"parent\": \"b\"}`, tags: []string{"destructive", "tasks"}, destructive: true},
	{name: "sin_todo_deps", description: "Show dependency tree of a todo", example: `{\"child\": \"abc123\"}`, tags: []string{"read-only", "tasks"}, readOnly: true},
	{name: "sin_todo_list", description: "List todos with filters (status/priority/tag/project/limit)", example: `{\"status\": \"open\", \"limit\": 50}`, tags: []string{"read-only", "tasks"}, readOnly: true},
	{name: "sin_todo_prime", description: "Print ready/blocked/mine context for agent prompts", example: `{}`, tags: []string{"read-only", "tasks"}, readOnly: true},
	{name: "sin_todo_ready", description: "List unblocked open work (P0 first) — what should I work on next?", example: `{}`, tags: []string{"read-only", "tasks"}, readOnly: true},
	{name: "sin_todo_search", description: "Full-text search in todo titles + descriptions", example: `{\"query\": \"auth\"}`, tags: []string{"read-only", "tasks"}, readOnly: true},
	{name: "sin_todo_show", description: "Show full details of a todo by ID (includes audit log + dependencies)", example: `{\"id\": \"abc123\"}`, tags: []string{"read-only", "tasks"}, readOnly: true},
	{name: "sin_todo_stats", description: "Counts by status/priority/type/assignee (JSON)", example: `{}`, tags: []string{"read-only", "tasks"}, readOnly: true},
	{name: "sin_write", description: "Write a file atomically (temp+fsync+rename) with syntax pre-validation (Go parse, JSON parse, bracket-balance elsewhere). A failed validation never touches disk. Always prefer over native write.", example: `{\"path\": \"out.txt\", \"content\": \"hello\"}`, tags: []string{"destructive", "filesystem"}, destructive: true},
}

// firstSentence returns the first sentence of a description, trimmed.
func firstSentence(s string) string {
	if i := findAny(s, []string{". ", ".\n", " — "}); i >= 0 {
		return s[:i+1]
	}
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

func findAny(s string, subs []string) int {
	best := -1
	for _, sub := range subs {
		if i := strings.Index(s, sub); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	return best
}
