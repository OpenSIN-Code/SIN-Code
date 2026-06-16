// SPDX-License-Identifier: MIT
// Purpose: smoke tests for internal/mcpclient (issue #50).
// Verifies the "additive, never fatal" guarantee: unreachable servers
// are logged to stderr and skipped, ConnectAll never returns an error.
package mcpclient

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// captureStderr redirects os.Stderr for the duration of fn.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = buf.ReadFrom(r)
	}()
	fn()
	w.Close()
	wg.Wait()
	return buf.String()
}

func TestConnectAllUnreachableServerIsAdditiveNeverFatal(t *testing.T) {
	mgr := NewManager([]ServerConfig{
		{Name: "ghost-http", Transport: "http", URL: "http://127.0.0.1:1/mcp"},
		{Name: "ghost-stdio", Transport: "stdio", Command: "/nonexistent/sin-mcp-binary"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	logged := captureStderr(t, func() {
		err = mgr.ConnectAll(ctx)
	})

	if err != nil {
		t.Fatalf("ConnectAll must never be fatal, got: %v", err)
	}
	for _, name := range []string{"ghost-http", "ghost-stdio"} {
		if !strings.Contains(logged, name) {
			t.Errorf("expected stderr warning for %q, got: %s", name, logged)
		}
	}
	if tools := mgr.Tools(); len(tools) != 0 {
		t.Fatalf("expected 0 tools from unreachable servers, got %d", len(tools))
	}
}

func TestConnectUnknownTransportIsLogged(t *testing.T) {
	mgr := NewManager([]ServerConfig{
		{Name: "bad", Transport: "carrier-pigeon"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	logged := captureStderr(t, func() { err = mgr.ConnectAll(ctx) })
	if err != nil {
		t.Fatalf("ConnectAll must never be fatal, got: %v", err)
	}
	if !strings.Contains(logged, "unknown transport") {
		t.Errorf("expected 'unknown transport' warning, got: %s", logged)
	}
}

func TestCallRoutingErrors(t *testing.T) {
	mgr := NewManager(nil)
	ctx := context.Background()

	if _, err := mgr.Call(ctx, "sin_read", nil); err == nil ||
		!strings.Contains(err.Error(), "not an external tool") {
		t.Fatalf("expected 'not an external tool' error, got: %v", err)
	}
	if _, err := mgr.Call(ctx, "ghost__do_thing", nil); err == nil ||
		!strings.Contains(err.Error(), `no MCP session for server "ghost"`) {
		t.Fatalf("expected 'no MCP session' error, got: %v", err)
	}
}

func TestToolsReturnsCopy(t *testing.T) {
	mgr := NewManager(nil)
	mgr.tools = []Tool{{Server: "s", Name: "t", Qualified: "s__t"}}
	got := mgr.Tools()
	got[0].Name = "mutated"
	if mgr.tools[0].Name != "t" {
		t.Fatal("Tools() must return a defensive copy")
	}
}

func TestToolsConcurrentAccessRaceClean(t *testing.T) {
	mgr := NewManager(nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = mgr.Tools() }()
		go func() {
			defer wg.Done()
			mgr.mu.Lock()
			mgr.tools = append(mgr.tools, Tool{Server: "x", Name: "y", Qualified: "x__y"})
			mgr.mu.Unlock()
		}()
	}
	wg.Wait()
}

// startTestServer starts an MCP server on an in-memory transport and returns
// a hook that hands the paired client-side transport to Manager.connect.
func startTestServer(t *testing.T, srv *sdk.Server) func(context.Context, ServerConfig) (sdk.Transport, error) {
	t.Helper()
	serverTrans, clientTrans := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	var runDone sync.WaitGroup
	runDone.Add(1)
	go func() {
		defer runDone.Done()
		_ = srv.Run(ctx, serverTrans)
	}()
	t.Cleanup(func() {
		cancel()
		runDone.Wait()
	})
	return func(context.Context, ServerConfig) (sdk.Transport, error) {
		return clientTrans, nil
	}
}

func TestConnectAllSuccessful(t *testing.T) {
	orig := connectTransportHook
	t.Cleanup(func() { connectTransportHook = orig })

	srv := sdk.NewServer(&sdk.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	sdk.AddTool(srv, &sdk.Tool{Name: "greet", Description: "say hi"}, func(ctx context.Context, _ *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "hello"}}}, nil, nil
	})
	connectTransportHook = startTestServer(t, srv)

	mgr := NewManager([]ServerConfig{{Name: "test", Transport: "stdio", Command: "unused", Env: map[string]string{"K": "V"}}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	tools := mgr.Tools()
	if len(tools) != 1 || tools[0].Qualified != "test__greet" {
		t.Fatalf("expected 1 tool, got %+v", tools)
	}
}

func TestConnectAllSuccessfulHTTP(t *testing.T) {
	orig := connectTransportHook
	t.Cleanup(func() { connectTransportHook = orig })

	srv := sdk.NewServer(&sdk.Implementation{Name: "test-http", Version: "1.0.0"}, nil)
	sdk.AddTool(srv, &sdk.Tool{Name: "ping", Description: "ping"}, func(ctx context.Context, _ *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "pong"}}}, nil, nil
	})
	connectTransportHook = startTestServer(t, srv)

	mgr := NewManager([]ServerConfig{{Name: "http", Transport: "http", URL: "http://unused"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if tools := mgr.Tools(); len(tools) != 1 || tools[0].Name != "ping" {
		t.Fatalf("expected ping tool, got %+v", tools)
	}
}

func TestConnectAllDuplicateWarning(t *testing.T) {
	orig := connectTransportHook
	t.Cleanup(func() { connectTransportHook = orig })
	origWarned := warnedServers
	t.Cleanup(func() { warnedServers = origWarned })

	warnedServers = map[string]bool{"dup": true}
	connectTransportHook = func(context.Context, ServerConfig) (sdk.Transport, error) {
		return nil, errors.New("boom")
	}

	mgr := NewManager([]ServerConfig{{Name: "dup", Transport: "stdio", Command: "unused"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logged := captureStderr(t, func() { _ = mgr.ConnectAll(ctx) })
	if logged != "" {
		t.Fatalf("expected no duplicate warning, got: %q", logged)
	}
}

func TestConnectAllListToolsError(t *testing.T) {
	orig := connectTransportHook
	t.Cleanup(func() { connectTransportHook = orig })
	origListTools := listToolsHook
	t.Cleanup(func() { listToolsHook = origListTools })

	srv := sdk.NewServer(&sdk.Implementation{Name: "no-tools", Version: "1.0.0"}, nil)
	connectTransportHook = startTestServer(t, srv)
	listToolsHook = func(context.Context, *sdk.ClientSession) (*sdk.ListToolsResult, error) {
		return nil, errors.New("list tools boom")
	}

	mgr := NewManager([]ServerConfig{{Name: "no-tools", Transport: "stdio", Command: "unused"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logged := captureStderr(t, func() { _ = mgr.ConnectAll(ctx) })
	if !strings.Contains(logged, "no-tools") {
		t.Fatalf("expected warning for no-tools, got: %s", logged)
	}
}

func TestCallSuccessful(t *testing.T) {
	orig := connectTransportHook
	t.Cleanup(func() { connectTransportHook = orig })

	srv := sdk.NewServer(&sdk.Implementation{Name: "call", Version: "1.0.0"}, nil)
	sdk.AddTool(srv, &sdk.Tool{Name: "echo", Description: "echo"}, func(ctx context.Context, req *sdk.CallToolRequest, args any) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "echoed"}}}, nil, nil
	})
	connectTransportHook = startTestServer(t, srv)

	mgr := NewManager([]ServerConfig{{Name: "call", Transport: "stdio", Command: "unused"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	out, err := mgr.Call(ctx, "call__echo", map[string]any{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out != "echoed" {
		t.Fatalf("expected echoed, got %q", out)
	}
}

func TestCallToolError(t *testing.T) {
	orig := connectTransportHook
	t.Cleanup(func() { connectTransportHook = orig })

	srv := sdk.NewServer(&sdk.Implementation{Name: "err", Version: "1.0.0"}, nil)
	sdk.AddTool(srv, &sdk.Tool{Name: "ok", Description: "ok"}, func(ctx context.Context, req *sdk.CallToolRequest, args any) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "ok"}}}, nil, nil
	})
	connectTransportHook = startTestServer(t, srv)

	mgr := NewManager([]ServerConfig{{Name: "err", Transport: "stdio", Command: "unused"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if _, err := mgr.Call(ctx, "err__missing", map[string]any{}); err == nil {
		t.Fatal("expected error for missing tool")
	}
}

func TestCallToolReturnsError(t *testing.T) {
	orig := connectTransportHook
	t.Cleanup(func() { connectTransportHook = orig })

	srv := sdk.NewServer(&sdk.Implementation{Name: "tool-err", Version: "1.0.0"}, nil)
	sdk.AddTool(srv, &sdk.Tool{Name: "fail", Description: "fail"}, func(ctx context.Context, req *sdk.CallToolRequest, args any) (*sdk.CallToolResult, any, error) {
		return nil, nil, errors.New("tool failure")
	})
	connectTransportHook = startTestServer(t, srv)

	mgr := NewManager([]ServerConfig{{Name: "tool-err", Transport: "stdio", Command: "unused"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if _, err := mgr.Call(ctx, "tool-err__fail", map[string]any{}); err == nil {
		t.Fatal("expected error for tool failure")
	}
}

func TestIsExternal(t *testing.T) {
	mgr := NewManager(nil)
	if !mgr.IsExternal("server__tool") {
		t.Error("expected server__tool to be external")
	}
	if mgr.IsExternal("sin_read") {
		t.Error("expected sin_read not to be external")
	}
}

func TestClose(t *testing.T) {
	orig := connectTransportHook
	t.Cleanup(func() { connectTransportHook = orig })

	srv := sdk.NewServer(&sdk.Implementation{Name: "close", Version: "1.0.0"}, nil)
	sdk.AddTool(srv, &sdk.Tool{Name: "x", Description: "x"}, func(ctx context.Context, _ *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "x"}}}, nil, nil
	})
	connectTransportHook = startTestServer(t, srv)

	mgr := NewManager([]ServerConfig{{Name: "close", Transport: "stdio", Command: "unused"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	mgr.Close()
	if _, err := mgr.Call(ctx, "close__x", nil); err == nil {
		t.Fatal("expected error after close")
	}
}
