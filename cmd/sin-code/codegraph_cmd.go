// SPDX-License-Identifier: MIT
// Purpose: `sin-code codegraph` — operator + agent entry point for the
// CodeGraph bridge (issue #126), an external multi-language static-analysis
// engine exposed as an MCP tool. Follows the Bridged-External-Contract:
// CodeGraph is never vendored; we shell out to the user's installed binary.
// Docs: docs/codegraph-integration.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/codegraph"
)

// codegraphBridge is the minimal interface used by the codegraph subcommand so
// tests can inject a fake bridge without a real CodeGraph binary.
type codegraphBridge interface {
	Analyze(ctx context.Context, path string) (*codegraph.Graph, error)
	Find() (string, error)
	Version(ctx context.Context) (string, error)
}

// codegraphHookVars holds injectable dependencies for the codegraph
// subcommand. Coverage tests replace these fields to avoid requiring a real
// CodeGraph binary on PATH.
var codegraphHookVars = struct {
	newBridge func() codegraphBridge
}{
	newBridge: func() codegraphBridge { return codegraph.New() },
}

// NewCodeGraphCmd builds the `codegraph` cobra subcommand (analyze + doctor),
// matching the gh/vane/dox/rtk external-bridge pattern.
func NewCodeGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codegraph",
		Short: "Bridge to CodeGraph for multi-language code analysis",
		Long: `sin-code codegraph bridges CodeGraph (https://github.com/codegraph-ai/codegraph,
never vendored), a static-analysis engine that builds a symbol/edge graph
across many languages. It powers code-aware navigation for the agent.

  sin-code codegraph analyze .            # graph summary for the repo
  sin-code codegraph analyze --json .     # raw JSON graph for tooling/MCP
  sin-code codegraph doctor               # check CodeGraph is installed

When CodeGraph is not installed, commands fail with a clear install hint.`,
	}
	cmd.AddCommand(newCodeGraphAnalyzeCmd())
	cmd.AddCommand(newCodeGraphDoctorCmd())
	return cmd
}

// newCodeGraphAnalyzeCmd runs an analysis and prints either a human summary
// or the raw JSON graph (for MCP / downstream tooling).
func newCodeGraphAnalyzeCmd() *cobra.Command {
	var asJSON bool
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "analyze [path]",
		Short: "Analyze a path and print the code graph (summary or --json)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			g, err := codegraphHookVars.newBridge().Analyze(ctx, path)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(g)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "CodeGraph: %s\n  nodes: %d\n  edges: %d\n",
				g.Root, len(g.Nodes), len(g.Edges))
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit the raw JSON graph instead of a summary")
	c.Flags().DurationVar(&timeout, "timeout", 120*time.Second, "max time to wait for analysis (0 = no timeout)")
	return c
}

// newCodeGraphDoctorCmd verifies CodeGraph is installed and prints its version.
func newCodeGraphDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that CodeGraph is installed and reachable",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			b := codegraphHookVars.newBridge()
			path, err := b.Find()
			if err != nil {
				fmt.Fprintln(os.Stderr, "codegraph: NOT installed")
				return err
			}
			ver, verr := b.Version(ctx)
			fmt.Fprintf(cmd.OutOrStdout(), "codegraph: OK\n  path:    %s\n", path)
			if verr == nil && ver != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  version: %s\n", ver)
			}
			return nil
		},
	}
}
