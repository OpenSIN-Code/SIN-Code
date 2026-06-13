// SPDX-License-Identifier: MIT
// Purpose: tests for the vane package. All tests are hermetic — no
// network, no shared state. Vane itself is mocked with httptest.Server.
// The MCP server tests drive the stdio loop with bytes.Buffer / pipe
// and assert on parsed JSON-RPC responses.
// Docs: vane.doc.md
package vane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── Test helpers ──────────────────────────────────────────────────────

// setupTestHome returns a fresh temp dir, points SIN_CODE_HOME at it,
// and registers cleanup via t.Setenv so the test framework restores the
// env after the test exits. Every test that touches ConfigPath() /
// MCPConfigPath() MUST call this first.
func setupTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	t.Setenv("VANE_API_URL", "") // ensure no leak from parent env
	return dir
}

// mockVane spins up an httptest.Server that emulates Vane's contract:
// GET / returns the given status, POST /api/search returns a JSON body
// built from `answer` + `sources`. The returned URL is a fully-qualified
// http://127.0.0.1:NNNN — assign to cfg.BaseURL.
func mockVane(t *testing.T, status int, answer string, sources []Source) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "vane-mock")
		case "/api/search":
			// Honor the configured status: callers can simulate a
			// 5xx/4xx upstream by passing a non-2xx status. This
			// mirrors how a real Vane instance would behave when
			// its model is overloaded.
			if status < 200 || status >= 300 {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "vane-mock: simulated error")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			payload := map[string]any{
				"message": answer,
				"sources": sources,
			}
			_ = json.NewEncoder(w).Encode(payload)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}


// newListener opens a TCP listener on the requested address and returns
// it. The caller is expected to Close() it and use Addr().String() as
// an unbound target for tests that need a "guaranteed-unreachable"
// endpoint. Mirrors the net.Listen helper from the stdlib.
func newListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}



// safeBuffer is a goroutine-safe wrapper around bytes.Buffer. The
// stdlib bytes.Buffer is NOT safe for concurrent Write+Read, and our
// test loop has the server goroutine writing responses while the test
// goroutine polls for them. This wrapper serializes the access.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Len()
}

func (s *safeBuffer) ReadString(delim byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.ReadString(delim)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}


// ── Config ────────────────────────────────────────────────────────────

func TestConfigRoundtripAndEnvOverride(t *testing.T) {
	dir := setupTestHome(t)

	// 1. Defaults when no file present.
	cfg, present, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if present {
		t.Errorf("expected present=false on fresh home, got true")
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL: got %q want %q", cfg.BaseURL, DefaultBaseURL)
	}
	if cfg.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Errorf("TimeoutSeconds: got %d want %d", cfg.TimeoutSeconds, DefaultTimeoutSeconds)
	}

	// 2. Save + reload roundtrip.
	saved := Config{
		BaseURL:        "http://vane.example:9000",
		ChatProvider:   "anthropic",
		ChatModel:      "claude-3-5-sonnet",
		EmbedProvider:  "openai",
		EmbedModel:     "text-embedding-3-large",
		TimeoutSeconds: 120,
	}
	if err := SaveConfig(saved); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	cfg, present, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig (after save): %v", err)
	}
	if !present {
		t.Errorf("expected present=true after save, got false")
	}
	if cfg != saved {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", cfg, saved)
	}
	if !filepath.IsAbs(cfg.BaseURL) && !strings.HasPrefix(cfg.BaseURL, "http") {
		// BaseURL should be a URL, not a path.
		t.Errorf("BaseURL does not look like a URL: %q", cfg.BaseURL)
	}

	// 3. Confirm the file actually landed in $SIN_CODE_HOME.
	if _, err := os.Stat(filepath.Join(dir, "vane.json")); err != nil {
		t.Errorf("expected vane.json in %s: %v", dir, err)
	}

	// 4. VANE_API_URL overrides BaseURL but leaves other fields alone.
	t.Setenv("VANE_API_URL", "http://override.invalid:5555/")
	cfg, _, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig (with override): %v", err)
	}
	if cfg.BaseURL != "http://override.invalid:5555" {
		t.Errorf("VANE_API_URL override not honored: BaseURL=%q", cfg.BaseURL)
	}
	if cfg.ChatProvider != "anthropic" {
		t.Errorf("override leaked into other fields: ChatProvider=%q", cfg.ChatProvider)
	}
}

