// SPDX-License-Identifier: MIT
// Purpose: coverage tests for the remaining uncovered branches in serve.go.
package internal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/apiweb"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpcompress"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/plugins"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServePluginTimeoutDefault(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "plugin")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pt := plugins.MCPToolDef{
		Name:       "test",
		Binary:     script,
		Args:       []string{},
		Timeout:    0,
		PluginPath: dir,
	}
	out, err := runPluginMCPTool(context.Background(), pt, nil)
	if err != nil {
		t.Fatalf("expected plugin to run: %v", err)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("expected ok output, got %q", out)
	}
}

func TestServeScout_AbsError(t *testing.T) {
	old := pathAbs
	pathAbs = func(string) (string, error) { return "", errors.New("injected abs error") }
	defer func() { pathAbs = old }()

	_, err := handleScout(context.Background(), map[string]any{
		"query": "x", "path": ".", "search_type": "regex",
	})
	if err == nil || !strings.Contains(err.Error(), "injected abs error") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestServeSecurityText_AllStatuses(t *testing.T) {
	r := SecurityResult{
		ProjectType: "generic",
		Path:        "/tmp/test",
		Duration:    1 * time.Second,
		Strict:      true,
		Tools: []ToolResult{
			{Name: "ok-tool", Status: "ok", Duration: "1s"},
			{Name: "issue-tool", Status: "issues", Issues: 3, Duration: "1s"},
			{Name: "error-tool", Status: "error", Duration: "1s", Error: "boom"},
			{Name: "not-found-tool", Status: "not_found", Duration: "1s"},
			{Name: "skipped-tool", Status: "skipped", Duration: "1s"},
		},
		Summary: SecuritySummary{ToolsRun: 3, Issues: 3, Errors: 1, NotFound: 1, Skipped: 1},
	}
	out := formatSecurityResultText(r)
	for _, want := range []string{
		"Security Scan Summary", "ok-tool", "issue-tool", "error-tool",
		"not-found-tool", "skipped-tool", "Strict mode", "3 issues",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got %q", want, out)
		}
	}
}

func TestServeSecurityText_NonStrictIssues(t *testing.T) {
	r := SecurityResult{
		ProjectType: "go",
		Path:        "/tmp/test",
		Duration:    1 * time.Second,
		Strict:      false,
		Tools:       []ToolResult{{Name: "gosec", Status: "issues", Issues: 2, Duration: "1s"}},
		Summary:     SecuritySummary{ToolsRun: 1, Issues: 2},
	}
	out := formatSecurityResultText(r)
	if !strings.Contains(out, "review recommended") {
		t.Errorf("expected review recommended message, got %q", out)
	}
}

func TestServeSecurityText_NoIssues(t *testing.T) {
	r := SecurityResult{
		ProjectType: "generic",
		Path:        "/tmp/test",
		Duration:    1 * time.Second,
		Tools:       []ToolResult{{Name: "secrets", Status: "ok", Duration: "1s"}},
		Summary:     SecuritySummary{ToolsRun: 1},
	}
	out := formatSecurityResultText(r)
	if !strings.Contains(out, "No security issues detected") {
		t.Errorf("expected no issues message, got %q", out)
	}
}

func TestServeSecurity_TextFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.env"), []byte(`password = "supersecret12345"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := handleSecurity(context.Background(), map[string]any{
		"path":   dir,
		"format": "text",
		"type":   "generic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Security Scan Summary") {
		t.Errorf("expected text summary, got %q", out)
	}
}

