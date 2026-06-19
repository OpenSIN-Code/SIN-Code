package agentloop

import (
	"context"
	"testing"
)

func TestCovDebug(t *testing.T) {
	h := &Loop{
		MaxTurns: 5,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			t.Logf("h.MaxTurns=%d", h.MaxTurns)
			return nil, nil
		},
	}
	if h.Completion != nil {
		ctx := context.Background()
		_, _ = h.Completion(ctx, nil, nil)
	}
}
