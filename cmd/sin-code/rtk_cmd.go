// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when audit CLI is refactored
// Purpose: rtk — Bridge to rtk (Rust Token Killer) to cut LLM token usage 60-90%.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/rtk"
)

func NewRtkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rtk",
		Short: "Bridge to rtk (Rust Token Killer) to cut LLM token usage 60-90%",
		Long: `sin-code rtk bridges rtk (https://github.com/rtk-ai/rtk, never vendored),
a CLI proxy that filters command output (git, go test, cargo, …) to reduce
the tokens an LLM agent must read by 60-90%.

  sin-code rtk run -- git status        # filtered git status
  sin-code rtk run -- go test ./...      # filtered test output
  sin-code rtk doctor                    # check rtk is installed

When rtk is not installed, commands fail with a clear install hint; the
agent can always fall back to running the raw command directly.`,
	}
	cmd.AddCommand(newRtkRunCmd())
	cmd.AddCommand(newRtkDoctorCmd())
	return cmd
}

func newRtkRunCmd() *cobra.Command {
	var workdir string
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "run [-- command args...]",
		Short: "Run a command through rtk and print the filtered output",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			out, err := rtk.New().Run(ctx, workdir, args)
			if out != "" {
				fmt.Fprintln(cmd.OutOrStdout(), out)
			}
			return err
		},
	}
	c.Flags().StringVarP(&workdir, "dir", "C", "", "working directory for the command (default: cwd)")
	c.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "max time to wait for the command (0 = no timeout)")
	return c
}

func newRtkDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that rtk is installed and reachable",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			b := rtk.New()
			path, err := b.Find()
			if err != nil {
				fmt.Fprintln(os.Stderr, "rtk: NOT installed")
				return err
			}
			ver, verr := b.Version(ctx)
			fmt.Fprintf(cmd.OutOrStdout(), "rtk: OK\n  path:    %s\n", path)
			if verr == nil && ver != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  version: %s\n", ver)
			}
			return nil
		},
	}
}