func TestServeSecurity_AbsError(t *testing.T) {
	old := pathAbs
	pathAbs = func(string) (string, error) { return "", errors.New("injected abs error") }
	defer func() { pathAbs = old }()

	_, err := handleSecurity(context.Background(), map[string]any{"path": "."})
	if err == nil || !strings.Contains(err.Error(), "injected abs error") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestServeSecurity_JSONMarshalError(t *testing.T) {
	old := securityMarshalIndent
	securityMarshalIndent = func(any, string, string) ([]byte, error) {
		return nil, errors.New("injected marshal error")
	}
	defer func() { securityMarshalIndent = old }()

	_, err := handleSecurity(context.Background(), map[string]any{
		"path":   t.TempDir(),
		"format": "json",
		"type":   "generic",
	})
	if err == nil || !strings.Contains(err.Error(), "injected marshal error") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestServeSecurity_AutoType(t *testing.T) {
	dir := t.TempDir()
	_, err := handleSecurity(context.Background(), map[string]any{
		"path":   dir,
		"format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServeSecurity_TimeoutClamp(t *testing.T) {
	dir := t.TempDir()
	_, err := handleSecurity(context.Background(), map[string]any{
		"path":    dir,
		"format":  "text",
		"timeout": float64(99999),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServeSecurity_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	out, err := handleSecurity(context.Background(), map[string]any{
		"path":   dir,
		"format": "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "{") {
		t.Errorf("expected JSON output, got %q", out)
	}
}

func TestServeCmd_RunEStdio(t *testing.T) {
	oldTransport, _ := ServeCmd.Flags().GetString("transport")
	ServeCmd.Flags().Set("transport", "stdio")
	defer ServeCmd.Flags().Set("transport", oldTransport)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ServeCmd.SetContext(ctx)
	defer ServeCmd.SetContext(context.Background())

	_ = ServeCmd.RunE(ServeCmd, []string{})
}

func TestServeCmd_RunENilContext(t *testing.T) {
	oldTransport, _ := ServeCmd.Flags().GetString("transport")
	ServeCmd.Flags().Set("transport", "stdio")
	defer ServeCmd.Flags().Set("transport", oldTransport)

	oldCtx := ServeCmd.Context()
	ServeCmd.SetContext(nil)
	defer ServeCmd.SetContext(oldCtx)

	_ = ServeCmd.RunE(ServeCmd, []string{})
}

func TestServeCmd_RunEHttp(t *testing.T) {
	oldTransport, _ := ServeCmd.Flags().GetString("transport")
	ServeCmd.Flags().Set("transport", "http")
	defer ServeCmd.Flags().Set("transport", oldTransport)

	oldPort := servePort
	servePort = 0
	defer func() { servePort = oldPort }()

	oldFactory := httpLoopFactory
	httpLoopFactory = nil
	defer func() { httpLoopFactory = oldFactory }()

	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr; _ = w.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ServeCmd.SetContext(ctx)
	defer ServeCmd.SetContext(context.Background())

	if err := ServeCmd.RunE(ServeCmd, []string{}); err != nil {
		t.Fatalf("expected http transport to shutdown cleanly, got %v", err)
	}
}

func TestServeHTTPRegister_Nil(t *testing.T) {
	old := httpLoopFactory
	defer func() { httpLoopFactory = old }()
	if err := RegisterHTTPLoopFactory(nil); err == nil {
		t.Fatal("expected error for nil factory")
	}
}

func TestServeHTTPRegister_Success(t *testing.T) {
	old := httpLoopFactory
	defer func() { httpLoopFactory = old }()
	stub := apiweb.NewLoopFunc(func(context.Context, string, string) (*agentloop.Loop, func() error, error) {
		return nil, func() error { return nil }, nil
	})
	if err := RegisterHTTPLoopFactory(stub); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if httpLoopFactory == nil {
		t.Fatal("factory was not set")
	}
}

func TestServeHTTPTransport_Success(t *testing.T) {
	oldPort := servePort
	servePort = 0
	defer func() { servePort = oldPort }()

	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr; _ = w.Close() }()

	oldFactory := httpLoopFactory
	httpLoopFactory = nil
	defer func() { httpLoopFactory = oldFactory }()

	oldHook := httpServerHook
	httpServerHook = nil
	defer func() { httpServerHook = oldHook }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := runHTTPTransport(ctx, nil); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
}

func TestServeHTTPTransport_GetwdError(t *testing.T) {
	old := osGetwd
	osGetwd = func() (string, error) { return "", errors.New("injected getwd error") }
	defer func() { osGetwd = old }()

	err := runHTTPTransport(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "injected getwd error") {
		t.Fatalf("expected getwd error, got %v", err)
	}
}

func TestServeHTTPTransport_ListenError(t *testing.T) {
	oldPort := servePort
	servePort = -1
	defer func() { servePort = oldPort }()

	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr; _ = w.Close() }()

	err := runHTTPTransport(context.Background(), nil)
	if err == nil {
		t.Fatal("expected listen error")
	}
}

func TestServeHTTPTransport_ServerClosed(t *testing.T) {
	oldPort := servePort
	servePort = 0
	defer func() { servePort = oldPort }()

	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr; _ = w.Close() }()

	oldHook := httpServerHook
	httpServerHook = func(srv *http.Server) {
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = srv.Close()
		}()
	}
	defer func() { httpServerHook = oldHook }()

	if err := runHTTPTransport(context.Background(), nil); err != nil {
		t.Fatalf("expected clean return on server close, got %v", err)
	}
}

func TestServePlugin_RelativePath(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "plugin")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho rel\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pt := plugins.MCPToolDef{
		Name:       "test",
		Binary:     "plugin",
		PluginPath: dir,
		Timeout:    5,
	}
	out, err := runPluginMCPTool(context.Background(), pt, nil)
	if err != nil {
		t.Fatalf("expected plugin to run: %v", err)
	}
	if !strings.Contains(out, "rel") {
		t.Errorf("expected rel output, got %q", out)
	}
}

func TestServePlugin_WithArgs(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "plugin")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nfor a in \"$@\"; do printf '%s ' \"$a\"; done\necho\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pt := plugins.MCPToolDef{
		Name:       "test",
		Binary:     script,
		Args:       []string{"input"},
		Timeout:    5,
		PluginPath: dir,
	}
	out, err := runPluginMCPTool(context.Background(), pt, map[string]any{"input": "hello"})
	if err != nil {
		t.Fatalf("expected plugin to run: %v", err)
	}
	if !strings.Contains(out, "--input hello") {
		t.Errorf("expected args in output, got %q", out)
	}
}

func TestServePlugin_ErrorExit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "plugin")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pt := plugins.MCPToolDef{
		Name:       "test",
		Binary:     script,
		Timeout:    5,
		PluginPath: dir,
	}
	_, err := runPluginMCPTool(context.Background(), pt, nil)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestServeHealthHandler(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	serveHealthHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "ok") {
		t.Errorf("expected ok body, got %q", rr.Body.String())
	}
}

