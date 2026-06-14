// SPDX-License-Identifier: MIT
// Purpose: `sin-code trace` — configure + sanity-check OpenTelemetry
// exporter wiring (issue #75). No first-party code emits spans
// without an explicit InitProvider call; this command is a thin
// configure-only surface so operators can verify the env wiring in
// isolation before an `eval run --trace` step.
//
// Subcommands:
//
//	trace doctor --exporter stdout|otlp [--endpoint host:port]
//
// Docs: trace_cmd.doc.md
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	sinctrace "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
)

func NewTraceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Configure + verify OpenTelemetry tracer setup",
		Long: `sin-code trace is the configure-only companion to 'sin-code eval --trace'.
Use ` + "`sin-code trace doctor`" + ` to confirm your OTEL_ENDPOINT,
OTEL_EXPORTER_OTLP_HEADERS and chosen exporter resolve without
having to run a full eval suite.`,
	}
	cmd.AddCommand(newTraceDoctorCmd())
	return cmd
}

func newTraceDoctorCmd() *cobra.Command {
	var (
		exporter string
		endpoint string
		insecure bool
		timeout  time.Duration
		writeSD  bool // synthesize one span so the operator sees real output
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verify that the chosen OTel exporter is wired correctly",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			tp, err := sinctrace.InitProvider(ctx, &sinctrace.ProviderConfig{
				ServiceName:  "sin-code-trace-doctor",
				Environment:  os.Getenv("SIN_ENV"),
				Exporter:     mustParseExporter(exporter),
				OTLPEndpoint: endpoint,
				OTLPInsecure: insecure,
				OTLPTimeout:  timeout,
			})
			if err != nil {
				return fmt.Errorf("trace doctor: init: %w", err)
			}
			if writeSD {
				tr := sinctrace.Tracer("sin-code-doctor")
				_, span := tr.Start(ctx, "doctor.synth")
				span.End()
			}
			shCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := sinctrace.Shutdown(shCtx, tp); err != nil {
				fmt.Fprintf(os.Stderr, "warn: trace shutdown error: %v\n", err)
			}
			fmt.Printf("trace ok: exporter=%s endpoint=%s insecure=%v\n", exporter, endpoint, insecure)
			return nil
		},
	}
	cmd.Flags().StringVar(&exporter, "exporter", "stdout", "stdout|otlp|noop")
	cmd.Flags().StringVar(&endpoint, "endpoint", "localhost:4318", "OTLP endpoint for --exporter=otlp")
	cmd.Flags().BoolVar(&insecure, "insecure", true, "OTLP/HTTP plain text (default true to match eval --trace)")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "OTLP HTTP timeout")
	cmd.Flags().BoolVar(&writeSD, "emit-sample-span", false, "Synthesize one span so operators see real exporter output")
	return cmd
}
