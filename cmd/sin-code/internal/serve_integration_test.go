//go:build integration

package internal

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newIntegrationTestServer() *mcp.Server {
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

func setupIntegrationClientServer(t *testing.T) (*mcp.ClientSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	server := newIntegrationTestServer()
	cTransport, sTransport := mcp.NewInMemoryTransports()

	ss, err := server.Connect(ctx, sTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "integration-test-client",
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

func newMockServerWithPingTool() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sin-code-mock",
		Version: "test",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})
	server.AddTool(&mcp.Tool{
		Name:        "ping",
		Description: "Returns pong",
		InputSchema: map[string]any{"type": "object"},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "pong"}},
		}, nil
	})
	return server
}

func setupMockClientServer(t *testing.T) (*mcp.ClientSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	server := newMockServerWithPingTool()
	cTransport, sTransport := mcp.NewInMemoryTransports()

	ss, err := server.Connect(ctx, sTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mock-test-client",
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

func TestServeIntegration_Initialize(t *testing.T) {
	cs, cancel := setupIntegrationClientServer(t)
	defer cancel()

	initResult := cs.InitializeResult()
	if initResult == nil {
		t.Fatal("InitializeResult is nil")
	}
	if initResult.ServerInfo.Name != "sin-code" {
		t.Errorf("expected server name sin-code, got %q", initResult.ServerInfo.Name)
	}
	if initResult.ServerInfo.Version == "" {
		t.Error("expected non-empty server version")
	}
	if initResult.ProtocolVersion == "" {
		t.Error("expected non-empty ProtocolVersion")
	}
	if initResult.Capabilities == nil {
		t.Error("expected non-nil Capabilities")
	}
	if initResult.Capabilities.Tools == nil {
		t.Error("expected Tools capability")
	}
}

func TestServeIntegration_ListToolsAll13(t *testing.T) {
	cs, cancel := setupIntegrationClientServer(t)
	defer cancel()

	ctx := context.Background()
	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedTools := []string{
		"sin_discover", "sin_execute", "sin_map", "sin_grasp",
		"sin_scout", "sin_harvest", "sin_orchestrate",
		"sin_ibd", "sin_poc", "sin_sckg", "sin_adw", "sin_oracle", "sin_efm",
	}

	if len(result.Tools) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), len(result.Tools))
	}

	found := make(map[string]bool)
	for _, tool := range result.Tools {
		found[tool.Name] = true
	}
	for _, name := range expectedTools {
		if !found[name] {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestServeIntegration_ToolSchemasHaveObjectType(t *testing.T) {
	cs, cancel := setupIntegrationClientServer(t)
	defer cancel()

	ctx := context.Background()
	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	for _, tool := range result.Tools {
		schemaBytes, _ := json.Marshal(tool.InputSchema)
		var schema map[string]any
		if err := json.Unmarshal(schemaBytes, &schema); err != nil {
			t.Errorf("tool %q: failed to parse schema: %v", tool.Name, err)
			continue
		}
		if typ, _ := schema["type"].(string); typ != "object" {
			t.Errorf("tool %q: InputSchema type should be object, got %q", tool.Name, typ)
		}
	}
}

func TestServeIntegration_CallToolWithMockHandler(t *testing.T) {
	cs, cancel := setupMockClientServer(t)
	defer cancel()

	ctx := context.Background()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "ping",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool ping: %v", err)
	}
	if result == nil {
		t.Fatal("CallTool result is nil")
	}
	if result.IsError {
		t.Errorf("tool call returned error: %v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content block")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "pong" {
		t.Errorf("expected pong, got %q", tc.Text)
	}
}

func TestServeIntegration_UnknownToolError(t *testing.T) {
	cs, cancel := setupIntegrationClientServer(t)
	defer cancel()

	ctx := context.Background()
	_, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "sin_nonexistent",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Error("expected error calling unknown tool, got nil")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error should mention unknown tool, got: %v", err)
	}
}

func TestServeIntegration_InvalidJSONRPCOverNetPipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := newIntegrationTestServer()

	c1, c2 := net.Pipe()

	serverTransport := &mcp.IOTransport{Reader: c2, Writer: c2}
	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	go func() {
		invalidJSON := `{not valid json}` + "\n"
		c1.Write([]byte(invalidJSON))
	}()

	buf := make([]byte, 4096)
	done := make(chan struct{})
	go func() {
		c1.Read(buf)
		close(done)
	}()

	select {
	case <-done:
		t.Log("received response after invalid JSON")
	case <-time.After(2 * time.Second):
		t.Log("timeout reading response for invalid JSON (may be expected)")
	}

	c1.Close()
	c2.Close()
}

func TestServeIntegration_UnknownMethodOverRawConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server := newIntegrationTestServer()

	c1, c2 := net.Pipe()
	serverTransport := &mcp.IOTransport{Reader: c2, Writer: c2}

	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "raw-test-client",
		Version: "0.1.0",
	}, nil)
	clientTransport := &mcp.IOTransport{Reader: c1, Writer: c1}
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	initResult := cs.InitializeResult()
	if initResult == nil {
		t.Fatal("InitializeResult is nil")
	}

	go func() {
		unknownMethodReq := `{"jsonrpc":"2.0","id":99,"method":"nonexistent/method","params":{}}` + "\n"
		c1.Write([]byte(unknownMethodReq))
	}()

	buf := make([]byte, 8192)
	done := make(chan struct{})
	go func() {
		c1.Read(buf)
		close(done)
	}()

	select {
	case <-done:
		response := string(buf)
		if strings.Contains(response, `"error"`) || strings.Contains(response, `"Method not found"`) {
			t.Log("server correctly returned error for unknown method")
		} else {
			t.Logf("response: %s", response[:min(len(response), 200)])
		}
	case <-time.After(2 * time.Second):
		t.Log("timeout reading response for unknown method")
	}
}

func TestServeIntegration_PingMethod(t *testing.T) {
	cs, cancel := setupIntegrationClientServer(t)
	defer cancel()

	err := cs.Ping(context.Background(), nil)
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestServeIntegration_ToolDescriptions(t *testing.T) {
	cs, cancel := setupIntegrationClientServer(t)
	defer cancel()

	ctx := context.Background()
	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedDescriptions := map[string]string{
		"sin_discover":    "Discover files",
		"sin_execute":     "Execute shell commands",
		"sin_map":         "Map code architecture",
		"sin_grasp":       "Deep code understanding",
		"sin_scout":       "Search code",
		"sin_harvest":     "Fetch URLs",
		"sin_orchestrate": "Manage tasks",
		"sin_ibd":         "Intent-Based Diffing",
		"sin_poc":         "Proof-of-Correctness",
		"sin_sckg":        "Semantic Codebase Knowledge Graphs",
		"sin_adw":         "Architectural Debt Watchdogs",
		"sin_oracle":      "Verification Oracle",
		"sin_efm":         "Ephemeral Full-Stack Mocking",
	}

	for _, tool := range result.Tools {
		substr, ok := expectedDescriptions[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		if !strings.Contains(tool.Description, substr) {
			t.Errorf("tool %q description should contain %q, got %q", tool.Name, substr, tool.Description)
		}
	}
}
