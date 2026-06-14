// SPDX-License-Identifier: MIT
// Purpose: additional tests for the vane package targeting the remaining
// statement-coverage gaps. Uses package-level test hooks declared in
// vane.go and mcpserver.go to exercise error paths without heavy
// refactoring. All tests are hermetic and single-threaded (no t.Parallel)
// because they mutate global hook variables.
// Docs: vane.doc.md
package vane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setHook replaces *target with value and restores it on test cleanup.
func setHook[T any](t *testing.T, target *T, value T) {
	t.Helper()
	old := *target
	*target = value
	t.Cleanup(func() { *target = old })
}

// failingWriter always returns an error so Serve's json.Encoder.Encode path
// can be exercised.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errors.New("write fail") }

// errorReader returns a configured error on every Read.
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) { return 0, r.err }

// ── Config / Home ─────────────────────────────────────────────────────

func TestHomeFallbackWhenUserHomeDirFails(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "")
	setHook(t, &osUserHomeDirFn, func() (string, error) { return "", errors.New("no home") })
	got := Home()
	want := filepath.Join(".", ".sin-code-home")
	if got != want {
		t.Errorf("Home() = %q, want %q", got, want)
	}
}

func TestLoadConfigReadError(t *testing.T) {
	dir := setupTestHome(t)
	// Create a directory named vane.json so os.ReadFile returns a non-ErrNotExist error.
	path := filepath.Join(dir, "vane.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, _, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "vane: read") {
		t.Errorf("error: %v", err)
	}
}

func TestLoadConfigUnmarshalError(t *testing.T) {
	setupTestHome(t)
	if err := os.WriteFile(ConfigPath(), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "vane: parse") {
		t.Errorf("error: %v", err)
	}
}

func TestLoadConfigFallbacks(t *testing.T) {
	setupTestHome(t)
	if err := SaveConfig(Config{BaseURL: "  ", TimeoutSeconds: -1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL fallback: got %q", cfg.BaseURL)
	}
	if cfg.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Errorf("TimeoutSeconds fallback: got %d", cfg.TimeoutSeconds)
	}
}

func TestSaveConfigBlankBaseURL(t *testing.T) {
	setupTestHome(t)
	if err := SaveConfig(Config{BaseURL: "  "}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("blank BaseURL not normalized: got %q", cfg.BaseURL)
	}
}

// ── Client construction ───────────────────────────────────────────────

func TestNewClientTimeoutFallback(t *testing.T) {
	c := NewClient(Config{TimeoutSeconds: 0})
	if c.http.Timeout != DefaultTimeoutSeconds*time.Second {
		t.Errorf("timeout: got %v want %v", c.http.Timeout, DefaultTimeoutSeconds*time.Second)
	}
}

func TestBreakerStatsNil(t *testing.T) {
	var c *Client
	if c.BreakerStats() != nil {
		t.Error("nil *Client should return nil")
	}
	c2 := &Client{}
	if c2.BreakerStats() != nil {
		t.Error("Client with nil breaker should return nil")
	}
}

// ── Healthy / Search error paths ────────────────────────────────────────

func TestHealthyRequestError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseURL = "://invalid"
	c := NewClient(cfg)
	if err := c.Healthy(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("error: %v", err)
	}
}

func TestHealthyServerError(t *testing.T) {
	srv := mockVane(t, http.StatusInternalServerError, "err", nil)
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	c := NewClient(cfg)
	if err := c.Healthy(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Errorf("error: %v", err)
	}
}

func TestHealthyClientError(t *testing.T) {
	srv := mockVane(t, http.StatusBadRequest, "err", nil)
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	c := NewClient(cfg)
	if err := c.Healthy(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Errorf("error: %v", err)
	}
}

func TestSearchMarshalError(t *testing.T) {
	setHook(t, &jsonMarshalFn, func(v any) ([]byte, error) { return nil, errors.New("marshal") })
	srv := mockVane(t, http.StatusOK, "ok", nil)
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	c := NewClient(cfg)
	_, err := c.Search(context.Background(), "q", "webSearch", "balanced")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "vane: marshal") {
		t.Errorf("error: %v", err)
	}
}

