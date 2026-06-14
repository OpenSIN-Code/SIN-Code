// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for the trace package.
package trace

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
)

func TestParseExporterError(t *testing.T) {
	_, err := ParseExporter("unknown")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStdoutTraceOptions(t *testing.T) {
	if opts := StdoutTraceOptions(nil, false); len(opts) != 0 {
		t.Fatalf("expected 0 opts, got %d", len(opts))
	}
	if opts := StdoutTraceOptions(nil, true); len(opts) != 1 {
		t.Fatalf("expected 1 opts, got %d", len(opts))
	}
}

func TestOLTPHTTPOptions(t *testing.T) {
	opts := OLTPHTTPOptions("host:4318", true)
	if len(opts) != 2 {
		t.Fatalf("expected 2 opts, got %d", len(opts))
	}
}

func TestProviderConfigNormalize(t *testing.T) {
	c := &ProviderConfig{}
	c.normalize()
	if c.ServiceName != "sin-code" {
		t.Errorf("got %q", c.ServiceName)
	}
	if c.OTLPEndpoint != "localhost:4318" {
		t.Errorf("got %q", c.OTLPEndpoint)
	}
	if c.OTLPTimeout != 10*time.Second {
		t.Errorf("got %v", c.OTLPTimeout)
	}
	if c.SampleRate != 1.0 {
		t.Errorf("got %v", c.SampleRate)
	}
}

func TestInitProviderNilConfig(t *testing.T) {
	ctx := context.Background()
	tp, err := InitProvider(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tp == nil {
		t.Fatal("expected provider")
	}
	_ = Shutdown(ctx, tp)
}

func TestShutdownWithContext(t *testing.T) {
	ctx := context.Background()
	tp, _ := InitProvider(ctx, &ProviderConfig{Exporter: ExporterNoop})
	if err := Shutdown(ctx, tp); err != nil {
		t.Fatal(err)
	}
}

func TestParseHeaders(t *testing.T) {
	out := parseHeaders("K1=V1,K2=V2")
	if out["K1"] != "V1" || out["K2"] != "V2" {
		t.Fatalf("got %v", out)
	}
	out = parseHeaders("K1=V1=extra")
	if out["V1"] != "extra" {
		t.Fatalf("got %v", out)
	}
	out = parseHeaders("=V1")
	if len(out) != 0 {
		t.Fatalf("expected empty, got %v", out)
	}
	out = parseHeaders("")
	if len(out) != 0 {
		t.Fatalf("expected empty, got %v", out)
	}
}

func TestSpanNameFor(t *testing.T) {
	if got := spanNameFor(""); got != "sin.hook" {
		t.Errorf("got %q", got)
	}
	if got := spanNameFor("session.start"); got != "SinSessionStart" {
		t.Errorf("got %q", got)
	}
	if got := spanNameFor("a..b"); got != "SinAB" {
		t.Errorf("got %q", got)
	}
}

func TestSpanKindFor(t *testing.T) {
	if got := spanKindFor(hooks.ToolPre); got.String() != "client" {
		t.Errorf("got %v", got)
	}
	if got := spanKindFor(hooks.SessionStart); got.String() != "server" {
		t.Errorf("got %v", got)
	}
	if got := spanKindFor(hooks.TurnStart); got.String() != "internal" {
		t.Errorf("got %v", got)
	}
}

func TestApplyResult(t *testing.T) {
	// Tested via RecordHook in practice; we exercise branches
	// by calling applyResult with a noop span (valid but empty).
	tr, _ := InitProvider(context.Background(), &ProviderConfig{Exporter: ExporterNoop})
	defer Shutdown(context.Background(), tr)
	_, span := Tracer("test").Start(context.Background(), "x")
	applyResult(span, hooks.ToolError, map[string]any{"error": "boom"})
	span.End()
	_, span = Tracer("test").Start(context.Background(), "x")
	applyResult(span, hooks.VerifyFail, map[string]any{"reason": "bad"})
	span.End()
	_, span = Tracer("test").Start(context.Background(), "x")
	applyResult(span, hooks.VerifyFail, nil)
	span.End()
	_, span = Tracer("test").Start(context.Background(), "x")
	applyResult(span, hooks.VerifyPass, map[string]any{})
	span.End()
}

func TestTruncateJSON(t *testing.T) {
	if got := truncateJSON(nil, 10); got != "" {
		t.Errorf("got %q", got)
	}
	if got := truncateJSON(map[string]any{"k": "v"}, 100); got == "" {
		t.Errorf("unexpected empty")
	}
	if got := truncateJSON(map[string]any{"k": strings.Repeat("a", 3000)}, 2048); !strings.Contains(got, "truncated") {
		t.Errorf("expected truncation, got %q", got)
	}
	if got := truncateJSON(map[string]any{"k": make(chan int)}, 10); !strings.Contains(got, "marshal_err") {
		t.Errorf("expected marshal error, got %q", got)
	}
}

func TestEngineWrapperNilEngine(t *testing.T) {
	hl := NewHookListener()
	w := hl.WrapEngine(nil)
	res := w.Fire(context.Background(), hooks.Payload{Event: hooks.SessionStart})
	if res.PromptInjects != nil {
		t.Errorf("expected zero result, got %#v", res)
	}
}

func TestRegisterConditionalHook(t *testing.T) {
	RegisterConditionalHook(func(ctx context.Context, p hooks.Payload) {})
}

func TestStdoutTraceOptionsWithWriter(t *testing.T) {
	opts := StdoutTraceOptions(&bytes.Buffer{}, false)
	if len(opts) != 1 {
		t.Fatalf("expected 1 opts, got %d", len(opts))
	}
}

func TestInitProvider_StdoutExporterError(t *testing.T) {
	orig := stdouttraceNew
	stdouttraceNew = func(...stdouttrace.Option) (*stdouttrace.Exporter, error) { return nil, fmt.Errorf("stdout error") }
	defer func() { stdouttraceNew = orig }()
	_, err := InitProvider(context.Background(), &ProviderConfig{Exporter: ExporterStdout})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInitProvider_OTLP(t *testing.T) {
	orig := otlptracehttpNew
	otlptracehttpNew = func(ctx context.Context, opts ...otlptracehttp.Option) (*otlptrace.Exporter, error) {
		return nil, nil
	}
	defer func() { otlptracehttpNew = orig }()
	_, err := InitProvider(context.Background(), &ProviderConfig{Exporter: ExporterOTLP, OTLPInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInitProvider_OTLPError(t *testing.T) {
	orig := otlptracehttpNew
	otlptracehttpNew = func(ctx context.Context, opts ...otlptracehttp.Option) (*otlptrace.Exporter, error) {
		return nil, fmt.Errorf("otlp error")
	}
	defer func() { otlptracehttpNew = orig }()
	_, err := InitProvider(context.Background(), &ProviderConfig{Exporter: ExporterOTLP})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInitProvider_ResourceError(t *testing.T) {
	orig := resourceNew
	resourceNew = func(ctx context.Context, opts ...resource.Option) (*resource.Resource, error) {
		return nil, fmt.Errorf("resource error")
	}
	defer func() { resourceNew = orig }()
	_, err := InitProvider(context.Background(), &ProviderConfig{Exporter: ExporterNoop})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyResultFallback(t *testing.T) {
	tr, _ := InitProvider(context.Background(), &ProviderConfig{Exporter: ExporterNoop})
	defer Shutdown(context.Background(), tr)
	_, span := Tracer("test").Start(context.Background(), "x")
	applyResult(span, hooks.VerifyFail, map[string]any{})
	span.End()
}
