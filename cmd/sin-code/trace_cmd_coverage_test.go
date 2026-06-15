// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for trace_cmd.go.
// Docs: trace_cmd.go
package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sinctrace "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func resetTraceHooks(t *testing.T) {
	t.Helper()
	orig := traceHookVars
	t.Cleanup(func() { traceHookVars = orig })
}

func runTraceCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewTraceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestTraceDoctorSuccess(t *testing.T) {
	resetTraceHooks(t)
	var gotCfg *sinctrace.ProviderConfig
	traceHookVars.initProvider = func(ctx context.Context, cfg *sinctrace.ProviderConfig) (*sdktrace.TracerProvider, error) {
		gotCfg = cfg
		return nil, nil
	}
	traceHookVars.shutdown = func(ctx context.Context, tp *sdktrace.TracerProvider) error { return nil }
	traceHookVars.getenv = func(string) string { return "test-env" }
	out, err := runTraceCmd(t, "doctor", "--exporter", "stdout", "--endpoint", "x:1", "--insecure=false", "--timeout", "2s")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "trace ok") {
		t.Errorf("expected trace ok, got %q", out.String())
	}
	if gotCfg == nil {
		t.Fatal("expected provider config")
	}
	if gotCfg.ServiceName != "sin-code-trace-doctor" {
		t.Errorf("unexpected service name: %q", gotCfg.ServiceName)
	}
	if gotCfg.Environment != "test-env" {
		t.Errorf("unexpected environment: %q", gotCfg.Environment)
	}
	if gotCfg.OTLPEndpoint != "x:1" {
		t.Errorf("unexpected endpoint: %q", gotCfg.OTLPEndpoint)
	}
	if gotCfg.OTLPInsecure {
		t.Errorf("expected insecure=false")
	}
	if gotCfg.OTLPTimeout != 2*time.Second {
		t.Errorf("unexpected timeout: %v", gotCfg.OTLPTimeout)
	}
}

func TestTraceDoctorWithSampleSpan(t *testing.T) {
	resetTraceHooks(t)
	traceHookVars.initProvider = func(ctx context.Context, cfg *sinctrace.ProviderConfig) (*sdktrace.TracerProvider, error) {
		return nil, nil
	}
	traceHookVars.shutdown = func(ctx context.Context, tp *sdktrace.TracerProvider) error { return nil }
	var tracerName string
	traceHookVars.tracer = func(name string) oteltrace.Tracer {
		tracerName = name
		return oteltrace.NewNoopTracerProvider().Tracer(name)
	}
	out, err := runTraceCmd(t, "doctor", "--emit-sample-span")
	if err != nil {
		t.Fatal(err)
	}
	if tracerName != "sin-code-doctor" {
		t.Errorf("unexpected tracer name: %q", tracerName)
	}
	if !strings.Contains(out.String(), "trace ok") {
		t.Errorf("expected trace ok, got %q", out.String())
	}
	_ = out
}

func TestTraceDoctorInitError(t *testing.T) {
	resetTraceHooks(t)
	traceHookVars.initProvider = func(ctx context.Context, cfg *sinctrace.ProviderConfig) (*sdktrace.TracerProvider, error) {
		return nil, errors.New("init boom")
	}
	_, err := runTraceCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "trace doctor: init") {
		t.Fatalf("expected init error, got %v", err)
	}
}

func TestTraceDoctorShutdownError(t *testing.T) {
	resetTraceHooks(t)
	traceHookVars.initProvider = func(ctx context.Context, cfg *sinctrace.ProviderConfig) (*sdktrace.TracerProvider, error) {
		return nil, nil
	}
	traceHookVars.shutdown = func(ctx context.Context, tp *sdktrace.TracerProvider) error { return errors.New("shutdown boom") }
	out, err := runTraceCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "warn: trace shutdown error") {
		t.Errorf("expected shutdown warning, got %q", out.String())
	}
	if !strings.Contains(out.String(), "trace ok") {
		t.Errorf("expected trace ok, got %q", out.String())
	}
}
