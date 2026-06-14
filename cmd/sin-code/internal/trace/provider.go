// SPDX-License-Identifier: MIT
// Purpose: OpenTelemetry TracerProvider bootstrap for SIN-Code eval /
// agent tracing (issue #75, mandate M2). Pure-Go, no CGO. Uses the
// canonical OTel SDK + stdout/OTLP-HTTP exporters so the same code
// can be smoke-tested locally (stdout) or shipped to Langfuse /
// Jaeger / Arize Phoenix (OTLP) with no changes.
//
// Docs: provider.doc.md (companion overview)
package trace

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ExporterKind enumerates the supported span exporters.
type ExporterKind string

const (
	ExporterStdout ExporterKind = "stdout"
	ExporterOTLP   ExporterKind = "otlp"
	ExporterNoop   ExporterKind = "noop"
)

// defaultShutdownTimeout caps Shutdown so a misbehaving exporter can
// never block the CLI indefinitely. 5s is below the agent loop's
// own per-turn timeout so a stuck flush surfaces before any user-
// visible hang (mandate C6 hard-time-limit).
const defaultShutdownTimeout = 5 * time.Second

// Test hooks for error paths.
var (
	stdouttraceNew    = stdouttrace.New
	otlptracehttpNew  = otlptracehttp.New
	resourceNew       = resource.New
)

// ProviderConfig configures InitProvider.
type ProviderConfig struct {
	ServiceName    string        // semconv service.name (required for prod)
	ServiceVersion string        // semconv service.version
	Environment    string        // semconv deployment.environment (defaults to SIN_ENV)
	Exporter       ExporterKind  // stdout | otlp | noop (default noop)
	OTLPEndpoint   string        // host:port for OTLP/HTTP (default localhost:4318)
	OTLPInsecure   bool          // true -> http, false -> https
	OTLPTimeout    time.Duration // OTLP HTTP timeout (default 10s)
	SampleRate     float64       // 0.0–1.0; 0 == 1.0 (AlwaysSample)
}

// normalize fills defaults for any zero-valued fields so a CLI caller
// can pass only the fields it actually cares about.
func (c *ProviderConfig) normalize() {
	if c.ServiceName == "" {
		c.ServiceName = "sin-code"
	}
	if c.Environment == "" {
		c.Environment = os.Getenv("SIN_ENV")
	}
	if c.OTLPEndpoint == "" {
		c.OTLPEndpoint = "localhost:4318"
	}
	if c.OTLPTimeout == 0 {
		c.OTLPTimeout = 10 * time.Second
	}
	if c.SampleRate == 0 {
		c.SampleRate = 1.0
	}
}

// InitProvider builds and installs a global OTel TracerProvider.
// Pass nil for cfg to get a noop provider (still safe to Shutdown).
func InitProvider(ctx context.Context, cfg *ProviderConfig) (*sdktrace.TracerProvider, error) {
	if cfg == nil {
		cfg = &ProviderConfig{}
	}
	cfg.normalize()
	if cfg.Exporter == "" {
		cfg.Exporter = ExporterNoop
	}

	// Resource is mandatory even for noop — semconv attributes keep
	// the trace dashboard readable when the exporter is OTLP.
	res, err := resourceNew(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("trace: build resource: %w", err)
	}

	switch cfg.Exporter {
	case ExporterStdout:
		exp, err := stdouttraceNew(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(os.Stderr))
		if err != nil {
			return nil, fmt.Errorf("trace: build stdout exporter: %w", err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSyncer(exp), // stdout flushes synchronously so test output is deterministic
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		)
		setGlobal(tp)
		return tp, nil

	case ExporterOTLP:
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
			otlptracehttp.WithTimeout(cfg.OTLPTimeout),
		}
		if cfg.OTLPInsecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		if h := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); h != "" {
			opts = append(opts, otlptracehttp.WithHeaders(parseHeaders(h)))
		}
		exp, err := otlptracehttpNew(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("trace: build OTLP exporter: %w", err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exp,
				sdktrace.WithMaxQueueSize(10000),
				sdktrace.WithMaxExportBatchSize(512),
				sdktrace.WithBatchTimeout(5*time.Second),
			),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))),
		)
		setGlobal(tp)
		return tp, nil

	case ExporterNoop:
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.NeverSample()),
		)
		setGlobal(tp)
		return tp, nil

	default:
		return nil, fmt.Errorf("trace: unknown exporter %q (want stdout|otlp|noop)", cfg.Exporter)
	}
}

// Shutdown flushes any pending spans and tears the provider down.
// Safe to call with a nil receiver.
func Shutdown(ctx context.Context, tp *sdktrace.TracerProvider) error {
	if tp == nil {
		return nil
	}
	c, cancel := context.WithTimeout(ctx, defaultShutdownTimeout)
	defer cancel()
	return tp.Shutdown(c)
}

// Tracer returns a named tracer from the global provider. Falls back to
// the OTel noop tracer if SetTracerProvider was never called so callers
// never see nil.
func Tracer(name string) oteltrace.Tracer { return otel.Tracer(name) }

// setGlobal installs the provider + W3C propagators. Centralised so
// all three exporter arms of InitProvider stay symmetric.
func setGlobal(tp *sdktrace.TracerProvider) {
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

// parseHeaders splits a `K1=V1,K2=V2` env-shaped header string into the
// map[string]string that otlptracehttp.WithHeaders expects. Tolerant of
// spaces around `=`.
func parseHeaders(s string) map[string]string {
	out := make(map[string]string)
	cur, key := "", ""
	for _, r := range s {
		switch r {
		case ',':
			if key != "" {
				out[key] = cur
			}
			key, cur = "", ""
		case '=':
			key = cur
			cur = ""
		default:
			cur += string(r)
		}
	}
	if key != "" {
		out[key] = cur
	}
	return out
}
