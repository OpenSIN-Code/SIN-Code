// SPDX-License-Identifier: MIT
// Purpose: production NewLoopFunc for the WebUI v2 HTTP API (issue #52).
// Lives in package main so it can call combinedTool/combinedSpecs from
// chat_mcp.go (which is also package main). The apiweb package cannot
// import those directly, so it expects the caller to inject this
// factory before serving any chat traffic.
package main

import (
	"context"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/apiweb"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
)

// newAPILoop is the production loop factory. It opens a fresh lessons
// store and delegates the rest of the wiring to loopbuilder. Mandate
// M4: Headless=true so the permission engine denies any "ask" rule and
// the daemon cannot self-escalate.
func newAPILoop(ctx context.Context, sessionID, workspace string) (*agentloop.Loop, func() error, error) {
	memStore, err := lessons.Open("")
	if err != nil {
		return nil, nil, fmt.Errorf("open lessons: %w", err)
	}
	return loopbuilder.Build(ctx, loopbuilder.Config{
		Workspace: workspace,
		SessionID: sessionID,
		Headless:  true,
		MaxTurns:  80,
		// chat_mcp.go merges builtin local tools with the
		// loopbuilder-managed external MCP servers.
		ToolFactory: func(mgr *mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {
			return combinedTool(mgr), combinedSpecs(mgr)
		},
	}, memStore)
}

// newAPIServerForServe is the construction point used by serve.go when
// --transport=http is passed. The returned *APIServer has its NewLoop
// field wired to the production factory.
func newAPIServerForServe(workspace string) *apiweb.APIServer {
	a := apiweb.NewAPIServer(workspace)
	a.NewLoop = newAPILoop
	return a
}

func init() {
	// Register the production NewLoop factory with the internal
	// serve package so --transport=http mounts a fully functional
	// /api/v1/chat SSE endpoint. Registration is idempotent — the
	// internal package refuses nil factories.
	if err := internal.RegisterHTTPLoopFactory(newAPILoop); err != nil {
		panic(fmt.Sprintf("serve_api_loop: %v", err))
	}
}
