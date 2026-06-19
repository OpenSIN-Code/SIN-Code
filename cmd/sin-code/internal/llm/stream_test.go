// SPDX-License-Identifier: MIT
// Purpose: tests for the SSE streaming layer — parseSSELine unit tests,
// readSSEStream against a mock io.Reader, and a full httptest roundtrip
// of Client.ChatStream including usage recording and error status codes.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSSELineDataChunk(t *testing.T) {
	line := `data: {"choices":[{"index":0,"delta":{"content":"hello"}}]}`
	chunk, done, err := parseSSELine(line)
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("expected done=false for data chunk")
	}
	if chunk == nil {
		t.Fatal("expected non-nil chunk")
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Content != "hello" {
		t.Errorf("content: %q", chunk.Choices[0].Delta.Content)
	}
}

func TestParseSSELineDone(t *testing.T) {
	chunk, done, err := parseSSELine("data: [DONE]")
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Error("expected done=true for [DONE]")
	}
	if chunk != nil {
		t.Error("expected nil chunk for [DONE]")
	}
}

func TestParseSSELineNonDataField(t *testing.T) {
	// event:, id:, retry: fields should be skipped (nil, false, nil)
	chunk, done, err := parseSSELine("event: chat.completion")
	if err != nil {
		t.Fatal(err)
	}
	if chunk != nil || done {
		t.Errorf("expected (nil, false, nil) for non-data field, got (%v, %v)", chunk, done)
	}
}

func TestParseSSELineComment(t *testing.T) {
	// SSE comments start with ':' — parseSSELine treats them as non-data
	chunk, done, err := parseSSELine(": keep-alive")
	if err != nil {
		t.Fatal(err)
	}
	if chunk != nil || done {
		t.Errorf("expected (nil, false, nil) for comment, got (%v, %v)", chunk, done)
	}
}

func TestParseSSELineEmptyData(t *testing.T) {
	chunk, done, err := parseSSELine("data: ")
	if err != nil {
		t.Fatal(err)
	}
	if chunk != nil || done {
		t.Errorf("expected (nil, false, nil) for empty data, got (%v, %v)", chunk, done)
	}
}

func TestParseSSELineMalformedJSON(t *testing.T) {
	_, _, err := parseSSELine("data: {not json}")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decode data payload") {
		t.Errorf("expected decode error, got %v", err)
	}
}

