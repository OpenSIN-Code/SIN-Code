// SPDX-License-Identifier: MIT
// Purpose: coverage tests for the LSP package. Exercises every statement
// branch of client.go and registry.go using package-level test hooks.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestLSPFakeServer is invoked as a subprocess by the tests below when
// LSP_FAKE_SERVER=1 is set. It reads LSP requests from stdin and writes
// responses to stdout based on LSP_FAKE_RESPONSES.
func TestLSPFakeServer(t *testing.T) {
	if os.Getenv("LSP_FAKE_SERVER") != "1" {
		t.Skip("not running as fake server")
	}
	responses := map[string]map[string]any{}
	if v := os.Getenv("LSP_FAKE_RESPONSES"); v != "" {
		if err := json.Unmarshal([]byte(v), &responses); err != nil {
			t.Fatalf("bad LSP_FAKE_RESPONSES: %v", err)
		}
	}
	shutdownHang := os.Getenv("LSP_FAKE_SHUTDOWN_HANG") == "1"
	noResponse := os.Getenv("LSP_FAKE_NO_RESPONSE") == "1"

	scanner := bufio.NewReader(os.Stdin)
	for {
		frame, err := readRawLSPFrame(scanner, time.Now().Add(time.Hour))
		if err != nil {
			return
		}
		var req Response
		if err := json.Unmarshal(frame, &req); err != nil {
			continue
		}
		if req.ID == nil {
			continue
		}
		if noResponse {
			continue
		}
		if shutdownHang && req.Method == "shutdown" {
			// Acknowledge shutdown so the client closes gracefully, then sleep
			// so the client is forced to kill us after closeTimeout.
			resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
			raw, _ := json.Marshal(resp)
			fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(raw), raw)
			os.Stdout.Sync()
			time.Sleep(10 * time.Minute)
			return
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if cfg, ok := responses[req.Method]; ok {
			if errObj, ok := cfg["error"]; ok {
				resp["error"] = errObj
			} else if result, ok := cfg["result"]; ok {
				resp["result"] = result
			}
		}
		raw, _ := json.Marshal(resp)
		fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(raw), raw)
		os.Stdout.Sync()
	}
}

// fakeServerCmd returns an exec.Cmd that runs this test binary in server mode.
func fakeServerCmd(t *testing.T, env ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestLSPFakeServer", "-test.v")
	cmd.Env = append(os.Environ(), append(env, "LSP_FAKE_SERVER=1")...)
	return cmd
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// fakeWriteCloser is an io.WriteCloser for testing stdin writes.
type fakeWriteCloser struct {
	writes []string
	err    error
	closed bool
}

func (f *fakeWriteCloser) Write(p []byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.writes = append(f.writes, string(p))
	return len(p), nil
}

func (f *fakeWriteCloser) Close() error {
	f.closed = true
	return nil
}

// slowReader blocks for a long time on Read.
type slowReader struct{}

func (slowReader) Read(p []byte) (int, error) {
	time.Sleep(10 * time.Second)
	return 0, nil
}

// errReader returns a configured error after content is exhausted.
type errReader struct {
	content string
	err     error
	idx     int
}

func (e *errReader) Read(p []byte) (int, error) {
	if e.idx < len(e.content) {
		n := copy(p, e.content[e.idx:])
		e.idx += n
		return n, nil
	}
	if e.err != nil {
		return 0, e.err
	}
	return 0, io.EOF
}

func setExecCommandHook(t *testing.T, hook func(string, ...string) *exec.Cmd) {
	old := execCommandHook
	execCommandHook = hook
	t.Cleanup(func() { execCommandHook = old })
}

func setTimeNowHook(t *testing.T, hook func() time.Time) {
	old := timeNowHook
	timeNowHook = hook
	t.Cleanup(func() { timeNowHook = old })
}

func setCloseTimeout(t *testing.T, d time.Duration) {
	old := closeTimeout
	closeTimeout = d
	t.Cleanup(func() { closeTimeout = old })
}

func setLookPathHook(t *testing.T, hook func(string) (string, error)) {
	old := lookPathHook
	lookPathHook = hook
	t.Cleanup(func() { lookPathHook = old })
}

func responseFrame(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func clientWithResponse(body string) *Client {
	return &Client{
		stdout: bufio.NewReader(strings.NewReader(responseFrame(body))),
		stdin:  &fakeWriteCloser{},
	}
}

// --- Start error branches ---

func TestStartBinaryRequired(t *testing.T) {
	_, err := Start("", nil, "go", "file:///tmp")
	if err == nil || err.Error() != "binary required" {
		t.Fatalf("expected binary required error, got %v", err)
	}
}

func TestStartStdinPipeError(t *testing.T) {
	setExecCommandHook(t, func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", "hello")
		cmd.Stdin = strings.NewReader("")
		return cmd
	})
	_, err := Start("fake", nil, "go", "file:///tmp")
	if err == nil {
		t.Fatal("expected error for StdinPipe")
	}
}

func TestStartStdoutPipeError(t *testing.T) {
	setExecCommandHook(t, func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", "hello")
		cmd.Stdout = io.Discard
		return cmd
	})
	_, err := Start("fake", nil, "go", "file:///tmp")
	if err == nil {
		t.Fatal("expected error for StdoutPipe")
	}
}

