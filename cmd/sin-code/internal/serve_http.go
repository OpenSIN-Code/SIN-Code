// SPDX-License-Identifier: MIT
// Purpose: serve — HTTP transport for the WebUI v2 API (issue #52).
// sin-debt: shrink, upgrade: when a second http-related function is needed, merge into a shared file
package internal

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/apiweb"
)

var (
	osGetwd        = os.Getwd
	httpServerHook func(*http.Server)
)

// httpLoopFactory is the production NewLoopFunc used by runHTTPTransport.
// It is implemented in serve_api_loop.go (package main) and assigned
// from main.go's wireServeLoop(); this declaration lives here so the
// internal package compiles even if the wire-up is missing.
var httpLoopFactory apiweb.NewLoopFunc

// RegisterHTTPLoopFactory is called by main.go to wire the production
// NewLoop factory for the WebUI v2 chat endpoint. Returns an error if
// factory is nil (refusing to register a misconfigured handler).
func RegisterHTTPLoopFactory(factory apiweb.NewLoopFunc) error {
	if factory == nil {
		return fmt.Errorf("RegisterHTTPLoopFactory: nil factory")
	}
	httpLoopFactory = factory
	return nil
}

// runHTTPTransport starts an *http.Server on servePort that mounts the
// WebUI v2 HTTP API (issue #52) at /api/v1/*. The MCP server itself is
// not exposed over HTTP in this version — stdio remains the canonical
// MCP transport; the HTTP listener is for the WebUI frontend only.
// Auth: bearer token via SIN_API_TOKEN, or loopback-only when unset.
func runHTTPTransport(ctx context.Context, _ *mcp.Server) error {
	workspace, err := osGetwd()
	if err != nil {
		return err
	}
	api := apiweb.NewAPIServer(workspace)
	api.NewLoop = httpLoopFactory
	mux := http.NewServeMux()
	api.Routes(mux)
	mux.HandleFunc("GET /api/v1/health", serveHealthHandler)
	addr := fmt.Sprintf(":%d", servePort)
	srv := &http.Server{Addr: addr, Handler: mux}
	if httpServerHook != nil {
		httpServerHook(srv)
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	fmt.Fprintf(os.Stderr, "sin-code serve: HTTP API listening on %s (token=%q)\n", addr, api.Token)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errc:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func serveHealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","transport":"http"}`)
}
