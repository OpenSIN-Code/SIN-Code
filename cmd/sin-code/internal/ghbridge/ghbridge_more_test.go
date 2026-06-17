// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for ghbridge branches that need
// package-level hooks (missing binary, I/O failures, defensive branches,
// and the MCP stdio edge cases). All tests are hermetic.
// Docs: ghbridge.doc.md
package ghbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── Bridge and classifier edge cases ──────────────────────────────────

func TestExecRunner(t *testing.T) {
	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nprintf 'auth ok\\n'\nexit 0\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	out, errOut, err := ExecRunner(context.Background(), []string{"auth", "status"})
	if err != nil {
		t.Fatalf("ExecRunner: expected nil err, got %v", err)
	}
	if out != "auth ok\n" {
		t.Fatalf("stdout: want %q, got %q", "auth ok\n", out)
	}
	if errOut != "" {
		t.Fatalf("stderr: want empty, got %q", errOut)
	}
}

func TestExecRunnerError(t *testing.T) {
	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nprintf 'boom\\n' >&2\nexit 1\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	out, errOut, err := ExecRunner(context.Background(), []string{"auth", "status"})
	if err == nil {
		t.Fatal("ExecRunner: expected err")
	}
	if out != "" {
		t.Fatalf("stdout: want empty, got %q", out)
	}
	if errOut != "boom\n" {
		t.Fatalf("stderr: want %q, got %q", "boom\n", errOut)
	}
}

func TestHealthLookPathError(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "", errors.New("missing") }
	t.Cleanup(func() { execLookPath = orig })
	b := New()
	err := b.Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing gh error, got %v", err)
	}
}

func TestHealthAuthErrorStderr(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "/gh", nil }
	t.Cleanup(func() { execLookPath = orig })
	b := NewWithRunner(Runner(func(context.Context, []string) (string, string, error) {
		return "", "auth bad", errors.New("exit 1")
	}), time.Second)
	err := b.Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "auth bad") {
		t.Fatalf("expected stderr in error, got %v", err)
	}
}

func TestHealthAuthErrorStdout(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "/gh", nil }
	t.Cleanup(func() { execLookPath = orig })
	b := NewWithRunner(Runner(func(context.Context, []string) (string, string, error) {
		return "auth bad", "", errors.New("exit 1")
	}), time.Second)
	err := b.Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "auth bad") {
		t.Fatalf("expected stdout in error, got %v", err)
	}
}

func TestHealthAuthErrorRaw(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "/gh", nil }
	t.Cleanup(func() { execLookPath = orig })
	b := NewWithRunner(Runner(func(context.Context, []string) (string, string, error) {
		return "", "", errors.New("broken")
	}), time.Second)
	err := b.Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("expected raw error, got %v", err)
	}
}

func TestExecuteDefensiveForbidden(t *testing.T) {
	orig := classifyFunc
	classifyFunc = func([]string) (Tier, error) { return TierForbidden, nil }
	t.Cleanup(func() { classifyFunc = orig })
	var called bool
	b := NewWithRunner(Runner(func(context.Context, []string) (string, string, error) {
		called = true
		return "", "", nil
	}), time.Second)
	_, tier, err := b.Execute(context.Background(), []string{"issue", "list"})
	if err == nil || tier != TierForbidden {
		t.Fatalf("expected TierForbidden error, got tier=%s err=%v", tier, err)
	}
	if called {
		t.Fatal("runner must not be called for defensive forbidden")
	}
}

func TestExecuteErrorStdoutFallback(t *testing.T) {
	b := NewWithRunner(Runner(func(context.Context, []string) (string, string, error) {
		return "out", "", errors.New("exit 1")
	}), time.Second)
	_, _, err := b.Execute(context.Background(), []string{"issue", "list"})
	if err == nil || !strings.Contains(err.Error(), "out") {
		t.Fatalf("expected stdout in error, got %v", err)
	}
}

func TestExecuteErrorRawFallback(t *testing.T) {
	b := NewWithRunner(Runner(func(context.Context, []string) (string, string, error) {
		return "", "", errors.New("raw")
	}), time.Second)
	_, _, err := b.Execute(context.Background(), []string{"issue", "list"})
	if err == nil || !strings.Contains(err.Error(), "raw") {
		t.Fatalf("expected raw error, got %v", err)
	}
}