func TestStartStderrPipeError(t *testing.T) {
	setExecCommandHook(t, func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("echo", "hello")
		cmd.Stderr = io.Discard
		return cmd
	})
	_, err := Start("fake", nil, "go", "file:///tmp")
	if err == nil {
		t.Fatal("expected error for StderrPipe")
	}
}

func TestStartCmdStartError(t *testing.T) {
	setExecCommandHook(t, func(name string, args ...string) *exec.Cmd {
		return exec.Command("/nonexistent/binary/for/sin/lsp/test")
	})
	_, err := Start("fake", nil, "go", "file:///tmp")
	if err == nil {
		t.Fatal("expected error for cmd.Start")
	}
}

func TestStartInitializeError(t *testing.T) {
	setExecCommandHook(t, func(name string, args ...string) *exec.Cmd {
		return fakeServerCmd(t,
			"LSP_FAKE_RESPONSES="+mustJSON(map[string]map[string]any{
				"initialize": {"error": map[string]any{"code": -1, "message": "init failed"}},
			}),
		)
	})
	_, err := Start("fake", nil, "go", "file:///tmp")
	if err == nil {
		t.Fatal("expected error for initialize")
	}
}

func TestStartSuccess(t *testing.T) {
	setExecCommandHook(t, func(name string, args ...string) *exec.Cmd {
		return fakeServerCmd(t,
			"LSP_FAKE_RESPONSES="+mustJSON(map[string]map[string]any{
				"initialize": {"result": map[string]any{"capabilities": map[string]any{}}},
			}),
		)
	})
	c, err := Start("fake", nil, "go", "file:///tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Lang() != "go" {
		t.Errorf("expected lang go, got %s", c.Lang())
	}
	if c.RootURI() != "file:///tmp" {
		t.Errorf("expected rootURI, got %s", c.RootURI())
	}
}

// --- Close branches ---

