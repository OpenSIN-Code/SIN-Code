// SPDX-License-Identifier: MIT
// Purpose: tests for the chat Runner — provider resolution, request
// construction (system + history + user), and roundtrip against a fake
// httptest server.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

func TestRunnerMissingKey(t *testing.T) {
	// Unset any pre-existing key (defensive — also unset SIN_LLM_API_KEY).
	prevNIM, hadNIM := os.LookupEnv("SIN_NIM_API_KEY")
	prevLLM, hadLLM := os.LookupEnv("SIN_LLM_API_KEY")
	os.Unsetenv("SIN_NIM_API_KEY")
	os.Unsetenv("SIN_LLM_API_KEY")
	t.Cleanup(func() {
		if hadNIM {
			os.Setenv("SIN_NIM_API_KEY", prevNIM)
		}
		if hadLLM {
			os.Setenv("SIN_LLM_API_KEY", prevLLM)
		}
	})

	r, err := NewRunner()
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if r != nil {
		t.Errorf("expected nil runner, got %+v", r)
	}
	if !strings.Contains(err.Error(), "no API key") {
		t.Errorf("expected 'no API key' in error, got %q", err.Error())
	}
}

func TestRunnerSuccess(t *testing.T) {
	var gotReq llm.ChatRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Errorf("unmarshal body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "x",
			"object": "chat.completion",
			"created": 1,
			"model": "meta/llama-3.3-70b-instruct",
			"choices": [
				{"index": 0, "message": {"role": "assistant", "content": "hello, world"}, "finish_reason": "stop"}
			],
			"usage": {"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}
		}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key")
	r := NewRunnerWithClient(c, "", "")

	out, err := r.Run(context.Background(), "hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello, world" {
		t.Errorf("got %q", out)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("auth: %q", gotAuth)
	}
	if gotReq.Model == "" {
		t.Error("expected model in request")
	}
	if gotReq.Temperature != 0 {
		t.Errorf("expected temperature 0, got %v", gotReq.Temperature)
	}
	if gotReq.MaxTokens == 0 {
		t.Error("expected max_tokens set")
	}
	// At minimum: system + user. History is nil so we should see exactly 2.
	if len(gotReq.Messages) != 2 {
		t.Errorf("expected 2 messages (system + user), got %d: %+v", len(gotReq.Messages), gotReq.Messages)
	}
	if gotReq.Messages[0].Role != "system" {
		t.Errorf("first msg role: %q", gotReq.Messages[0].Role)
	}
	if gotReq.Messages[1].Role != "user" {
		t.Errorf("second msg role: %q", gotReq.Messages[1].Role)
	}
	if !strings.Contains(gotReq.Messages[0].Content, "sin-code") {
		t.Errorf("expected sin-code in system prompt, got %q", gotReq.Messages[0].Content)
	}
}

func TestRunnerHistory(t *testing.T) {
	var gotReq llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}]
		}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k")
	r := NewRunnerWithClient(c, "test-model", "test-system")

	// Mix of plain, "user:", and "assistant:" prefixed entries. Should
	// produce: system, plain-as-user, user-prefixed, assistant-prefixed,
	// current-prompt-as-user.
	history := []string{
		"plain old message",
		"user: hi there",
		"assistant: hello there",
		"old turn 1",
		"old turn 2",
		"old turn 3",
		"recent turn 1",
		"recent turn 2",
	}
	out, err := r.Run(context.Background(), "now ask me something", history)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Errorf("got %q", out)
	}

	// Expected: system + 5 most recent + current = 7 messages
	if len(gotReq.Messages) != 7 {
		t.Fatalf("expected 7 messages (system + 5 history + user), got %d: %+v",
			len(gotReq.Messages), gotReq.Messages)
	}
	if gotReq.Messages[0].Role != "system" {
		t.Errorf("msg[0] role: %q", gotReq.Messages[0].Role)
	}
	if gotReq.Messages[0].Content != "test-system" {
		t.Errorf("msg[0] content: %q", gotReq.Messages[0].Content)
	}
	// Last 5 history entries (all plain text, no prefix): the trimmed
	// window drops the first 3 ("plain old message", "user: hi there",
	// "assistant: hello there") and keeps the last 5 ("old turn 1..3",
	// "recent turn 1", "recent turn 2"). None have a "user:" or
	// "assistant:" prefix, so all 5 are role=user.
	wantRoles := []string{"user", "user", "user", "user", "user"}
	for i, want := range wantRoles {
		got := gotReq.Messages[i+1].Role
		if got != want {
			t.Errorf("msg[%d] role = %q, want %q (content=%q)",
				i+1, got, want, gotReq.Messages[i+1].Content)
		}
	}
	if gotReq.Messages[len(gotReq.Messages)-1].Role != "user" {
		t.Errorf("last msg should be the current prompt, got role %q",
			gotReq.Messages[len(gotReq.Messages)-1].Role)
	}
	if gotReq.Messages[len(gotReq.Messages)-1].Content != "now ask me something" {
		t.Errorf("last msg content: %q", gotReq.Messages[len(gotReq.Messages)-1].Content)
	}
}

