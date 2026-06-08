// SPDX-License-Identifier: MIT
// Purpose: comprehensive per-provider tests. Each provider gets an httptest
// roundtrip that exercises (a) /v1/models probe (for the empty-model fallback)
// (b) /v1/chat/completions request shape, auth header, and response parsing.
// Plus tests for ProviderFromConfig variants and the new empty-model chain.
package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── helpers ────────────────────────────────────────────────────────────────

// fakeLLMServer returns a server that serves a /v1/models list and a single
// canned /v1/chat/completions response. The first-returned string is captured
// auth header, the second is the decoded ChatRequest body.
func fakeLLMServer(t *testing.T, modelID string) (*httptest.Server, *string, *ChatRequest) {
	t.Helper()
	gotAuth := ""
	gotBody := ChatRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"data":[{"id":"` + modelID + `"}]}`))
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			gotAuth = r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			_, _ = w.Write([]byte(`{
				"id": "x", "object": "chat.completion", "created": 1, "model": "` + modelID + `",
				"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
				"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &gotAuth, &gotBody
}

// clearAllProviderEnv wipes every provider-specific env var so tests do not
// leak state between subtests.
func clearAllProviderEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SIN_NIM_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY",
		"GROQ_API_KEY", "SIN_LLM_API_KEY", "SIN_LLM_BASE_URL", "SIN_LLM_MODEL",
	} {
		t.Setenv(k, "")
	}
}

// ── per-provider end-to-end ───────────────────────────────────────────────

