// SPDX-License-Identifier: MIT
// Purpose: cover provider_adapter.go statements including every error
// branch and tool-call parsing path.
package agentloop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

type fakeRoundTripper struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.fn(req)
}

func newFakeClient(fn func(req *http.Request) (*http.Response, error)) *llm.Client {
	return &llm.Client{
		BaseURL: "http://fake.local",
		APIKey:  "secret",
		HTTP:    &http.Client{Transport: &fakeRoundTripper{fn: fn}},
	}
}

func TestNewProviderCompletion_MarshalRequestError(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		t.Fatal("should not reach HTTP layer")
		return nil, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	_, err := fn(context.Background(), []session.Message{}, []ToolSpec{{
		Name:        "bad",
		Description: "bad",
		InputSchema: map[string]any{"x": func() {}},
	}})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestNewProviderCompletion_NewRequestError(t *testing.T) {
	c := &llm.Client{
		BaseURL: "://invalid-url",
		HTTP:    &http.Client{Transport: &fakeRoundTripper{}},
	}
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	_, err := fn(context.Background(), []session.Message{}, nil)
	if err == nil {
		t.Fatal("expected request creation error")
	}
}

func TestNewProviderCompletion_HTTPDoError(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	_, err := fn(context.Background(), []session.Message{}, nil)
	if err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestNewProviderCompletion_Non200Status(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(bytes.NewReader([]byte("server busy"))),
		}, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	_, err := fn(context.Background(), []session.Message{}, nil)
	if err == nil {
		t.Fatal("expected non-200 error")
	}
}

func TestNewProviderCompletion_InvalidJSON(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte("not json"))),
		}, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	_, err := fn(context.Background(), []session.Message{}, nil)
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestNewProviderCompletion_NoChoices(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"choices":[]}`))),
		}, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	_, err := fn(context.Background(), []session.Message{}, nil)
	if err == nil {
		t.Fatal("expected no-choices error")
	}
}

func TestNewProviderCompletion_EmptyRole(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))),
		}, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	comp, err := fn(context.Background(), []session.Message{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if comp.Raw.Role != "assistant" {
		t.Fatalf("expected role default to assistant, got %q", comp.Raw.Role)
	}
}

func TestNewProviderCompletion_ToolCallsSuccess(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		body := `{"choices":[{"message":{"role":"assistant","content":"using tool","tool_calls":[{"id":"tc1","type":"function","function":{"name":"sin_read","arguments":"{\"path\":\"/tmp\"}"}}]}}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		}, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	comp, err := fn(context.Background(), []session.Message{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(comp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(comp.ToolCalls))
	}
	if comp.ToolCalls[0].Name != "sin_read" {
		t.Fatalf("unexpected tool name %q", comp.ToolCalls[0].Name)
	}
	if comp.ToolCalls[0].Args["path"] != "/tmp" {
		t.Fatalf("unexpected args %+v", comp.ToolCalls[0].Args)
	}
	if comp.Raw.Role != "assistant" {
		t.Fatalf("unexpected raw role %q", comp.Raw.Role)
	}
	if len(comp.Raw.ToolCalls) == 0 {
		t.Fatal("expected raw tool calls to be populated")
	}
}

func TestNewProviderCompletion_ToolCallsBadArgs(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		body := `{"choices":[{"message":{"role":"assistant","content":"using tool","tool_calls":[{"id":"tc1","type":"function","function":{"name":"sin_read","arguments":"not-json"}}]}}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		}, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	_, err := fn(context.Background(), []session.Message{}, nil)
	if err == nil {
		t.Fatal("expected bad args error")
	}
}

func TestNewProviderCompletion_ToolCallsMarshalError(t *testing.T) {
	marshalToolCallsHook = func(v any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	defer func() { marshalToolCallsHook = nil }()

	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		body := `{"choices":[{"message":{"role":"assistant","content":"using tool","tool_calls":[{"id":"tc1","type":"function","function":{"name":"sin_read","arguments":"{}"}}]}}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		}, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	_, err := fn(context.Background(), []session.Message{}, nil)
	if err == nil {
		t.Fatal("expected re-marshal error")
	}
}

func TestNewProviderCompletion_SuccessNoToolCalls(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		// Verify Authorization header is set when APIKey is present.
		if auth := req.Header.Get("Authorization"); auth != "Bearer secret" {
			t.Fatalf("expected bearer auth, got %q", auth)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`))),
		}, nil
	})
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	comp, err := fn(context.Background(), []session.Message{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if comp.Text != "done" {
		t.Fatalf("unexpected text %q", comp.Text)
	}
	if comp.Raw.Role != "assistant" {
		t.Fatalf("unexpected role %q", comp.Raw.Role)
	}
	if len(comp.ToolCalls) != 0 {
		t.Fatal("expected no tool calls")
	}
}

func TestNewProviderCompletion_NoAPIKey(t *testing.T) {
	c := &llm.Client{
		BaseURL: "http://fake.local",
		HTTP: &http.Client{Transport: &fakeRoundTripper{fn: func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Authorization") != "" {
				t.Fatal("expected no Authorization header when APIKey is empty")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"choices":[{"message":{"role":"assistant","content":"done"}}]}`))),
			}, nil
		}}},
	}
	fn := NewProviderCompletion(c, "model", 100, 0.0)
	comp, err := fn(context.Background(), []session.Message{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if comp.Text != "done" {
		t.Fatalf("unexpected text %q", comp.Text)
	}
}

// --- Streaming completion tests (Feature 1) ---------------------------------

func TestNewProviderCompletionStream_ContentOnly(t *testing.T) {
	sseBody := "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream header")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(sseBody)),
		}, nil
	})
	var collected strings.Builder
	fn := NewProviderCompletionStream(c, "model", 100, 0.0, nil, nil, func(s string) {
		collected.WriteString(s)
	})
	comp, err := fn(context.Background(), []session.Message{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if comp.Text != "hello world" {
		t.Fatalf("expected 'hello world', got %q", comp.Text)
	}
	if collected.String() != "hello world" {
		t.Fatalf("stream callback should receive full content, got %q", collected.String())
	}
	if len(comp.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %d", len(comp.ToolCalls))
	}
	if comp.Raw.Role != "assistant" {
		t.Fatalf("expected role assistant, got %q", comp.Raw.Role)
	}
}

func TestNewProviderCompletionStream_ToolCalls(t *testing.T) {
	sseBody := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"sin_read\",\"arguments\":\"\"}}]}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"/tmp\\\"}\"}}]}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(sseBody)),
		}, nil
	})
	fn := NewProviderCompletionStream(c, "model", 100, 0.0, nil, nil, nil)
	comp, err := fn(context.Background(), []session.Message{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(comp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(comp.ToolCalls))
	}
	if comp.ToolCalls[0].Name != "sin_read" {
		t.Fatalf("expected sin_read, got %q", comp.ToolCalls[0].Name)
	}
	if comp.ToolCalls[0].ID != "call_1" {
		t.Fatalf("expected call_1, got %q", comp.ToolCalls[0].ID)
	}
	if comp.ToolCalls[0].Args["path"] != "/tmp" {
		t.Fatalf("expected path=/tmp, got %v", comp.ToolCalls[0].Args["path"])
	}
	if len(comp.Raw.ToolCalls) == 0 {
		t.Fatal("expected raw tool calls to be populated")
	}
}

