// SPDX-License-Identifier: MIT
// Purpose: export helpers re-exported from the OTel SDK so callers
// (eval_cmd.go, trace_cmd.go) can switch on ExporterKind without
// importing the upstream SDK packages directly. Keeps the
// "NOT a place to vendor" mandate honest by isolating the SDK
// surface to a single package.
//
// Docs: exporter.doc.md
package trace

import (
	"io"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
)

// ParseExporter maps the CLI string ("stdout"|"otlp"|"noop"|"") to an
// ExporterKind. Empty input defaults to ExporterNoop without error.
func ParseExporter(s string) (ExporterKind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "noop":
		return ExporterNoop, nil
	case "stdout":
		return ExporterStdout, nil
	case "otlp", "otlphttp", "http":
		return ExporterOTLP, nil
	default:
		return ExporterNoop, &UnknownExporterError{Value: s}
	}
}

// UnknownExporterError is returned by ParseExporter for unrecognized
// exporter strings. Named so callers can errors.As() without
// hard-coding the message text.
type UnknownExporterError struct{ Value string }

func (e *UnknownExporterError) Error() string {
	return "trace: unknown exporter kind " + e.Value + " (want stdout|otlp|noop)"
}

// StdoutTraceOptions returns the upstream stdouttrace.Option slice
// implied by the user-supplied writer + pretty-print flag. Rebuilt on
// each call so the caller can decide between stderr-default, file, or
// in-memory buffer (tests use the buffer).
func StdoutTraceOptions(w io.Writer, pretty bool) []stdouttrace.Option {
	opts := []stdouttrace.Option{}
	if w != nil {
		opts = append(opts, stdouttrace.WithWriter(w))
	}
	if pretty {
		opts = append(opts, stdouttrace.WithPrettyPrint())
	}
	return opts
}

// OLTPHTTPOptions returns the upstream otlptracehttp.Option slice
// implied by endpoint + insecure flag. Caller passes the resolved
// timeout / headers via WithTimeout / WithHeaders on the result.
func OLTPHTTPOptions(endpoint string, insecure bool) []otlptracehttp.Option {
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
	if insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	return opts
}
