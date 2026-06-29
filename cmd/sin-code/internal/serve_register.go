// SPDX-License-Identifier: MIT
// Purpose: serve — registerAllMCPTools: registers all 54+ sin-code tools
// onto the MCP server, with optional ponytail-tag description compression.
// sin-debt: shrink, upgrade: when a second register-related function is needed, merge into a shared file
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpcompress"
)

func registerAllMCPTools(server *mcp.Server) {
	type toolDef struct {
		name        string
		description string
		handler     func(ctx context.Context, args map[string]any) (string, error)
		schema      map[string]any
	}

	tools := []toolDef{
		{
			name:        "sin_discover",
			description: "Discover files with relevance scoring, pattern matching, and dependency analysis",
			handler:     handleDiscover,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Directory to search"},
					"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. **/*.py)"},
					"format":  map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
					"limit":   map[string]any{"type": "integer", "default": 100},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_execute",
			description: "Execute shell commands safely with secret redaction, timeout, and error analysis",
			handler:     handleExecute,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "Command to execute"},
					"timeout": map[string]any{"type": "integer", "default": 60},
					"format":  map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"command"},
			},
		},
		{
			name:        "sin_map",
			description: "Map code architecture with dependency graphs, entry points, and hot paths",
			handler:     handleMap,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Project root"},
					"action": map[string]any{"type": "string", "default": "map"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_grasp",
			description: "Deep code understanding for a single file — structure, dependencies, usage",
			handler:     handleGrasp,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "File to analyze"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_scout",
			description: "Search code with regex, semantic, symbol, and usage search",
			handler:     handleScout,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":       map[string]any{"type": "string", "description": "Search query"},
					"path":        map[string]any{"type": "string", "default": "."},
					"search_type": map[string]any{"type": "string", "enum": []string{"regex", "semantic", "symbol", "usage"}, "default": "regex"},
					"format":      map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"query"},
			},
		},
		{
			name:        "sin_harvest",
			description: "Fetch URLs with caching, structure extraction, and change detection",
			handler:     handleHarvest,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":     map[string]any{"type": "string", "description": "URL to fetch"},
					"method":  map[string]any{"type": "string", "default": "GET"},
					"timeout": map[string]any{"type": "integer", "default": 30},
					"format":  map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"url"},
			},
		},
		{
			name:        "sin_index",
			description: "Manage persistent incremental code index (build, refresh, status, clear)",
			handler:     handleIndex,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"build", "refresh", "status", "clear"}, "default": "status"},
					"root":   map[string]any{"type": "string", "default": "."},
				},
			},
		},
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
		{
			name:        "sin_todo_add",
			description: "Add a todo (v2 bbolt store, hash ID, supports priority/type/tags/project/assignee)",
			handler:     handleTodoAdd,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"title"},
				"properties": map[string]any{
					"title":       map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"priority":    map[string]any{"type": "string", "enum": []string{"P0", "P1", "P2", "P3"}, "default": "P2"},
					"type":        map[string]any{"type": "string", "enum": []string{"task", "bug", "feature", "chore", "epic", "question"}, "default": "task"},
					"tags":        map[string]any{"type": "string", "description": "Comma-separated"},
					"project":     map[string]any{"type": "string"},
					"assignee":    map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_todo_list",
			description: "List todos with filters (status/priority/tag/project/limit)",
			handler:     handleTodoList,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status":   map[string]any{"type": "string"},
					"priority": map[string]any{"type": "string"},
					"tag":      map[string]any{"type": "string"},
					"project":  map[string]any{"type": "string"},
					"limit":    map[string]any{"type": "integer", "default": 50},
				},
			},
		},
		{
			name:        "sin_todo_show",
			description: "Show full details of a todo by ID (includes audit log + dependencies)",
			handler:     handleTodoShow,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_todo_complete",
			description: "Mark a todo as done (status=done, sets closed_at, fires hooks+notifications)",
			handler:     handleTodoComplete,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_todo_claim",
			description: "Atomically claim a todo (assigns to --as, sets status=in_progress)",
			handler:     handleTodoClaim,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
					"as": map[string]any{"type": "string", "description": "Actor (default git user.name)"},
				},
			},
		},
		{
			name:        "sin_todo_ready",
			description: "List unblocked open work (P0 first) — what should I work on next?",
			handler:     handleTodoReady,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_todo_blocked",
			description: "List blocked todos (have open dependencies)",
			handler:     handleTodoBlocked,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_todo_search",
			description: "Full-text search in todo titles + descriptions",
			handler:     handleTodoSearch,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_todo_prime",
			description: "Print ready/blocked/mine context for agent prompts",
			handler:     handleTodoPrime,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_todo_stats",
			description: "Counts by status/priority/type/assignee (JSON)",
			handler:     handleTodoStats,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_todo_dep_add",
			description: "Add a dependency between two todos (child depends on parent)",
			handler:     handleTodoDepAdd,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"child", "parent"},
				"properties": map[string]any{
					"child":  map[string]any{"type": "string"},
					"parent": map[string]any{"type": "string"},
					"rel":    map[string]any{"type": "string", "enum": []string{"blocks", "parent-child", "related", "discovered-from", "duplicates", "supersedes"}, "default": "blocks"},
				},
			},
		},
		{
			name:        "sin_todo_deps",
			description: "Show dependency tree of a todo",
			handler:     handleTodoDep,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"child"},
				"properties": map[string]any{
					"child": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_memory_add",
			description: "Add a long-term project memory (insight, project, tags). Used by orchestrator agents via prime context.",
			handler:     handleMemoryAdd,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"insight"},
				"properties": map[string]any{
					"insight": map[string]any{"type": "string"},
					"project": map[string]any{"type": "string"},
					"tags":    map[string]any{"type": "string", "description": "Comma-separated"},
				},
			},
		},
		{
			name:        "sin_memory_list",
			description: "List project memories (filter by project/tag)",
			handler:     handleMemoryList,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": map[string]any{"type": "string"},
					"tag":     map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_memory_search",
			description: "Semantic search (uses NIM embeddings if SIN_NIM_API_KEY is set; substring fallback otherwise)",
			handler:     handleMemorySearch,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query":   map[string]any{"type": "string"},
					"project": map[string]any{"type": "string"},
					"top":     map[string]any{"type": "integer", "default": 10},
				},
			},
		},
		{
			name:        "sin_memory_prime",
			description: "Print top-K relevant memories for an LLM prompt (markdown formatted, ready to inject)",
			handler:     handleMemoryPrime,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query":   map[string]any{"type": "string"},
					"project": map[string]any{"type": "string"},
					"top":     map[string]any{"type": "integer", "default": 10},
				},
			},
		},
		{
			name:        "sin_memory_stats",
			description: "Memory DB statistics (total, links, embeddings, embedder status)",
			handler:     handleMemoryStats,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_notifications_list",
			description: "List recent non-dismissed notifications (JSON, top 50)",
			handler:     handleNotificationsList,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"type": "integer", "default": 50},
				},
			},
		},
		{
			name:        "sin_notifications_stats",
			description: "Notification statistics (total, unread, by type)",
			handler:     handleNotificationsStats,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_notifications_mark_read",
			description: "Mark a notification as read by ID",
			handler:     handleNotificationsMarkRead,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
			},
		},
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
		{
			name:        "sin_analyse_image",
			description: "Analyze an image with a vision-capable LLM (no Tesseract). Returns a structured description including visible text, UI elements, and layout.",
			handler:     handleAnalyseImage,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Path to the image file"},
					"prompt": map[string]any{"type": "string", "description": "Custom prompt for the vision model (optional)"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_read",
			description: "Read files token-efficiently: hashline mode emits LINE:HASH anchors for sin_edit, outline mode returns structure only for large files, raw mode is offset/limit guarded. Always prefer over native read.",
			handler:     handleRead,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": "File to read"},
					"mode":      map[string]any{"type": "string", "enum": []string{"hashline", "raw", "outline"}, "default": "hashline"},
					"offset":    map[string]any{"type": "integer", "default": 1, "description": "1-based start line"},
					"limit":     map[string]any{"type": "integer", "default": 2000, "description": "Max lines"},
					"max_bytes": map[string]any{"type": "integer", "description": "Size guard for raw/hashline reads"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_write",
			description: "Write a file atomically (temp+fsync+rename) with syntax pre-validation (Go parse, JSON parse, bracket-balance elsewhere). A failed validation never touches disk. Always prefer over native write.",
			handler:     handleWrite,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "Destination file"},
					"content":     map[string]any{"type": "string", "description": "Full file content"},
					"no_validate": map[string]any{"type": "boolean", "default": false},
					"backup":      map[string]any{"type": "boolean", "default": false, "description": "Keep .bak of previous content"},
					"mkdir":       map[string]any{"type": "boolean", "default": false, "description": "Create parent directories"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			name:        "sin_edit",
			description: "Surgical file edit, three addressing modes. Symbol mode (preferred for whole definitions): pass symbol=NAME to replace/delete/insert around an entire function/class/struct located via AST (go/ast, tree-sitter, or structural engine — ambiguity fails with candidates). Anchor mode: LINE:HASH anchors from sin_read, drift-tolerant. String mode: old_string/new_string with ambiguity detection. Result is syntax-validated and written atomically. Always prefer over native edit.",
			handler:     handleEdit,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "File to edit"},
					"symbol":      map[string]any{"type": "string", "description": "Symbol name for AST-anchored edits, e.g. \"handleScout\" or \"Server.Start\""},
					"anchor":      map[string]any{"type": "string", "description": "Start anchor LINE:HASH from sin_read"},
					"end_anchor":  map[string]any{"type": "string", "description": "End anchor for range edits (inclusive)"},
					"new_text":    map[string]any{"type": "string", "description": "Replacement/insertion text"},
					"old_string":  map[string]any{"type": "string", "description": "Exact string to replace (string mode)"},
					"new_string":  map[string]any{"type": "string", "description": "Replacement (string mode)"},
					"replace_all": map[string]any{"type": "boolean", "default": false},
					"insert":      map[string]any{"type": "string", "enum": []string{"before", "after"}, "description": "Insert relative to anchor/symbol"},
					"delete":      map[string]any{"type": "boolean", "default": false, "description": "Delete anchored line/range/symbol"},
					"dry_run":     map[string]any{"type": "boolean", "default": false, "description": "Return diff without writing"},
					"no_validate": map[string]any{"type": "boolean", "default": false},
					"drift":       map[string]any{"type": "integer", "default": 25, "description": "Anchor drift tolerance"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_run_loop",
			description: "Run a prompt through the full SIN-Code agent loop (PLAN→ACT→VERIFY→DONE). Returns {session_id, summary, verified, turns}. Blocks until completion. Includes Verify-Gate, Stop-Gate, Lessons, Compaction, Loop-Detection. This is the synchronous delegation path — one call, one verified task.",
			handler:     handleRunLoop,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt":     map[string]any{"type": "string"},
					"workspace":  map[string]any{"type": "string", "default": "."},
					"model":      map[string]any{"type": "string"},
					"max_turns":  map[string]any{"type": "integer", "default": 80},
					"verify_cmd": map[string]any{"type": "string"},
					"yolo":       map[string]any{"type": "boolean", "default": false},
					"agent":      map[string]any{"type": "string"},
					"style":      map[string]any{"type": "string", "enum": []string{"default", "verbose", "normal", "terse", "ultra"}, "default": "default"},
					"criteria":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"prompt"},
			},
		},
		{
			name:        "sin_goal_add",
			description: "Enqueue a goal for autonomous execution by the sin-code daemon. Returns immediately with the goal ID. The daemon will pick it up, run the full agent loop (with Verify-Gate, Stop-Gate, Lessons), and mark it verified/failed. Use sin_goal_status to poll progress.",
			handler:     handleGoalAdd,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt":    map[string]any{"type": "string"},
					"workspace": map[string]any{"type": "string", "default": "."},
					"priority":  map[string]any{"type": "integer", "default": 0},
					"retries":   map[string]any{"type": "integer", "default": 3},
					"criteria":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"prompt"},
			},
		},
		{
			name:        "sin_goal_list",
			description: "List goals in the autonomous goal queue. Filter by status: pending, running, verified, failed, exhausted.",
			handler:     handleGoalList,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "enum": []string{"pending", "running", "verified", "failed", "exhausted"}},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
			},
		},
		{
			name:        "sin_goal_status",
			description: "Show one goal's progress, attempts, and children (subtasks).",
			handler:     handleGoalStatus,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":     map[string]any{"type": "string"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"id"},
			},
		},
		{
			name:        "sin_goal_complete",
			description: "Mark a goal as verified/done. Maps to autonomy.Queue.Complete().",
			handler:     handleGoalComplete,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string"},
					"session": map[string]any{"type": "string"},
				},
				"required": []string{"id"},
			},
		},
	}

	// Apply the ponytail-tag compressor (issue #173) before registration
	// if any of --compress-tools / --print-stats / --compress-tags is set.
	// Tool names are public API (AGENTS.md §10) and are NEVER modified —
	// only the Description byte field is shrunk.
	if serveCompressTools || servePrintStats || serveCompressTags != "" {
		var pipeline mcpcompress.Pipeline
		if serveCompressTags != "" {
			pipeline = mcpcompress.Selected(mcpcompress.FromCSV(serveCompressTags).List())
		} else {
			pipeline = mcpcompress.All()
		}
		specs := make([]mcpcompress.Spec, len(tools))
		stats := make([]mcpcompress.Stats, len(tools))
		for i := range tools {
			specs[i] = mcpcompress.Spec{Name: tools[i].name, Description: tools[i].description}
			comp, st := mcpcompress.CompressSpec(specs[i], pipeline)
			tools[i].description = comp.Description
			stats[i] = st
			specs[i] = comp
		}
		if servePrintStats {
			printCompressionStats(os.Stderr, pipeline, stats)
		}
	}

	for _, t := range tools {
		tool := t
		server.AddTool(&mcp.Tool{
			Name:        tool.name,
			Description: tool.description,
			InputSchema: tool.schema,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := make(map[string]any)
			if req.Params.Arguments != nil {
				_ = json.Unmarshal(req.Params.Arguments, &args)
			}
			result, err := tool.handler(ctx, args)
			if err != nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("ERROR: %v", err)}},
					IsError: true,
				}, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: result}},
			}, nil
		})
	}

	// Plugin tools: each one becomes a sin_plugin_<plugin>_<tool> MCP tool
	// that exec's the plugin binary with the caller's args.
	registerPluginMCPTools(server)
}