func TestCloseNil(t *testing.T) {
	var c *Client
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseNoProcess(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseKill(t *testing.T) {
	setExecCommandHook(t, func(name string, args ...string) *exec.Cmd {
		return fakeServerCmd(t,
			"LSP_FAKE_SHUTDOWN_HANG=1",
			"LSP_FAKE_RESPONSES="+mustJSON(map[string]map[string]any{
				"initialize": {"result": map[string]any{"capabilities": map[string]any{}}},
			}),
		)
	})
	setCloseTimeout(t, 50*time.Millisecond)
	c, err := Start("fake", nil, "go", "file:///tmp")
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
}

// --- Accessor & notification handler ---

func TestSetNotificationHandler(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}}
	var got string
	c.SetNotificationHandler(func(method string, params json.RawMessage) {
		got = method
	})
	notif := responseFrame(`{"jsonrpc":"2.0","method":"window/logMessage","params":{"type":1,"message":"hi"}}`)
	resp := responseFrame(`{"jsonrpc":"2.0","id":1,"result":null}`)
	c.stdout = bufio.NewReader(strings.NewReader(notif + resp))
	_, err := c.readLSPFrame(time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got != "window/logMessage" {
		t.Errorf("expected notification handler to be called, got %q", got)
	}
}

// --- Notify/Did* methods ---

func TestDidOpen(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":null}`)
	if err := c.DidOpen(TextDocumentItem{URI: "file:///x.go", LanguageID: "go", Version: 1, Text: "package main"}); err != nil {
		t.Fatal(err)
	}
	if len(c.stdin.(*fakeWriteCloser).writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(c.stdin.(*fakeWriteCloser).writes))
	}
}

func TestDidChange(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":null}`)
	if err := c.DidChange("file:///x.go", 2, []TextDocumentContentChangeEvent{{Text: "package main"}}); err != nil {
		t.Fatal(err)
	}
}

func TestDidClose(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":null}`)
	if err := c.DidClose("file:///x.go"); err != nil {
		t.Fatal(err)
	}
}

func TestNotifyMarshalError(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}}
	err := c.Notify("foo", map[string]any{"ch": make(chan int)})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestNotifyWriteError(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{err: errors.New("write fail")}}
	err := c.Notify("foo", map[string]any{})
	if err == nil {
		t.Fatal("expected write error")
	}
}

// --- textDocument/* methods ---

func TestDefinition(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":[{"uri":"file:///x.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}]}`)
	locs, err := c.Definition("file:///x.go", Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 1 || locs[0].URI != "file:///x.go" {
		t.Fatalf("unexpected locations: %+v", locs)
	}
}

func TestDefinitionLink(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":[{"targetUri":"file:///x.go","targetRange":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"targetSelectionRange":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}]}`)
	locs, err := c.Definition("file:///x.go", Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 1 || locs[0].URI != "file:///x.go" {
		t.Fatalf("unexpected locations: %+v", locs)
	}
}

func TestDefinitionError(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"fail"}}`)
	_, err := c.Definition("file:///x.go", Position{Line: 0, Character: 0})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReferences(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":[{"uri":"file:///x.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":1}}}]}`)
	locs, err := c.References("file:///x.go", Position{Line: 0, Character: 0}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(locs))
	}
}

func TestReferencesError(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"fail"}}`)
	_, err := c.References("file:///x.go", Position{Line: 0, Character: 0}, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHover(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":{"contents":"hello"}}`)
	h, err := c.Hover("file:///x.go", Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatal(err)
	}
	if h == nil || h.Contents != "hello" {
		t.Fatalf("unexpected hover: %+v", h)
	}
}

func TestHoverEmpty(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":{"contents":null}}`)
	h, err := c.Hover("file:///x.go", Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatal(err)
	}
	if h != nil {
		t.Fatalf("expected nil hover, got %+v", h)
	}
}

func TestHoverError(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"fail"}}`)
	_, err := c.Hover("file:///x.go", Position{Line: 0, Character: 0})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSymbols(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":[{"name":"Foo","kind":1,"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"selectionRange":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}}]}`)
	syms, err := c.Symbols("file:///x.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 || syms[0].Name != "Foo" {
		t.Fatalf("unexpected symbols: %+v", syms)
	}
}

func TestSymbolsError(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"fail"}}`)
	_, err := c.Symbols("file:///x.go")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDefinitionRaw(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":"raw"}`)
	raw, err := c.DefinitionRaw("file:///x.go", Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatal(err)
	}
	if raw != `"raw"` {
		t.Fatalf("unexpected raw: %q", raw)
	}
}

