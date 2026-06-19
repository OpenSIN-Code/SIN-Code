// SPDX-License-Identifier: MIT
// Purpose: thinking-budget wire-shape and Usage.ThinkingTokens
// roundtrip tests for the agentloop provider adapter.
//
// Covers:
//
//	(a) wireThinking JSON shape — nil/disabled/enabled omit correctly.
//	(b) NewProviderCompletionFull emits the wire block on the HTTP request.
//	(c) response-side Usage.ThinkingTokens comes back through *Completion.
//	(d) race-clean: concurrent invocations don't corrupt the payload.
//
// Docs: cmd/sin-code/internal/agentloop/thinking_budget_test.go
package agentloop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

// newTestLLMClient builds a real *llm.Client pointing at the test URL.
func newTestLLMClient(baseURL string) *llm.Client {
	return llm.NewClient(baseURL, "k")
}

type captureRequestOnce struct {
	mu      sync.Mutex
	bodyBuf []byte
}

func (c *captureRequestOnce) record(r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.bodyBuf = append([]byte(nil), b...)
	c.mu.Unlock()
}

func (c *captureRequestOnce) body() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.bodyBuf)
}

func TestWireThinkingJSONShape_NilDisabledEnabled(t *testing.T) {
	enabled := wireThinking{Type: "enabled", Budget: 4096}
	disabled := wireThinking{Type: "disabled"}
	bEn, _ := json.Marshal(struct {
		Thinking *wireThinking `json:"thinking,omitempty"`
	}{Thinking: &enabled})
	bDis, _ := json.Marshal(struct {
		Thinking *wireThinking `json:"thinking,omitempty"`
	}{Thinking: &disabled})
	bNone, _ := json.Marshal(struct {
		Thinking *wireThinking `json:"thinking,omitempty"`
	}{Thinking: nil})
	if !strings.Contains(string(bEn), `"type":"enabled"`) {
		t.Fatalf("enabled marshal missing type: %s", bEn)
	}
	if !strings.Contains(string(bEn), `"budget_tokens":4096`) {
		t.Fatalf("enabled marshal missing budget: %s", bEn)
	}
	if strings.Contains(string(bDis), `"budget_tokens"`) {
		t.Fatalf("disabled marshal should omit budget: %s", bDis)
	}
	if strings.Contains(string(bNone), `"thinking"`) {
		t.Fatalf("nil marshal should omit the field: %s", bNone)
	}
}

func TestProviderCompletion_EmitsWireThinking(t *testing.T) {
	capture := &captureRequestOnce{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.record(r)
		_, _ = io.WriteString(w, `{
  "id": "abc",
  "choices": [{
    "message": {"role": "assistant", "content": "ok"},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"thinking_tokens":42}
}`)
	}))
	defer srv.Close()

	c := newTestLLMClient(srv.URL)
	complete := NewProviderCompletionFull(c, "m", 0, 0, nil,
		&ThinkingConfig{Enabled: true, Budget: 8192})
	got, err := complete(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if got.Usage.ThinkingTokens != 42 {
		t.Fatalf("Usage.ThinkingTokens: want 42, got %d", got.Usage.ThinkingTokens)
	}
	if !strings.Contains(capture.body(), `"type":"enabled"`) {
		t.Fatalf("wire body missing thinking.type=enabled: %s", capture.body())
	}
	if !strings.Contains(capture.body(), `"budget_tokens":8192`) {
		t.Fatalf("wire body missing budget_tokens=8192: %s", capture.body())
	}
}

func TestProviderCompletion_NilThinking_NoWireBlock(t *testing.T) {
	capture := &captureRequestOnce{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.record(r)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"thinking_tokens":0}}`)
	}))
	defer srv.Close()

	c := newTestLLMClient(srv.URL)
	complete := NewProviderCompletionFull(c, "m", 0, 0, nil, nil)
	got, err := complete(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if got.Usage.ThinkingTokens != 0 {
		t.Fatalf("Usage.ThinkingTokens: want 0 on nil thinking, got %d", got.Usage.ThinkingTokens)
	}
	if strings.Contains(capture.body(), `"thinking"`) {
		t.Fatalf("nil thinking should NOT emit thinking block: %s", capture.body())
	}
}

func TestProviderCompletion_DisabledThinking_NoBudget(t *testing.T) {
	capture := &captureRequestOnce{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.record(r)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()
	c := newTestLLMClient(srv.URL)
	complete := NewProviderCompletionFull(c, "m", 0, 0, nil,
		&ThinkingConfig{Enabled: false, Budget: 9999})
	_, err := complete(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if strings.Contains(capture.body(), `"thinking"`) {
		t.Fatalf("disabled thinking should NOT emit thinking block: %s", capture.body())
	}
}

func TestProviderCompletion_RaceClean_ConcurrentThinking(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
		n      int32
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()
	c := newTestLLMClient(srv.URL)
	complete := NewProviderCompletionFull(c, "m", 0, 0, nil,
		&ThinkingConfig{Enabled: true, Budget: 256})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := complete(context.Background(), nil, nil)
			if err != nil {
				t.Errorf("chat error in race: %v", err)
			}
		}()
	}
	wg.Wait()
	if atomic.LoadInt32(&n) != 16 {
		t.Errorf("server saw %d requests, want 16", n)
	}
	mu.Lock()
	defer mu.Unlock()
	for i, b := range bodies {
		if !strings.Contains(b, `"type":"enabled"`) {
			t.Errorf("body[%d] missing thinking.type=enabled: %s", i, b)
		}
	}
}

func TestProviderCompletion_ThinkingTokensParser_ZeroMeansUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer srv.Close()
	c := newTestLLMClient(srv.URL)
	complete := NewProviderCompletionFull(c, "m", 0, 0, nil,
		&ThinkingConfig{Enabled: true, Budget: 256})
	got, err := complete(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if got.Usage.ThinkingTokens != 0 {
		t.Errorf("absent thinking_tokens should parse as 0: %d", got.Usage.ThinkingTokens)
	}
}
