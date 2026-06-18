// SPDX-License-Identifier: MIT
// Purpose: Tests for the streaming result handler (issue #371).
package agentloop

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStreaming_WriteAndClose(t *testing.T) {
	var chunks []string
	var final string
	h := NewStreamHandler(StreamingResult{
		OnChunk: func(c string) { chunks = append(chunks, c) },
		OnDone:  func(r string) { final = r },
	})

	if _, err := h.Write([]byte("hello ")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := h.Write([]byte("world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if final != "hello world" {
		t.Errorf("final = %q, want %q", final, "hello world")
	}
}

func TestStreaming_OnErrorCalled(t *testing.T) {
	var streamErr error
	h := NewStreamHandler(StreamingResult{
		OnError: func(e error) { streamErr = e },
	})

	testErr := errors.New("boom")
	h.Fail(testErr)

	if streamErr == nil {
		t.Fatal("OnError was not called")
	}
	if streamErr.Error() != "boom" {
		t.Errorf("got %q, want %q", streamErr.Error(), "boom")
	}
}

func TestStreaming_WriteAfterClose(t *testing.T) {
	h := NewStreamHandler(StreamingResult{})
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := h.Write([]byte("late"))
	if err == nil {
		t.Fatal("Write after Close should return error")
	}
}

func TestStreaming_DoubleCloseSafe(t *testing.T) {
	calls := 0
	h := NewStreamHandler(StreamingResult{
		OnDone: func(r string) { calls++ },
	})
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if calls != 1 {
		t.Errorf("OnDone called %d times, want 1", calls)
	}
}

func TestStreaming_EmptyWrite(t *testing.T) {
	called := false
	h := NewStreamHandler(StreamingResult{
		OnChunk: func(c string) { called = true },
	})
	n, err := h.Write([]byte{})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 0 {
		t.Errorf("wrote %d bytes, want 0", n)
	}
	if called {
		t.Error("OnChunk should not be called for empty write")
	}
	_ = h.Close()
}

func TestStreaming_BufferMidStream(t *testing.T) {
	h := NewStreamHandler(StreamingResult{})
	_, _ = h.Write([]byte("partial"))
	got := h.Buffer()
	if got != "partial" {
		t.Errorf("Buffer() = %q, want %q", got, "partial")
	}
	_ = h.Close()
}

func TestStreaming_WaitBlocks(t *testing.T) {
	h := NewStreamHandler(StreamingResult{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = h.Close()
	}()
	select {
	case err := <-h.done:
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return within 1s")
	}
}

func TestStreaming_ConcurrentWrites(t *testing.T) {
	var mu sync.Mutex
	var allChunks []string
	h := NewStreamHandler(StreamingResult{
		OnChunk: func(c string) {
			mu.Lock()
			allChunks = append(allChunks, c)
			mu.Unlock()
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = h.Write([]byte(string(rune('A'+n))))
		}(i)
	}
	wg.Wait()
	_ = h.Close()

	full := h.Buffer()
	if len(full) != 10 {
		t.Errorf("buffer length = %d, want 10", len(full))
	}
	// Each goroutine wrote one byte; the buffer should contain 10 bytes total.
	for _, c := range "ABCDEFGHIJ" {
		if !strings.ContainsRune(full, c) {
			t.Errorf("buffer %q missing char %c", full, c)
		}
	}
}
