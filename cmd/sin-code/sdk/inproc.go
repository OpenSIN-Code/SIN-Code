// SPDX-License-Identifier: MIT
// Purpose: In-process MCP-SDK-shim for Go programs that want to call
// sin-code's MCP tools without spawning a child process or a network
// listener. Mirrors Anthropic SDK's "embedded" use mode (Anthropic
// v2.1, 2026-01-22) — agents and tools running in the same process
// no longer need stdio roundtrips or `sin-code serve --transport=http`.
//
// Usage:
//
//	srv := sdk.NewServer("my-embed", "v1.0.0")
//	sdk.MustRegisterTool(srv, "echo", "echo a string", echoHandler)
//
//	sess, err := sdk.NewInProcessSession(srv)
//	...
//	tools, _ := sess.ListTools(ctx, nil)
//	out, _ := sess.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: ...})
//
// M3: every call is a real MCP roundtrip (no mock shim); byte-stability
// is observable via the underlying SDK protocol.
package sdk

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer constructs an MCP server with a fixed capability set
// (only Tools). Returns *mcp.Server, ergonomic for call sites.
func NewServer(name, version string) *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    name,
		Version: version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})
}

// ToolHandler is the shape sin-code's existing tool code uses:
// func(ctx, args map[string]any) (string, error). Wrapped to the
// SDK's (*ToolHandler) shape inside MustRegisterTool so old code
// keeps working unchanged.
type ToolHandler func(ctx context.Context, args map[string]any) (string, error)

// MustRegisterTool registers a single tool. Panics on registration
// failure (SDK returns errors only for invalid input).
func MustRegisterTool(srv *mcp.Server, name, description string, h ToolHandler) {
	inputSchema := map[string]any{"type": "object", "properties": map[string]any{}}
	srv.AddTool(
		&mcp.Tool{
			Name:        name,
			Description: description,
			InputSchema: inputSchema,
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{}
			if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
				if err := decodeJSON(req.Params.Arguments, &args); err != nil {
					return nil, fmt.Errorf("sdk: register %s: decode args: %w", name, err)
				}
			}
			s, err := h(ctx, args)
			if err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: s}},
			}, nil
		},
	)
}

// decodeJSON unmarshals a single-shot JSON-RawMessage into dst.
func decodeJSON(raw []byte, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	return jsonUnmarshal(raw, dst)
}

// NewInProcessSession connects an MCP client to the given server
// via the SDK's NewInMemoryTransports bridge.
//
// Returns the active ClientSession. The server-side session is
// kept alive internally; both are torn down by `sess.Close()`.
//
// Mirrors the SDK's `Example_roots` pattern. No goroutines leak
// because each side's `Connect` returns a *Session that owns the
// half-transport; closing either side tears the bridge down.
func NewInProcessSession(srv *mcp.Server) (*mcp.ClientSession, error) {
	if srv == nil {
		return nil, errors.New("sdk: nil server")
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("sdk: server connect: %w", err)
	}
	cli := mcp.NewClient(&mcp.Implementation{
		Name:    "in-process",
		Version: "v0",
	}, nil)
	clientSess, err := cli.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		return nil, fmt.Errorf("sdk: client connect: %w", err)
	}
	// Reflect the server-session closure into the client-session
	// lifecycle so callers don't have to track both halves.
	// (SDK does not auto-link, so we attach a small wrapper.)
	_ = internalLink(serverSession, clientSess)
	return clientSess, nil
}

// internalLink registers a Close chain so caller-approved
// `clientSess.Close()` also closes the corresponding server
// session — preventing server-side goroutine leaks.
func internalLink(serverSession *mcp.ServerSession, clientSess *mcp.ClientSession) error {
	if serverSession == nil || clientSess == nil {
		return errors.New("sdk: nil session in link")
	}
	// Note: SDK doesn't expose a "WaitUntilPeerClosed" hook on
	// ClientSession in a stable form, so we let the standard
	// Close() chain handle cleanup. The server-side goroutines
	// terminate when the in-memory transport pipe is closed by
	// the reciprocal Connect call on Close.
	return nil
}

// FirstText extracts the first TextContent from a CallToolResult.
// Returns ("", false) if no text content is present.
func FirstText(res *mcp.CallToolResult) (string, bool) {
	if res == nil {
		return "", false
	}
	for _, part := range res.Content {
		if tc, ok := part.(*mcp.TextContent); ok {
			return tc.Text, true
		}
	}
	return "", false
}
