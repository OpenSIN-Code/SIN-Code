// SPDX-License-Identifier: MIT
// Purpose: `sin-code permission` — inspect and smoke-test the reactive
// permission policy engine.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
)

// NewPermissionCmd builds the `permission` cobra subcommand group.
func NewPermissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permission",
		Short: "Reactive permission engine utilities",
		Long: `sin-code permission inspects and smoke-tests the reactive
permission policy that scans tool results after execution for secret
leakage, destructive confirmations, and network egress markers.

Subcommands:

  result-log   print sample detections using the built-in pattern set`,
	}
	cmd.AddCommand(newPermissionResultLogCmd())
	return cmd
}

func newPermissionResultLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "result-log",
		Short: "Print sample reactive-permission detections",
		Long: `result-log runs the built-in ResultPolicy scanner over a
fixed set of synthetic tool outputs and prints the action/reason for each.
No real credentials are used; the samples are deterministic demo strings.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			scanner := permission.NewResultPolicy()
			for _, s := range permission.SampleDetections() {
				action, reason := scanner.ScanResult(s.Tool, s.Result)
				fmt.Printf("tool=%-12s action=%-9s reason=%q sample=%q\n",
					s.Tool, action.String(), reason, truncateSample(s.Result, 64))
			}
			return nil
		},
	}
}

func truncateSample(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