func TestSaveConfigDefaultsBlankAndZero(t *testing.T) {
	setupTestHome(t)
	if err := SaveConfig(Config{}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("blank BaseURL not normalized: %q", cfg.BaseURL)
	}
	if cfg.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Errorf("zero TimeoutSeconds not normalized: %d", cfg.TimeoutSeconds)
	}
}

// ── Search ────────────────────────────────────────────────────────────

func TestSearchSuccessWithCitations(t *testing.T) {
	srv := mockVane(t, http.StatusOK, "Vane is a self-hosted AI answer engine.",
		[]Source{
			{Title: "Vane on GitHub", URL: "https://github.com/example/vane"},
			{Title: "Vane Docs", URL: "https://vane.example/docs"},
		})
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	client := NewClient(cfg)
	ans, err := client.Search(context.Background(), "what is vane?", "webSearch", "balanced")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !strings.Contains(ans.Message, "self-hosted") {
		t.Errorf("Message missing content: %q", ans.Message)
	}
	if len(ans.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(ans.Sources))
	}
	// FormatAnswer must include the "Cited Sources" section.
	rendered := FormatAnswer(ans)
	if !strings.Contains(rendered, "## Cited Sources") {
		t.Errorf("FormatAnswer missing sources header; got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[Vane on GitHub](https://github.com/example/vane)") {
		t.Errorf("FormatAnswer missing formatted source; got:\n%s", rendered)
	}
}

func TestSearchValidation(t *testing.T) {
	srv := mockVane(t, http.StatusOK, "ok", nil)
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	client := NewClient(cfg)

	cases := []struct {
		name    string
		query   string
		focus   string
		opt     string
		wantMsg string
	}{
		{"empty query rejected", "", "webSearch", "balanced", "query is required"},
		{"blank query rejected", "   ", "webSearch", "balanced", "query is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.Search(context.Background(), tc.query, tc.focus, tc.opt)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error message: got %q want substring %q", err.Error(), tc.wantMsg)
			}
		})
	}

	// Invalid focus / optimization are silently coerced to defaults —
	// the test asserts the request still succeeds (i.e. the bridge does
	// not error on unknown enums).
	ans, err := client.Search(context.Background(), "ok", "unknownFocus", "unknownOpt")
	if err != nil {
		t.Fatalf("Search with unknown enums: %v", err)
	}
	if !strings.Contains(ans.Message, "ok") {
		t.Errorf("Message: got %q", ans.Message)
	}
}

func TestSearchServerError(t *testing.T) {
	srv := mockVane(t, http.StatusInternalServerError, "", nil)
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	client := NewClient(cfg)
	_, err := client.Search(context.Background(), "trigger 500", "webSearch", "balanced")
	if err == nil {
		t.Fatal("expected error on 5xx, got nil")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Errorf("error: got %q want substring 'server error'", err.Error())
	}
}

func TestGracefulDegradationUnreachable(t *testing.T) {
	cfg := DefaultConfig()
	// Reserve a port and immediately close it → the address is unbound
	// but well-formed, so dial fails fast.
	ln, err := newListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listener: %v", err)
	}
	deadAddr := ln.Addr().String()
	_ = ln.Close()
	cfg.BaseURL = "http://" + deadAddr
	cfg.TimeoutSeconds = 2
	client := NewClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = client.Search(ctx, "ping", "webSearch", "balanced")
	if err == nil {
		t.Fatal("expected unreachable error, got nil")
	}
	if !strings.Contains(err.Error(), "vane:") {
		t.Errorf("error missing vane prefix: %q", err.Error())
	}

	// Healthy should also error cleanly.
	if err := client.Healthy(ctx); err == nil {
		t.Fatal("expected Healthy to error on unreachable address")
	}
}

// ── RegisterMCP ──────────────────────────────────────────────────────

