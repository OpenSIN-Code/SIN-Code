// SPDX-License-Identifier: MIT
// Purpose: serve — plugin MCP tool registration and execution.
// sin-debt: shrink, upgrade: when a second plugin-related function is needed, merge into a shared file
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpcompress"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/plugins"
)

func registerPluginMCPTools(server *mcp.Server) {
	reg := plugins.NewRegistry()
	_ = reg.LoadFromDir("")
	registerPluginMCPToolsWithReg(server, reg)
}

func registerPluginMCPToolsWithReg(server *mcp.Server, reg *plugins.Registry) {
	// Plugin tools share the same compression pipeline (issue #173).
	// Compressed descriptions only — schema is untouched.
	var pipeline mcpcompress.Pipeline
	if serveCompressTools || servePrintStats || serveCompressTags != "" {
		if serveCompressTags != "" {
			pipeline = mcpcompress.Selected(mcpcompress.FromCSV(serveCompressTags).List())
		} else {
			pipeline = mcpcompress.All()
		}
	}

	for _, pt := range reg.MCPTools() {
		pt := pt
		if pipeline != nil {
			compressed, _ := mcpcompress.CompressSpec(
				mcpcompress.Spec{Name: pt.Name, Description: pt.Description}, pipeline)
			pt.Description = compressed.Description
		}
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
	fullPath := pt.Binary
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(pt.PluginPath, fullPath)
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
	c.Dir = pt.PluginPath
	c.Env = append(os.Environ(), "SIN_PLUGIN="+pt.Plugin, "SIN_PLUGIN_TOOL="+pt.Tool)
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("plugin tool %q: %w", pt.Name, err)
	}
	return string(out), nil
}
