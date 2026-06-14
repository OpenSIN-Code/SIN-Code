// SPDX-License-Identifier: MIT
// Purpose: Hook Listener for automatic span generation from lifecycle events
package trace

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
)

var tracer = otel.Tracer("sin-code-agent")

// SessionSpanMap speichert aktive Session-Spans (Session-Level Span bleibt offen während ganze Session)
type SessionSpanMap struct {
	mu    sync.RWMutex
	spans map[string]trace.Span
}

var sessionSpans = &SessionSpanMap{spans: make(map[string]trace.Span)}

// RegisterHookListener registriert einen Hook-Listener in der Hook-Engine
// um automatisch Spans für Lifecycle-Events zu generieren
func RegisterHookListener(hookEngine *hooks.Engine) {
	if hookEngine == nil {
		return
	}

	// Hinweis: SIN-Code Hook-Engine ist event-basiert und feuer synchron.
	// Wir erzeugen Spans inline bei Hook-Fire.
	// Für span.End(): Single-Event-Spans (z.B. tool.pre, turn.start) werden sofort geschlossen.
	// Für Multi-Event-Spans (z.B. session.start → session.end) speichern wir sie in sessionSpans.
}

// FireWithTrace wraps einen Hook-Fire mit OTel-Tracing
func FireWithTrace(ctx context.Context, hookEngine *hooks.Engine, p hooks.Payload) hooks.Result {
	if hookEngine == nil {
		return hooks.Result{}
	}

	// Span-Name basierend auf Event
	spanName := p.Event

	// Für Sessions: öffne/schließe Root-Span
	sessionID := p.SessionID
	if p.Event == hooks.SessionStart {
		sessionSpans.mu.Lock()
		ctx, span := tracer.Start(ctx, "session", trace.WithAttributes(
			attribute.String("session.id", sessionID),
			attribute.String("workspace", p.Workspace),
		))
		sessionSpans.spans[sessionID] = span
		sessionSpans.mu.Unlock()
	}

	// Für alle Events: erstelle Sub-Span unter Session-Span (falls existiert)
	sessionSpans.mu.RLock()
	sessionSpan, hasSession := sessionSpans.spans[sessionID]
	sessionSpans.mu.RUnlock()

	if hasSession && sessionSpan != nil {
		ctx = trace.ContextWithSpan(ctx, sessionSpan)
	}

	// Event-spezifische Spans
	switch p.Event {
	case hooks.TurnStart:
		ctx, span := tracer.Start(ctx, "turn.start", trace.WithAttributes(
			attribute.String("session.id", sessionID),
		))
		span.End() // Single-point event
	case hooks.TurnEnd:
		ctx, span := tracer.Start(ctx, "turn.end", trace.WithAttributes(
			attribute.String("session.id", sessionID),
		))
		span.End()

	case hooks.ToolPre:
		toolName := extractString(p.Data, "tool_name", "unknown")
		ctx, span := tracer.Start(ctx, "tool.pre", trace.WithAttributes(
			attribute.String("tool.name", toolName),
			attribute.String("session.id", sessionID),
		))
		span.End()
	case hooks.ToolPost:
		toolName := extractString(p.Data, "tool_name", "unknown")
		ctx, span := tracer.Start(ctx, "tool.post", trace.WithAttributes(
			attribute.String("tool.name", toolName),
			attribute.String("session.id", sessionID),
		))
		span.End()

	case hooks.VerifyPre:
		ctx, span := tracer.Start(ctx, "verify.pre", trace.WithAttributes(
			attribute.String("session.id", sessionID),
		))
		span.End()
	case hooks.VerifyPass:
		ctx, span := tracer.Start(ctx, "verify.pass", trace.WithAttributes(
			attribute.String("session.id", sessionID),
		))
		span.End()
	case hooks.VerifyFail:
		reason := extractString(p.Data, "reason", "")
		ctx, span := tracer.Start(ctx, "verify.fail", trace.WithAttributes(
			attribute.String("session.id", sessionID),
			attribute.String("reason", reason),
		))
		span.SetStatus(codes.Error, reason)
		span.End()

	case hooks.MemoryWrite:
		ctx, span := tracer.Start(ctx, "memory.write", trace.WithAttributes(
			attribute.String("session.id", sessionID),
		))
		span.End()

	case hooks.SessionEnd:
		ctx, span := tracer.Start(ctx, "session.end", trace.WithAttributes(
			attribute.String("session.id", sessionID),
		))
		span.End()

		// Schließe Session-Root-Span
		sessionSpans.mu.Lock()
		if rootSpan, exists := sessionSpans.spans[sessionID]; exists {
			rootSpan.End()
			delete(sessionSpans.spans, sessionID)
		}
		sessionSpans.mu.Unlock()
	}

	// Führe Hook-Fire durch
	_ = ctx
	return hookEngine.Fire(ctx, p)
}

// extractString extrahiert einen String-Wert aus Payload.Data (mit Fallback)
func extractString(data map[string]any, key, fallback string) string {
	if data == nil {
		return fallback
	}
	if val, ok := data[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return fallback
}
