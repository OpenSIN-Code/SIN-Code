// SPDX-License-Identifier: MIT
// Purpose: serve — registerAllMCPTools: registers all 54+ sin-code tools
// onto the MCP server, with optional ponytail-tag description compression.
// sin-debt: shrink, upgrade: consolidate when serve handlers are refactored
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpcompress"
)

// toolDef describes a single MCP tool for registration.
type toolDef struct {
	name        string
	description string
	handler     func(ctx context.Context, args map[string]any) (string, error)
	schema      map[string]any
}

func registerAllMCPTools(server *mcp.Server) {
	tools := []toolDef{}
	tools = append(tools, codeToolDefs()...)
	tools = append(tools, analysisToolDefs()...)
	tools = append(tools, todoToolDefs()...)
	tools = append(tools, memoryToolDefs()...)
	tools = append(tools, orchestratorToolDefs()...)
	tools = append(tools, autonomyToolDefs()...)

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
