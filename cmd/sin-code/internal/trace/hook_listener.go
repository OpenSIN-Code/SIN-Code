// SPDX-License-Identifier: MIT
// Purpose: Hook Listener for automatic span generation from lifecycle events
package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
)

var tracer = otel.Tracer("sin-code-agent")

// spanContextMap speichert aktive Spans pro Session/Event
var spanContextMap = make(map[string]context.Context)

// RegisterHookListener registriert OTel-Span-Erzeugung für alle Lifecycle-Events
func RegisterHookListener(hookMgr *hooks.Manager) {
	// Session-Start
	hookMgr.On(hooks.SessionStart, func(ctx context.Context, event hooks.Payload) {
		sessionID := event.SessionID
		ctx, span := tracer.Start(ctx, "Session",
			trace.WithAttributes(
				attribute.String("session.id", sessionID),
			),
		)
		spanContextMap[sessionID] = ctx
		_ = span // Span wird bei SessionEnd geschlossen
	})

	// Session-Ende
	hookMgr.On(hooks.SessionEnd, func(ctx context.Context, event hooks.Payload) {
		sessionID := event.SessionID
		if span := trace.SpanFromContext(ctx); span != nil {
			span.End()
		}
		delete(spanContextMap, sessionID)
	})

	// Turn-Start
	hookMgr.On(hooks.TurnStart, func(ctx context.Context, event hooks.Payload) {
		ctx, span := tracer.Start(ctx, "Turn",
			trace.WithAttributes(
				attribute.String("session.id", event.SessionID),
			),
		)
		_ = span
	})

	// Tool-Call
	hookMgr.On(hooks.ToolPre, func(ctx context.Context, event hooks.Payload) {
		toolName := ""
		if event.Data != nil {
			if name, ok := event.Data["tool_name"]; ok {
				toolName = name.(string)
			}
		}
		ctx, span := tracer.Start(ctx, "ToolCall",
			trace.WithAttributes(
				attribute.String("tool.name", toolName),
				attribute.String("session.id", event.SessionID),
			),
		)
		_ = span
	})

	// Verify-Gate
	hookMgr.On(hooks.VerifyPre, func(ctx context.Context, event hooks.Payload) {
		ctx, span := tracer.Start(ctx, "Verify",
			trace.WithAttributes(
				attribute.String("session.id", event.SessionID),
			),
		)
		_ = span
	})

	hookMgr.On(hooks.VerifyPass, func(ctx context.Context, event hooks.Payload) {
		if span := trace.SpanFromContext(ctx); span != nil {
			span.SetAttributes(
				attribute.Bool("verify.passed", true),
			)
			span.End()
		}
	})

	hookMgr.On(hooks.VerifyFail, func(ctx context.Context, event hooks.Payload) {
		if span := trace.SpanFromContext(ctx); span != nil {
			span.SetStatus(codes.Error, "Verification failed")
			span.SetAttributes(
				attribute.Bool("verify.passed", false),
			)
			span.End()
		}
	})

	// Memory-Write
	hookMgr.On(hooks.MemoryWrite, func(ctx context.Context, event hooks.Payload) {
		ctx, span := tracer.Start(ctx, "MemoryWrite",
			trace.WithAttributes(
				attribute.String("session.id", event.SessionID),
			),
		)
		_ = span
	})

	// Error-Handling
	hookMgr.On(hooks.SessionEnd, func(ctx context.Context, event hooks.Payload) {
		if event.Data != nil {
			if errMsg, ok := event.Data["error"]; ok && errMsg != "" {
				if span := trace.SpanFromContext(ctx); span != nil {
					span.SetStatus(codes.Error, errMsg.(string))
				}
			}
		}
	})
}

// truncate kürzt Strings für Attribute (OTel hat Limits)
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
