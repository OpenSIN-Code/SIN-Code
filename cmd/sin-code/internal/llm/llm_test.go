// SPDX-License-Identifier: MIT
// Purpose: tests for the llm package: provider, ChatRequest/Response, NIM
// alias resolution, and an httptest-backed roundtrip of Client.Chat.
package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatRequestMarshal(t *testing.T) {
	req := ChatRequest{
		Model:       "meta/llama-3.1-70b-instruct",
		Messages:    []Message{{Role: "user", Content: "hi"}},
		MaxTokens:   256,
		Temperature: 0.5,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`"model":"meta/llama-3.1-70b-instruct"`,
		`"role":"user"`,
		`"content":"hi"`,
		`"max_tokens":256`,
		`"temperature":0.5`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestChatRequestMarshalOmitsZero(t *testing.T) {
	req := ChatRequest{
		Model:    "m",
		Messages: []Message{{Role: "user", Content: "x"}},
	}
	data, _ := json.Marshal(req)
	s := string(data)
	if strings.Contains(s, "max_tokens") {
		t.Errorf("expected max_tokens omitted, got %s", s)
	}
	if strings.Contains(s, "temperature") {
		t.Errorf("expected temperature omitted, got %s", s)
	}
	if strings.Contains(s, "stream") {
		t.Errorf("expected stream omitted, got %s", s)
	}
}

func TestChatResponseUnmarshal(t *testing.T) {
	body := `{
		"id": "cmpl-abc",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "meta/llama-3.1-70b-instruct",
		"choices": [
			{
				"index": 0,
				"message": {"role": "assistant", "content": "hello back"},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 12,
			"completion_tokens": 3,
			"total_tokens": 15
		}
	}`
	var resp ChatResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "cmpl-abc" {
		t.Errorf("id: %s", resp.ID)
	}
	if resp.Model != "meta/llama-3.1-70b-instruct" {
		t.Errorf("model: %s", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices: %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("role: %s", resp.Choices[0].Message.Role)
	}
	if resp.Choices[0].Message.Content != "hello back" {
		t.Errorf("content: %s", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish: %s", resp.Choices[0].FinishReason)
	}
	if resp.Usage.PromptTokens != 12 || resp.Usage.CompletionTokens != 3 || resp.Usage.TotalTokens != 15 {
		t.Errorf("usage: %+v", resp.Usage)
	}
}

func TestExtractText(t *testing.T) {
	resp := &ChatResponse{}
	if got := resp.ExtractText(); got != "" {
		t.Errorf("empty: %q", got)
	}
	resp.Choices = append(resp.Choices, struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	}{Message: Message{Role: "assistant", Content: "yo"}})
	if got := resp.ExtractText(); got != "yo" {
		t.Errorf("got %q", got)
	}
}

func TestResolveModelAliases(t *testing.T) {
	cases := map[string]string{
		"haiku":                 NIMHaikuModel,
		"sonnet":                NIMClaudeModel,
		"llama-70b":             NIMDefaultModel,
		"llama-8b":              "meta/llama-3.1-8b-instruct",
		"meta/llama-3.1-70b":    "meta/llama-3.1-70b",
		"anthropic/claude-3-7":  "anthropic/claude-3-7",
		"":                      "",
	}
	for in, want := range cases {
		if got := ResolveModel(in); got != want {
			t.Errorf("ResolveModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient("https://x", "k")
	if c.BaseURL != "https://x" || c.APIKey != "k" {
		t.Errorf("got %+v", c)
	}
	if c.HTTP == nil {
		t.Fatal("nil http client")
	}
	if c.HTTP.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
}

func TestClientChatSuccess(t *testing.T) {
	var gotAuth string
	var gotBody ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Errorf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "x",
			"object": "chat.completion",
			"created": 1,
			"model": "m",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "world"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3}
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "secret-123")
	req := ChatRequest{
		Model:     "m",
		Messages:  []Message{{Role: "user", Content: "hello"}},
		MaxTokens: 50,
	}
	resp, err := c.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret-123" {
		t.Errorf("auth: %q", gotAuth)
	}
	if gotBody.Model != "m" {
		t.Errorf("model: %s", gotBody.Model)
	}
	if gotBody.Messages[0].Content != "hello" {
		t.Errorf("msg: %+v", gotBody.Messages[0])
	}
	if resp.ExtractText() != "world" {
		t.Errorf("text: %q", resp.ExtractText())
	}
	if resp.Usage.TotalTokens != 3 {
		t.Errorf("tokens: %d", resp.Usage.TotalTokens)
	}
}

func TestClientChatNoAuthHeaderWhenEmptyKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("expected no auth header")
		}
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	if _, err := c.Chat(context.Background(), ChatRequest{Model: "m"}); err != nil {
		t.Fatal(err)
	}
}

func TestClientChatErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "bad")
	_, err := c.Chat(context.Background(), ChatRequest{Model: "m"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "bad key") {
		t.Errorf("expected body in error, got %v", err)
	}
}

func TestClientChatMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	_, err := c.Chat(context.Background(), ChatRequest{Model: "m"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientChatContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Chat(ctx, ChatRequest{Model: "m"})
	if err == nil {
		t.Fatal("expected context error")
	}
}
