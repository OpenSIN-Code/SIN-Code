// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for mcpclient package to reach 100% statement coverage.
package mcpclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeSession struct {
	result  *sdk.CallToolResult
	callErr error
	closed  bool
}

func (f *fakeSession) CallTool(ctx context.Context, params *sdk.CallToolParams) (*sdk.CallToolResult, error) {
	return f.result, f.callErr
}

func (f *fakeSession) Close() error {
	f.closed = true
	return nil
}

func TestConnectAll_SuccessAndDuplicateWarning(t *testing.T) {
	testConnectHook = func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error) {
		return &fakeSession{}, []Tool{{Server: cfg.Name, Name: "echo", Qualified: cfg.Name + "__echo"}}, nil
	}
	defer func() { testConnectHook = nil }()

	mgr := NewManager([]ServerConfig{{Name: "demo", Transport: "stdio", Command: "noop"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if len(mgr.Tools()) != 1 || mgr.Tools()[0].Qualified != "demo__echo" {
		t.Fatalf("expected demo__echo tool, got %+v", mgr.Tools())
	}

	// Second attempt with the same name should hit the already-warned branch.
	warnedMu.Lock()
	warnedServers["demo"] = true
	warnedMu.Unlock()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll on duplicate: %v", err)
	}
}

func TestConnectAll_HookReturnsError(t *testing.T) {
	const serverName = "hook-bad"
	testConnectHook = func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error) {
		return nil, nil, errors.New("hook fail")
	}
	defer func() { testConnectHook = nil }()

	// Pre-mark the server as warned so ConnectAll skips logging and avoids
	// interfering with the global os.Stderr state used by other tests.
	warnedMu.Lock()
	warnedServers[serverName] = true
	warnedMu.Unlock()
	defer func() {
		warnedMu.Lock()
		delete(warnedServers, serverName)
		warnedMu.Unlock()
	}()

	mgr := NewManager([]ServerConfig{{Name: serverName, Transport: "stdio", Command: "noop"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll should never be fatal: %v", err)
	}
	if len(mgr.Tools()) != 0 {
		t.Fatal("expected 0 tools")
	}
}

func TestConnect_HookEnvExpansion(t *testing.T) {
	testConnectHook = func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error) {
		if cfg.Env["EXTRA"] != "value" {
			return nil, nil, errors.New("env not expanded")
		}
		return &fakeSession{}, nil, nil
	}
	defer func() { testConnectHook = nil }()

	mgr := NewManager([]ServerConfig{{Name: "env", Transport: "stdio", Command: "noop", Env: map[string]string{"EXTRA": "value"}}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
}

func TestCall_SuccessTextContent(t *testing.T) {
	testConnectHook = func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error) {
		return &fakeSession{result: &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "hello"}}}}, nil, nil
	}
	defer func() { testConnectHook = nil }()

	mgr := NewManager([]ServerConfig{{Name: "demo", Transport: "stdio"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatal(err)
	}
	out, err := mgr.Call(ctx, "demo__echo", map[string]any{})
	if err != nil || out != "hello" {
		t.Fatalf("got %q / %v", out, err)
	}
}

func TestCall_ToolError(t *testing.T) {
	testConnectHook = func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error) {
		return &fakeSession{result: &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "oops"}}, IsError: true}}, nil, nil
	}
	defer func() { testConnectHook = nil }()

	mgr := NewManager([]ServerConfig{{Name: "demo", Transport: "stdio"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatal(err)
	}
	out, err := mgr.Call(ctx, "demo__echo", nil)
	if err == nil || !strings.Contains(err.Error(), "returned an error") {
		t.Fatalf("expected tool error, got %q / %v", out, err)
	}
}

func TestCall_CallToolError(t *testing.T) {
	testConnectHook = func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error) {
		return &fakeSession{callErr: errors.New("call fail")}, nil, nil
	}
	defer func() { testConnectHook = nil }()

	mgr := NewManager([]ServerConfig{{Name: "demo", Transport: "stdio"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Call(ctx, "demo__echo", nil); err == nil || !strings.Contains(err.Error(), "call fail") {
		t.Fatalf("expected call fail error, got %v", err)
	}
}

func TestCall_NonTextContentIgnored(t *testing.T) {
	testConnectHook = func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error) {
		return &fakeSession{result: &sdk.CallToolResult{Content: []sdk.Content{&sdk.ImageContent{}}}}, nil, nil
	}
	defer func() { testConnectHook = nil }()

	mgr := NewManager([]ServerConfig{{Name: "demo", Transport: "stdio"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatal(err)
	}
	out, err := mgr.Call(ctx, "demo__echo", nil)
	if err != nil || out != "" {
		t.Fatalf("expected empty output, got %q / %v", out, err)
	}
}

func TestIsExternal_Coverage(t *testing.T) {
	mgr := NewManager(nil)
	if !mgr.IsExternal("demo__tool") {
		t.Fatal("expected demo__tool to be external")
	}
	if mgr.IsExternal("sin_read") {
		t.Fatal("expected sin_read not to be external")
	}
}

func TestClose_Coverage(t *testing.T) {
	fs := &fakeSession{}
	testConnectHook = func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error) {
		return fs, nil, nil
	}
	defer func() { testConnectHook = nil }()

	mgr := NewManager([]ServerConfig{{Name: "demo", Transport: "stdio"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatal(err)
	}
	mgr.Close()
	if !fs.closed {
		t.Fatal("expected session closed")
	}
}

func TestConnect_RealStdioEnvExpansion(t *testing.T) {
	mgr := NewManager([]ServerConfig{{Name: "env", Transport: "stdio", Command: "/nonexistent", Env: map[string]string{"FOO": "bar"}}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if len(mgr.Tools()) != 0 {
		t.Fatal("expected 0 tools")
	}
}

// startFakeServer wires an in-memory transport pair to a tiny MCP server and
// returns the client-side transport.
func startFakeServer(t *testing.T) sdk.Transport {
	t.Helper()
	clientTransport, serverTransport := sdk.NewInMemoryTransports()

	server := sdk.NewServer(&sdk.Implementation{Name: "fake", Version: "1.0"}, nil)
	server.AddTool(&sdk.Tool{
		Name:        "echo",
		Description: "echo input",
		InputSchema: map[string]any{"type": "object"},
	}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "pong"}}}, nil
	})

	go func() {
		if err := server.Run(context.Background(), serverTransport); err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("fake server exited: %v", err)
		}
	}()
	return clientTransport
}

func TestRealConnect_Success(t *testing.T) {
	clientTransport := startFakeServer(t)
	testTransportProvider = func(cfg ServerConfig) (sdk.Transport, error) {
		return clientTransport, nil
	}
	defer func() { testTransportProvider = nil }()

	mgr := NewManager([]ServerConfig{{Name: "mem", Transport: "stdio", Env: map[string]string{"FOO": "bar"}}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll: %v", err)
	}
	if len(mgr.Tools()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(mgr.Tools()))
	}
	out, err := mgr.Call(ctx, "mem__echo", map[string]any{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out != "pong" {
		t.Fatalf("expected pong, got %q", out)
	}
	mgr.Close()
}

func TestRealConnect_ListToolsError(t *testing.T) {
	clientTransport := startFakeServer(t)
	testTransportProvider = func(cfg ServerConfig) (sdk.Transport, error) {
		return clientTransport, nil
	}
	defer func() { testTransportProvider = nil }()
	testListToolsErr = errors.New("list tools failed")
	defer func() { testListToolsErr = nil }()

	mgr := NewManager([]ServerConfig{{Name: "mem", Transport: "stdio"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// ConnectAll is additive and never fatal; the error path is logged.
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll should be non-fatal: %v", err)
	}
	if len(mgr.Tools()) != 0 {
		t.Fatal("expected 0 tools after ListTools error")
	}
}

func TestRealConnect_TransportProviderError(t *testing.T) {
	testTransportProvider = func(cfg ServerConfig) (sdk.Transport, error) {
		return nil, errors.New("transport broke")
	}
	defer func() { testTransportProvider = nil }()

	mgr := NewManager([]ServerConfig{{Name: "mem", Transport: "stdio"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.ConnectAll(ctx); err != nil {
		t.Fatalf("ConnectAll should be non-fatal: %v", err)
	}
	if len(mgr.Tools()) != 0 {
		t.Fatal("expected 0 tools after transport error")
	}
}

func TestRealSession_Close_NilSession(t *testing.T) {
	mgr := NewManager(nil)
	mgr.sessions["nil"] = &realSession{}
	mgr.Close()
}

func TestDefaultServers_PythonSkills(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(string) (string, error) { return "", os.ErrNotExist }
	defer func() { lookPathHook = orig }()

	dir := t.TempDir()
	testSkillsDir = &dir
	defer func() { testSkillsDir = nil }()

	// Create a hardcoded Python skill checkout so the py() builder uses python3.
	pyDir := filepath.Join(dir, "SIN-Code-Scheduler-Skill")
	if err := os.MkdirAll(pyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pyDir, "mcp_server.py"), []byte("# fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	servers := DefaultServers()
	for _, s := range servers {
		if s.Name == "scheduler" {
			if s.Command != "python3" {
				t.Fatalf("expected python3 command, got %q", s.Command)
			}
			want := filepath.Join(dir, "SIN-Code-Scheduler-Skill", "mcp_server.py")
			if len(s.Args) != 1 || s.Args[0] != want {
				t.Fatalf("expected args %q, got %q", []string{want}, s.Args)
			}
			return
		}
	}
	t.Fatalf("expected scheduler server, got %+v", servers)
}

func TestDefaultServers_GoNativeFallback(t *testing.T) {
	dir := t.TempDir()
	testSkillsDir = &dir
	defer func() { testSkillsDir = nil }()

	// web_search_bundle directory exists but has no sin-websearch binary, so
	// goNative falls back to the binary name on PATH.
	if err := os.MkdirAll(filepath.Join(dir, "web_search_bundle"), 0o755); err != nil {
		t.Fatal(err)
	}

	servers := DefaultServers()
	for _, s := range servers {
		if s.Name == "websearch" {
			if s.Command != "sin-websearch" {
				t.Fatalf("expected sin-websearch command, got %q", s.Command)
			}
			return
		}
	}
	t.Fatalf("expected websearch server, got %+v", servers)
}

func TestDefaultServers_EmptySkillsDir(t *testing.T) {
	empty := ""
	testSkillsDir = &empty
	defer func() { testSkillsDir = nil }()

	orig := lookPathHook
	lookPathHook = func(string) (string, error) { return "", os.ErrNotExist }
	defer func() { lookPathHook = orig }()

	servers := DefaultServers()
	for _, s := range servers {
		if s.Name == "scheduler" {
			if s.Command != "sin-scheduler" {
				t.Fatalf("expected sin-scheduler command, got %q", s.Command)
			}
		}
		if s.Name == "websearch" {
			if s.Command != "sin-websearch" {
				t.Fatalf("expected sin-websearch command, got %q", s.Command)
			}
		}
	}
}

func TestShortNameFallback(t *testing.T) {
	if got := shortName("unknown-repo"); got != "unknown-repo" {
		t.Fatalf("expected unknown-repo, got %q", got)
	}
}
