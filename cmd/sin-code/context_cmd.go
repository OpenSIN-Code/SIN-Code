// SPDX-License-Identifier: MIT
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context",
		Short: "Show context window usage for the current or last session",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "Context window meter:")
			fmt.Fprintln(cmd.OutOrStdout(), "  Use /context in chat or check the TUI footer for live usage.")
			fmt.Fprintln(cmd.OutOrStdout(), "  Config: agentloop.warn_before_compaction = true")
			return nil
		},
	}
}