func TestRegisterMCP(t *testing.T) {
	dir := setupTestHome(t)
	mcpPath := filepath.Join(dir, "mcp.json")

	// 1. Fresh file: just the vane entry.
	if _, err := RegisterMCP(mcpPath); err != nil {
		t.Fatalf("RegisterMCP (fresh): %v", err)
	}
	raw, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parse mcp.json: %v", err)
	}
	servers, _ := got["mcpServers"].(map[string]any)
	if servers == nil {
		t.Fatalf("mcpServers missing in: %s", raw)
	}
	entry, ok := servers[ServerName].(map[string]any)
	if !ok {
		t.Fatalf("vane entry missing: %v", servers)
	}
	cmd, _ := entry["command"].(string)
	if !strings.HasSuffix(cmd, "sin-code") && !filepath.IsAbs(cmd) {
		t.Errorf("command: got %q, want path containing sin-code or absolute", cmd)
	}
	args, _ := entry["args"].([]any)
	if len(args) != 2 || fmt.Sprint(args[0]) != "vane" || fmt.Sprint(args[1]) != "serve" {
		t.Errorf("args: got %v, want [vane serve]", args)
	}

	// 2. Pre-existing superpowers entry MUST survive.
	preserved := map[string]any{
		"mcpServers": map[string]any{
			"superpowers": map[string]any{
				"command": "sin-code",
				"args":    []string{"superpowers", "serve"},
			},
		},
	}
	preservedRaw, _ := json.MarshalIndent(preserved, "", "  ")
	if err := os.WriteFile(mcpPath, preservedRaw, 0o644); err != nil {
		t.Fatalf("seed mcp.json: %v", err)
	}
	if _, err := RegisterMCP(mcpPath); err != nil {
		t.Fatalf("RegisterMCP (preserved): %v", err)
	}
	raw, _ = os.ReadFile(mcpPath)
	var got2 map[string]any
	if err := json.Unmarshal(raw, &got2); err != nil {
		t.Fatalf("parse after preserve: %v", err)
	}
	servers2, _ := got2["mcpServers"].(map[string]any)
	if _, ok := servers2["superpowers"]; !ok {
		t.Errorf("superpowers entry was clobbered: %s", raw)
	}
	if _, ok := servers2[ServerName]; !ok {
		t.Errorf("vane entry missing after preserve: %s", raw)
	}

	// 3. Idempotency: second call with the same command must not change
	// the file (mtime stable).
	info1, _ := os.Stat(mcpPath)
	time.Sleep(20 * time.Millisecond)
	if _, err := RegisterMCP(mcpPath); err != nil {
		t.Fatalf("RegisterMCP (idempotent): %v", err)
	}
	info2, _ := os.Stat(mcpPath)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("idempotent RegisterMCP touched the file: mtime %v -> %v", info1.ModTime(), info2.ModTime())
	}
}

