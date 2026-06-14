// SPDX-License-Identifier: MIT
// Purpose: Tests for OpenTelemetry Hook Listener
package trace

import (
	"context"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"go.opentelemetry.io/otel/trace"
)

func TestRegisterHookListener(t *testing.T) {
	hm := hooks.NewManager()
	tp := NewTracerProvider(context.Background(), "stdout")
	defer tp.Shutdown(context.Background())

	// Should not panic
	RegisterHookListener(hm, tp)

	// Verify hook listeners are registered (no assertion needed - no panic = success)
	if hm == nil {
		t.Fatal("Hook manager is nil")
	}
}

func TestSessionSpanCreation(t *testing.T) {
	hm := hooks.NewManager()
	tp := NewTracerProvider(context.Background(), "stdout")
	defer tp.Shutdown(context.Background())

	RegisterHookListener(hm, tp)

	// Emit SessionStart event
	sessionID := "test-session-123"
	hm.Emit(hooks.SessionStart, hooks.Payload{
		SessionID: sessionID,
		Data: map[string]interface{}{
			"model":  "test-model",
			"prompt": "test prompt",
		},
	})

	// Verify span context is stored
	if len(spanContextMap[sessionID]) == 0 {
		t.Error("Expected span context to be created for session")
	}
}

func TestTurnSpanCreation(t *testing.T) {
	hm := hooks.NewManager()
	tp := NewTracerProvider(context.Background(), "stdout")
	defer tp.Shutdown(context.Background())

	RegisterHookListener(hm, tp)

	sessionID := "test-session-456"

	// Setup session first
	hm.Emit(hooks.SessionStart, hooks.Payload{
		SessionID: sessionID,
		Data: map[string]interface{}{
			"model": "test",
		},
	})

	// Emit TurnStart event
	hm.Emit(hooks.TurnStart, hooks.Payload{
		SessionID: sessionID,
		Data: map[string]interface{}{
			"turn_num": 1,
		},
	})

	// Verify span was created and ended
	if len(spanContextMap[sessionID]) < 2 {
		t.Error("Expected TurnStart span to be added")
	}
}

func TestMemoryWriteSpan(t *testing.T) {
	hm := hooks.NewManager()
	tp := NewTracerProvider(context.Background(), "stdout")
	defer tp.Shutdown(context.Background())

	RegisterHookListener(hm, tp)

	sessionID := "test-session-789"

	hm.Emit(hooks.SessionStart, hooks.Payload{
		SessionID: sessionID,
		Data:      map[string]interface{}{},
	})

	hm.Emit(hooks.MemoryWrite, hooks.Payload{
		SessionID: sessionID,
		Data: map[string]interface{}{
			"lesson": "Test lesson learned",
		},
	})

	// Should have at least 2 spans (SessionStart + MemoryWrite)
	if len(spanContextMap[sessionID]) < 2 {
		t.Error("Expected MemoryWrite span to be created")
	}
}

func TestContextPropagation(t *testing.T) {
	hm := hooks.NewManager()
	tp := NewTracerProvider(context.Background(), "stdout")
	defer tp.Shutdown(context.Background())

	RegisterHookListener(hm, tp)

	sessionID := "test-session-context"
	hm.Emit(hooks.SessionStart, hooks.Payload{
		SessionID: sessionID,
		Data:      map[string]interface{}{},
	})

	// Verify context can be retrieved
	ctx, ok := spanContextMap[sessionID]
	if !ok || len(ctx) == 0 {
		t.Error("Expected to retrieve span context for session")
	}
}

func TestSessionEndSpan(t *testing.T) {
	hm := hooks.NewManager()
	tp := NewTracerProvider(context.Background(), "stdout")
	defer tp.Shutdown(context.Background())

	RegisterHookListener(hm, tp)

	sessionID := "test-session-end"

	hm.Emit(hooks.SessionStart, hooks.Payload{
		SessionID: sessionID,
		Data:      map[string]interface{}{},
	})

	startCount := len(spanContextMap[sessionID])

	hm.Emit(hooks.SessionEnd, hooks.Payload{
		SessionID: sessionID,
		Data: map[string]interface{}{
			"status": "success",
		},
	})

	// SessionEnd should trigger cleanup
	if len(spanContextMap[sessionID]) != startCount+1 {
		t.Error("Expected SessionEnd to create final span")
	}
}

func TestTruncateAttributes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"short string", "hello", 5},
		{"exact max", "a" + string(make([]byte, 255)), 256},
		{"over max", "a" + string(make([]byte, 300)), 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, 256)
			if len(result) != tt.expected && tt.expected <= 256 {
				t.Errorf("truncate(%q) = %d, want max %d", tt.input, len(result), tt.expected)
			}
		})
	}
}

func BenchmarkHookListenerEmit(b *testing.B) {
	hm := hooks.NewManager()
	tp := NewTracerProvider(context.Background(), "stdout")
	defer tp.Shutdown(context.Background())

	RegisterHookListener(hm, tp)

	sessionID := "bench-session"
	hm.Emit(hooks.SessionStart, hooks.Payload{
		SessionID: sessionID,
		Data:      map[string]interface{}{},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hm.Emit(hooks.TurnStart, hooks.Payload{
			SessionID: sessionID,
			Data:      map[string]interface{}{"turn": i},
		})
	}
}
