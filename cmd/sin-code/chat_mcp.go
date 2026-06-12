// SPDX-License-Identifier: MIT
// Purpose: merge builtin local tools with external MCP tools
// ("server__tool" namespacing, mandate C5) into the single
// LocalTool/LocalSpec surface that agentloop consumes.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
)

func combinedSpecs(mgr *mcpclient.Manager) []agentloop.ToolSpec {
	specs := builtinSpecs()
	for _, t := range mgr.Tools() {
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

func combinedTool(mgr *mcpclient.Manager) agentloop.LocalToolFunc {
	return func(ctx context.Context, name string, args map[string]any) (string, error) {
		if strings.Contains(name, "__") {
			return mgr.Call(ctx, name, args)
		}
		return builtinTool(ctx, name, args)
	}
}
