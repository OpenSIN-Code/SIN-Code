// SPDX-License-Identifier: MIT
// Purpose: coverage tests for the adapters package. Stubs external
// dependencies via package-level hook variables to reach 100% statement
// coverage without real network or database I/O.
package adapters

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func chatResponse(text string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []struct {
			Index        int         `json:"index"`
			Message      llm.Message `json:"message"`
			FinishReason string      `json:"finish_reason"`
		}{{Message: llm.Message{Content: text}}},
	}
}

type fakeTransport struct {
	response string
	status   int
}

func (ft *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	status := ft.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(ft.response)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestVerifyGate_QualityGate(t *testing.T) {
	ctx := context.Background()
	workdir := "."

	t.Run("nil gate", func(t *testing.T) {
		a := VerifyGate{}
		passed, report, err := a.QualityGate(ctx, workdir)
		if err != nil || !passed || report != "" {
			t.Fatalf("nil gate: got passed=%v report=%q err=%v", passed, report, err)
		}
	})

	t.Run("passing gate", func(t *testing.T) {
		gate := verify.NewGate("poc", func(context.Context, string) (bool, string, error) {
			return true, "ok", nil
		}, nil)
		a := VerifyGate{Gate: gate}
		passed, report, err := a.QualityGate(ctx, workdir)
		if err != nil || !passed || report != "ok" {
			t.Fatalf("passing gate: got passed=%v report=%q err=%v", passed, report, err)
		}
	})

	t.Run("failing gate", func(t *testing.T) {
		gate := verify.NewGate("poc", func(context.Context, string) (bool, string, error) {
			return false, "fail", nil
		}, nil)
		a := VerifyGate{Gate: gate}
		passed, report, err := a.QualityGate(ctx, workdir)
		if err != nil || passed || report != "fail" {
			t.Fatalf("failing gate: got passed=%v report=%q err=%v", passed, report, err)
		}
	})
}

func TestMemoryBridge_RecordInstinct(t *testing.T) {
	ctx := context.Background()

	t.Run("nil store", func(t *testing.T) {
		b := MemoryBridge{}
		if err := b.RecordInstinct(ctx, "t", "a", "d", 0.5); err != nil {
			t.Fatalf("nil store: got err=%v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		old := memoryAddHook
		defer func() { memoryAddHook = old }()

		var got *memory.Memory
		memoryAddHook = func(s *memory.Store, m *memory.Memory) error {
			got = m
			return nil
		}
		b := MemoryBridge{Store: &memory.Store{}}
		if err := b.RecordInstinct(ctx, "t", "a", "d", 0.5); err != nil {
			t.Fatalf("success: got err=%v", err)
		}
		if got == nil {
			t.Fatal("success: Add hook was not called")
		}
		if got.Insight != "instinct: t -> a" {
			t.Fatalf("success: insight=%q, want %q", got.Insight, "instinct: t -> a")
		}
	})

	t.Run("error", func(t *testing.T) {
		old := memoryAddHook
		defer func() { memoryAddHook = old }()

		memoryAddHook = func(s *memory.Store, m *memory.Memory) error {
			return errors.New("boom")
		}
		b := MemoryBridge{Store: &memory.Store{}}
		if err := b.RecordInstinct(ctx, "t", "a", "d", 0.5); err == nil {
			t.Fatal("error: expected error, got nil")
		}
	})

	t.Run("default hook", func(t *testing.T) {
		tmp := t.TempDir()
		store, err := memory.Open(filepath.Join(tmp, "test.db"))
		if err != nil {
			t.Fatalf("default hook: open store: %v", err)
		}
		defer store.Close()
		b := MemoryBridge{Store: store}
		if err := b.RecordInstinct(ctx, "t", "a", "d", 0.5); err != nil {
			t.Fatalf("default hook: record: %v", err)
		}
	})
}

func TestBackgroundCompleter_Complete(t *testing.T) {
	ctx := context.Background()

	t.Run("nil client", func(t *testing.T) {
		c := BackgroundCompleter{}
		out, err := c.Complete(ctx, "sys", "usr")
		if err != nil || out != "" {
			t.Fatalf("nil client: got out=%q err=%v", out, err)
		}
	})

	t.Run("error", func(t *testing.T) {
		old := chatHook
		defer func() { chatHook = old }()

		chatHook = func(context.Context, *llm.Client, llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, errors.New("boom")
		}
		c := BackgroundCompleter{Client: &llm.Client{}}
		out, err := c.Complete(ctx, "sys", "usr")
		if err == nil || out != "" {
			t.Fatalf("error: got out=%q err=%v", out, err)
		}
	})

	t.Run("default model", func(t *testing.T) {
		old := chatHook
		defer func() { chatHook = old }()

		chatHook = func(_ context.Context, _ *llm.Client, req llm.ChatRequest) (*llm.ChatResponse, error) {
			if req.Model != "anthropic/claude-haiku-4-5" {
				t.Fatalf("default model: got model=%q", req.Model)
			}
			return chatResponse("hello"), nil
		}
		c := BackgroundCompleter{Client: &llm.Client{}}
		out, err := c.Complete(ctx, "sys", "usr")
		if err != nil || out != "hello" {
			t.Fatalf("default model: got out=%q err=%v", out, err)
		}
	})

	t.Run("explicit model", func(t *testing.T) {
		old := chatHook
		defer func() { chatHook = old }()

		chatHook = func(_ context.Context, _ *llm.Client, req llm.ChatRequest) (*llm.ChatResponse, error) {
			if req.Model != "custom-model" {
				t.Fatalf("explicit model: got model=%q", req.Model)
			}
			return chatResponse("world"), nil
		}
		c := BackgroundCompleter{Client: &llm.Client{}, Model: "custom-model"}
		out, err := c.Complete(ctx, "sys", "usr")
		if err != nil || out != "world" {
			t.Fatalf("explicit model: got out=%q err=%v", out, err)
		}
	})

	t.Run("default hook", func(t *testing.T) {
		client := &llm.Client{
			BaseURL: "http://test",
			HTTP:    &http.Client{Transport: &fakeTransport{response: `{"id":"1","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`}},
		}
		c := BackgroundCompleter{Client: client}
		out, err := c.Complete(ctx, "sys", "usr")
		if err != nil || out != "hello" {
			t.Fatalf("default hook: got out=%q err=%v", out, err)
		}
	})
}

func TestFtoa2(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{0.5, "0.50"},
		{0.05, "0.05"},
		{1.0, "1.00"},
		{-0.05, "0.04"}, // exercises frac < 0 branch
	}
	for _, tc := range cases {
		if got := ftoa2(tc.in); got != tc.want {
			t.Fatalf("ftoa2(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIntToStr(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{5, "5"},
		{-5, "-5"},
		{123, "123"},
	}
	for _, tc := range cases {
		if got := intToStr(tc.in); got != tc.want {
			t.Fatalf("intToStr(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("hello", "el", "x") {
		t.Fatal("containsAny: expected match")
	}
	if containsAny("hello", "x", "y") {
		t.Fatal("containsAny: expected no match")
	}
}