func TestParseSSELineWithUsage(t *testing.T) {
	line := `data: {"id":"x","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	chunk, done, err := parseSSELine(line)
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("expected done=false")
	}
	if chunk == nil || chunk.Usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if chunk.Usage.TotalTokens != 15 {
		t.Errorf("total_tokens: %d", chunk.Usage.TotalTokens)
	}
	if chunk.Usage.PromptTokens != 10 {
		t.Errorf("prompt_tokens: %d", chunk.Usage.PromptTokens)
	}
}

func TestParseSSELineNoChoices(t *testing.T) {
	// Some providers send a chunk with empty choices (e.g. usage-only)
	line := `data: {"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	chunk, _, err := parseSSELine(line)
	if err != nil {
		t.Fatal(err)
	}
	if chunk == nil {
		t.Fatal("expected non-nil chunk")
	}
	if len(chunk.Choices) != 0 {
		t.Errorf("expected 0 choices, got %d", len(chunk.Choices))
	}
	if chunk.Usage == nil || chunk.Usage.TotalTokens != 2 {
		t.Errorf("usage: %+v", chunk.Usage)
	}
}

func TestExtractDeltaContent(t *testing.T) {
	payload := []byte(`{"choices":[{"index":0,"delta":{"content":"world"}}]}`)
	got, err := extractDeltaContent(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got != "world" {
		t.Errorf("got %q", got)
	}
}

func TestExtractDeltaContentEmptyChoices(t *testing.T) {
	payload := []byte(`{"choices":[]}`)
	got, err := extractDeltaContent(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestReadSSEStreamBasic(t *testing.T) {
	// Simulate a complete SSE response with three content chunks + [DONE]
	body := strings.Join([]string{
		`data: {"id":"x","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
		"",
		`data: {"id":"x","model":"m","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
		"",
		`data: {"id":"x","model":"m","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		"",
		`data: {"id":"x","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	var chunks []string
	resp, err := readSSEStream(context.Background(), strings.NewReader(body), func(c StreamChunk) {
		if c.Content != "" {
			chunks = append(chunks, c.Content)
		}
	}, nil, "m")
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// "Hello" and " world" are the two content fragments
	wantChunks := []string{"Hello", " world"}
	if len(chunks) != len(wantChunks) {
		t.Fatalf("expected %d chunks, got %d: %v", len(wantChunks), len(chunks), chunks)
	}
	for i, want := range wantChunks {
		if chunks[i] != want {
			t.Errorf("chunk[%d]: %q, want %q", i, chunks[i], want)
		}
	}
	if resp.ExtractText() != "Hello world" {
		t.Errorf("full text: %q", resp.ExtractText())
	}
	if resp.Usage.TotalTokens != 7 {
		t.Errorf("total_tokens: %d", resp.Usage.TotalTokens)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason: %q", resp.Choices[0].FinishReason)
	}
}

func TestReadSSEStreamNoUsage(t *testing.T) {
	// Some providers don't send usage in the stream — Usage should be zero
	body := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
		"",
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	resp, err := readSSEStream(context.Background(), strings.NewReader(body), func(StreamChunk) {}, nil, "m")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ExtractText() != "hi" {
		t.Errorf("text: %q", resp.ExtractText())
	}
	if resp.Usage.TotalTokens != 0 {
		t.Errorf("expected zero usage, got %d", resp.Usage.TotalTokens)
	}
}

func TestReadSSEStreamContextCancel(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"content":"a"}}]}`,
		"",
		`data: {"choices":[{"index":0,"delta":{"content":"b"}}]}`,
		"",
	}, "\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before reading

	_, err := readSSEStream(ctx, strings.NewReader(body), func(StreamChunk) {}, nil, "m")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestReadSSEStreamCommentLines(t *testing.T) {
	// SSE comments (starting with :) and empty lines should be skipped
	body := strings.Join([]string{
		`: keep-alive`,
		"",
		`data: {"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		"",
		`: another comment`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	resp, err := readSSEStream(context.Background(), strings.NewReader(body), func(StreamChunk) {}, nil, "m")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ExtractText() != "ok" {
		t.Errorf("text: %q", resp.ExtractText())
	}
}

func TestReadSSEStreamMalformedJSON(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"content":"a"}}]}`,
		"",
		`data: {broken}`,
		"",
	}, "\n")

	_, err := readSSEStream(context.Background(), strings.NewReader(body), func(StreamChunk) {}, nil, "m")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse SSE event") {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestReadSSEStreamEmptyBody(t *testing.T) {
	resp, err := readSSEStream(context.Background(), strings.NewReader(""), func(StreamChunk) {}, nil, "m")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ExtractText() != "" {
		t.Errorf("expected empty text, got %q", resp.ExtractText())
	}
}

func TestReadSSEStreamWithRecorder(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"content":"hi"}}]}`,
		"",
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	rec := &fakeRecorder{}
	resp, err := readSSEStream(context.Background(), strings.NewReader(body), func(StreamChunk) {}, rec, "m")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.TotalTokens != 4 {
		t.Errorf("total_tokens: %d", resp.Usage.TotalTokens)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 recorder event, got %d", len(rec.events))
	}
	if rec.events[0].Total != 4 {
		t.Errorf("expected total=4, got %d", rec.events[0].Total)
	}
}

func TestChatStreamHTTPRoundtrip(t *testing.T) {
	var gotReq ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream, got %q", r.Header.Get("Accept"))
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		if !gotReq.Stream {
			t.Error("expected stream=true in request body")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// Write SSE chunks
		events := []string{
			`data: {"id":"s1","model":"m","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
			`data: {"id":"s1","model":"m","choices":[{"index":0,"delta":{"content":" stream"}}]}`,
			`data: {"id":"s1","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`,
			`data: [DONE]`,
		}
		for _, ev := range events {
			_, _ = w.Write([]byte(ev + "\n\n"))
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")
	var chunks []string
	resp, err := c.ChatStream(context.Background(), ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk StreamChunk) {
		if chunk.Content != "" {
			chunks = append(chunks, chunk.Content)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 content chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "Hello" || chunks[1] != " stream" {
		t.Errorf("chunks: %v", chunks)
	}
	if resp.ExtractText() != "Hello stream" {
		t.Errorf("full text: %q", resp.ExtractText())
	}
	if resp.Usage.TotalTokens != 4 {
		t.Errorf("total_tokens: %d", resp.Usage.TotalTokens)
	}
}

func TestChatStreamErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	_, err := c.ChatStream(context.Background(), ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "x"}},
	}, func(StreamChunk) {})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected 429 in error, got %v", err)
	}
}

func TestChatStreamContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response — will be interrupted by context cancel
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"index":0,"delta":{"content":"x"}}]}` + "\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done() // block until client cancels
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		cancel()
	}()
	_, err := c.ChatStream(ctx, ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "x"}},
	}, func(StreamChunk) {})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestHasStreaming(t *testing.T) {
	c := NewClient("https://x", "k")
	if !c.HasStreaming() {
		t.Error("expected HasStreaming=true for NewClient")
	}
	var nilClient *Client
	if nilClient.HasStreaming() {
		t.Error("expected HasStreaming=false for nil client")
	}
	empty := &Client{}
	if empty.HasStreaming() {
		t.Error("expected HasStreaming=false for client without HTTP")
	}
}

func TestChatStreamNilClient(t *testing.T) {
	var c *Client
	_, err := c.ChatStream(context.Background(), ChatRequest{}, func(StreamChunk) {})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestChatStreamNilCallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	// nil onChunk should not panic
	_, err := c.ChatStream(context.Background(), ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "x"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}
