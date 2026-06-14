// SPDX-License-Identifier: MIT
// Purpose: tests for registerAllMCPTools and plugin tool dispatch.
package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/plugins"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestServerWithBinary registers all MCP tools and points subcommand
// dispatch at the fake binary provided by the caller.
func newTestServerWithBinary(t *testing.T, fakeBin string) *mcp.Server {
	t.Setenv("SIN_CODE_BIN", fakeBin)
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sin-code",
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})
	registerAllMCPTools(server)
	return server
}

// connectTestServer wires an MCP client to the server via in-memory transports.
func connectTestServer(t *testing.T, server *mcp.Server) (*mcp.ClientSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	cTransport, sTransport := mcp.NewInMemoryTransports()

	ss, err := server.Connect(ctx, sTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.1.0",
	}, nil)
	cs, err := client.Connect(ctx, cTransport, nil)
	if err != nil {
		cancel()
		ss.Close()
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	return cs, cancel
}

// fakeSinCodeBinary writes a script that returns a fixed JSON response and
// records the arguments it received (appended to the path's args.log).
func fakeSinCodeBinary(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "sin-code")
	script := `#!/bin/sh
# Record the invocation so tests can inspect it.
echo "$*" >> "` + dir + `/args.log"
# Return a generic JSON payload that every handler can pass through.
echo '{"ok":true}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}

func TestRegisterAllMCPTools_CoreToolSuccess(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSinCodeBinary(t, dir)

	server := newTestServerWithBinary(t, bin)
	cs, cancel := connectTestServer(t, server)
	defer cancel()

	ctx := context.Background()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sin_discover",
		Arguments: map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &parsed); err != nil {
		t.Fatalf("response not JSON: %q", tc.Text)
	}
	if parsed["ok"] != true {
		t.Fatalf("unexpected response: %v", parsed)
	}

	logData, _ := os.ReadFile(filepath.Join(dir, "args.log"))
	if len(logData) == 0 {
		t.Fatal("fake binary was not invoked")
	}
	if string(logData)[:8] != "discover" {
		t.Fatalf("expected 'discover ...' logged, got %q", logData)
	}
}

func TestRegisterAllMCPTools_CoreToolErrorPath(t *testing.T) {
	dir := t.TempDir()
	bin := fakeSinCodeBinary(t, dir)

	server := newTestServerWithBinary(t, bin)
	cs, cancel := connectTestServer(t, server)
	defer cancel()

	ctx := context.Background()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sin_todo_add",
		Arguments: map[string]any{}, // missing required title
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing title")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected error content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text[:6] != "ERROR:" {
		t.Fatalf("expected ERROR prefix, got %q", tc.Text)
	}
}

func TestRegisterAllMCPTools_PluginTool(t *testing.T) {
	dir := t.TempDir()
	pluginBin := filepath.Join(dir, "plugin-tool")
	if err := os.WriteFile(pluginBin, []byte("#!/bin/sh\necho '{\"plugin\":true}'\n"), 0o755); err != nil {
		t.Fatalf("write plugin binary: %v", err)
	}

	reg := plugins.NewRegistry()
	reg.Register(&plugins.Plugin{
		Name:    "test",
		Path:    dir,
		Enabled: true,
		Tools: []plugins.PluginTool{
			{
				Name:        "echo",
				Description: "Test plugin tool",
				Binary:      pluginBin,
				Args:        []string{},
				Timeout:     5,
			},
		},
	})

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sin-code",
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})
	registerPluginMCPToolsWithReg(server, reg)

	cs, cancel := connectTestServer(t, server)
	defer cancel()

	ctx := context.Background()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sin_plugin_test_echo",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "{\"plugin\":true}\n" {
		t.Fatalf("unexpected plugin output: %q", tc.Text)
	}
}

func TestRegisterAllMCPTools_PluginToolError(t *testing.T) {
	dir := t.TempDir()
	pluginBin := filepath.Join(dir, "plugin-tool")
	if err := os.WriteFile(pluginBin, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write plugin binary: %v", err)
	}

	reg := plugins.NewRegistry()
	reg.Register(&plugins.Plugin{
		Name:    "test",
		Path:    dir,
		Enabled: true,
		Tools: []plugins.PluginTool{
			{
				Name:        "fail",
				Description: "Test plugin tool that fails",
				Binary:      pluginBin,
				Args:        []string{},
				Timeout:     5,
			},
		},
	})

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sin-code",
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})
	registerPluginMCPToolsWithReg(server, reg)

	cs, cancel := connectTestServer(t, server)
	defer cancel()

	ctx := context.Background()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sin_plugin_test_fail",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for failing plugin")
	}
}
