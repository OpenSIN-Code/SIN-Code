// SPDX-License-Identifier: MIT
// Purpose: thread-safe, buffered JSONL writer for the CDP ground-truth log.
// Each Sink wraps a single file and serialises all writes through a mutex so
// the sequence numbers assigned in Recorder.emit map 1-to-1 to lines on
// disk — there are no gaps and no reordering even under concurrent goroutines.
package cdp

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

// Sink writes ordered JSONL events to a file on disk.
// All writes are serialised so the on-disk order exactly matches the
// sequence numbers assigned by the Recorder.
type Sink struct {
	mu sync.Mutex
	f  *os.File
	w  *bufio.Writer
}

// NewSink creates (or truncates) the file at path and returns a ready Sink.
// The caller must call Close when recording is finished.
func NewSink(path string) (*Sink, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	// 64 KiB buffer: reduces syscall overhead for high-frequency events
	// while keeping memory per Sink predictable.
	return &Sink{f: f, w: bufio.NewWriterSize(f, 64*1024)}, nil
}

// write marshals e to JSON and appends it as a single line.
// It is safe to call from multiple goroutines.
func (s *Sink) write(e *Event) {
	b, err := json.Marshal(e)
	if err != nil {
		// Marshalling a struct that contains only json.RawMessage and
		// primitive types should never fail; skip rather than panic.
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.w.Write(b)
	_ = s.w.WriteByte('\n')
}

// Close flushes the write buffer and closes the underlying file.
// Must be called exactly once after recording stops.
func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.w.Flush(); err != nil {
		return err
	}
	return s.f.Close()
}