func TestSearchRequestError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseURL = "://invalid"
	c := NewClient(cfg)
	_, err := c.Search(context.Background(), "q", "webSearch", "balanced")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "build request") {
		t.Errorf("error: %v", err)
	}
}

func TestSearchReadBodyError(t *testing.T) {
	setHook(t, &ioReadAllFn, func(r io.Reader) ([]byte, error) { return nil, errors.New("read body") })
	srv := mockVane(t, http.StatusOK, "ok", nil)
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	c := NewClient(cfg)
	_, err := c.Search(context.Background(), "q", "webSearch", "balanced")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "vane: read body") {
		t.Errorf("error: %v", err)
	}
}

func TestSearchServerError(t *testing.T) {
	srv := mockVane(t, http.StatusInternalServerError, "boom", nil)
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	c := NewClient(cfg)
	_, err := c.Search(context.Background(), "q", "webSearch", "balanced")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Errorf("error: %v", err)
	}
}

func TestSearchClientError(t *testing.T) {
	srv := mockVane(t, http.StatusBadRequest, "bad", nil)
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	c := NewClient(cfg)
	_, err := c.Search(context.Background(), "q", "webSearch", "balanced")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "client error") {
		t.Errorf("error: %v", err)
	}
}

func TestSearchDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "not json")
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	c := NewClient(cfg)
	_, err := c.Search(context.Background(), "q", "webSearch", "balanced")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "vane: decode") {
		t.Errorf("error: %v", err)
	}
}

