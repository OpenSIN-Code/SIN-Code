// SPDX-License-Identifier: MIT
// Purpose: tests for LLMFiller. All tests use httptest to stub the LLM
// endpoint, following the same pattern as llm/llm_test.go.
package testgen

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

func llmFillerFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	if err := os.WriteFile(src, []byte("package calc\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

func chatResponse(content string) string {
	b, _ := json.Marshal(map[string]any{
		"id":      "test-id",
		"object":  "chat.completion",
		"created": 1,
		"model":   "test-model",
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": content},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	})
	return string(b)
}

func TestLLMFiller_FillSuccess(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		code := "```go\npackage calc\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fail()\n\t}\n}\n```"
		_, _ = w.Write([]byte(chatResponse(code)))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")

	res, err := filler.Fill(context.Background(), FillRequest{
		SourceFile: src,
		MaxCases:   3,
	})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if res.TestCode == "" {
		t.Fatal("expected non-empty TestCode")
	}
	if !strings.Contains(res.TestCode, "func TestAdd") {
		t.Errorf("TestCode missing TestAdd: %s", res.TestCode)
	}
	if res.CasesGenerated < 1 {
		t.Errorf("expected >=1 cases, got %d", res.CasesGenerated)
	}
	if res.Model != "test-model" {
		t.Errorf("model: %q", res.Model)
	}
	if res.TokensUsed != 30 {
		t.Errorf("tokens: %d", res.TokensUsed)
	}
}

func TestLLMFiller_EmptyResponse(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[]}`))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")

	_, err := filler.Fill(context.Background(), FillRequest{
		SourceFile: src,
	})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLLMFiller_ParseCodeBlock(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantCode  bool
		wantCount int
	}{
		{
			name:      "go fenced block",
			input:     "```go\npackage x\nfunc TestA(t *testing.T) {}\nfunc TestB(t *testing.T) {}\n```",
			wantCode:  true,
			wantCount: 2,
		},
		{
			name:      "bare fenced block",
			input:     "```\npackage x\nfunc TestA(t *testing.T) {}\n```",
			wantCode:  true,
			wantCount: 1,
		},
		{
			name:      "golang fenced block",
			input:     "```golang\npackage x\nfunc TestA(t *testing.T) {}\n```",
			wantCode:  true,
			wantCount: 1,
		},
		{
			name:      "block with surrounding prose",
			input:     "Here are the tests:\n```go\npackage x\nfunc TestA(t *testing.T) {}\n```\nHope this helps!",
			wantCode:  true,
			wantCount: 1,
		},
		{
			name:      "code without Test func",
			input:     "```go\npackage x\nvar Y = 1\n```",
			wantCode:  true,
			wantCount: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, count := extractGoCodeBlock(c.input)
			if c.wantCode && code == "" {
				t.Errorf("expected non-empty code")
			}
			if !c.wantCode && code != "" {
				t.Errorf("expected empty code, got %s", code)
			}
			if count != c.wantCount {
				t.Errorf("count: got %d, want %d", count, c.wantCount)
			}
		})
	}
}

func TestLLMFiller_NoCodeBlock(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("I cannot generate tests for this file.")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")

	res, err := filler.Fill(context.Background(), FillRequest{
		SourceFile: src,
	})
	if err == nil {
		t.Fatal("expected error for no code block")
	}
	if !strings.Contains(err.Error(), "no Go code block") {
		t.Errorf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil partial result on parse failure")
	}
	if res.TokensUsed != 30 {
		t.Errorf("expected tokens preserved on parse failure: %d", res.TokensUsed)
	}
}

func TestLLMFiller_ContextCancel(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage x\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := filler.Fill(ctx, FillRequest{
		SourceFile: src,
	})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestLLMFiller_ModelResolution(t *testing.T) {
	src := llmFillerFixture(t)
	var chatCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"resolved-model"}]}`))
		case "/chat/completions":
			atomic.AddInt32(&chatCalls, 1)
			var body map[string]any
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			model, _ := body["model"].(string)
			if model != "resolved-model" {
				t.Errorf("expected resolved model in chat request, got %q", model)
			}
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"id": "test-id", "object": "chat.completion", "created": 1,
				"model": "resolved-model",
				"choices": []map[string]any{{
					"index":         0,
					"message":       map[string]any{"role": "assistant", "content": "```go\npackage x\nfunc TestA(t *testing.T) {}\n```"},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "")

	res, err := filler.Fill(context.Background(), FillRequest{
		SourceFile: src,
	})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if atomic.LoadInt32(&chatCalls) != 1 {
		t.Errorf("expected 1 chat call, got %d", atomic.LoadInt32(&chatCalls))
	}
	if res.Model != "resolved-model" {
		t.Errorf("expected resolved-model, got %q", res.Model)
	}
	if res.TestCode == "" {
		t.Error("expected non-empty test code")
	}
}

func TestLLMFiller_NilClient(t *testing.T) {
	filler := NewLLMFiller(nil, "test-model")
	_, err := filler.Fill(context.Background(), FillRequest{
		SourceFile: "foo.go",
	})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestLLMFiller_MissingSourceFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")

	_, err := filler.Fill(context.Background(), FillRequest{
		SourceFile: "/nonexistent/path/file.go",
	})
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
	if !strings.Contains(err.Error(), "read source") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildFillerPrompt(t *testing.T) {
	prompt := buildFillerPrompt(FillRequest{
		SourceFile:    "calc.go",
		FunctionName:  "Add",
		ExistingTests: "old code",
		Language:      "go",
		MaxCases:      5,
	}, "package calc\nfunc Add(a,b int) int { return a+b }", 5, "go")
	for _, want := range []string{
		"Generate 5 table-driven go test cases",
		"Focus on the function: Add",
		"Previous test code",
		"calc.go",
		"Return ONLY a",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q: %s", want, prompt)
		}
	}
}