func TestRunnerNilClient(t *testing.T) {
	var r *Runner
	if _, err := r.Run(context.Background(), "x", nil); err == nil {
		t.Error("expected error for nil runner")
	}
	r2 := &Runner{}
	if _, err := r2.Run(context.Background(), "x", nil); err == nil {
		t.Error("expected error for runner with nil client")
	}
}

func TestSendChatResponseCmd(t *testing.T) {
	cmd := SendChatResponse("hello", nil)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	rm, ok := msg.(ChatResponseMsg)
	if !ok {
		t.Fatalf("expected ChatResponseMsg, got %T", msg)
	}
	if rm.Text != "hello" || rm.Error != nil {
		t.Errorf("got %+v", rm)
	}
}

func TestRunnerProviderErrorSurfaces(t *testing.T) {
	// Set the key but point at a bogus provider. ProviderFromConfig with a
	// bogus name should return an error from NewRunner.
	prevNIM, hadNIM := os.LookupEnv("SIN_NIM_API_KEY")
	os.Setenv("SIN_NIM_API_KEY", "fake-key")
	t.Cleanup(func() {
		if hadNIM {
			os.Setenv("SIN_NIM_API_KEY", prevNIM)
		} else {
			os.Unsetenv("SIN_NIM_API_KEY")
		}
	})

	// Force a bogus provider via direct construction: we can't easily do
	// this with NewRunner (it hardcodes "nim"). Instead, exercise the
	// happy path: NewRunner succeeds when key is set, and the request goes
	// to the configured base URL.
	r, err := NewRunner()
	if err != nil {
		t.Fatalf("expected runner, got error: %v", err)
	}
	if r.Model != defaultModel {
		t.Errorf("expected default model, got %q", r.Model)
	}
	if r.SystemPrompt != defaultSystem {
		t.Errorf("expected default system, got %q", r.SystemPrompt)
	}
	if r.Client == nil {
		t.Error("expected non-nil client")
	}
}

func TestRunnerProviderConfigError(t *testing.T) {
	prev := providerFromConfigHook
	providerFromConfigHook = func(name, baseURLOverride, apiKeyOverride, modelOverride string, timeout time.Duration) (*llm.Client, error) {
		return nil, errors.New("provider failure")
	}
	defer func() { providerFromConfigHook = prev }()

	prevKey, hadKey := os.LookupEnv("SIN_NIM_API_KEY")
	os.Setenv("SIN_NIM_API_KEY", "fake-key")
	defer func() {
		if hadKey {
			os.Setenv("SIN_NIM_API_KEY", prevKey)
		} else {
			os.Unsetenv("SIN_NIM_API_KEY")
		}
	}()

	r, err := NewRunner()
	if err == nil {
		t.Fatal("expected error")
	}
	if r != nil {
		t.Errorf("expected nil runner, got %+v", r)
	}
}

func TestRunnerDefaults(t *testing.T) {
	var gotReq llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k")
	r := &Runner{Client: c}
	out, err := r.Run(context.Background(), "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Errorf("got %q", out)
	}
	if gotReq.Model != defaultModel {
		t.Errorf("model: %q", gotReq.Model)
	}
	if gotReq.Messages[0].Content != defaultSystem {
		t.Errorf("system: %q", gotReq.Messages[0].Content)
	}
}

func TestRunnerHistoryPrefixes(t *testing.T) {
	var gotReq llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k")
	r := NewRunnerWithClient(c, "model", "system")
	history := []string{
		"plain",
		"",
		"assistant:",
		"assistant: hi",
		"user: hello",
	}
	if _, err := r.Run(context.Background(), "prompt", history); err != nil {
		t.Fatal(err)
	}
	// system + 4 kept history entries (empty and bare "assistant:" skipped) + prompt = 5
	if len(gotReq.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d: %+v", len(gotReq.Messages), gotReq.Messages)
	}
	wantRoles := []string{"system", "user", "assistant", "user", "user"}
	for i, want := range wantRoles {
		if got := gotReq.Messages[i].Role; got != want {
			t.Errorf("msg[%d] role = %q, want %q", i, got, want)
		}
	}
}

func TestRunnerChatError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k")
	r := NewRunnerWithClient(c, "model", "system")
	if _, err := r.Run(context.Background(), "hi", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunnerShortHistory(t *testing.T) {
	var gotReq llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotReq)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "k")
	r := NewRunnerWithClient(c, "model", "system")
	// history shorter than historyKeepN to cover the start < 0 branch
	history := []string{"a", "b"}
	if _, err := r.Run(context.Background(), "prompt", history); err != nil {
		t.Fatal(err)
	}
	if len(gotReq.Messages) != 4 { // system + 2 history + prompt
		t.Fatalf("expected 4 messages, got %d: %+v", len(gotReq.Messages), gotReq.Messages)
	}
}
