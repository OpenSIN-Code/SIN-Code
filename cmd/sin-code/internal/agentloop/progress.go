// SPDX-License-Identifier: MIT
// Purpose: structured progress output for headless mode. Emits NDJSON
// to an io.Writer so the stable --json stdout contract is preserved.
package agentloop

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// ProgressEvent is a single structured progress line.
type ProgressEvent struct {
	Ts        string         `json:"ts"`
	Level     string         `json:"level"`
	SessionID string         `json:"session_id,omitempty"`
	GoalID    int64          `json:"goal_id,omitempty"`
	WorkerID  int            `json:"worker_id,omitempty"`
	Turn      int            `json:"turn,omitempty"`
	Event     string         `json:"event"`
	Tool      string         `json:"tool,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// ProgressWriter emits NDJSON progress events to an io.Writer in a race-safe way.
type ProgressWriter struct {
	mu     sync.Mutex
	w      io.Writer
	enc    *json.Encoder
	closed bool
	// Decorate, if set, is called before each event is written so callers
	// can inject per-consumer context (e.g. goal_id / worker_id for the
	// daemon). It must be safe for concurrent use.
	Decorate func(ev ProgressEvent) ProgressEvent
}

// NewProgressWriter creates a new NDJSON progress writer.
func NewProgressWriter(w io.Writer) *ProgressWriter {
	return &ProgressWriter{w: w, enc: json.NewEncoder(w)}
}

// Write emits a single progress event. It is safe for concurrent use.
func (p *ProgressWriter) Write(ev ProgressEvent) {
	if p == nil || p.w == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if p.Decorate != nil {
		ev = p.Decorate(ev)
	}
	ev.Ts = time.Now().UTC().Format(time.RFC3339)
	_ = p.enc.Encode(ev)
}

// Close marks the writer as closed. Further writes are ignored.
func (p *ProgressWriter) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
}
