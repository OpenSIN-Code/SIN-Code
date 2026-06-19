package agentloop

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestCovDebug(t *testing.T) {
	s := setupSession(t)
	gate := verify.NewGate("poc", func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil }, nil)
	turns := 0
	loop := &Loop{
		Gate:                   gate,
		Workspace:              "/tmp",
		MaxTurns:               5,
		MaxStopRejects:         1,
		CoverageForbiddenTools: []string{"sin_bash"},
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "ok", nil
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			turns++
			t.Logf("Completion call #%d, Coverage=%v", turns, loop.Coverage)
			return &Completion{
				Text:      "done",
				ToolCalls: []ToolCall{{ID: "t1", Name: "sin_bash", Args: map[string]any{"command": "ls"}}},
				Raw:       session.Message{Role: "assistant", Content: "done"},
			}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "test")
	t.Logf("Result: %v, Err: %v", res, err)
	if err != nil && strings.Contains(err.Error(), "forbidden") {
		t.Logf("✓ PASS got forbidden error")
	} else {
		t.Logf("✗ FAIL")
	}
}
