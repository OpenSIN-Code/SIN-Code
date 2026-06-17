// SPDX-License-Identifier: MIT
// Purpose: tests for LoopAgent — real LLM-backed agent using agentloop.Loop.
// Uses httptest to stub the LLM endpoint and temp SQLite for session isolation.
// No real API key or network access required. Issue #287.
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
	"sync"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func newLoopTestAgent(t *testing.T, srv *httptest.Server, cfg AgentConfig, opts ...LoopAgentOption) *LoopAgent {
	t.Helper()
	client := llm.NewClient(srv.URL, "test-key")
	store := newTestSessionStore(t)
	opts = append(opts, WithSessionStore(store))
	return NewLoopAgent(cfg, client, opts...)
}

func newTestSessionStore(t *testing.T) *session.Store {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func llmHandler(content string, usage *wireUsageProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": content,
					},
					"finish_reason": "stop",
				},
			},
		}
		if usage != nil {
			resp["usage"] = map[string]any{
				"prompt_tokens":     usage.Prompt,
				"completion_tokens": usage.Completion,
				"total_tokens":      usage.Total,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

type wireUsageProxy struct {
	Prompt     int
	Completion int
	Total      int
}

func TestLoopAgentNameAndConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "coder", Type: TaskCode, Model: "haiku", MaxTokens: 1000,
	})
	if a.Name() != "coder" {
		t.Errorf("name: %s", a.Name())
	}
	if a.Config().Model != "haiku" {
		t.Errorf("model: %s", a.Config().Model)
	}
}

func TestLoopAgentRunSuccess(t *testing.T) {
	srv := httptest.NewServer(llmHandler("hello from agent", &wireUsageProxy{
		Prompt: 50, Completion: 10, Total: 60,
	}))
	defer srv.Close()

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "coder", Type: TaskCode, Model: "haiku", MaxTokens: 2000,
	})
	task := &Task{ID: "tk-1", Type: TaskCode, Description: "say hi", AgentName: "coder"}
	scratch := NewScratchpad()
	out, err := a.Run(context.Background(), task, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello from agent" {
		t.Errorf("output: %q", out)
	}
}

func TestLoopAgentWritesScratchpad(t *testing.T) {
	srv := httptest.NewServer(llmHandler("answer", &wireUsageProxy{
		Prompt: 7, Completion: 3, Total: 10,
	}))
	defer srv.Close()

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "coder", Type: TaskCode, Model: "haiku",
	})
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
	if !strings.Contains(usage.Content, "tokens=10") {
		t.Errorf("usage missing tokens: %q", usage.Content)
	}
	if !strings.Contains(usage.Content, "verified=true") {
		t.Errorf("usage missing verified: %q", usage.Content)
	}
}

func TestLoopAgentCostTracking(t *testing.T) {
	srv := httptest.NewServer(llmHandler("done", &wireUsageProxy{
		Prompt: 100, Completion: 50, Total: 150,
	}))
	defer srv.Close()

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "tester", Type: TaskTest, Model: "qwen", MaxTokens: 2000,
	})
	scratch := NewScratchpad()
	_, err := a.Run(context.Background(), &Task{ID: "tk-ct", Description: "run tests"}, scratch)
	if err != nil {
		t.Fatal(err)
	}
	usage := scratch.ReadAll()["usage:tk-ct"].Content
	if !strings.Contains(usage, "tokens=150") {
		t.Errorf("expected tokens=150 in usage: %q", usage)
	}
	if !strings.Contains(usage, "model=") {
		t.Errorf("expected model= in usage: %q", usage)
	}
}

func TestLoopAgentSessionIsolation(t *testing.T) {
	srv := httptest.NewServer(llmHandler("isolated", nil))
	defer srv.Close()

	store := newTestSessionStore(t)

	var sessionIDs []string
	var mu sync.Mutex

	cfg := AgentConfig{Name: "coder", Type: TaskCode, Model: "haiku"}

	for i := 0; i < 3; i++ {
		a := NewLoopAgent(cfg, llm.NewClient(srv.URL, "k"), WithSessionStore(store))
		scratch := NewScratchpad()
		_, err := a.Run(context.Background(), &Task{
			ID:          "tk-iso",
			Type:        TaskCode,
			Description: "isolated run",
		}, scratch)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		sessionIDs = append(sessionIDs, a.Config().Name)
		mu.Unlock()
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) < 3 {
		t.Errorf("expected >=3 sessions, got %d", len(sessions))
	}

	seen := map[string]bool{}
	for _, s := range sessions {
		seen[s.ID] = true
	}
	if len(seen) < 3 {
		t.Errorf("expected >=3 unique session IDs, got %d", len(seen))
	}
}

func TestLoopAgentSystemPromptDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	cfg := AgentConfig{Name: "docs", Type: TaskDocs, Description: "writes docs"}
	_ = newLoopTestAgent(t, srv, cfg)
	prompt, err := loadLoopSystemPromptHook(cfg)
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

func TestLoopAgentSystemPromptFromFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	dir := t.TempDir()
	promptPath := filepath.Join(dir, "system.md")
	if err := os.WriteFile(promptPath, []byte("YOU ARE A TEST AGENT.\nDo X."), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_AGENTS_DIR", dir)

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "x", Type: TaskCode, SystemFile: "system.md",
	})
	prompt, err := loadLoopSystemPromptHook(a.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "YOU ARE A TEST AGENT") {
		t.Errorf("expected file contents, got %q", prompt)
	}
}

func TestLoopAgentSystemPromptFallsBackWhenNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	cfg := AgentConfig{Name: "x", Type: TaskCode, Description: "d", SystemFile: "no/such/file.md"}
	_ = newLoopTestAgent(t, srv, cfg)
	prompt, err := loadLoopSystemPromptHook(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "You are x") {
		t.Errorf("expected default prompt, got %q", prompt)
	}
}

func TestLoopAgentVerifyGatePass(t *testing.T) {
	srv := httptest.NewServer(llmHandler("verified output", nil))
	defer srv.Close()

	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) {
			return true, "tests pass", nil
		}, nil)

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "coder", Type: TaskCode, Model: "haiku",
	}, WithVerifyGate(gate))

	scratch := NewScratchpad()
	out, err := a.Run(context.Background(), &Task{ID: "tk-vp", Description: "do work"}, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if out != "verified output" {
		t.Errorf("output: %q", out)
	}
	usage := scratch.ReadAll()["usage:tk-vp"].Content
	if !strings.Contains(usage, "verified=true") {
		t.Errorf("expected verified=true: %q", usage)
	}
}

func TestLoopAgentVerifyGateFail(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		content := "done"
		if callCount < 3 {
			content = "not done yet"
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"role": "assistant", "content": content},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	verifyCalls := 0
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) {
			verifyCalls++
			if verifyCalls < 3 {
				return false, "tests fail", nil
			}
			return true, "tests pass", nil
		}, nil)

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "coder", Type: TaskCode, Model: "haiku",
	}, WithVerifyGate(gate), WithMaxTurns(10))

	scratch := NewScratchpad()
	out, err := a.Run(context.Background(), &Task{ID: "tk-vf", Description: "do work"}, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if out != "done" {
		t.Errorf("expected final 'done', got %q", out)
	}
	if verifyCalls < 3 {
		t.Errorf("expected >=3 verify calls, got %d", verifyCalls)
	}
}

func TestLoopAgentNoClient(t *testing.T) {
	a := NewLoopAgent(AgentConfig{Name: "x", Type: TaskCode}, nil,
		WithSessionStore(newTestSessionStore(t)))
	_, err := a.Run(context.Background(), &Task{ID: "t1", Description: "d"}, NewScratchpad())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no LLM client") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoopAgentBuildUserPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	_ = newLoopTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode})

	task := &Task{ID: "tk-9", Type: TaskCode, Description: "do work", AgentName: "x", ExpectedOutput: "a function"}
	prior := []string{"[outputs:tk-1]\nfirst answer"}
	prompt := buildLoopUserPrompt(task, "shared input text", prior)

	for _, want := range []string{
		"## Task",
		"ID: tk-9",
		"Type: code",
		"do work",
		"Assigned Agent: x",
		"Expected Output: a function",
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

func TestLoopAgentBuildUserPromptMinimal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	_ = newLoopTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode})
	task := &Task{ID: "t1", Type: TaskCode, Description: "d"}
	prompt := buildLoopUserPrompt(task, "", nil)
	if strings.Contains(prompt, "Prior Context") {
		t.Errorf("should not have Prior Context: %s", prompt)
	}
	if strings.Contains(prompt, "Prior Outputs") {
		t.Errorf("should not have Prior Outputs: %s", prompt)
	}
}

func TestLoopAgentContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay to allow context cancellation to fire
		select {
		case <-r.Context().Done():
			return
		default:
		}
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	a := newLoopTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode, Model: "haiku"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Run(ctx, &Task{ID: "t1", Description: "d"}, NewScratchpad())
	if err == nil {
		t.Error("expected context error")
	}
}

