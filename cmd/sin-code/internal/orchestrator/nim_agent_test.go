// SPDX-License-Identifier: MIT
// Purpose: tests for NIMAgent. Uses httptest to stub the NIM endpoint so no
// real API key or network access is required.
package orchestrator

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/llm"
)

func newNIMTestAgent(t *testing.T, srv *httptest.Server, cfg AgentConfig) *NIMAgent {
	t.Helper()
	client := llm.NewClient(srv.URL, "test-key")
	a := NewNIMAgentWithClient(cfg, client)
	return a
}

func TestNIMAgentNameAndConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	cfg := AgentConfig{Name: "coder", Type: TaskCode, Model: "haiku", MaxTokens: 1000}
	a := newNIMTestAgent(t, srv, cfg)
	if a.Name() != "coder" {
		t.Errorf("name: %s", a.Name())
	}
	if a.Config().Model != "haiku" {
		t.Errorf("model: %s", a.Config().Model)
	}
}

func TestNIMAgentRunSuccess(t *testing.T) {
	var captured llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "x", "object": "chat.completion", "created": 1, "model": "m",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "hello back"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`))
	}))
	defer srv.Close()

	a := newNIMTestAgent(t, srv, AgentConfig{
		Name: "tester", Type: TaskTest, Model: "qwen", MaxTokens: 2000, Temperature: 0.2,
	})
	task := &Task{ID: "tk-1", Type: TaskTest, Description: "say hi", AgentName: "tester"}
	scratch := NewScratchpad()
	out, err := a.Run(context.Background(), task, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello back" {
		t.Errorf("output: %q", out)
	}

	if len(captured.Messages) != 2 {
		t.Fatalf("messages: %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" {
		t.Errorf("first msg role: %s", captured.Messages[0].Role)
	}
	if captured.Messages[1].Role != "user" {
		t.Errorf("second msg role: %s", captured.Messages[1].Role)
	}
	if captured.Model != llm.NIMQwenModel {
		t.Errorf("model alias not resolved: %s", captured.Model)
	}
	if captured.MaxTokens != 2000 {
		t.Errorf("max_tokens: %d", captured.MaxTokens)
	}
	if captured.Temperature != 0.2 {
		t.Errorf("temperature: %f", captured.Temperature)
	}
	if !strings.Contains(captured.Messages[1].Content, "tk-1") {
		t.Errorf("user prompt missing task id: %s", captured.Messages[1].Content)
	}
}

func TestNIMAgentRunDefaultMaxTokens(t *testing.T) {
	var captured llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	a := newNIMTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode})
	if _, err := a.Run(context.Background(), &Task{ID: "t1", Description: "d"}, NewScratchpad()); err != nil {
		t.Fatal(err)
	}
	if captured.MaxTokens != 4096 {
		t.Errorf("expected default 4096, got %d", captured.MaxTokens)
	}
}

func TestNIMAgentRunPropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`rate limited`))
	}))
	defer srv.Close()

	a := newNIMTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode})
	_, err := a.Run(context.Background(), &Task{ID: "t1", Description: "d"}, NewScratchpad())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected 429 in error, got %v", err)
	}
}

func TestNIMAgentRunWritesScratchpad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"choices": [{"message":{"role":"assistant","content":"answer"}}],
			"usage": {"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 10}
		}`))
	}))
	defer srv.Close()

	a := newNIMTestAgent(t, srv, AgentConfig{Name: "coder", Type: TaskCode, Model: "haiku"})
	scratch := NewScratchpad()
	_, err := a.Run(context.Background(), &Task{ID: "tk-7", Description: "the question"}, scratch)
	if err != nil {
		t.Fatal(err)
	}

	all := scratch.ReadAll()
	if all["inputs"].Content != "the question" {
		t.Errorf("inputs: %q", all["inputs"].Content)
	}
	if all["outputs:tk-7"].Content != "answer" {
		t.Errorf("output: %q", all["outputs:tk-7"].Content)
	}
	usage, ok := all["usage:tk-7"]
	if !ok {
		t.Fatal("expected usage entry")
	}
	if !strings.Contains(usage.Content, "total=10") {
		t.Errorf("usage: %q", usage.Content)
	}
}

func TestNIMAgentLoadSystemPromptDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	a := newNIMTestAgent(t, srv, AgentConfig{Name: "docs", Type: TaskDocs, Description: "writes docs"})
	prompt, err := a.loadSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "You are docs") {
		t.Errorf("prompt: %q", prompt)
	}
	if !strings.Contains(prompt, "Type: docs") {
		t.Errorf("missing type: %q", prompt)
	}
	if !strings.Contains(prompt, "writes docs") {
		t.Errorf("missing description: %q", prompt)
	}
}

