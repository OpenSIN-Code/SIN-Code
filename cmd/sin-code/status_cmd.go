// SPDX-License-Identifier: MIT
// Purpose: `sin-code status` — readiness snapshot / status report with
// markdown and JSON output. Issue #326.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/status"
)

// NewStatusCmd builds the `status` cobra subcommand.
func NewStatusCmd() *cobra.Command {
	var (
		workspace string
		outPath   string
		markdown  bool
		jsonOut   bool
		sinceStr  string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print a readiness snapshot of the local SIN-Code state",
		Long: `sin-code status reads the goal queue, todo store, session store,
ledger, sin-debt markers, and skill status and produces a deterministic
readiness report. Missing or empty stores are reported as "No data yet"
instead of failing the command.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if markdown && jsonOut {
				return fmt.Errorf("--markdown and --json are mutually exclusive")
			}
			if workspace == "" {
				var err error
				workspace, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve workspace: %w", err)
				}
			}
			workspace, err := filepath.Abs(workspace)
			if err != nil {
				return fmt.Errorf("resolve workspace: %w", err)
			}

			cfg := status.Config{
				Workspace: workspace,
				Markdown:  markdown,
				JSON:      jsonOut,
				OutPath:   outPath,
			}
			if sinceStr != "" {
				since, err := time.Parse(time.RFC3339, sinceStr)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
				cfg.Since = since
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			rep, err := status.Collect(ctx, cfg)
			if err != nil {
				return err
			}

			var output []byte
			if jsonOut {
				output, err = status.RenderJSON(rep)
				if err != nil {
					return err
				}
			} else {
				output = []byte(status.RenderMarkdown(rep))
			}

			if outPath != "" {
				if err := os.WriteFile(outPath, output, 0o644); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Status report written to %s\n", outPath)
				return nil
			}
			_, _ = cmd.OutOrStdout().Write(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace directory to scan (default: current working directory)")
	cmd.Flags().StringVar(&outPath, "out", "", "Write report to this file instead of stdout")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "Render markdown output (default)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Render JSON output")
	cmd.Flags().StringVar(&sinceStr, "since", "", "Ledger time filter in RFC3339 (e.g. 2026-01-01T00:00:00Z)")
	return cmd
}
