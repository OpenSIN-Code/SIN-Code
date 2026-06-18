// SPDX-License-Identifier: MIT
// Purpose: Streaming tool results for long-running commands (issue #371).
// StreamHandler implements io.Writer so subprocesses can pipe stdout/stderr
// through it, invoking OnChunk for each write and OnDone when Close is called.
// Thread-safe (mandate M7) via a mutex on the shared buffer.
package agentloop

import (
	"errors"
	"strings"
	"sync"
)

// StreamingResult holds optional callbacks for stream events.
type StreamingResult struct {
	OnChunk func(chunk string)
	OnDone  func(result string)
	OnError func(err error)
}

// StreamHandler implements io.Writer and collects chunks from a streaming
// source. It is safe for concurrent use (mandate M7).
type StreamHandler struct {
	callbacks StreamingResult
	buffer    strings.Builder
	mu        sync.Mutex
	chunks    chan string
	done      chan error
	closed    bool
}

// NewStreamHandler creates a new StreamHandler with the given callbacks.
// The chunks channel has a buffer of 64 to avoid blocking fast writers.
func NewStreamHandler(callbacks StreamingResult) *StreamHandler {
	return &StreamHandler{
		callbacks: callbacks,
		chunks:    make(chan string, 64),
		done:      make(chan error, 1),
	}
}

// Write implements io.Writer. Each call appends to the internal buffer
// (under mutex) and invokes OnChunk with the written data. It returns
// the number of bytes written and any error from the callback.
func (h *StreamHandler) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	chunk := string(p)

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return 0, errors.New("stream handler is closed")
	}
	h.buffer.WriteString(chunk)
	h.mu.Unlock()

	if h.callbacks.OnChunk != nil {
		h.callbacks.OnChunk(chunk)
	}

	return len(p), nil
}

// Close finalises the stream. It calls OnDone with the full accumulated
// buffer content. Calling Close more than once is safe and returns nil
// on subsequent calls without re-invoking OnDone.
func (h *StreamHandler) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	full := h.buffer.String()
	h.mu.Unlock()

	if h.callbacks.OnDone != nil {
		h.callbacks.OnDone(full)
	}

	select {
	case h.done <- nil:
	default:
	}
	return nil
}

// Fail signals an error condition. It calls OnError with the given error
// and marks the handler as closed so further Write calls are rejected.
// Safe to call instead of Close or in addition to it.
func (h *StreamHandler) Fail(err error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.mu.Unlock()

	if h.callbacks.OnError != nil {
		h.callbacks.OnError(err)
	}

	select {
	case h.done <- err:
	default:
	}
}

// Wait blocks until the stream is closed or fails, returning the error
// (nil on clean close). This is useful for callers that need to
// synchronise on stream completion.
func (h *StreamHandler) Wait() error {
	return <-h.done
}

// Buffer returns the full accumulated content so far. This is safe to
// call at any point (including before Close).
func (h *StreamHandler) Buffer() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.buffer.String()
}