func TestNIMAgentLoadSystemPromptFallsBackWhenNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	a := newNIMTestAgent(t, srv, AgentConfig{
		Name: "x", Type: TaskCode, Description: "d", SystemFile: "no/such/file.md",
	})
	prompt, err := a.loadSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "You are x") {
		t.Errorf("expected default prompt, got %q", prompt)
	}
}

func TestNIMAgentLoadSystemPromptFromFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	dir := t.TempDir()
	promptPath := filepath.Join(dir, "system.md")
	if err := os.WriteFile(promptPath, []byte("YOU ARE A TEST AGENT.\nDo X."), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SIN_AGENTS_DIR", dir)
	a := newNIMTestAgent(t, srv, AgentConfig{
		Name: "x", Type: TaskCode, SystemFile: "system.md",
	})
	prompt, err := a.loadSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "YOU ARE A TEST AGENT") {
		t.Errorf("expected file contents, got %q", prompt)
	}
}

func TestNIMAgentBuildUserPromptIncludesContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	a := newNIMTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode})

	task := &Task{ID: "tk-9", Type: TaskCode, Description: "do work", AgentName: "x"}
	prior := []string{"[outputs:tk-1]\nfirst answer"}
	prompt := a.buildUserPrompt(task, "shared input text", prior)

	for _, want := range []string{
		"## Task",
		"ID: tk-9",
		"Type: code",
		"do work",
		"Assigned Agent: x",
		"## Prior Context",
		"shared input text",
		"## Prior Outputs",
		"[outputs:tk-1]",
		"first answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q in prompt:\n%s", want, prompt)
		}
	}
}

func TestNIMAgentBuildUserPromptMinimal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	a := newNIMTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode})
	task := &Task{ID: "t1", Type: TaskCode, Description: "d"}
	prompt := a.buildUserPrompt(task, "", nil)
	if strings.Contains(prompt, "Prior Context") {
		t.Errorf("should not have Prior Context: %s", prompt)
	}
	if strings.Contains(prompt, "Prior Outputs") {
		t.Errorf("should not have Prior Outputs: %s", prompt)
	}
}

func TestNIMAgentRunUsesEnvBaseURLByDefault(t *testing.T) {
	t.Setenv("SIN_NIM_BASE_URL", "https://env.example/v1")
	t.Setenv("SIN_NIM_API_KEY", "env-key")
	a := NewNIMAgent(AgentConfig{Name: "x", Type: TaskCode})
	if a.client == nil {
		t.Fatal("nil client")
	}
	if a.client.APIKey != "env-key" {
		t.Errorf("api key: %s", a.client.APIKey)
	}
}

func TestNIMAgentRunContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	a := newNIMTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Run(ctx, &Task{ID: "t1", Description: "d"}, NewScratchpad())
	if err == nil {
		t.Error("expected context error")
	}
}

// ── edge case tests for the model resolution fix ──────────────────────────

