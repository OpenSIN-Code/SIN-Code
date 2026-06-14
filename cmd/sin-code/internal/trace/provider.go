// SPDX-License-Identifier: MIT
// Purpose: OpenTelemetry Tracer Provider Setup for SIN-Code
// Integrates with the hook lifecycle events for automatic span generation
package trace

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// ProviderConfig konfiguriert den OTel Tracer
type ProviderConfig struct {
	ServiceName    string
	ServiceVersion string
	ExporterType   string // "stdout" oder "otlp"
	OTLPEndpoint   string // z.B. "localhost:4318" für Langfuse/Jaeger
	Insecure       bool
}

// InitProvider initialisiert den globalen OTel Tracer
func InitProvider(ctx context.Context, cfg ProviderConfig) (*sdktrace.TracerProvider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	var exporter sdktrace.SpanExporter

	switch cfg.ExporterType {
	case "stdout":
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
	case "otlp":
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exporter, err = otlptracehttp.New(ctx, opts...)
	default:
		// Default: Noop (kein Export)
		return sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		), nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

// Shutdown beendet den Provider sauber
func Shutdown(ctx context.Context, tp *sdktrace.TracerProvider) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return tp.Shutdown(ctx)
}
