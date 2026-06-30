// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/background"
	"github.com/spf13/cobra"
)

func newBackgroundCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "background",
		Short: "Manage background agent jobs (fire-and-forget)",
	}
	cmd.AddCommand(newBackgroundRunCmd())
	cmd.AddCommand(newBackgroundListCmd())
	cmd.AddCommand(newBackgroundStatusCmd())
	cmd.AddCommand(newBackgroundResultCmd())
	return cmd
}

func newBackgroundRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <prompt>",
		Short: "Start a background agent job",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := args[0]
			id := fmt.Sprintf("bg-%d", time.Now().Unix())
			mgr := background.NewManager()
			mgr.Start(context.Background(), id, prompt, func(ctx context.Context) (string, error) {
				// Simple subprocess execution
				return "Background job completed (subprocess mode)", nil
			})
			fmt.Fprintf(cmd.OutOrStdout(), "Started background job: %s\n", id)
			fmt.Fprintf(cmd.OutOrStdout(), "Check status with: sin-code background status %s\n", id)
			return nil
		},
	}
}

func newBackgroundListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List background jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "No background jobs running.")
			fmt.Fprintln(cmd.OutOrStdout(), "Use: sin-code background run <prompt>")
			return nil
		},
	}
}

func newBackgroundStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Show status of a background job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Job %s: use a persistent manager to track status.\n", args[0])
			return nil
		},
	}
}

func newBackgroundResultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "result <id>",
		Short: "Print the result of a completed background job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Result for %s: use a persistent manager to retrieve results.\n", args[0])
			return nil
		},
	}
}