func TestLoopAgentModelResolution(t *testing.T) {
	clearAllProviderEnv(t)
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "coder", Type: TaskCode, Model: "haiku",
	})
	_, err := a.Run(context.Background(), &Task{ID: "t1", Description: "x"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	model, _ := capturedBody["model"].(string)
	if model == "" {
		t.Fatal("model must not be empty")
	}
	resolved := llm.ResolveModel(model)
	if resolved != llm.NIMHaikuModel {
		t.Errorf("expected ResolveModel(%q) == %q, got %q", model, llm.NIMHaikuModel, resolved)
	}
}

func TestLoopAgentDefaultMaxTokens(t *testing.T) {
	clearAllProviderEnv(t)
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	a := newLoopTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode, Model: "haiku"})
	_, err := a.Run(context.Background(), &Task{ID: "t1", Description: "x"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	maxTokens, _ := capturedBody["max_tokens"].(float64)
	if int(maxTokens) != 4096 {
		t.Errorf("expected default 4096, got %d", int(maxTokens))
	}
}

func TestLoopAgentPropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`rate limited`))
	}))
	defer srv.Close()

	a := newLoopTestAgent(t, srv, AgentConfig{Name: "x", Type: TaskCode, Model: "haiku"})
	_, err := a.Run(context.Background(), &Task{ID: "t1", Description: "d"}, NewScratchpad())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected 429 in error, got %v", err)
	}
}

func TestLoopAgentWithTools(t *testing.T) {
	toolCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		// First turn: model calls a tool. Second turn: model returns text.
		choices, _ := req["messages"].([]any)
		turnNum := len(choices)

		var resp map[string]any
		if turnNum <= 3 {
			// Return a tool call
			resp = map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "tc-1",
									"type": "function",
									"function": map[string]any{
										"name":      "read_file",
										"arguments": `{"path":"test.txt"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
		} else {
			resp = map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": "tool result processed",
						},
						"finish_reason": "stop",
					},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	localTool := func(ctx context.Context, name string, args map[string]any) (string, error) {
		if name == "read_file" {
			toolCalled = true
			return "file contents here", nil
		}
		return "unknown tool", nil
	}
	localSpec := []agentloop.ToolSpec{
		{Name: "read_file", Description: "Read a file", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		}},
	}

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "coder", Type: TaskCode, Model: "haiku",
		ToolsAllow: []string{"read_file"},
	}, WithTools(localTool, localSpec), WithMaxTurns(10))

	out, err := a.Run(context.Background(), &Task{ID: "tk-tool", Description: "read a file"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if !toolCalled {
		t.Error("expected tool to be called")
	}
	if out != "tool result processed" {
		t.Errorf("output: %q", out)
	}
}

func TestLoopAgentEnsureSessionsLazy(t *testing.T) {
	srv := httptest.NewServer(llmHandler("ok", nil))
	defer srv.Close()

	store := newTestSessionStore(t)
	origHook := sessionOpenHook
	sessionOpenHook = func(path string) (*session.Store, error) {
		t.Error("ensureSessions should not call sessionOpenHook when store is injected")
		return nil, nil
	}
	defer func() { sessionOpenHook = origHook }()

	a := NewLoopAgent(AgentConfig{Name: "x", Type: TaskCode, Model: "haiku"},
		llm.NewClient(srv.URL, "k"), WithSessionStore(store))

	_, err := a.Run(context.Background(), &Task{ID: "t1", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoopAgentPrimeContext(t *testing.T) {
	srv := httptest.NewServer(llmHandler("ok", nil))
	defer srv.Close()

	origMemoryHook := memoryOpenHook
	memoryOpenHook = func(path string) (memoryStore, error) {
		return &fakeMemoryStore{primeText: "remembered context"}, nil
	}
	defer func() { memoryOpenHook = origMemoryHook }()

	a := newLoopTestAgent(t, srv, AgentConfig{
		Name: "coder", Type: TaskCode, Model: "haiku",
	})

	var capturedBody map[string]any
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	})

	_, err := a.Run(context.Background(), &Task{ID: "t1", Description: "do work"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}

	msgs, _ := capturedBody["messages"].([]any)
	var foundMemory bool
	for _, m := range msgs {
		if mm, ok := m.(map[string]any); ok {
			if content, _ := mm["content"].(string); strings.Contains(content, "Relevant Project Memory") {
				foundMemory = true
			}
		}
	}
	if !foundMemory {
		t.Error("expected 'Relevant Project Memory' in messages")
	}
}

type fakeMemoryStore struct {
	primeText string
}

func (f *fakeMemoryStore) Prime(query, project string, topK int) (string, error) {
	return f.primeText, nil
}
func (f *fakeMemoryStore) Close() error { return nil }