func TestSearchNilSources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"message":"ok"}`)
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	c := NewClient(cfg)
	ans, err := c.Search(context.Background(), "q", "webSearch", "balanced")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if ans.Sources == nil {
		t.Error("Sources should be a non-nil empty slice")
	}
}

// ── FormatAnswer ────────────────────────────────────────────────────────

func TestFormatAnswerEmptySource(t *testing.T) {
	got := FormatAnswer(&Answer{
		Message: "msg",
		Sources: []Source{
			{Title: "", URL: ""},        // skipped entirely
			{Title: "T", URL: ""},       // title-only
			{Title: "", URL: "http://u"}, // url-only, title falls back to url
		},
	})
	if strings.Contains(got, "1.") {
		t.Errorf("empty source should be skipped: %q", got)
	}
	if !strings.Contains(got, "2. T") {
		t.Errorf("missing title-only source: %q", got)
	}
	if !strings.Contains(got, "3. [http://u](http://u)") {
		t.Errorf("missing url-only source: %q", got)
	}
}

// ── RegisterMCP / writeJSONAtomic ──────────────────────────────────────

func TestRegisterMCPExecutableError(t *testing.T) {
	setupTestHome(t)
	setHook(t, &osExecutableFn, func() (string, error) { return "", errors.New("exe") })
	_, err := RegisterMCP("")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "vane: resolve executable") {
		t.Errorf("error: %v", err)
	}
}

func TestWriteJSONAtomicMarshalError(t *testing.T) {
	dir := setupTestHome(t)
	setHook(t, &jsonMarshalIndentFn, func(v any, prefix, indent string) ([]byte, error) { return nil, errors.New("marshal") })
	err := writeJSONAtomic(filepath.Join(dir, "x.json"), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Errorf("error: %v", err)
	}
}

func TestWriteJSONAtomicMkdirError(t *testing.T) {
	dir := setupTestHome(t)
	// Create a file where the parent directory is expected so MkdirAll fails.
	badPath := filepath.Join(dir, "file", "x.json")
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	err := writeJSONAtomic(badPath, map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteJSONAtomicCreateTempError(t *testing.T) {
	dir := setupTestHome(t)
	setHook(t, &osCreateTempFn, func(dir, pattern string) (*os.File, error) { return nil, errors.New("temp") })
	err := writeJSONAtomic(filepath.Join(dir, "x.json"), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "temp") {
		t.Errorf("error: %v", err)
	}
}

func TestWriteJSONAtomicCopyError(t *testing.T) {
	dir := setupTestHome(t)
	setHook(t, &writeJSONCopyFn, func(dst io.Writer, src io.Reader) (int64, error) { return 0, errors.New("copy") })
	err := writeJSONAtomic(filepath.Join(dir, "x.json"), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "copy") {
		t.Errorf("error: %v", err)
	}
}

func TestWriteJSONAtomicWriteError(t *testing.T) {
	dir := setupTestHome(t)
	setHook(t, &writeJSONWriteFn, func(w io.Writer, p []byte) (int, error) { return 0, errors.New("write") })
	err := writeJSONAtomic(filepath.Join(dir, "x.json"), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "write") {
		t.Errorf("error: %v", err)
	}
}

func TestWriteJSONAtomicCloseError(t *testing.T) {
	dir := setupTestHome(t)
	setHook(t, &writeJSONCloseFn, func(c io.Closer) error { return errors.New("close") })
	err := writeJSONAtomic(filepath.Join(dir, "x.json"), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "close") {
		t.Errorf("error: %v", err)
	}
}

// ── MCP Server construction / lifecycle ───────────────────────────────

func TestNewServerDefaultCfgDir(t *testing.T) {
	dir := setupTestHome(t)
	s := NewServer("")
	if s.cfgDir != dir {
		t.Errorf("cfgDir: got %q want %q", s.cfgDir, dir)
	}
}

func TestServeWrapper(t *testing.T) {
	dir := setupTestHome(t)
	if err := SaveConfig(Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	r, w := io.Pipe()
	out := &safeBuffer{}
	setHook(t, &serveStdin, r)
	setHook(t, &serveStdout, out)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = Serve(ctx)
		close(done)
	}()
	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}
	line, _ := json.Marshal(req)
	if _, err := w.Write(append(line, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && out.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if out.Len() == 0 {
		t.Fatal("no response")
	}
	_ = w.Close()
	<-done
}

func TestServeMalformedJSON(t *testing.T) {
	dir := setupTestHome(t)
	if err := SaveConfig(Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	inR, inW := io.Pipe()
	outBuf := &safeBuffer{}
	errBuf := &safeBuffer{}
	s := NewServerWithIO(inR, outBuf, errBuf, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()

	// Empty line should be ignored; malformed line should log to stderr.
	if _, err := inW.Write([]byte("\nnot json\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && errBuf.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if errBuf.Len() == 0 {
		t.Fatal("expected stderr output")
	}
	if !strings.Contains(errBuf.String(), "decode error") {
		t.Errorf("stderr: %q", errBuf.String())
	}

	// Ensure the server continues after a malformed line.
	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}
	line, _ := json.Marshal(req)
	if _, err := inW.Write(append(line, '\n')); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && outBuf.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if outBuf.Len() == 0 {
		t.Fatal("no ping response")
	}
	respLine, _ := outBuf.ReadString('\n')
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimRight(respLine, "\r\n")), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := resp["error"]; ok {
		t.Errorf("ping returned error: %v", resp["error"])
	}

	_ = inW.Close()
	<-done
}

func TestServeEncodeError(t *testing.T) {
	dir := setupTestHome(t)
	if err := SaveConfig(Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	inR, inW := io.Pipe()
	s := NewServerWithIO(inR, failingWriter{}, &safeBuffer{}, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()
	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}
	line, _ := json.Marshal(req)
	if _, err := inW.Write(append(line, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not exit after encode error")
	}
}

func TestServeScannerError(t *testing.T) {
	dir := setupTestHome(t)
	s := NewServerWithIO(&errorReader{err: errors.New("scanner")}, &safeBuffer{}, &safeBuffer{}, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := s.Serve(ctx)
	if err == nil || !strings.Contains(err.Error(), "scanner") {
		t.Errorf("error: %v", err)
	}
}

func TestServeContextCancel(t *testing.T) {
	dir := setupTestHome(t)
	if err := SaveConfig(Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	inR, inW := io.Pipe()
	outBuf := &safeBuffer{}
	s := NewServerWithIO(inR, outBuf, &safeBuffer{}, dir)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()

	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}
	line, _ := json.Marshal(req)
	if _, err := inW.Write(append(line, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && outBuf.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if outBuf.Len() == 0 {
		t.Fatal("no ping response")
	}

	// Cancel the context and feed another line so the loop hits ctx.Err().
	cancel()
	if _, err := inW.Write([]byte("{}\n")); err != nil && err != io.ErrClosedPipe {
		t.Fatalf("write after cancel: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not exit after context cancellation")
	}
}

func TestServeNotification(t *testing.T) {
	dir := setupTestHome(t)
	if err := SaveConfig(Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	inR, inW := io.Pipe()
	outBuf := &safeBuffer{}
	s := NewServerWithIO(inR, outBuf, &safeBuffer{}, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()

	// notifications/initialized is a notification with no reply.
	req1 := map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}
	line1, _ := json.Marshal(req1)
	if _, err := inW.Write(append(line1, '\n')); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	// Send a ping to confirm the server is still alive.
	req2 := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}
	line2, _ := json.Marshal(req2)
	if _, err := inW.Write(append(line2, '\n')); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && outBuf.Len() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if outBuf.Len() == 0 {
		t.Fatal("no ping response")
	}
	respLine, _ := outBuf.ReadString('\n')
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimRight(respLine, "\r\n")), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := resp["error"]; ok {
		t.Errorf("ping returned error: %v", resp["error"])
	}

	_ = inW.Close()
	<-done
}

// ── dispatch / result / tool calls ────────────────────────────────────

func TestDispatchNotification(t *testing.T) {
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, "")
	req := &jsonRPCRequest{Method: "ping"} // ID is nil
	resp := s.dispatch(context.Background(), req)
	if resp != nil {
		t.Errorf("expected nil response for notification, got %v", resp)
	}
}

func TestDispatchMethodNotFound(t *testing.T) {
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, "")
	id := json.RawMessage(`1`)
	req := &jsonRPCRequest{ID: &id, Method: "unknown", JSONRPC: "2.0"}
	resp := s.dispatch(context.Background(), req)
	if resp == nil || resp.Error == nil {
		t.Fatal("expected JSON-RPC error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("code: got %d want -32601", resp.Error.Code)
	}
}

func TestResultMarshalError(t *testing.T) {
	setHook(t, &jsonMarshalFn, func(v any) ([]byte, error) { return nil, errors.New("marshal") })
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, "")
	id := json.RawMessage(`1`)
	req := &jsonRPCRequest{ID: &id, Method: "ping", JSONRPC: "2.0"}
	resp := s.result(req, map[string]any{"x": "y"})
	if resp == nil || resp.Error == nil {
		t.Fatal("expected JSON-RPC error")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("code: got %d want -32603", resp.Error.Code)
	}
}

func TestCallResearchNoDeadline(t *testing.T) {
	dir := setupTestHome(t)
	srv := mockVane(t, http.StatusOK, "answer", nil)
	defer srv.Close()
	if err := SaveConfig(Config{BaseURL: srv.URL, TimeoutSeconds: 2}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, dir)
	id := json.RawMessage(`1`)
	params := toolCallParams{Name: "vane_research"}
	params.Arguments, _ = json.Marshal(map[string]any{"query": "q"})
	resp := s.callResearch(context.Background(), &jsonRPCRequest{ID: &id, Method: "tools/call", JSONRPC: "2.0"}, &params)
	if resp == nil || resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp)
	}
	var tres toolResult
	if err := json.Unmarshal(resp.Result, &tres); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tres.Content) == 0 {
		t.Fatal("no content")
	}
	if !strings.Contains(tres.Content[0].Text, "answer") {
		t.Errorf("missing answer: %q", tres.Content[0].Text)
	}
}

func TestCallResearchLazyClientError(t *testing.T) {
	dir := setupTestHome(t)
	if err := os.WriteFile(filepath.Join(dir, "vane.json"), []byte("bad"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, dir)
	id := json.RawMessage(`1`)
	params := toolCallParams{Name: "vane_research"}
	params.Arguments, _ = json.Marshal(map[string]any{"query": "q"})
	resp := s.callResearch(context.Background(), &jsonRPCRequest{ID: &id, Method: "tools/call", JSONRPC: "2.0"}, &params)
	if resp == nil || resp.Error != nil {
		t.Fatalf("expected tool error envelope, got %v", resp)
	}
	var tres toolResult
	if err := json.Unmarshal(resp.Result, &tres); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !tres.IsError {
		t.Errorf("expected isError=true")
	}
	if !strings.Contains(tres.Content[0].Text, "bridge init failed") {
		t.Errorf("missing bridge init message: %q", tres.Content[0].Text)
	}
}

func TestCallHealthLazyClientError(t *testing.T) {
	dir := setupTestHome(t)
	if err := os.WriteFile(filepath.Join(dir, "vane.json"), []byte("bad"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, dir)
	id := json.RawMessage(`1`)
	resp := s.callHealth(context.Background(), &jsonRPCRequest{ID: &id, Method: "tools/call", JSONRPC: "2.0"})
	if resp == nil || resp.Error != nil {
		t.Fatalf("expected tool error envelope, got %v", resp)
	}
	var tres toolResult
	if err := json.Unmarshal(resp.Result, &tres); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !tres.IsError {
		t.Errorf("expected isError=true")
	}
	if !strings.Contains(tres.Content[0].Text, "bridge init failed") {
		t.Errorf("missing bridge init message: %q", tres.Content[0].Text)
	}
}

func TestCallHealthHealthyError(t *testing.T) {
	dir := setupTestHome(t)
	srv := mockVane(t, http.StatusInternalServerError, "err", nil)
	defer srv.Close()
	if err := SaveConfig(Config{BaseURL: srv.URL, TimeoutSeconds: 2}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, dir)
	id := json.RawMessage(`1`)
	resp := s.callHealth(context.Background(), &jsonRPCRequest{ID: &id, Method: "tools/call", JSONRPC: "2.0"})
	if resp == nil || resp.Error != nil {
		t.Fatalf("expected tool error envelope, got %v", resp)
	}
	var tres toolResult
	if err := json.Unmarshal(resp.Result, &tres); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !tres.IsError {
		t.Errorf("expected isError=true")
	}
	if !strings.Contains(tres.Content[0].Text, "vane_health:") {
		t.Errorf("missing health error message: %q", tres.Content[0].Text)
	}
}

// ── lazyClient env restore ──────────────────────────────────────────────

func TestLazyClientEnvRestoreWhenUnset(t *testing.T) {
	dir := setupTestHome(t)
	os.Unsetenv("SIN_CODE_HOME")
	if err := SaveConfig(Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, dir)
	_, err := s.lazyClient()
	if err != nil {
		t.Fatalf("lazyClient: %v", err)
	}
	if os.Getenv("SIN_CODE_HOME") != "" {
		t.Errorf("SIN_CODE_HOME not restored to empty: %q", os.Getenv("SIN_CODE_HOME"))
	}
}

func TestLazyClientEnvRestoreWhenSet(t *testing.T) {
	dir := setupTestHome(t)
	other := dir + "-other"
	os.Setenv("SIN_CODE_HOME", other)
	if err := SaveConfig(Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	s := NewServerWithIO(strings.NewReader(""), &safeBuffer{}, &safeBuffer{}, dir)
	_, err := s.lazyClient()
	if err != nil {
		t.Fatalf("lazyClient: %v", err)
	}
	if os.Getenv("SIN_CODE_HOME") != other {
		t.Errorf("SIN_CODE_HOME not restored: got %q want %q", os.Getenv("SIN_CODE_HOME"), other)
	}
}

// ── Unused helper ───────────────────────────────────────────────────────

func TestFormatIntBytes(t *testing.T) {
	if got := formatIntBytes(123); got != "123B" {
		t.Errorf("formatIntBytes(123) = %q want 123B", got)
	}
}