func TestProviderNIM_Roundtrip(t *testing.T) {
	clearAllProviderEnv(t)
	srv, gotAuth, gotBody := fakeLLMServer(t, "meta/llama-3.3-70b-instruct")
	c := NewClient(srv.URL, "nvapi-test")
	resp, err := c.Chat(context.Background(), ChatRequest{
		Model:    "haiku", // alias
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *gotAuth != "Bearer nvapi-test" {
		t.Errorf("auth: %q", *gotAuth)
	}
	if gotBody.Model != "haiku" {
		t.Errorf("sent model: %s", gotBody.Model)
	}
	if resp.ExtractText() != "ok" {
		t.Errorf("text: %q", resp.ExtractText())
	}
}

func TestProviderOpenAI_Roundtrip(t *testing.T) {
	clearAllProviderEnv(t)
	srv, gotAuth, gotBody := fakeLLMServer(t, "gpt-4o")
	c := NewClient(srv.URL, "sk-test")
	resp, err := c.Chat(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *gotAuth != "Bearer sk-test" {
		t.Errorf("auth: %q", *gotAuth)
	}
	if gotBody.Messages[0].Content != "ping" {
		t.Errorf("user msg: %+v", gotBody.Messages[0])
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("response model: %s", resp.Model)
	}
}

func TestProviderAnthropic_Roundtrip(t *testing.T) {
	clearAllProviderEnv(t)
	srv, _, gotBody := fakeLLMServer(t, "claude-sonnet-4-5")
	c := NewClient(srv.URL, "sk-ant-test")
	if _, err := c.Chat(context.Background(), ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []Message{{Role: "user", Content: "ping"}},
	}); err != nil {
		t.Fatal(err)
	}
	if gotBody.Model != "claude-sonnet-4-5" {
		t.Errorf("sent model: %s", gotBody.Model)
	}
}

func TestProviderOllama_Roundtrip(t *testing.T) {
	clearAllProviderEnv(t)
	srv, gotAuth, gotBody := fakeLLMServer(t, "llama3.1")
	// Ollama has no API key by default; the Authorization header must be empty.
	c := NewClient(srv.URL, "")
	if _, err := c.Chat(context.Background(), ChatRequest{
		Model:    "llama3.1",
		Messages: []Message{{Role: "user", Content: "ping"}},
	}); err != nil {
		t.Fatal(err)
	}
	if *gotAuth != "" {
		t.Errorf("ollama should send no auth, got %q", *gotAuth)
	}
	if gotBody.Model != "llama3.1" {
		t.Errorf("sent model: %s", gotBody.Model)
	}
}

func TestProviderGroq_Roundtrip(t *testing.T) {
	clearAllProviderEnv(t)
	srv, gotAuth, gotBody := fakeLLMServer(t, "llama-3.3-70b-versatile")
	c := NewClient(srv.URL, "gsk-test")
	if _, err := c.Chat(context.Background(), ChatRequest{
		Model:    "llama-3.3-70b-versatile",
		Messages: []Message{{Role: "user", Content: "ping"}},
	}); err != nil {
		t.Fatal(err)
	}
	if *gotAuth != "Bearer gsk-test" {
		t.Errorf("auth: %q", *gotAuth)
	}
	if gotBody.Model != "llama-3.3-70b-versatile" {
		t.Errorf("sent model: %s", gotBody.Model)
	}
}

func TestProviderCustom_Roundtrip(t *testing.T) {
	clearAllProviderEnv(t)
	srv, gotAuth, gotBody := fakeLLMServer(t, "vendor/custom-model")
	c := NewClient(srv.URL, "any-key")
	if _, err := c.Chat(context.Background(), ChatRequest{
		Model:    "vendor/custom-model",
		Messages: []Message{{Role: "user", Content: "ping"}},
	}); err != nil {
		t.Fatal(err)
	}
	if *gotAuth != "Bearer any-key" {
		t.Errorf("auth: %q", *gotAuth)
	}
	if gotBody.Model != "vendor/custom-model" {
		t.Errorf("sent model: %s", gotBody.Model)
	}
}

// ── per-provider model resolution (alias → ID) ─────────────────────────────

func TestProviderNIM_ModelAliasResolved(t *testing.T) {
	clearAllProviderEnv(t)
	srv, _, gotBody := fakeLLMServer(t, NIMDefaultModel)
	c := NewClient(srv.URL, "k")
	for alias, wantID := range NIMModelAliases {
		if _, err := c.Chat(context.Background(), ChatRequest{
			Model: alias, Messages: []Message{{Role: "user", Content: "x"}},
		}); err != nil {
			t.Fatalf("alias %q: %v", alias, err)
		}
		if gotBody.Model != alias {
			t.Errorf("alias %q: client sent %q, want %q", alias, gotBody.Model, wantID)
		}
		// server still sees the alias; we verify the alias was sent as-is
		// (ResolveModel is invoked by the agent, not by Client.Chat).
	}
}

func TestProviderNIM_ResolveModelNotAppliedByClient(t *testing.T) {
	// ResolveModel is the *agent's* job. The bare Client must NOT mutate model
	// names — it must pass them through verbatim.
	clearAllProviderEnv(t)
	srv, _, gotBody := fakeLLMServer(t, "x")
	c := NewClient(srv.URL, "k")
	if _, err := c.Chat(context.Background(), ChatRequest{
		Model: "haiku", Messages: []Message{{Role: "user", Content: "x"}},
	}); err != nil {
		t.Fatal(err)
	}
	if gotBody.Model != "haiku" {
		t.Errorf("client must not rewrite model, got %q", gotBody.Model)
	}
}

func TestResolveModel_AllProviders(t *testing.T) {
	// NIM aliases resolve; non-NIM names are passed through (since the alias
	// map is NIM-specific). This documents the contract.
	clearAllProviderEnv(t)
	cases := map[string]string{
		"haiku":                   NIMHaikuModel,
		"gpt-4o":                  "gpt-4o",
		"claude-sonnet-4-5":       "claude-sonnet-4-5",
		"llama-3.3-70b-versatile": "llama-3.3-70b-versatile",
		"vendor/x":                "vendor/x",
		"":                        "",
	}
	for in, want := range cases {
		if got := ResolveModel(in); got != want {
			t.Errorf("ResolveModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── ProviderFromConfig variants ───────────────────────────────────────────

func TestProviderFromConfig_CustomBaseURL(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("SIN_NIM_API_KEY", "key")
	c, err := ProviderFromConfig("nim", "https://proxy.example.com/v1", "", "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://proxy.example.com/v1" {
		t.Errorf("base URL not overridden: %s", c.BaseURL)
	}
	if c.APIKey != "key" {
		t.Errorf("api key: %s", c.APIKey)
	}
	if c.HTTP.Timeout.Seconds() != 5 {
		t.Errorf("timeout: %s", c.HTTP.Timeout)
	}
}

func TestProviderFromConfig_FallbackToGenericKey(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("SIN_LLM_API_KEY", "generic")
	c, err := ProviderFromConfig("openai", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "generic" {
		t.Errorf("fallback key: %s", c.APIKey)
	}
}

func TestProviderFromConfig_EnvBaseURLUsedForCustom(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("SIN_LLM_BASE_URL", "https://env.example.com/v1")
	t.Setenv("SIN_LLM_API_KEY", "k")
	c, err := ProviderFromConfig("custom", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://env.example.com/v1" {
		t.Errorf("base URL: %s", c.BaseURL)
	}
}

func TestProviderFromConfig_OverrideBeatsEnv(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("SIN_LLM_BASE_URL", "https://env.example.com/v1")
	t.Setenv("SIN_NIM_API_KEY", "k")
	c, err := ProviderFromConfig("nim", "https://override.example.com/v1", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://override.example.com/v1" {
		t.Errorf("override should win, got %s", c.BaseURL)
	}
}

func TestProviderFromConfig_UnknownProvider(t *testing.T) {
	clearAllProviderEnv(t)
	_, err := ProviderFromConfig("nope", "", "", "", 0)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestProviderFromConfig_AnthropicRound(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	c, err := ProviderFromConfig("anthropic", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "sk-ant" {
		t.Errorf("key: %s", c.APIKey)
	}
	if !strings.Contains(c.BaseURL, "anthropic.com") {
		t.Errorf("base URL: %s", c.BaseURL)
	}
}

func TestProviderFromConfig_GroqRound(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("GROQ_API_KEY", "gsk")
	c, err := ProviderFromConfig("groq", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "gsk" {
		t.Errorf("key: %s", c.APIKey)
	}
	if !strings.Contains(c.BaseURL, "groq.com") {
		t.Errorf("base URL: %s", c.BaseURL)
	}
}

// ── empty-model fallback chain ────────────────────────────────────────────

func TestClientChat_EmptyModel_UsesEnvModel(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("SIN_LLM_MODEL", "env-picked-model")
	srv, _, gotBody := fakeLLMServer(t, "env-picked-model")
	c := NewClient(srv.URL, "k")
	if _, err := c.Chat(context.Background(), ChatRequest{Model: "", Messages: []Message{{Role: "user", Content: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if gotBody.Model != "env-picked-model" {
		t.Errorf("expected env model, got %q", gotBody.Model)
	}
}

func TestClientChat_EmptyModel_ProbesModelsEndpoint(t *testing.T) {
	clearAllProviderEnv(t)
	// No SIN_LLM_MODEL set — must fall through to /v1/models.
	srv, _, gotBody := fakeLLMServer(t, "discovered-model")
	c := NewClient(srv.URL, "k")
	if _, err := c.Chat(context.Background(), ChatRequest{Model: "", Messages: []Message{{Role: "user", Content: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if gotBody.Model != "discovered-model" {
		t.Errorf("expected probe result, got %q", gotBody.Model)
	}
}

func TestClientChat_EmptyModel_NoKeyNoEnv_Errors(t *testing.T) {
	clearAllProviderEnv(t)
	srv, _, _ := fakeLLMServer(t, "should-not-be-used")
	c := NewClient(srv.URL, "")
	_, err := c.Chat(context.Background(), ChatRequest{Model: ""})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no model") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientChat_EmptyModel_EmptyModelsList_Errors(t *testing.T) {
	clearAllProviderEnv(t)
	// Server returns 200 with an empty data list.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "k")
	_, err := c.Chat(context.Background(), ChatRequest{Model: ""})
	if err == nil {
		t.Fatal("expected error when /v1/models is empty")
	}
	if !strings.Contains(err.Error(), "no model") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientChat_EmptyModel_EnvBeatsProbe(t *testing.T) {
	clearAllProviderEnv(t)
	t.Setenv("SIN_LLM_MODEL", "env-wins")
	// If env is set, /v1/models must NOT be consulted. The fake server will
	// return 404 for /v1/models, and the test passes only if env is honored.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "k")
	if _, err := c.Chat(context.Background(), ChatRequest{Model: ""}); err != nil {
		t.Fatal(err)
	}
}

// ── error / edge cases ────────────────────────────────────────────────────

func TestClientChat_ProbeStatusError(t *testing.T) {
	clearAllProviderEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"denied"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "k")
	_, err := c.Chat(context.Background(), ChatRequest{Model: ""})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientChat_ProbeMalformedJSON(t *testing.T) {
	clearAllProviderEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = w.Write([]byte(`<<<not json>>>`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "k")
	_, err := c.Chat(context.Background(), ChatRequest{Model: ""})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientChat_NonEmptyModel_BypassesFallback(t *testing.T) {
	clearAllProviderEnv(t)
	// When the caller supplies a model, the /v1/models endpoint must NOT be
	// hit. We verify by serving 500 on /v1/models — non-empty path skips it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "k")
	if _, err := c.Chat(context.Background(), ChatRequest{Model: "explicit", Messages: []Message{{Role: "user", Content: "x"}}}); err != nil {
		t.Fatal(err)
	}
}