func TestClassifyVerbSlotReCheck(t *testing.T) {
	classifySkipForbiddenScan = true
	t.Cleanup(func() { classifySkipForbiddenScan = false })
	tier, err := Classify([]string{"issue", "delete", "1"})
	if err == nil || tier != TierForbidden {
		t.Fatalf("expected forbidden verb error, got tier=%s err=%v", tier, err)
	}
	if !strings.Contains(err.Error(), "forbidden verb") {
		t.Fatalf("expected forbidden verb message, got %v", err)
	}
}

func TestTierStringUnknown(t *testing.T) {
	if got := Tier(99).String(); got != "forbidden" {
		t.Fatalf("want forbidden, got %q", got)
	}
}

func TestTruncateEdge(t *testing.T) {
	if got := truncate("abc", 0); got != "abc" {
		t.Fatalf("n=0: want %q, got %q", "abc", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("n>=len: want %q, got %q", "abc", got)
	}
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Fatalf("truncation: want %q, got %q", "abc…", got)
	}
}

// ── MCP server lifecycle and I/O edge cases ─────────────────────────────

func TestNewServerMore(t *testing.T) {
	srv := NewServer()
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
}

type errReaderMore struct{}

func (errReaderMore) Read([]byte) (int, error) { return 0, errors.New("boom") }

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write fail") }

func TestServeContextCancelledEdge(t *testing.T) {
	in := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "ping"}
	writeReq(t, in, req)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := NewServerWithIO(in, &bytes.Buffer{}, &bytes.Buffer{})
	err := srv.Serve(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestServeScannerErrorEdge(t *testing.T) {
	srv := NewServerWithIO(errReaderMore{}, &bytes.Buffer{}, &bytes.Buffer{})
	err := srv.Serve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected scanner error, got %v", err)
	}
}

func TestServeEncodeErrorEdge(t *testing.T) {
	in := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "ping"}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, failWriter{}, &bytes.Buffer{})
	err := srv.Serve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "write fail") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestServeMalformedJSON(t *testing.T) {
	in := &bytes.Buffer{}
	in.Write([]byte("not json\n"))
	errW := &bytes.Buffer{}
	out := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, errW)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(errW.String(), "decode error") {
		t.Fatalf("expected decode error log, got %q", errW.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no response, got %q", out.String())
	}
}

func TestServeNotification(t *testing.T) {
	in := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized"}
	writeReq(t, in, req)
	out := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no response for notification, got %q", out.String())
	}
}

func TestServeUnknownMethod(t *testing.T) {
	in := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "frobnicate"}
	writeReq(t, in, req)
	out := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected method not found error, got %+v", resp.Error)
	}
}

func TestCallToolUnknownEdge(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "tools/call", Params: mustMarshalParams(t, "gh_unknown", map[string]any{})}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	if resp.Error == nil || resp.Error.Code != -32602 {
		t.Fatalf("expected unknown tool error, got %+v", resp.Error)
	}
}

// ── lazyBridge and health paths ────────────────────────────────────────

func TestLazyBridgeLookPathError(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "", errors.New("missing") }
	t.Cleanup(func() { execLookPath = orig })
	srv := NewServerWithIO(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	_, err := srv.lazyBridge()
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing gh error, got %v", err)
	}
}

func TestLazyBridgeSuccessLookPath(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "/gh", nil }
	t.Cleanup(func() { execLookPath = orig })
	srv := NewServerWithIO(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	bridge, err := srv.lazyBridge()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bridge == nil {
		t.Fatal("bridge is nil")
	}
}

func TestCallHealthLazyBridgeError(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "", errors.New("missing") }
	t.Cleanup(func() { execLookPath = orig })
	srv := NewServerWithIO(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	req := &jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "gh_health"}
	resp := srv.callHealth(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("expected tool result, got JSON-RPC error: %+v", resp.Error)
	}
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "bridge init failed") {
		t.Fatalf("expected bridge init failed tool error, got %+v", result)
	}
}

func TestCallHealthBridgeHealthError(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "/gh", nil }
	t.Cleanup(func() { execLookPath = orig })
	srv := NewServerWithIO(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	srv.bridgeOnce.Do(func() {})
	srv.bridge = NewWithRunner(Runner(func(context.Context, []string) (string, string, error) {
		return "", "auth failed", errors.New("exit 1")
	}), 0)
	req := &jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "gh_health"}
	resp := srv.callHealth(context.Background(), req)
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "auth failed") {
		t.Fatalf("expected auth failed tool error, got %+v", result)
	}
}