func TestNewProviderCompletionStream_Usage(t *testing.T) {
	sseBody := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n" +
		"data: [DONE]\n\n"
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(sseBody)),
		}, nil
	})
	fn := NewProviderCompletionStream(c, "model", 100, 0.0, nil, nil, nil)
	comp, err := fn(context.Background(), []session.Message{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if comp.Usage.TotalTokens != 15 {
		t.Fatalf("expected 15 total tokens, got %d", comp.Usage.TotalTokens)
	}
	if comp.Usage.PromptTokens != 10 {
		t.Fatalf("expected 10 prompt tokens, got %d", comp.Usage.PromptTokens)
	}
}

func TestNewProviderCompletionStream_Non200Status(t *testing.T) {
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(bytes.NewReader([]byte("busy"))),
		}, nil
	})
	fn := NewProviderCompletionStream(c, "model", 100, 0.0, nil, nil, nil)
	_, err := fn(context.Background(), []session.Message{}, nil)
	if err == nil {
		t.Fatal("expected non-200 error")
	}
}

func TestNewProviderCompletionStream_MixedContentAndToolCalls(t *testing.T) {
	sseBody := "data: {\"choices\":[{\"delta\":{\"content\":\"Let me read that.\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"sin_read\",\"arguments\":\"{\\\"path\\\":\\\"/a\\\"}\"}}]}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"
	c := newFakeClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(sseBody)),
		}, nil
	})
	var collected strings.Builder
	fn := NewProviderCompletionStream(c, "model", 100, 0.0, nil, nil, func(s string) {
		collected.WriteString(s)
	})
	comp, err := fn(context.Background(), []session.Message{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if comp.Text != "Let me read that." {
		t.Fatalf("expected content, got %q", comp.Text)
	}
	if collected.String() != "Let me read that." {
		t.Fatalf("stream callback should receive content, got %q", collected.String())
	}
	if len(comp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(comp.ToolCalls))
	}
}
