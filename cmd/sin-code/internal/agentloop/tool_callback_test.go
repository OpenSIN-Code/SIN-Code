// SPDX-License-Identifier: MIT
// Purpose: live ToolStart/ToolEnd callback tests (issue for live TUI tool tree
// and headless structured progress).
package agentloop

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestToolStartEndCallbacks(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	var mu sync.Mutex
	var startCalls []ToolCall
	var endCalls []struct {
		tc     ToolCall
		d      time.Duration
		output string
		err    error
	}
	toolCalls := 0

	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			toolCalls++
			time.Sleep(10 * time.Millisecond)
			return "ok", nil
		},
		ToolStart: func(ctx context.Context, tc ToolCall) {
			mu.Lock()
			defer mu.Unlock()
			startCalls = append(startCalls, tc)
		},
		ToolEnd: func(ctx context.Context, tc ToolCall, duration time.Duration, output string, err error) {
			mu.Lock()
			defer mu.Unlock()
			endCalls = append(endCalls, struct {
				tc     ToolCall
				d      time.Duration
				output string
				err    error
			}{tc, duration, output, err})
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			if toolCalls == 0 {
				return &Completion{
					Text:      "",
					ToolCalls: []ToolCall{{ID: "tc1", Name: "sin_test", Args: map[string]any{}}},
					Raw:       session.Message{Role: "assistant", Content: ""},
				}, nil
			}
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}

	res, err := loop.Run(context.Background(), s, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected verified")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(startCalls) != 1 {
		t.Fatalf("ToolStart calls want 1, got %d", len(startCalls))
	}
	if startCalls[0].Name != "sin_test" {
		t.Fatalf("ToolStart name want sin_test, got %s", startCalls[0].Name)
	}

	if len(endCalls) != 1 {
		t.Fatalf("ToolEnd calls want 1, got %d", len(endCalls))
	}
	if endCalls[0].tc.Name != "sin_test" {
		t.Fatalf("ToolEnd name want sin_test, got %s", endCalls[0].tc.Name)
	}
	if endCalls[0].output != "ok" {
		t.Fatalf("ToolEnd output want ok, got %s", endCalls[0].output)
	}
	if endCalls[0].err != nil {
		t.Fatalf("ToolEnd err want nil, got %v", endCalls[0].err)
	}
	if endCalls[0].d <= 0 {
		t.Fatalf("ToolEnd duration want >0, got %v", endCalls[0].d)
	}
}
