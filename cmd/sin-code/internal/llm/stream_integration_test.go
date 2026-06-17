// SPDX-License-Identifier: MIT
// Purpose: real-world SSE streaming integration tests. These exercise
// the full Client.ChatStream → HTTP → SSE parse → callback pipeline
// against an httptest.Server that emits genuine text/event-stream
// chunks with simulated network latency. Distinct from the unit tests
// in stream_test.go which test parseSSELine and readSSEStream in
// isolation; these verify the end-to-end HTTP roundtrip including
// request-body marshaling, header negotiation, multi-chunk assembly,
// usage propagation, error status codes, and context cancellation.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatStreamIntegration(t *testing.T) {
	var gotReq ChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream, got %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
		}

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		if !gotReq.Stream {
			t.Error("expected stream=true in request body")
		}
		if gotReq.Model != "test-model" {
			t.Errorf("expected model test-model, got %q", gotReq.Model)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}

		chunks := []string{
			`data: {"id":"s1","model":"test-model","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
			`data: {"id":"s1","model":"test-model","choices":[{"index":0,"delta":{"content":" world"}}]}`,
			`data: {"id":"s1","model":"test-model","choices":[{"index":0,"delta":{"content":" from"}}]}`,
			`data: {"id":"s1","model":"test-model","choices":[{"index":0,"delta":{"content":" SIN-Code"}}]}`,
			`data: {"id":"s1","model":"test-model","choices":[{"index":0,"delta":{"content":"!"}}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`,
			`data: [DONE]`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "%s\n\n", chunk)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	var receivedChunks []string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.ChatStream(ctx, ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk StreamChunk) {
		if chunk.Content != "" {
			receivedChunks = append(receivedChunks, chunk.Content)
		}
	})

	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	if len(receivedChunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	fullText := strings.Join(receivedChunks, "")
	expected := "Hello world from SIN-Code!"
	if fullText != expected {
		t.Errorf("expected full text %q, got %q", expected, fullText)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Usage.TotalTokens != 10 {
		t.Errorf("expected 10 total tokens, got %d", resp.Usage.TotalTokens)
	}
	if resp.Usage.PromptTokens != 5 {
		t.Errorf("expected 5 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
	if resp.ExtractText() != expected {
		t.Errorf("response text mismatch: %q", resp.ExtractText())
	}
	if resp.ID != "s1" {
		t.Errorf("expected id s1, got %q", resp.ID)
	}
	if resp.Model != "test-model" {
		t.Errorf("expected model test-model, got %q", resp.Model)
	}
}

func TestChatStreamIntegrationErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid API key"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "bad-key")

	_, err := client.ChatStream(context.Background(), ChatRequest{
		Model:    "test",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk StreamChunk) {})

	if err == nil {
		t.Fatal("expected error for 401 status")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error message, got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid API key") {
		t.Errorf("expected error body in message, got %v", err)
	}
}

func TestChatStreamIntegrationContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}

		for i := 0; i < 100; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"x\"}}]}\n\n")
			flusher.Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := NewClient(server.URL, "test-key")

	_, err := client.ChatStream(ctx, ChatRequest{
		Model:    "test",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk StreamChunk) {})

	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
