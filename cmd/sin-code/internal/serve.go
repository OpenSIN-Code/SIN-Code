// SPDX-License-Identifier: MIT
// Purpose: serve — start an MCP (Model Context Protocol) server that exposes
// all 13 sin-code subcommands as MCP tools. This replaces the 7 separate
// MCP server registrations in opencode.json with a single one.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/plugins"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var (
	serveTransport string
	servePort      int
)

// ServerVersion is set at build time via -ldflags "-X github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal.ServerVersion=..."
var ServerVersion = "dev"

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start an MCP server exposing all 13 sin-code tools",
	Long: `Start a Model Context Protocol (MCP) server that exposes all 13 sin-code
subcommands as MCP tools. This allows opencode (and any MCP-compatible client)
to use sin-code as a single registered MCP server instead of registering 13
separate binaries.

Note: security, sbom, config, self-update, and tui are CLI-only subcommands
and are NOT exposed as MCP tools. The MCP server only exposes the 13 core
analysis tools listed below.

Example opencode.json entry:

  "sin-code": {
    "command": ["/Users/jeremy/.local/bin/sin-code", "serve"],
    "description": "SIN-Code unified toolchain (13 MCP tools)",
    "enabled": true,
    "type": "local"
  }

Then use sin_discover, sin_execute, sin_map, sin_grasp, sin_scout, sin_harvest,
sin_orchestrate, sin_ibd, sin_poc, sin_sckg, sin_adw, sin_oracle, sin_efm as
MCP tools.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := mcp.NewServer(&mcp.Implementation{
			Name:    "sin-code",
			Version: ServerVersion,
		}, &mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{
				Tools: &mcp.ToolCapabilities{},
			},
		})

		registerAllMCPTools(server)

		if serveTransport == "stdio" {
			return server.Run(ctx, &mcp.StdioTransport{})
		}
		return fmt.Errorf("unsupported transport: %s (only stdio supported)", serveTransport)
	},
}

func init() {
	ServeCmd.Flags().StringVarP(&serveTransport, "transport", "t", "stdio", "Transport: stdio")
	ServeCmd.Flags().IntVarP(&servePort, "port", "p", 0, "Port (unused for stdio)")
}

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
					"tag":    map[string]any{"type": "string"},
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
					"kvs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
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

func registerPluginMCPTools(server *mcp.Server) {
	reg := plugins.NewRegistry()
	_ = reg.LoadFromDir("")
	for _, pt := range reg.MCPTools() {
		pt := pt
		server.AddTool(&mcp.Tool{
			Name:        pt.Name,
			Description: pt.Description,
			InputSchema: pt.Schema,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := make(map[string]any)
			if req.Params.Arguments != nil {
				_ = json.Unmarshal(req.Params.Arguments, &args)
			}
			result, err := runPluginMCPTool(ctx, pt, args)
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
}

// runPluginMCPTool exec's a plugin binary with the caller's args. Binary
// path is resolved relative to the plugin dir; stdout/stderr are merged
// and returned as a string. Timeout defaults to 60s.
func runPluginMCPTool(ctx context.Context, pt plugins.MCPToolDef, args map[string]any) (string, error) {
	plugin, ok := pluginLookup(pt.Plugin)
	if !ok {
		return "", fmt.Errorf("plugin %q not loaded", pt.Plugin)
	}
	fullPath := pt.Binary
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(plugin.Path, fullPath)
	}
	cmdArgs := make([]string, 0, len(pt.Args)+len(args))
	for _, a := range pt.Args {
		cmdArgs = append(cmdArgs, "--"+a)
		if v, ok := args[a]; ok {
			cmdArgs = append(cmdArgs, fmt.Sprintf("%v", v))
		}
	}
	timeout := time.Duration(pt.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(execCtx, fullPath, cmdArgs...)
	c.Dir = plugin.Path
	c.Env = append(os.Environ(), "SIN_PLUGIN="+pt.Plugin, "SIN_PLUGIN_TOOL="+pt.Tool)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s\nERROR: %v", string(out), err), nil
	}
	return string(out), nil
}

func pluginLookup(name string) (*plugins.Plugin, bool) {
	reg := plugins.NewRegistry()
	_ = reg.LoadFromDir("")
	return reg.Get(name)
}

func handleDiscover(ctx context.Context, args map[string]any) (string, error) {
	// discover takes path as positional argument, not --path
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"discover", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
		case bool:
			if val {
				cmdArgs = append(cmdArgs, "--"+k)
			}
		case float64:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%v", int(val)))
		case int:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%d", val))
		}
	}
	return runSubcommandRaw(ctx, cmdArgs)
}

func handleExecute(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "execute", args)
}

func handleMap(ctx context.Context, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"map", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
		case bool:
			if val {
				cmdArgs = append(cmdArgs, "--"+k)
			}
		case float64:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%v", int(val)))
		case int:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%d", val))
		}
	}
	return runSubcommandRaw(ctx, cmdArgs)
}

func handleGrasp(ctx context.Context, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"grasp", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
		case bool:
			if val {
				cmdArgs = append(cmdArgs, "--"+k)
			}
		case float64:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%v", int(val)))
		case int:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%d", val))
		}
	}
	return runSubcommandRaw(ctx, cmdArgs)
}

func handleScout(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "scout", args)
}

func handleHarvest(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "harvest", args)
}

func handleOrchestrate(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "orchestrate", args)
}

func handleIbd(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "ibd", args)
}

func handlePoc(ctx context.Context, args map[string]any) (string, error) {
	code := "."
	if c, ok := args["code"].(string); ok && c != "" {
		code = c
		delete(args, "code")
	} else if s, ok := args["spec"].(string); ok && s != "" {
		code = s
		delete(args, "spec")
	}
	cmdArgs := []string{"poc", code}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
		case bool:
			if val {
				cmdArgs = append(cmdArgs, "--"+k)
			}
		case float64:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%v", int(val)))
		case int:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%d", val))
		}
	}
	return runSubcommandRaw(ctx, cmdArgs)
}

func handleSckg(ctx context.Context, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"sckg", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
		case bool:
			if val {
				cmdArgs = append(cmdArgs, "--"+k)
			}
		case float64:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%v", int(val)))
		case int:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%d", val))
		}
	}
	return runSubcommandRaw(ctx, cmdArgs)
}

func handleAdw(ctx context.Context, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"adw", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
		case bool:
			if val {
				cmdArgs = append(cmdArgs, "--"+k)
			}
		case float64:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%v", int(val)))
		case int:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%d", val))
		}
	}
	return runSubcommandRaw(ctx, cmdArgs)
}

func handleOracle(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "oracle", args)
}

func handleEfm(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "efm", args)
}

func runSubcommand(ctx context.Context, name string, args map[string]any) (string, error) {
	cmdArgs := []string{name}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val == "" {
				continue
			}
			cmdArgs = append(cmdArgs, "--"+k, val)
		case bool:
			if val {
				cmdArgs = append(cmdArgs, "--"+k)
			}
		case float64:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%v", int(val)))
		case int:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%d", val))
		}
	}
	return runSubcommandRaw(ctx, cmdArgs)
}

func runSubcommandRaw(ctx context.Context, cmdArgs []string) (string, error) {
	selfPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot find self: %w", err)
	}

	c := exec.CommandContext(ctx, selfPath, cmdArgs...)
	c.Env = os.Environ()
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s\nERROR: %v", string(out), err), nil
	}
	return string(out), nil
}

func init() {
	_ = filepath.Separator
	_ = strings.Builder{}
	_ = time.Now
}
