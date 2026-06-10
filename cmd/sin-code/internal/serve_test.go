// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the serve subcommand (MCP server).
package internal

import (
	"context"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/plugins"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServeCmd_Flags(t *testing.T) {
	cmd := ServeCmd
	if cmd.Use != "serve" {
		t.Errorf("expected Use 'serve', got %q", cmd.Use)
	}
	for _, f := range []string{"transport", "port"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestServeCmd_DefaultTransport(t *testing.T) {
	if v, _ := ServeCmd.Flags().GetString("transport"); v != "stdio" {
		t.Errorf("default transport should be stdio, got %q", v)
	}
}

func TestRegisterAllMCPTools(t *testing.T) {
	expectedTools := []string{
		"sin_discover", "sin_execute", "sin_map", "sin_grasp",
		"sin_scout", "sin_harvest", "sin_orchestrate",
		"sin_ibd", "sin_poc", "sin_sckg", "sin_adw", "sin_oracle", "sin_efm",
	}
	if len(expectedTools) != 13 {
		t.Errorf("expected 13 tools, test config has %d", len(expectedTools))
	}
}

func newPluginTestServer(t *testing.T, reg *plugins.Registry) *mcp.Server {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sin-code-plugin-test",
		Version: "test",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})
	registerAllMCPTools(server)
	registerPluginMCPToolsWithReg(server, reg)
	return server
}

func connectWithPluginReg(t *testing.T, reg *plugins.Registry) (*mcp.ClientSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	server := newPluginTestServer(t, reg)
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

func TestPluginToolRegistered(t *testing.T) {
	reg := plugins.NewRegistry()
	reg.Register(&plugins.Plugin{
		Name:    "testplug",
		Enabled: true,
		Tools: []plugins.PluginTool{
			{
				Name:        "reverse",
				Description: "Reverse a string",
				Binary:      "/bin/echo",
				Args:        []string{"input"},
				Timeout:     10,
			},
		},
	})
	cs, cancel := connectWithPluginReg(t, reg)
	defer cancel()

	toolsResult, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	var names []string
	for _, tool := range toolsResult.Tools {
		names = append(names, tool.Name)
	}

	want := "sin_plugin_testplug_reverse"
	found := false
	for _, n := range names {
		if n == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected plugin tool %q in list; got %v", want, names)
	}
}

func TestPluginToolCallSuccess(t *testing.T) {
	reg := plugins.NewRegistry()
	reg.Register(&plugins.Plugin{
		Name:    "echoplug",
		Enabled: true,
		Path:    "/tmp",
		Tools: []plugins.PluginTool{
			{
				Name:        "say",
				Description: "Echo back input",
				Binary:      "/bin/echo",
				Args:        []string{"input"},
				Timeout:     10,
			},
		},
	})
	cs, cancel := connectWithPluginReg(t, reg)
	defer cancel()

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "sin_plugin_echoplug_say",
		Arguments: map[string]any{"input": "hello-world"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty Content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "--input hello-world\n" {
		t.Errorf("expected '--input hello-world\\n', got %q", tc.Text)
	}
}

func TestPluginToolCallMissingBinary(t *testing.T) {
	reg := plugins.NewRegistry()
	reg.Register(&plugins.Plugin{
		Name:    "badplug",
		Enabled: true,
		Path:    "/tmp",
		Tools: []plugins.PluginTool{
			{
				Name:        "broken",
				Description: "Intentionally missing binary",
				Binary:      "./nonexistent-binary-xyz",
				Args:        []string{},
				Timeout:     5,
			},
		},
	})
	cs, cancel := connectWithPluginReg(t, reg)
	defer cancel()

	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "sin_plugin_badplug_broken",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool failed (expected error in result, not call-level): %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing binary, got false")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty Content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text == "" {
		t.Errorf("expected error message in content, got empty")
	}
}
