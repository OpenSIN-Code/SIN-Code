// SPDX-License-Identifier: MIT
// Purpose: tests for Loop.SetModel — mid-session model switching that
// preserves conversation context (mandate M7: race-free).
package agentloop

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestSetModel_UpdatesModel(t *testing.T) {
	loop := &Loop{
		Model: "initial-model",
	}
	loop.SetModel("new-model")
	if got := loop.GetModel(); got != "new-model" {
		t.Fatalf("GetModel() = %q, want %q", got, "new-model")
	}
}

func TestSetModel_EmptyInitial(t *testing.T) {
	loop := &Loop{}
	loop.SetModel("first-model")
	if got := loop.GetModel(); got != "first-model" {
		t.Fatalf("GetModel() = %q, want %q", got, "first-model")
	}
}

func TestSetModel_RebuildsCompletion(t *testing.T) {
	var currentModel atomic.Value
	currentModel.Store("model-a")

	builder := func(m string) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		currentModel.Store(m)
		return func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{
				Text: fmt.Sprintf("response from %s", currentModel.Load().(string)),
				Raw:  session.Message{Role: "assistant", Content: "ok"},
			}, nil
		}
	}

	loop := &Loop{
		Model:             "model-a",
		CompletionBuilder: builder,
	}
	loop.Completion = builder("model-a")

	loop.SetModel("model-b")
	if got := loop.GetModel(); got != "model-b" {
		t.Fatalf("GetModel() = %q, want %q", got, "model-b")
	}

	if loop.Completion == nil {
		t.Fatal("Completion is nil after SetModel")
	}

	completion := loop.getCompletion()
	result, err := completion(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Completion error: %v", err)
	}
	if result.Text != "response from model-b" {
		t.Fatalf("Completion text = %q, want %q", result.Text, "response from model-b")
	}
}

func TestSetModel_NilBuilderPreservesCompletion(t *testing.T) {
	originalCompletion := func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		return &Completion{Text: "original", Raw: session.Message{Role: "assistant", Content: "original"}}, nil
	}
	loop := &Loop{
		Model:     "model-a",
		Completion: originalCompletion,
	}

	loop.SetModel("model-b")

	if got := loop.GetModel(); got != "model-b" {
		t.Fatalf("GetModel() = %q, want %q", got, "model-b")
	}
	if loop.Completion == nil {
		t.Fatal("Completion should still be the original when builder is nil")
	}
}

func TestSetModel_PreservesMessages(t *testing.T) {
	s := setupSession(t)

	messages := []session.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "do something"},
	}
	_ = s.SaveHistory(messages)

	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	var modelUsed atomic.Value
	modelUsed.Store("initial")

	builder := func(m string) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		modelUsed.Store(m)
		return func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		}
	}

	loop := &Loop{
		Gate:              gate,
		Workspace:         "/tmp",
		Model:             "initial",
		CompletionBuilder: builder,
	}
	loop.Completion = builder("initial")

	loop.SetModel("switched-model")

	res, err := loop.Run(context.Background(), s, "continue")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !res.Verified {
		t.Fatal("expected verified=true")
	}

	if modelUsed.Load().(string) != "switched-model" {
		t.Fatalf("model used = %q, want %q", modelUsed.Load(), "switched-model")
	}

	hist := s.History()
	hasUser := false
	hasAssistant := false
	for _, m := range hist {
		if m.Role == "user" && m.Content == "hello" {
			hasUser = true
		}
		if m.Role == "assistant" && m.Content == "hi there" {
			hasAssistant = true
		}
	}
	if !hasUser {
		t.Fatal("prior user message 'hello' was lost after SetModel + Run")
	}
	if !hasAssistant {
		t.Fatal("prior assistant message 'hi there' was lost after SetModel + Run")
	}
}

func TestSetModel_RaceFree(t *testing.T) {
	loop := &Loop{
		Model: "initial",
	}
	loop.Completion = func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		return &Completion{Text: "ok", Raw: session.Message{Role: "assistant", Content: "ok"}}, nil
	}
	loop.CompletionBuilder = func(m string) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		return func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "ok", Raw: session.Message{Role: "assistant", Content: "ok"}}, nil
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			loop.SetModel(fmt.Sprintf("model-%d", n))
		}(i)
		go func() {
			defer wg.Done()
			_ = loop.GetModel()
			_ = loop.getCompletion()
		}()
	}
	wg.Wait()
}
