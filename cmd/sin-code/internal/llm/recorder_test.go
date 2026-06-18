// SPDX-License-Identifier: MIT
// Purpose: tests for the LLM-Usage Recorder interface.
// Docs: docs/TOKEN-TRACKING.md
package llm

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNopRecorder_RecordUsage(t *testing.T) {
	r := NopRecorder{}
	err := r.RecordUsage(context.Background(), "sess-1", "claude-haiku-4-5", SourceAdHoc, 100, 50, 150, 25)
	if err != nil {
		t.Fatalf("NopRecorder should never error, got: %v", err)
	}
}

func TestSessionIDFromContext_Absent(t *testing.T) {
	if got := SessionIDFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty session id, got %q", got)
	}
}

func TestWithSessionID_Present(t *testing.T) {
	ctx := WithSessionID(context.Background(), "sess-abc")
	if got := SessionIDFromContext(ctx); got != "sess-abc" {
		t.Fatalf("expected sess-abc, got %q", got)
	}
}

func TestWithSessionID_Overwrite(t *testing.T) {
	ctx := WithSessionID(context.Background(), "outer")
	ctx = WithSessionID(ctx, "inner")
	if got := SessionIDFromContext(ctx); got != "inner" {
		t.Fatalf("expected inner to overwrite outer, got %q", got)
	}
}

func TestWithSessionID_WrongType(t *testing.T) {
	// Defensive: if something else stored an int at the same key
	// (e.g. another package using the same context-key strategy),
	// SessionIDFromContext should return "" rather than panic.
	ctx := context.WithValue(context.Background(), sessionIDKey{}, 42)
	if got := SessionIDFromContext(ctx); got != "" {
		t.Fatalf("expected empty on type-mismatch, got %q", got)
	}
}

func TestDefaultSessionID_Stable(t *testing.T) {
	a := DefaultSessionID()
	b := DefaultSessionID()
	if a == "" {
		t.Fatal("DefaultSessionID returned empty")
	}
	if a != b {
		t.Fatalf("DefaultSessionID is not stable across calls: %q vs %q", a, b)
	}
}

func TestDefaultSessionID_Format(t *testing.T) {
	// 8 bytes hex = 16 chars
	id := DefaultSessionID()
	if len(id) != 16 {
		t.Fatalf("expected 16-char hex id, got %d chars: %q", len(id), id)
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("non-hex char in id: %q", id)
		}
	}
}

// fakeRecorder is a minimal in-memory recorder for concurrency
// tests. Production code uses internal/usage.Store; this fake is
// for the interface contract only.
type fakeRecorder struct {
	mu     sync.Mutex
	events []fakeEvent
	failOn int32 // atomic; if >= 0, fail the Nth call
	calls  int32
}

type fakeEvent struct {
	SessionID, Model, Source  string
	Prompt, Completion, Total int
	Thinking                  int
}

func (f *fakeRecorder) RecordUsage(_ context.Context, sessionID, model string, source Source, p, c, t, thinking int) error {
	n := atomic.AddInt32(&f.calls, 1)
	if atomic.LoadInt32(&f.failOn) == n {
		return errors.New("synthetic failure")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, fakeEvent{
		SessionID: sessionID, Model: model, Source: string(source),
		Prompt: p, Completion: c, Total: t, Thinking: thinking,
	})
	return nil
}

func TestRecorder_ConcurrentSafe(t *testing.T) {
	// The Recorder contract requires safe concurrent use. Hammer it
	// from many goroutines and assert the event count matches.
	f := &fakeRecorder{}
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = f.RecordUsage(context.Background(), "s", "m", SourceAdHoc, i, i, i*2, i)
		}(i)
	}
	wg.Wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := len(f.events); got != n {
		t.Fatalf("expected %d events, got %d", n, got)
	}
}

func TestRecorder_ErrorIsReported(t *testing.T) {
	f := &fakeRecorder{failOn: 3}
	for i := 1; i <= 5; i++ {
		err := f.RecordUsage(context.Background(), "s", "m", SourceChat, i, i, i*2, i)
		wantErr := i == 3
		if (err != nil) != wantErr {
			t.Fatalf("call %d: want err=%v, got %v", i, wantErr, err)
		}
	}
}
