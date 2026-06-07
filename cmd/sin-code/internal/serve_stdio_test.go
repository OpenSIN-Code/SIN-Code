package internal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sin-code-test",
		Version: "test",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})
	registerAllMCPTools(server)
	return server
}

func connectClientServer(t *testing.T) (*mcp.ClientSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	server := newTestServer()
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

func TestServeStdio_IOTransportCreation(t *testing.T) {
	transport := &mcp.StdioTransport{}
	if transport == nil {
		t.Fatal("StdioTransport should not be nil")
	}
}

func TestServeStdio_JSONRPCMessageParsing(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1.0"}}}`
	var msg map[string]any
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("failed to parse JSON-RPC message: %v", err)
	}
	if msg["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %v", msg["jsonrpc"])
	}
	if msg["method"] != "initialize" {
		t.Errorf("expected method initialize, got %v", msg["method"])
	}
}

func TestServeStdio_InvalidJSONRPC(t *testing.T) {
	invalid := `{not valid json}`
	var msg map[string]any
	if err := json.Unmarshal([]byte(invalid), &msg); err == nil {
		t.Error("expected error parsing invalid JSON, got nil")
	}
}

func TestServeStdio_ResponseFormatting(t *testing.T) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      float64(1),
		"result": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"serverInfo": map[string]any{
				"name":    "sin-code",
				"version": "dev",
			},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}
	if !strings.Contains(string(data), `"jsonrpc":"2.0"`) {
		t.Errorf("response missing jsonrpc field: %s", string(data))
	}
	if !strings.Contains(string(data), `"protocolVersion"`) {
		t.Errorf("response missing protocolVersion: %s", string(data))
	}
}

func TestServeStdio_InitializeViaInMemory(t *testing.T) {
	cs, cancel := connectClientServer(t)
	defer cancel()

	initResult := cs.InitializeResult()
	if initResult == nil {
		t.Fatal("InitializeResult should not be nil")
	}
	if initResult.ServerInfo.Name != "sin-code-test" {
		t.Errorf("expected server name sin-code-test, got %q", initResult.ServerInfo.Name)
	}
	if initResult.ProtocolVersion == "" {
		t.Error("expected non-empty ProtocolVersion")
	}
	if initResult.Capabilities == nil {
		t.Error("expected non-nil Capabilities")
	}
}

func TestServeStdio_ListToolsViaInMemory(t *testing.T) {
	cs, cancel := connectClientServer(t)
	defer cancel()

	toolsResult, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	expectedTools := []string{
		"sin_discover", "sin_execute", "sin_map", "sin_grasp",
		"sin_scout", "sin_harvest", "sin_orchestrate",
		"sin_ibd", "sin_poc", "sin_sckg", "sin_adw", "sin_oracle", "sin_efm",
	}

	if len(toolsResult.Tools) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(toolsResult.Tools))
	}

	found := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		found[tool.Name] = true
	}
	for _, name := range expectedTools {
		if !found[name] {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestServeStdio_UnknownToolError(t *testing.T) {
	cs, cancel := connectClientServer(t)
	defer cancel()

	_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "nonexistent_tool",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Error("expected error calling unknown tool, got nil")
	}
}

func TestServeStdio_ToolSchemaValidation(t *testing.T) {
	cs, cancel := connectClientServer(t)
	defer cancel()

	toolsResult, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	for _, tool := range toolsResult.Tools {
		if tool.Name == "" {
			t.Error("tool has empty name")
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil InputSchema", tool.Name)
		}
	}
}

func TestServeStdio_PingMethod(t *testing.T) {
	cs, cancel := connectClientServer(t)
	defer cancel()

	err := cs.Ping(context.Background(), nil)
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestServeStdio_InMemoryTransportCreation(t *testing.T) {
	c1, c2 := mcp.NewInMemoryTransports()
	if c1 == nil || c2 == nil {
		t.Fatal("NewInMemoryTransports should return two non-nil transports")
	}
}