func TestDefinitionRawError(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"fail"}}`)
	_, err := c.DefinitionRaw("file:///x.go", Position{Line: 0, Character: 0})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRename(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":{"changes":{"file:///x.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"newText":"New"}]}}}`)
	w, err := c.Rename("file:///x.go", Position{Line: 0, Character: 0}, "New")
	if err != nil {
		t.Fatal(err)
	}
	if w == nil || len(w.Changes) != 1 {
		t.Fatalf("unexpected workspace edit: %+v", w)
	}
}

func TestRenameError(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"fail"}}`)
	_, err := c.Rename("file:///x.go", Position{Line: 0, Character: 0}, "New")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFormat(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"newText":"  "}]}`)
	edits, err := c.Format("file:///x.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("expected 1 edit, got %d", len(edits))
	}
}

func TestFormatError(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"fail"}}`)
	_, err := c.Format("file:///x.go")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Call branches ---

func TestCallMarshalError(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(strings.NewReader(""))}
	err := c.Call("foo", map[string]any{"ch": make(chan int)}, nil, time.Second)
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestCallWriteError(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{err: errors.New("write fail")}, stdout: bufio.NewReader(strings.NewReader(""))}
	err := c.Call("foo", nil, nil, time.Second)
	if err == nil || err.Error() != "write fail" {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestCallEOF(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(strings.NewReader(""))}
	err := c.Call("foo", nil, nil, time.Second)
	if err == nil || !strings.Contains(err.Error(), "server closed before response") {
		t.Fatalf("expected EOF error, got %v", err)
	}
}

func TestCallTimeout(t *testing.T) {
	base := time.Now().Add(-1 * time.Second)
	var callCount int
	setTimeNowHook(t, func() time.Time {
		callCount++
		return base.Add(time.Duration(callCount) * time.Second)
	})
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(strings.NewReader(""))}
	err := c.Call("foo", nil, nil, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "LSP timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestCallReadError(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(&errReader{err: errors.New("boom")})}
	err := c.Call("foo", nil, nil, time.Second)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
}

func TestCallResponseError(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"fail"}}`)
	err := c.Call("foo", nil, nil, time.Second)
	if err == nil || !strings.Contains(err.Error(), "LSP error -1") {
		t.Fatalf("expected response error, got %v", err)
	}
}