func TestRegisterMCPMergesIntoExistingConfig(t *testing.T) {
	dir := setupTestHome(t)
	mcpPath := filepath.Join(dir, "mcp.json")

	// Pre-existing vane entry with a different command → must be replaced.
	seed := map[string]any{
		"mcpServers": map[string]any{
			"vane": map[string]any{
				"command": "/old/path/sin-code",
				"args":    []string{"vane", "serve"},
			},
		},
		"someOtherKey": "preserved",
	}
	raw, _ := json.MarshalIndent(seed, "", "  ")
	if err := os.WriteFile(mcpPath, raw, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := RegisterMCP(mcpPath); err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	updated, _ := os.ReadFile(mcpPath)
	var parsed map[string]any
	if err := json.Unmarshal(updated, &parsed); err != nil {
		t.Fatalf("parse updated: %v", err)
	}
	if parsed["someOtherKey"] != "preserved" {
		t.Errorf("sibling key was clobbered: %s", updated)
	}
	servers := parsed["mcpServers"].(map[string]any)
	vane := servers["vane"].(map[string]any)
	if vane["command"] == "/old/path/sin-code" {
		t.Errorf("stale command was not replaced: %s", updated)
	}
}

// ── MCP server (stdio) ────────────────────────────────────────────────


// TestServeEndToEnd exercises the full stdio loop in a hermetic fashion:
// one goroutine runs Server.Serve on a pipe, the main goroutine writes
// requests and reads responses. All assertions on the wire format.
func TestServeEndToEnd(t *testing.T) {
	dir := setupTestHome(t)

	// Point the vane bridge at a mock upstream so vane_research succeeds.
	srv := mockVane(t, http.StatusOK, "Vane is a self-hosted AI answer engine.",
		[]Source{{Title: "Vane", URL: "https://example/vane"}})
	defer srv.Close()

	// Persist a Config so the lazyClient has a real BaseURL.
	if err := SaveConfig(Config{
		BaseURL:        srv.URL,
		ChatProvider:   "openai",
		ChatModel:      "gpt-4o-mini",
		EmbedProvider:  "openai",
		EmbedModel:     "text-embedding-3-small",
		TimeoutSeconds: 5,
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	inR, inW := io.Pipe()
	outBuf := &safeBuffer{}
	errBuf := &safeBuffer{}
	srv2 := NewServerWithIO(inR, outBuf, errBuf, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serveDone := make(chan struct{})
	go func() {
		_ = srv2.Serve(ctx)
		close(serveDone)
	}()

	// Helper: write a request line and read the next response line.
	writeReq := func(t *testing.T, method string, id any, params any) map[string]any {
		t.Helper()
		req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
		if params != nil {
			b, _ := json.Marshal(params)
			req["params"] = json.RawMessage(b)
		}
		line, _ := json.Marshal(req)
		if _, err := inW.Write(append(line, '\n')); err != nil {
			t.Fatalf("write %s: %v", method, err)
		}
		// Read response with a small deadline. We poll because outBuf
		// is a plain buffer (not a pipe), so we just wait until the
		// server has had a chance to write.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if outBuf.Len() > 0 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if outBuf.Len() == 0 {
			t.Fatalf("no response within 2s for %s (stderr=%q)", method, errBuf.String())
		}
		// Parse the first complete JSON object (one line).
		line2, err := outBuf.ReadString('\n')
		if err != nil && line2 == "" {
			t.Fatalf("read response: %v", err)
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(strings.TrimRight(line2, "\r\n")), &resp); err != nil {
			t.Fatalf("parse response: %v (raw=%q)", err, line2)
		}
		return resp
	}

	// 1. initialize → expect protocolVersion "2025-06-18".
	resp := writeReq(t, "initialize", 1, map[string]any{})
	if _, ok := resp["error"]; ok {
		t.Fatalf("initialize returned error: %v", resp["error"])
	}
	result, _ := resp["result"].(map[string]any)
	if pv, _ := result["protocolVersion"].(string); pv != "2025-06-18" {
		t.Errorf("protocolVersion: got %q want %q", pv, "2025-06-18")
	}
	sinfo, _ := result["serverInfo"].(map[string]any)
	if sinfo["name"] != "sin-code-vane" {
		t.Errorf("serverInfo.name: got %v", sinfo["name"])
	}

	// 2. tools/list → expect vane_research + vane_health.
	resp = writeReq(t, "tools/list", 2, map[string]any{})
	result, _ = resp["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) < 2 {
		t.Fatalf("expected >= 2 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		names[tool["name"].(string)] = true
	}
	if !names["vane_research"] || !names["vane_health"] {
		t.Errorf("expected vane_research + vane_health, got %v", names)
	}

	// 3. tools/call vane_research → expect success with markdown body.
	resp = writeReq(t, "tools/call", 3, map[string]any{
		"name": "vane_research",
		"arguments": map[string]any{
			"query":     "what is vane?",
			"focus_mode": "webSearch",
		},
	})
	if _, ok := resp["error"]; ok {
		t.Fatalf("vane_research returned JSON-RPC error: %v", resp["error"])
	}
	tres, _ := resp["result"].(map[string]any)
	if isErr, _ := tres["isError"].(bool); isErr {
		t.Fatalf("vane_research isError=true; content=%v", tres["content"])
	}
	contents, _ := tres["content"].([]any)
	if len(contents) == 0 {
		t.Fatal("vane_research returned no content")
	}
	first, _ := contents[0].(map[string]any)
	body, _ := first["text"].(string)
	if !strings.Contains(body, "self-hosted") {
		t.Errorf("vane_research body missing expected text: %q", body)
	}
	if !strings.Contains(body, "## Cited Sources") {
		t.Errorf("vane_research body missing '## Cited Sources': %q", body)
	}

	// 4. tools/call vane_health → expect healthy.
	resp = writeReq(t, "tools/call", 4, map[string]any{"name": "vane_health"})
	tres, _ = resp["result"].(map[string]any)
	if isErr, _ := tres["isError"].(bool); isErr {
		t.Errorf("vane_health isError=true: %v", tres)
	}

	// 5. tools/call unknown → expect JSON-RPC error -32602.
	resp = writeReq(t, "tools/call", 5, map[string]any{"name": "vane_nope"})
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope for unknown tool, got %v", resp)
	}
	if code, _ := errObj["code"].(float64); int(code) != -32602 {
		t.Errorf("error code: got %v want -32602", errObj["code"])
	}

	// 6. ping → expect pong.
	resp = writeReq(t, "ping", 6, map[string]any{})
	if pong, _ := resp["result"].(map[string]any)["pong"].(string); pong != "ok" {
		t.Errorf("ping: got %v", resp)
	}

	// Tear down: close the input pipe so the server exits.
	_ = inW.Close()
	select {
	case <-serveDone:
	case <-time.After(2 * time.Second):
		t.Errorf("server did not exit after stdin close")
	}
}

func TestServeGracefulDegradationOnUnreachable(t *testing.T) {
	dir := setupTestHome(t)

	// Point at a port we then close → dial fails.
	ln, err := newListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listener: %v", err)
	}
	dead := "http://" + ln.Addr().String()
	_ = ln.Close()

	if err := SaveConfig(Config{BaseURL: dead, TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	inR, inW := io.Pipe()
	outBuf := &safeBuffer{}
	srv := NewServerWithIO(inR, outBuf, &safeBuffer{}, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serveDone := make(chan struct{})
	go func() {
		_ = srv.Serve(ctx)
		close(serveDone)
	}()

	req := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "vane_research",
			"arguments": map[string]any{"query": "ping"},
		},
	}
	line, _ := json.Marshal(req)
	if _, err := inW.Write(append(line, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && outBuf.Len() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if outBuf.Len() == 0 {
		t.Fatal("no response within 3s")
	}
	line2, _ := outBuf.ReadString('\n')
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimRight(line2, "\r\n")), &resp); err != nil {
		t.Fatalf("parse: %v (raw=%q)", err, line2)
	}
	// MUST be a successful envelope carrying isError=true + fallback hint.
	if _, ok := resp["error"]; ok {
		t.Fatalf("tool errors must surface as isError, not JSON-RPC error: %v", resp)
	}
	tres, _ := resp["result"].(map[string]any)
	if isErr, _ := tres["isError"].(bool); !isErr {
		t.Errorf("expected isError=true on unreachable; got %v", tres)
	}
	contents, _ := tres["content"].([]any)
	if len(contents) == 0 {
		t.Fatal("no content in isError response")
	}
	first, _ := contents[0].(map[string]any)
	body, _ := first["text"].(string)
	if !strings.Contains(body, "Fallback: use the websearch ecosystem skill instead.") {
		t.Errorf("missing graceful-degradation hint: %q", body)
	}

	_ = inW.Close()
	<-serveDone
}

// TestServeValidationError checks the empty-query validation path —
// still routed through Graceful Degradation so the model sees a
// friendly message rather than a JSON-RPC error.
func TestServeValidationError(t *testing.T) {
	dir := setupTestHome(t)
	if err := SaveConfig(Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	inR, inW := io.Pipe()
	outBuf := &safeBuffer{}
	srv := NewServerWithIO(inR, outBuf, &safeBuffer{}, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	serveDone := make(chan struct{})
	go func() {
		_ = srv.Serve(ctx)
		close(serveDone)
	}()

	req := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "vane_research",
			"arguments": map[string]any{"query": "   "},
		},
	}
	line, _ := json.Marshal(req)
	if _, err := inW.Write(append(line, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && outBuf.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if outBuf.Len() == 0 {
		t.Fatal("no response within 2s")
	}
	line2, _ := outBuf.ReadString('\n')
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimRight(line2, "\r\n")), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	tres, _ := resp["result"].(map[string]any)
	if isErr, _ := tres["isError"].(bool); !isErr {
		t.Errorf("expected isError=true for empty query; got %v", tres)
	}
	contents, _ := tres["content"].([]any)
	first, _ := contents[0].(map[string]any)
	body, _ := first["text"].(string)
	if !strings.Contains(body, "query is required") {
		t.Errorf("missing 'query is required' message: %q", body)
	}

	_ = inW.Close()
	<-serveDone
}

// ── FormatAnswer edge cases ──────────────────────────────────────────

func TestFormatAnswerNoSources(t *testing.T) {
	got := FormatAnswer(&Answer{Message: "Just text."})
	if !strings.Contains(got, "Just text.") {
		t.Errorf("missing body: %q", got)
	}
	if strings.Contains(got, "## Cited Sources") {
		t.Errorf("should NOT emit sources section when empty: %q", got)
	}
}

func TestFormatAnswerNil(t *testing.T) {
	if got := FormatAnswer(nil); got != "" {
		t.Errorf("FormatAnswer(nil): got %q want \"\"", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short string should pass through: %q", got)
	}
	if got := truncate("hello world", 5); got != "hello…" {
		t.Errorf("truncated: got %q want %q", got, "hello…")
	}
	if got := truncate("x", 0); got != "x" {
		t.Errorf("n=0 should pass through: got %q", got)
	}
	if got := truncate("x", -1); got != "x" {
		t.Errorf("n<0 should pass through: got %q", got)
	}
}
