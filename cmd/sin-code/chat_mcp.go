// SPDX-License-Identifier: MIT
// Purpose: merge builtin local tools with external MCP tools
// ("server__tool" namespacing, mandate C5) into the single
// LocalTool/LocalSpec surface that agentloop consumes.
// Issue #270: when --lazy-tools is enabled, only a tool_search meta-tool
// is sent initially; the LLM discovers real tools on demand.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
)

// mcp hook variables — injected by coverage tests to mock external MCP calls.
var (
	mcpManagerToolsFn = func(mgr *mcpclient.Manager) []mcpclient.Tool { return mgr.Tools() }
	mcpManagerCallFn  = func(mgr *mcpclient.Manager, ctx context.Context, name string, args map[string]any) (string, error) {
		return mgr.Call(ctx, name, args)
	}
)

func combinedSpecs(mgr *mcpclient.Manager) []agentloop.ToolSpec {
	specs := builtinSpecs()
	for _, t := range mcpManagerToolsFn(mgr) {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		desc := t.Description
		if desc == "" {
			desc = fmt.Sprintf("External MCP tool %s on server %s", t.Name, t.Server)
		}
		specs = append(specs, agentloop.ToolSpec{
			Name:        t.Qualified,
			Description: desc,
			InputSchema: schema,
		})
	}
	return specs
}

func combinedTool(workspace string, mgr *mcpclient.Manager) agentloop.LocalToolFunc {
	return func(ctx context.Context, name string, args map[string]any) (string, error) {
		if strings.Contains(name, "__") {
			return mcpManagerCallFn(mgr, ctx, name, args)
		}
		return builtinTool(ctx, workspace, name, args)
	}
}

// lazyCombinedSpecs returns only the tool_search meta-tool spec. The LLM
// calls tool_search to discover and load real tool definitions on demand,
// reducing tool-prompt tokens from ~134K to ~5K (issue #270).
func lazyCombinedSpecs() []agentloop.ToolSpec {
	s := mcpclient.ToolSearchSpec()
	return []agentloop.ToolSpec{{
		Name:        s.Name,
		Description: s.Description,
		InputSchema: s.InputSchema,
	}}
}

// lazyCombinedTool wraps the base combined tool dispatcher with tool_search
// handling. When the LLM calls tool_search, matching specs are returned as
// JSON and simultaneously appended to the loop's LocalSpec so they become
// callable on subsequent turns. Duplicate additions are suppressed.
// Thread-safe (mandate M7).
func lazyCombinedTool(
	workspace string,
	mgr *mcpclient.Manager,
	loader *mcpclient.LazyToolLoader,
	loop *agentloop.Loop,
) agentloop.LocalToolFunc {
	base := combinedTool(workspace, mgr)
	var mu sync.Mutex
	seen := map[string]bool{"tool_search": true}

	return func(ctx context.Context, name string, args map[string]any) (string, error) {
		if name != "tool_search" {
			return base(ctx, name, args)
		}

		query := argStr(args, "query")
		limit := 10
		switch v := args["limit"].(type) {
		case float64:
			if v > 0 {
				limit = int(v)
			}
		case int:
			if v > 0 {
				limit = v
			}
		}

		results := loader.Search(query, limit)
		agentSpecs := make([]agentloop.ToolSpec, 0, len(results))
		for _, s := range results {
			agentSpecs = append(agentSpecs, agentloop.ToolSpec{
				Name:        s.Name,
				Description: s.Description,
				InputSchema: s.InputSchema,
			})
		}

		mu.Lock()
		for _, s := range agentSpecs {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			loop.LocalSpec = append(loop.LocalSpec, s)
		}
		mu.Unlock()

		data, err := json.Marshal(agentSpecs)
		if err != nil {
			return "", fmt.Errorf("tool_search: marshal: %w", err)
		}
		return string(data), nil
	}
}

// allSpecsAsMCPClient builds the full tool set as mcpclient.ToolSpec for
// indexing by the LazyToolLoader.
func allSpecsAsMCPClient(mgr *mcpclient.Manager) []mcpclient.ToolSpec {
	agentSpecs := combinedSpecs(mgr)
	out := make([]mcpclient.ToolSpec, len(agentSpecs))
	for i, s := range agentSpecs {
		out[i] = mcpclient.ToolSpec{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: s.InputSchema,
		}
	}
	return out
}
