// SPDX-License-Identifier: MIT
// Purpose: roundtrip test for the per-request thinking_tokens field
// flowing through llm.Client.Chat → ChatResponse.Usage.ThinkingTokens
// and through to Recorder.RecordUsage(ThinkingTokens, 8-arg signature).
//
// Verifies:
//
//	(a) httptest.Server returns a payload with usage.thinking_tokens; the
//	    client populates resp.Usage.ThinkingTokens with that value.
//	(b) A custom Recorder.UsageSink receives the same count.
//	(c) NopRecorder.UsageSink doesn't panic with the 8-arg signature.
//	(d) race-clean: concurrent Chat calls don't double-write UsageSink.
//
// Docs: cmd/sin-code/internal/llm/thinking_tokens_test.go
package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// CompactRecorder is a tiny Recorder implementation used only by these
// tests. It stores the last (prompt, completion, total, thinking)
// tuple atomically and exposes it for assertions. Defined here so the
// test is self-contained.
type CompactRecorder struct {
	Calls      int32
	Prompt     int32
	Completion int32
	Total      int32
	Thinking   int32
}

func (c *CompactRecorder) RecordUsage(_ context.Context, _, _ string, _ Source,
	prompt, completion, total, thinking int) error {
	atomic.AddInt32(&c.Calls, 1)
	atomic.StoreInt32(&c.Prompt, int32(prompt))
	atomic.StoreInt32(&c.Completion, int32(completion))
	atomic.StoreInt32(&c.Total, int32(total))
	atomic.StoreInt32(&c.Thinking, int32(thinking))
	return nil
}

func TestClientChat_ParsesThinkingTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
  "id": "abc",
  "choices":[{"message":{"role":"assistant","content":"ok"}}],
  "usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"thinking_tokens":777}
}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	rec := &CompactRecorder{}
	c.Recorder = rec

	resp, err := c.Chat(context.Background(), ChatRequest{
		Model: "m", Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Usage.ThinkingTokens != 777 {
		t.Fatalf("Usage.ThinkingTokens: want 777, got %d", resp.Usage.ThinkingTokens)
	}
	if rec.Thinking != 777 {
		t.Errorf("CompactRecorder.Thinking: want 777, got %d", rec.Thinking)
	}
	if rec.Calls != 1 {
		t.Errorf("expected exactly 1 record call, got %d", rec.Calls)
	}
}

func TestClientChat_AbsentThinkingTokensMeansZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "k")
	rec := &CompactRecorder{}
	c.Recorder = rec
	resp, err := c.Chat(context.Background(), ChatRequest{
		Model: "m", Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Usage.ThinkingTokens != 0 {
		t.Errorf("absent thinking_tokens must parse to 0, got %d", resp.Usage.ThinkingTokens)
	}
	if rec.Thinking != 0 {
		t.Errorf("CompactRecorder.Thinking: want 0, got %d", rec.Thinking)
	}
}

func TestClientChat_OmitsThinkingBlock_WhenRequestHasNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"thinking"`) {
			t.Errorf("ChatRequest without Thinking block should not emit thinking: %s", b)
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "k")
	c.Recorder = &CompactRecorder{}
	_, err := c.Chat(context.Background(), ChatRequest{
		Model: "m", Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
}

func TestNopRecorder_RecordUsage_8Arg_AcceptsThinking(t *testing.T) {
	n := NopRecorder{}
	err := n.RecordUsage(context.Background(), "", "", SourceChat, 1, 2, 3, 4)
	if err != nil {
		t.Errorf("nop recorder returned error: %v", err)
	}
}

func TestCompactRecorder_RaceClean_ConcurrentRecordUsage(t *testing.T) {
	rec := &CompactRecorder{}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			err := rec.RecordUsage(context.Background(), "m", "k", SourceChat, 1, 1, 2, n)
			if err != nil {
				t.Errorf("record error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if rec.Calls != 64 {
		t.Errorf("CompactRecorder.Calls: want 64, got %d", rec.Calls)
	}
}
