# trace/provider.go

## What

Bootstrap for the OpenTelemetry TracerProvider used by `sin-code eval`
and any other first-party code path that emits spans (issue #75).

`InitProvider(ctx, cfg)` returns a `*sdktrace.TracerProvider` and installs it
as the global provider; `Shutdown(ctx, tp)` flushes and tears it down; `Tracer(name)`
returns a named tracer from the global provider.

## Configuration surface

| Field | Default | Meaning |
|-------|---------|---------|
| `ServiceName` | `"sin-code"` | `service.name` resource attribute |
| `ServiceVersion` | `""` | `service.version` resource attribute |
| `Environment` | `$SIN_ENV` | `deployment.environment` attribute |
| `Exporter` | `noop` | `stdout` / `otlp` / `noop` |
| `OTLPEndpoint` | `localhost:4318` | OTLP/HTTP host:port |
| `OTLPInsecure` | `false` | plain HTTP vs HTTPS |
| `OTLPTimeout` | `10s` | OTLP HTTP client timeout |
| `SampleRate` | `1.0` | fraction for `TraceIDRatioBased` (0 → 1.0) |

`OTEL_EXPORTER_OTLP_HEADERS` (`K1=V1,K2=V2`) is parsed into the OTLP
exporter map automatically so caller code can set Langfuse /
Phoenix auth headers from the env without changing the CLI surface.

## Exports

- **stdout** — uses `stdouttrace.WithSyncer` (synchronous flush) so
  test runs can capture deterministic span output; writes to `os.Stderr`
  to keep stdout clean for `--json` consumers.
- **otlp** — `otlptracehttp` + `WithBatcher` (10k queue, 512 batch,
  5s window). ParentBased + TraceIDRatioBased sampler respects
  upstream parents while honoring `SampleRate` for root traces.
- **noop** — provider with `NeverSample()`; safe default when
  tracing is requested but no exporter is configured.

All three arms share the same resource attributes (ServiceName /
Version / Environment) so dashboards stay consistent regardless of
the exporter.

## Why this shape

- **M2 (pure-Go, CGO_ENABLED=0):** `go.opentelemetry.io/otel/sdk` and
  the two exporters are pure Go. Verified by
  `CGO_ENABLED=0 go build ./...`.
- **No vendor fork:** we pin to upstream v1.24.0 with semconv v1.24.0;
  no patched fork (mandate §2 §3 line 46: "NOT a place to vendor").
- **Idempotent global install:** `setGlobal(tp)` runs once per call;
  callers that only want a local provider can ignore the global and
  pass `tp` to a context-bound tracer.

## Tests

`provider_test.go` covers:
- stdout exporter flushes to stderr and produces a valid span,
- noop path is cheap and never panics,
- Shutdown is safe with nil,
- unknown Exporter kind is rejected.
