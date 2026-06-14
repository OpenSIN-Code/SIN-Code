// SPDX-License-Identifier: MIT
// Purpose: verify the agent loop invokes CompressMessages before each model
// request and forwards the compressed history (issue #118 wiring).
package agentloop

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestRun_InvokesCompressMessages(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	compressCalls := 0
	var sawCompressed bool

	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		CompressMessages: func(ctx context.Context, msgs []session.Message) ([]session.Message, error) {
			compressCalls++
			out := make([]session.Message, len(msgs))
			copy(out, msgs)
			for i := range out {
				out[i].Content = "[c]" + out[i].Content
			}
			return out, nil
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			for _, m := range msgs {
				if strings.HasPrefix(m.Content, "[c]") {
					sawCompressed = true
				}
			}
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}

	if _, err := loop.Run(context.Background(), s, "hello"); err != nil {
		t.Fatal(err)
	}
	if compressCalls == 0 {
		t.Fatal("expected CompressMessages to be invoked")
	}
	if !sawCompressed {
		t.Fatal("expected Completion to receive compressed messages")
	}

	// The persisted session history must remain uncompressed.
	for _, m := range s.History() {
		if strings.HasPrefix(m.Content, "[c]") {
			t.Fatal("session history should not be mutated by compression")
		}
	}
}

func TestRun_CompressMessagesErrorFallsBack(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	loop := &Loop{
		Gate:      gate,
		Workspace: "/tmp",
		CompressMessages: func(ctx context.Context, msgs []session.Message) ([]session.Message, error) {
			return nil, context.DeadlineExceeded // simulate failure
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			if len(msgs) == 0 {
				t.Fatal("expected original messages on compression failure")
			}
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}

	res, err := loop.Run(context.Background(), s, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("run should succeed even when compression fails")
	}
}