func TestCallUnmarshalResult(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":123}`)
	var out []Location
	err := c.Call("foo", nil, &out, time.Second)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestCallEmptyResult(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":null}`)
	var out []Location
	err := c.Call("foo", nil, &out, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("expected nil out, got %+v", out)
	}
}

func TestCallNilResult(t *testing.T) {
	c := clientWithResponse(`{"jsonrpc":"2.0","id":1,"result":{"x":1}}`)
	err := c.Call("foo", nil, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

// --- readLSPFrame branches ---

func TestReadLSPFrameEOF(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(strings.NewReader(""))}
	_, err := c.readLSPFrame(time.Now().Add(time.Second))
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestReadLSPFrameTimeout(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(strings.NewReader(""))}
	_, err := c.readLSPFrame(time.Now().Add(-1 * time.Millisecond))
	if !isTimeoutErr(err) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestReadLSPFrameError(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(&errReader{err: errors.New("boom")})}
	_, err := c.readLSPFrame(time.Now().Add(time.Second))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom, got %v", err)
	}
}

func TestReadLSPFrameOuterTimeout(t *testing.T) {
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(strings.NewReader(""))}
	_, err := c.readLSPFrame(time.Now().Add(-1 * time.Millisecond))
	if !isTimeoutErr(err) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestReadLSPFrameInvalidJSON(t *testing.T) {
	bad := responseFrame(`not json`)
	good := responseFrame(`{"jsonrpc":"2.0","id":1,"result":null}`)
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(strings.NewReader(bad + good))}
	resp, err := c.readLSPFrame(time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID == nil {
		t.Fatal("expected response")
	}
}

func TestReadLSPFrameNotificationHandler(t *testing.T) {
	notif := responseFrame(`{"jsonrpc":"2.0","method":"$/progress","params":{}}`)
	resp := responseFrame(`{"jsonrpc":"2.0","id":1,"result":null}`)
	c := &Client{stdin: &fakeWriteCloser{}, stdout: bufio.NewReader(strings.NewReader(notif + resp))}
	var got string
	c.SetNotificationHandler(func(method string, params json.RawMessage) {
		got = method
	})
	_, err := c.readLSPFrame(time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got != "$/progress" {
		t.Fatalf("expected notification, got %q", got)
	}
}

// --- readRawLSPFrame branches ---

func TestReadRawLSPFrameDeadline(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(""))
	_, err := readRawLSPFrame(r, time.Now().Add(-1*time.Millisecond))
	if !isTimeoutErr(err) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestReadRawLSPFrameEmptyLine(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":null}`
	r := bufio.NewReader(strings.NewReader("\r\n\r\nContent-Length: 38\r\n\r\n" + body))
	frame, err := readRawLSPFrame(r, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(frame), `"id"`) {
		t.Fatalf("expected frame, got %s", frame)
	}
}

func TestReadRawLSPFrameNoColon(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("badheader\r\n\r\n"))
	_, err := readRawLSPFrame(r, time.Now().Add(time.Second))
	if err == nil {
		t.Fatal("expected error for no Content-Length")
	}
}

func TestReadRawLSPFrameNoContentLength(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("X-Header: 1\r\n\r\n"))
	_, err := readRawLSPFrame(r, time.Now().Add(time.Second))
	if err == nil {
		t.Fatal("expected error for no Content-Length")
	}
}

func TestReadRawLSPFramePostHeaderDeadline(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":null}`
	r := bufio.NewReader(strings.NewReader("Content-Length: " + intToStr(len(body)) + "\r\n\r\n" + body))
	base := time.Now()
	var callCount int
	setTimeNowHook(t, func() time.Time {
		callCount++
		if callCount <= 2 {
			return base
		}
		return base.Add(2 * time.Second)
	})
	_, err := readRawLSPFrame(r, base.Add(1*time.Second))
	if !isTimeoutErr(err) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestReadRawLSPFrameReadError(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":null}`
	r := bufio.NewReader(&errReader{content: "Content-Length: " + intToStr(len(body)) + "\r\n\r\n", err: errors.New("boom")})
	_, err := readRawLSPFrame(r, time.Now().Add(time.Second))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom, got %v", err)
	}
}

func TestReadFullWithDeadlineTimeout(t *testing.T) {
	r := bufio.NewReader(slowReader{})
	err := readFullWithDeadline(r, make([]byte, 10), 50*time.Millisecond)
	if !isTimeoutErr(err) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestIsTimeoutErr(t *testing.T) {
	if !isTimeoutErr(errTimeout) {
		t.Error("expected errTimeout to be a timeout")
	}
	if isTimeoutErr(errors.New("other")) {
		t.Error("expected other error not to be a timeout")
	}
}

// --- Registry branches ---

func TestManagerGetMissingBinaryWithHook(t *testing.T) {
	setLookPathHook(t, func(name string) (string, error) {
		return "", errors.New("not found")
	})
	m := NewManager()
	defer m.Close()
	_, err := m.Get("go", "file:///tmp")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestManagerGetStartErrorWithHook(t *testing.T) {
	setLookPathHook(t, func(name string) (string, error) {
		return "/fake/gopls", nil
	})
	setExecCommandHook(t, func(name string, args ...string) *exec.Cmd {
		return exec.Command("/nonexistent/binary/for/sin/lsp/test")
	})
	m := NewManager()
	defer m.Close()
	_, err := m.Get("go", "file:///tmp")
	if err == nil {
		t.Fatal("expected error")
	}
}
