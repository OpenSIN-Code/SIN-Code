// SPDX-License-Identifier: MIT
// Purpose: fix a protocol mismatch between the go-sdk MCP client and Python
// MCP servers (e.g. Simone-MCP) that send capabilities.extensions as a JSON
// array instead of a map. The MCP spec requires extensions to be a map with
// string keys, but the Python MCP SDK used by some servers serializes it as
// an array of objects with a "uri" field. This wrapper intercepts the raw
// JSON-RPC stream at the io.Reader level and normalises the extensions field
// before the go-sdk unmarshaler sees it.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultFixTerminateDuration is how long Close waits after closing stdin
// for the process to exit before sending SIGTERM.
const defaultFixTerminateDuration = 5 * time.Second

// newExtensionsFixTransport creates a Transport that starts the given command,
// wraps its stdout in a JSON-normalising reader, and handles process lifecycle
// (SIGTERM/SIGKILL on Close) identically to sdk.CommandTransport.
func newExtensionsFixTransport(cmd *exec.Cmd) sdk.Transport {
	return &extensionsFixTransport{cmd: cmd}
}

type extensionsFixTransport struct {
	cmd *exec.Cmd
}

func (t *extensionsFixTransport) Connect(ctx context.Context) (sdk.Connection, error) {
	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := t.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := t.cmd.Start(); err != nil {
		return nil, err
	}
	rwc := &fixRWC{
		cmd:    t.cmd,
		stdout: &extensionsFixReadCloser{raw: stdout},
		stdin:  stdin,
	}
	return rwc.connect(ctx)
}

// fixRWC is an io.ReadWriteCloser that wraps a subprocess with a fixing
// reader on stdout. It handles process termination on Close identically
// to sdk.CommandTransport's pipeRWC.
type fixRWC struct {
	cmd    *exec.Cmd
	stdout *extensionsFixReadCloser
	stdin  io.WriteCloser
	mu     sync.Mutex
	closed bool
}

func (s *fixRWC) Read(p []byte) (int, error) {
	return s.stdout.Read(p)
}

func (s *fixRWC) Write(p []byte) (int, error) {
	return s.stdin.Write(p)
}

func (s *fixRWC) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if err := s.stdin.Close(); err != nil {
		return fmt.Errorf("closing stdin: %v", err)
	}
	resChan := make(chan error, 1)
	go func() { resChan <- s.cmd.Wait() }()
	wait := func() (error, bool) {
		select {
		case err := <-resChan:
			return err, true
		case <-time.After(defaultFixTerminateDuration):
		}
		return nil, false
	}
	if err, ok := wait(); ok {
		return err
	}
	if err := s.cmd.Process.Signal(syscall.SIGTERM); err == nil {
		if err, ok := wait(); ok {
			return err
		}
	}
	if err := s.cmd.Process.Kill(); err != nil {
		return err
	}
	if err, ok := wait(); ok {
		return err
	}
	return fmt.Errorf("unresponsive subprocess")
}

// connect creates an sdk.Connection from the fixRWC. We use IOTransport
// internally since it accepts separate Reader/Writer and delegates to
// newIOConn — exactly what CommandTransport does, but with our fixing reader.
func (s *fixRWC) connect(ctx context.Context) (sdk.Connection, error) {
_transport := &sdk.IOTransport{
	Reader: s.stdout,
	Writer: s.stdin,
}
	// IOTransport.Connect creates an ioConn with a no-op Close. We need
	// our own Close to handle process termination. So we wrap the
	// connection returned by IOTransport with a closingConn.
	conn, err := _transport.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &closingConn{Connection: conn, closeFn: s.Close}, nil
}

// closingConn wraps an sdk.Connection and delegates Close to a custom
// function (process termination) instead of the default no-op.
type closingConn struct {
	sdk.Connection
	closeFn func() error
}

func (c *closingConn) Close() error {
	return c.closeFn()
}

// extensionsFixReadCloser wraps an io.ReadCloser and normalises the
// capabilities.extensions field in JSON-RPC messages from array to map.
// The JSON decoder in the go-sdk reads JSON values using json.Decoder,
// so we intercept at the Read level by buffering and fixing each JSON
// value before returning it.
type extensionsFixReadCloser struct {
	raw    io.ReadCloser
	buf    bytes.Buffer
	dec    *json.Decoder
	done   bool
	initMu sync.Once
}

func (r *extensionsFixReadCloser) Read(p []byte) (int, error) {
	r.initMu.Do(func() {
		r.dec = json.NewDecoder(r.raw)
	})

	// If we have buffered fixed data, return it first
	if r.buf.Len() > 0 {
		return r.buf.Read(p)
	}
	if r.done {
		return 0, io.EOF
	}

	// Read the next JSON value from the raw stream
	var raw json.RawMessage
	if err := r.dec.Decode(&raw); err != nil {
		return 0, err
	}

	// Fix the JSON value if needed
	fixed := fixExtensionsInJSON(raw)
	r.buf.Write(fixed)

	// Write a trailing newline (json.Decoder expects whitespace between values)
	r.buf.WriteByte('\n')

	return r.buf.Read(p)
}

func (r *extensionsFixReadCloser) Close() error {
	return r.raw.Close()
}

// fixExtensionsInJSON scans a JSON-RPC message for the InitializeResult
// response and, if capabilities.extensions is an array, converts it to a
// map keyed by each element's "uri" field. Returns the original data
// unchanged if no fix is needed.
func fixExtensionsInJSON(data []byte) []byte {
	// Quick check: only process messages that contain "extensions"
	if !bytes.Contains(data, []byte(`"extensions"`)) {
		return data
	}

	// Parse as a generic JSON-RPC response to access the result
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return data
	}

	result, ok := msg["result"]
	if !ok {
		return data
	}

	// Parse the result to get capabilities
	var resultObj map[string]json.RawMessage
	if err := json.Unmarshal(result, &resultObj); err != nil {
		return data
	}

	caps, ok := resultObj["capabilities"]
	if !ok {
		return data
	}

	// Parse capabilities to get extensions
	var capsObj map[string]json.RawMessage
	if err := json.Unmarshal(caps, &capsObj); err != nil {
		return data
	}

	extRaw, ok := capsObj["extensions"]
	if !ok {
		return data
	}

	// Check if extensions is an array
	var extArr []map[string]any
	if err := json.Unmarshal(extRaw, &extArr); err != nil {
		// Not an array — probably already a map, leave as-is
		return data
	}

	// Convert array to map keyed by "uri"
	extMap := make(map[string]any)
	for _, item := range extArr {
		key, _ := item["uri"].(string)
		if key == "" {
			continue
		}
		delete(item, "uri")
		extMap[key] = item
	}

	// Re-serialize the fixed extensions
	fixedExt, err := json.Marshal(extMap)
	if err != nil {
		return data
	}
	capsObj["extensions"] = fixedExt

	// Re-serialize capabilities
	fixedCaps, err := json.Marshal(capsObj)
	if err != nil {
		return data
	}
	resultObj["capabilities"] = fixedCaps

	// Re-serialize result
	fixedResult, err := json.Marshal(resultObj)
	if err != nil {
		return data
	}
	msg["result"] = fixedResult

	// Re-serialize the entire message
	fixed, err := json.Marshal(msg)
	if err != nil {
		return data
	}
	return fixed
}