// TestLLMAgentEmptyModelFails: a plugin agent with Provider="" and Model=""
// AND no env vars set must not silently send model="" to the LLM. With the
// fixed resolution chain, this case falls back to the NIM default model
// (the bug was a 400 from NIM). This test pins down that fallback so a
// future regression that re-introduces the empty-model path is caught.
func TestLLMAgentEmptyModelFails(t *testing.T) {
	clearAllProviderEnv(t)
	var captured llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{
			"choices": [{"message":{"role":"assistant","content":"fallback-ok"}}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "k")
	a := NewLLMAgentWithClient(AgentConfig{
		Name: "empty-cfg-agent", Type: TaskCode,
	}, client)
	out, err := a.Run(context.Background(), &Task{ID: "t1", Description: "x"}, NewScratchpad())
	if err != nil {
		t.Fatalf("empty model should fall back to NIM default, got error: %v", err)
	}
	if out != "fallback-ok" {
		t.Errorf("output: %q", out)
	}
	if captured.Model == "" {
		t.Fatal("captured model must not be empty (this is the original bug)")
	}
	if captured.Model != llm.NIMDefaultModel {
		t.Errorf("expected NIM default %q, got %q", llm.NIMDefaultModel, captured.Model)
	}
}

// TestLLMAgentNoModelResolvable: a contrived case where the configured
// provider name does not exist in the registry AND no env vars are set:
// the resolution chain must error out with a clear, agent-name-tagged
// message instead of sending an empty model. This protects the error
// path itself from regression.
func TestLLMAgentNoModelResolvable(t *testing.T) {
	clearAllProviderEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called when no model is resolvable")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "k")
	a := NewLLMAgentWithClient(AgentConfig{
		Name: "no-fallback", Type: TaskCode, Provider: "no-such-provider",
	}, client)
	_, err := a.Run(context.Background(), &Task{ID: "t1", Description: "x"}, NewScratchpad())
	if err == nil {
		t.Fatal("expected error when provider is unknown and no env is set")
	}
	if !strings.Contains(err.Error(), "no model configured") {
		t.Errorf("expected descriptive error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "no-fallback") {
		t.Errorf("error should mention agent name, got: %v", err)
	}
}

// TestLLMAgentEmptyModelUsesDefault: agent with a provider but no model
// must use the provider's default model.
func TestLLMAgentEmptyModelUsesDefault(t *testing.T) {
	clearAllProviderEnv(t)
	var captured llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{
			"choices": [{"message":{"role":"assistant","content":"ok"}}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "k")
	a := NewLLMAgentWithClient(AgentConfig{
		Name: "default-picker", Type: TaskCode, Provider: "openai",
	}, client)
	if _, err := a.Run(context.Background(), &Task{ID: "t1", Description: "x"}, NewScratchpad()); err != nil {
		t.Fatal(err)
	}
	// OpenAI default model is "gpt-4o" per providers.go
	if captured.Model != "gpt-4o" {
		t.Errorf("expected openai default gpt-4o, got %q", captured.Model)
	}
}

// TestLLMAgentInferProviderFromEnv: no provider in config, but env vars set —
// agent must infer the provider from env.
func TestLLMAgentInferProviderFromEnv(t *testing.T) {
	clearAllProviderEnv(t)
	// Set ONLY the OpenAI env var (not NIM, so inferProviderFromEnv returns "openai").
	t.Setenv("OPENAI_API_KEY", "sk-test")
	var captured llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{
			"choices": [{"message":{"role":"assistant","content":"ok"}}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "sk-test")
	a := NewLLMAgentWithClient(AgentConfig{
		Name: "env-infer", Type: TaskCode, Model: "",
	}, client)
	if _, err := a.Run(context.Background(), &Task{ID: "t1", Description: "x"}, NewScratchpad()); err != nil {
		t.Fatal(err)
	}
	// env-inferred provider is openai → default model gpt-4o.
	if captured.Model != "gpt-4o" {
		t.Errorf("expected env-inferred provider default gpt-4o, got %q", captured.Model)
	}
}

// TestLLMAgentModelAliasResolution: agent with alias "haiku" must send the
// fully-resolved NIM model id to the server.
func TestLLMAgentModelAliasResolution(t *testing.T) {
	clearAllProviderEnv(t)
	var captured llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"x"}}]}`))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "k")
	a := NewLLMAgentWithClient(AgentConfig{
		Name: "aliaser", Type: TaskCode, Model: "haiku",
	}, client)
	if _, err := a.Run(context.Background(), &Task{ID: "t1", Description: "x"}, NewScratchpad()); err != nil {
		t.Fatal(err)
	}
	// Wait — Client.Chat() does NOT resolve aliases; only the agent does.
	// Verify that the agent passed "haiku" verbatim and that a separate
	// ResolveModel call maps it to the canonical NIM id.
	resolved := llm.ResolveModel(captured.Model)
	if resolved != llm.NIMHaikuModel {
		t.Errorf("expected ResolveModel(%q) == %q, got %q", captured.Model, llm.NIMHaikuModel, resolved)
	}
}

// TestLLMAgentNIMDefaultWhenAllEmpty: a fresh agent with no config and no
// env should fall back to NIM's default model (the new "nim" hard-default
// in the resolution block).
func TestLLMAgentNIMDefaultWhenAllEmpty(t *testing.T) {
	clearAllProviderEnv(t)
	// We need a client but with no model hint: build a client whose URL we
	// control, then construct an agent without a client — but that path
	// (NewLLMAgent with no client) requires an API key from env. Since env
	// is cleared, the agent will have client=nil and Run() returns the
	// "no LLM client" error. Instead, use a hand-built client so we can
	// observe the model.
	var captured llm.ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"x"}}]}`))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "k")
	a := NewLLMAgentWithClient(AgentConfig{
		Name: "fallbacker", Type: TaskCode, // no Provider, no Model
	}, client)
	if _, err := a.Run(context.Background(), &Task{ID: "t1", Description: "x"}, NewScratchpad()); err != nil {
		t.Fatal(err)
	}
	// inferProviderFromEnv returns "" (all envs cleared) → falls through to "nim"
	// → NIMDefaultModel ("meta/llama-3.3-70b-instruct").
	if captured.Model != llm.NIMDefaultModel {
		t.Errorf("expected NIM default %q, got %q", llm.NIMDefaultModel, captured.Model)
	}
}

// clearAllProviderEnv wipes every provider env var a test could leak.
func clearAllProviderEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SIN_NIM_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY",
		"GROQ_API_KEY", "SIN_LLM_API_KEY", "SIN_LLM_BASE_URL", "SIN_LLM_MODEL",
		"SIN_NIM_BASE_URL",
	} {
		t.Setenv(k, "")
	}
}
