package observability

import (
	"context"
	"fmt"
	"time"
)

// Tracer implements OpenTelemetry-style tracing
type Tracer struct {
	traces map[string]*Trace
}

// NewTracer creates a new tracer
func NewTracer() *Tracer {
	return &Tracer{
		traces: make(map[string]*Trace),
	}
}

// StartSpan starts a new trace span
func (t *Tracer) StartSpan(ctx context.Context, traceID string, spanName string, parentID string) *TraceSpan {
	span := &TraceSpan{
		ID:        fmt.Sprintf("%s_%s", traceID, spanName),
		ParentID:  parentID,
		Name:      spanName,
		StartTime: time.Now(),
		Tags:      make(map[string]string),
		Events:    []string{},
	}

	if _, exists := t.traces[traceID]; !exists {
		t.traces[traceID] = &Trace{
			ID:    traceID,
			Spans: []*TraceSpan{},
		}
	}

	t.traces[traceID].Spans = append(t.traces[traceID].Spans, span)

	return span
}

// EndSpan ends a trace span
func (t *Tracer) EndSpan(span *TraceSpan) {
	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)
}

// AddEvent adds an event to a span
func (t *Tracer) AddEvent(span *TraceSpan, event string) {
	span.Events = append(span.Events, event)
}

// SetTag sets a tag on a span
func (t *Tracer) SetTag(span *TraceSpan, key string, value string) {
	span.Tags[key] = value
}

// GetTrace retrieves a complete trace
func (t *Tracer) GetTrace(traceID string) *Trace {
	return t.traces[traceID]
}
