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

Example opencode.json entry:

  "sin-code": {
    "command": ["/Users/jeremy/.local/bin/sin-code", "serve"],
    "description": "SIN-Code unified toolchain (13 tools)",
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