func TestTagNames(t *testing.T) {
	p := mcpcompress.All()
	names := tagNames(p.Tags())
	if len(names) != len(p) {
		t.Fatalf("expected %d names, got %d", len(p), len(names))
	}
	for i, n := range names {
		if n != string(p[i].Tag()) {
			t.Errorf("expected %q, got %q", p[i].Tag(), n)
		}
	}
}

func TestPrintCompressionStats(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stats")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	p := mcpcompress.All()
	stats := []mcpcompress.Stats{
		{Name: "sin_negative", Original: 50, Compressed: 60, BytesSaved: -10, Ratio: -0.2},
	}
	printCompressionStats(f, p, stats)

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{"mcpcompress:", "sin_negative", "TOTAL", "active rules", "ponytail tags"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got %q", want, out)
		}
	}
}

func testWithCompressionFlags(t *testing.T, setFlags func()) (*mcp.Server, func()) {
	oldCompress := serveCompressTools
	oldPrint := servePrintStats
	oldTags := serveCompressTags
	oldStderr := os.Stderr

	setFlags()

	_, w, _ := os.Pipe()
	os.Stderr = w

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sin-code",
		Version: ServerVersion,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})
	registerAllMCPTools(server)

	cleanup := func() {
		serveCompressTools = oldCompress
		servePrintStats = oldPrint
		serveCompressTags = oldTags
		os.Stderr = oldStderr
		_ = w.Close()
	}
	return server, cleanup
}

func TestRegisterAllMCPTools_CompressionEnabled(t *testing.T) {
	server, cleanup := testWithCompressionFlags(t, func() {
		serveCompressTools = true
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cTransport, sTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, sTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	cs, err := client.Connect(ctx, cTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range result.Tools {
		if tool.Name == "sin_discover" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sin_discover in compressed manifest, got %d tools", len(result.Tools))
	}
}

func TestRegisterAllMCPTools_PrintStats(t *testing.T) {
	server, cleanup := testWithCompressionFlags(t, func() {
		servePrintStats = true
	})
	defer cleanup()

	// registerAllMCPTools already ran during setup; success path is that it
	// does not panic and prints stats to stderr.
	if server == nil {
		t.Fatal("expected server")
	}
}

func TestRegisterAllMCPTools_CompressTags(t *testing.T) {
	server, cleanup := testWithCompressionFlags(t, func() {
		serveCompressTags = "delete"
	})
	defer cleanup()
	if server == nil {
		t.Fatal("expected server")
	}
}

func TestRegisterPluginMCPToolsWithReg_Compression(t *testing.T) {
	oldCompress := serveCompressTools
	oldTags := serveCompressTags
	defer func() {
		serveCompressTools = oldCompress
		serveCompressTags = oldTags
	}()
	serveCompressTools = true

	reg := plugins.NewRegistry()
	reg.Register(&plugins.Plugin{
		Name:    "comp",
		Enabled: true,
		Tools: []plugins.PluginTool{
			{
				Name:        "echo",
				Description: "Echo input safely and carefully (e.g. hello)",
				Binary:      "/bin/echo",
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cTransport, sTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, sTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	cs, err := client.Connect(ctx, cTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range result.Tools {
		if tool.Name == "sin_plugin_comp_echo" {
			found = true
			if strings.Contains(tool.Description, "safely") {
				t.Errorf("expected hedges to be compressed, got %q", tool.Description)
			}
		}
	}
	if !found {
		t.Errorf("expected plugin tool in compressed manifest, got %v", result.Tools)
	}
}
