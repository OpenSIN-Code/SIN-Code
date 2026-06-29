// SPDX-License-Identifier: MIT
// Purpose: serve — start an MCP (Model Context Protocol) server that exposes
// all sin-code subcommands as MCP tools.
// MCP server registrations in opencode.json with a single one.
// Docs: serve.doc.md
package internal

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var (
	serveTransport     string
	servePort          int
	serveCompressTools bool
	servePrintStats    bool
	serveCompressTags  string
)

// ServerVersion is set at build time via -ldflags "-X github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal.ServerVersion=..."
var ServerVersion = "dev"

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start an MCP server exposing all 54+ sin-code tools",
	Long: `Start a Model Context Protocol (MCP) server that exposes all 54+ sin-code
subcommands as MCP tools. This allows opencode (and any MCP-compatible client)
to use sin-code as a single registered MCP server instead of registering
separate binaries.

	Note: config, self-update, and tui are CLI-only subcommands
	and are NOT exposed as MCP tools. The MCP server exposes 54+ analysis
	and manipulation tools.


Example opencode.json entry:

  "sin-code": {
    "command": ["/Users/jeremy/.local/bin/sin-code", "serve"],
		"description": "SIN-Code unified toolchain (52+ MCP tools)",
    "enabled": true,
    "type": "local"
  }

Then use sin_discover, sin_execute, sin_map, sin_grasp, sin_scout, sin_harvest,
sin_orchestrate, sin_ibd, sin_poc, sin_sckg, sin_adw, sin_oracle, sin_efm,
sin_security_scan, sin_sbom_generate, sin_run_loop, sin_goal_add, sin_edit,
sin_write, sin_read, sin_scout, sin_grasp, sin_analyse_image, and more as MCP tools.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		parent := cmd.Context()
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithCancel(parent)
		defer cancel()

		cfg, _ := LoadMergedConfig()
		{
			key := os.Getenv("SIN_LLM_API_KEY")
			if key == "" {
				key = os.Getenv("NVIDIA_API_KEY")
			}
			if key == "" {
				key = os.Getenv("OPENAI_API_KEY")
			}
			if key == "" {
				key = strings.TrimSpace(cfg.LLMAPIKey)
			}
			if key == "" {
				fmt.Fprintln(os.Stderr, "Warning: No LLM API key configured. sin-code serve will expose MCP tools but cannot run autonomous loops.")
			}
		}

		server := mcp.NewServer(&mcp.Implementation{
			Name:    "sin-code",
			Version: ServerVersion,
		}, &mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{
				Tools: &mcp.ToolCapabilities{},
			},
		})

		registerAllMCPTools(server)

		switch serveTransport {
		case "stdio":
			return server.Run(ctx, &mcp.StdioTransport{})
		case "http":
			return runHTTPTransport(ctx, server)
		}
		return fmt.Errorf("unsupported transport: %s (use stdio or http)", serveTransport)
	},
}

func init() {
	ServeCmd.Flags().StringVarP(&serveTransport, "transport", "t", "stdio", "Transport: stdio (default) or http (mounts /api/v1/* WebUI v2 endpoints)")
	ServeCmd.Flags().IntVarP(&servePort, "port", "p", 0, "Port (unused for stdio)")
	ServeCmd.Flags().BoolVar(&serveCompressTools, "compress-tools", false,
		"Compress MCP tool descriptions on the wire using the ponytail tag set "+
			"(delete|stdlib|native|yagni|shrink). Tool names, schemas, and behaviour "+
			"are unchanged (AGENTS.md §10). Saved bytes show up as less input per turn.")
	ServeCmd.Flags().StringVar(&serveCompressTags, "compress-tags", "",
		"Override the default ponytail tag set for --compress-tools (comma-separated, "+
			"e.g. \"delete,yagni,shrink\"). Unknown tags are dropped silently; use --print-stats "+
			"to see the active set per tool.")
	ServeCmd.Flags().BoolVar(&servePrintStats, "print-stats", false,
		"Print per-tool compression statistics to stderr (implies --compress-tools). "+
			"Format: name | original_bytes | compressed_bytes | bytes_saved | ratio.")
}