func TestCallHealthSuccessEdge(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "/gh", nil }
	t.Cleanup(func() { execLookPath = orig })
	srv := NewServerWithIO(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	srv.bridgeOnce.Do(func() {})
	srv.bridge = NewWithRunner(Runner(func(context.Context, []string) (string, string, error) {
		return "ok", "", nil
	}), 0)
	req := &jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "gh_health"}
	resp := srv.callHealth(context.Background(), req)
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content[0].Text, "healthy") {
		t.Fatalf("expected healthy result, got %+v", result)
	}
}

// ── dispatch and tool result paths ──────────────────────────────────────

func TestDispatchLazyBridgeError(t *testing.T) {
	orig := execLookPath
	execLookPath = func(string) (string, error) { return "", errors.New("missing") }
	t.Cleanup(func() { execLookPath = orig })
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "tools/call", Params: mustMarshalParams(t, "gh_query", map[string]any{"args": []string{"issue", "list"}})}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "bridge init failed") {
		t.Fatalf("expected bridge init failed, got %+v", result)
	}
}

func TestDispatchNoDeadline(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "tools/call", Params: mustMarshalParams(t, "gh_query", map[string]any{"args": []string{"issue", "list"}})}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	srv.bridgeOnce.Do(func() {})
	srv.bridge = NewWithRunner(runForBridge(newFakeRunner(fakeResponse{stdout: "ok"})), 0)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content[0].Text, "ok") {
		t.Fatalf("expected ok result, got %+v", result)
	}
}

func TestResultMarshalErrorEdge(t *testing.T) {
	orig := jsonMarshal
	jsonMarshal = func(v any) ([]byte, error) { return nil, errors.New("marshal") }
	t.Cleanup(func() { jsonMarshal = orig })
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "ping"}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	if resp.Error == nil || resp.Error.Code != -32603 {
		t.Fatalf("expected internal error, got %+v", resp.Error)
	}
}

func TestMustMarshalErrorEdge(t *testing.T) {
	orig := jsonMarshal
	jsonMarshal = func(v any) ([]byte, error) { return nil, errors.New("marshal") }
	t.Cleanup(func() { jsonMarshal = orig })
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "tools/call", Params: mustMarshalParams(t, "gh_query", map[string]any{"args": []string{"issue", "create", "--title", "x"}})}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	if resp.Error != nil {
		t.Fatalf("expected JSON-RPC success envelope, got error: %+v", resp.Error)
	}
	if !strings.Contains(string(resp.Result), `"isError":true`) {
		t.Fatalf("expected static isError result, got %q", resp.Result)
	}
}

// ── MCP registration and config paths ───────────────────────────────────

func TestMCPConfigPath(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "/sinhome")
	if got := MCPConfigPath(); got != "/sinhome/mcp.json" {
		t.Fatalf("SIN_CODE_HOME: want %q, got %q", "/sinhome/mcp.json", got)
	}
	t.Setenv("SIN_CODE_HOME", "")
	orig := userHomeDir
	userHomeDir = func() (string, error) { return "/home", nil }
	t.Cleanup(func() { userHomeDir = orig })
	want := filepath.Join("/home", ".local", "share", "sin-code", "mcp.json")
	if got := MCPConfigPath(); got != want {
		t.Fatalf("home dir: want %q, got %q", want, got)
	}
}

