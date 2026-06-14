// SPDX-License-Identifier: MIT
// Purpose: trace command - Configure and manage OpenTelemetry tracing
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
)

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Configure OpenTelemetry tracing for debugging and observability",
	Long: `Configure and manage OpenTelemetry tracing.
	
The trace command enables distributed tracing via OpenTelemetry, providing
visual debugging dashboards and integration with tools like Langfuse, Jaeger, 
and Arize Phoenix.`,
	RunE: runTrace,
}

var (
	traceExporter   string
	traceEndpoint   string
	traceInsecure   bool
	traceDebug      bool
)

func init() {
	traceCmd.Flags().StringVar(&traceExporter, "exporter", "stdout", 
		"Exporter type: stdout, otlp")
	traceCmd.Flags().StringVar(&traceEndpoint, "endpoint", "localhost:4318", 
		"OTLP endpoint for traces (e.g., localhost:4318 for Langfuse/Jaeger)")
	traceCmd.Flags().BoolVar(&traceInsecure, "insecure", true, 
		"Use insecure connection for OTLP (for dev/testing)")
	traceCmd.Flags().BoolVar(&traceDebug, "debug", false, 
		"Enable debug output")
	
	rootCmd.AddCommand(traceCmd)
}

func runTrace(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	fmt.Println("Initializing OpenTelemetry Tracer...")
	fmt.Printf("Exporter: %s\n", traceExporter)

	if traceExporter == "otlp" {
		fmt.Printf("Endpoint: %s\n", traceEndpoint)
		fmt.Printf("Insecure: %v\n", traceInsecure)
	}

	// Initialize provider
	config := trace.ProviderConfig{
		ServiceName:    "sin-code",
		ServiceVersion: "1.0.0",
		ExporterType:   traceExporter,
		OTLPEndpoint:   traceEndpoint,
		Insecure:       traceInsecure,
	}

	tp, err := trace.InitProvider(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to initialize tracer provider: %w", err)
	}

	defer func() {
		fmt.Println("\nShutting down tracer provider...")
		if err := trace.Shutdown(ctx, tp); err != nil {
			fmt.Fprintf(os.Stderr, "Error shutting down tracer: %v\n", err)
		}
	}()

	fmt.Println("\nTracer initialized successfully!")

	if traceExporter == "stdout" {
		fmt.Println("\nTraces will be printed to stdout.")
		fmt.Println("For integration with observability platforms:")
		fmt.Println("  - Langfuse: sin trace --exporter otlp --endpoint langfuse.com:443 --insecure=false")
		fmt.Println("  - Jaeger: sin trace --exporter otlp --endpoint localhost:4317")
		fmt.Println("  - Arize Phoenix: sin trace --exporter otlp --endpoint phoenix.localhost:4318")
	}

	fmt.Println("\nTrace system is running. Press Ctrl+C to exit.")
	fmt.Println("Agent lifecycle events are being captured automatically.")

	// Keep running until interrupted
	select {}
}
