// SPDX-License-Identifier: MIT
// Purpose: Register the production RunLoopFactory so the sin_run_loop
// MCP tool can call loopbuilder.Build in-process. Lives in package main
// to avoid the import cycle (internal ← loopbuilder ← internal).
package main

import (
	"context"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
)

func realRunLoopFactory(ctx context.Context, opts internal.RunLoopOptions, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
	cfg := loopbuilder.Config{
		Workspace:   opts.Workspace,
		Headless:    opts.Headless,
		VerifyMode:  opts.VerifyMode,
		VerifyCmd:   opts.VerifyCmd,
		MaxTurns:    opts.MaxTurns,
		Yolo:        opts.Yolo,
		Model:       opts.Model,
		Style:       opts.Style,
		AgentName:   opts.AgentName,
		SkipMCP:     opts.SkipMCP,
		Contract:    opts.Contract,
		ToolFactory: func(mgr *mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {
			return combinedTool(opts.Workspace, mgr), combinedSpecs(mgr)
		},
	}
	return loopbuilder.Build(ctx, cfg, memStore)
}

func init() {
	if err := internal.RegisterRunLoopFactory(realRunLoopFactory); err != nil {
		panic(fmt.Sprintf("serve_loop_factory: %v", err))
	}
}