func TestMCPConfigPathNoHome(t *testing.T) {
	orig := userHomeDir
	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	t.Cleanup(func() { userHomeDir = orig })
	t.Setenv("SIN_CODE_HOME", "")
	got := MCPConfigPath()
	want := filepath.Join(".", ".sin-code-home", "mcp.json")
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestRegisterMCPExecutableErrorEdge(t *testing.T) {
	orig := osExecutable
	osExecutable = func() (string, error) { return "", errors.New("no exe") }
	t.Cleanup(func() { osExecutable = orig })
	path := filepath.Join(t.TempDir(), "mcp.json")
	_, err := RegisterMCP(path)
	if err == nil || !strings.Contains(err.Error(), "resolve executable") {
		t.Fatalf("expected executable error, got %v", err)
	}
}

func TestRegisterMCPWriteMarshalError(t *testing.T) {
	orig := jsonMarshal
	jsonMarshal = func(v any) ([]byte, error) { return nil, errors.New("marshal") }
	t.Cleanup(func() { jsonMarshal = orig })
	path := filepath.Join(t.TempDir(), "mcp.json")
	_, err := RegisterMCP(path)
	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestRegisterMCPMkdirError(t *testing.T) {
	orig := osMkdirAll
	osMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	t.Cleanup(func() { osMkdirAll = orig })
	path := filepath.Join(t.TempDir(), "mcp.json")
	_, err := RegisterMCP(path)
	if err == nil || !strings.Contains(err.Error(), "mkdir") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestRegisterMCPCreateTempError(t *testing.T) {
	orig := osCreateTemp
	osCreateTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp") }
	t.Cleanup(func() { osCreateTemp = orig })
	path := filepath.Join(t.TempDir(), "mcp.json")
	_, err := RegisterMCP(path)
	if err == nil || !strings.Contains(err.Error(), "temp") {
		t.Fatalf("expected temp error, got %v", err)
	}
}

func TestRegisterMCPCopyError(t *testing.T) {
	orig := ioCopy
	ioCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy") }
	t.Cleanup(func() { ioCopy = orig })
	path := filepath.Join(t.TempDir(), "mcp.json")
	_, err := RegisterMCP(path)
	if err == nil || !strings.Contains(err.Error(), "copy") {
		t.Fatalf("expected copy error, got %v", err)
	}
}

func TestRegisterMCPCloseError(t *testing.T) {
	orig := closeFile
	closeFile = func(*os.File) error { return errors.New("close") }
	t.Cleanup(func() { closeFile = orig })
	path := filepath.Join(t.TempDir(), "mcp.json")
	_, err := RegisterMCP(path)
	if err == nil || !strings.Contains(err.Error(), "close") {
		t.Fatalf("expected close error, got %v", err)
	}
}

func TestRegisterMCPIdempotent(t *testing.T) {
	orig := osExecutable
	osExecutable = func() (string, error) { return "/my/exe", nil }
	t.Cleanup(func() { osExecutable = orig })
	path := filepath.Join(t.TempDir(), "mcp.json")
	existing := `{"mcpServers":{"gh":{"command":"/my/exe","args":["gh","serve"]}}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := RegisterMCP(path)
	if err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != existing {
		t.Fatalf("file changed unexpectedly:\nwant: %s\ngot:  %s", existing, b)
	}
}

func TestPingSuccess(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "ping"}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}
	if !strings.Contains(string(resp.Result), "pong") {
		t.Fatalf("expected pong result, got %q", resp.Result)
	}
}

func TestNotificationsInitializedRequest(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "notifications/initialized"}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no response, got %q", out.String())
	}
}

func TestServeEmptyLineEdge(t *testing.T) {
	in := &bytes.Buffer{}
	in.Write([]byte("\n"))
	out := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no response for empty line, got %q", out.String())
	}
}

func TestDispatchRunError(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	req := jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "tools/call", Params: mustMarshalParams(t, "gh_query", map[string]any{"args": []string{"issue", "list"}})}
	writeReq(t, in, req)
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	srv.bridgeOnce.Do(func() {})
	srv.bridge = NewWithRunner(runForBridge(newFakeRunner(fakeResponse{err: errors.New("boom")})), 0)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "boom") {
		t.Fatalf("expected run error result, got %+v", result)
	}
}

func TestDispatchAppliesTimeout(t *testing.T) {
	srv := NewServerWithIO(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	srv.bridgeOnce.Do(func() {})
	srv.bridge = NewWithRunner(runForBridge(newFakeRunner(fakeResponse{stdout: "ok"})), 0)
	req := &jsonRPCRequest{JSONRPC: "2.0", ID: mustRawID(`"1"`), Method: "tools/call"}
	resp := dispatch(srv, context.Background(), req, "gh_query", []string{"issue", "list"}, TierReadOnly)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content[0].Text, "ok") {
		t.Fatalf("expected ok result, got %+v", result)
	}
}

func TestRegisterMCPOverwrite(t *testing.T) {
	orig := osExecutable
	osExecutable = func() (string, error) { return "/new/exe", nil }
	t.Cleanup(func() { osExecutable = orig })
	path := filepath.Join(t.TempDir(), "mcp.json")
	existing := `{"mcpServers":{"gh":{"command":"/old/exe","args":["gh","serve"]}}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := RegisterMCP(path)
	if err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "/new/exe") {
		t.Fatalf("file not updated: %s", b)
	}
}
