// SPDX-License-Identifier: MIT
// Purpose: `sin-code summary` — build a session summary from the ledger.
// Docs: summary_cmd.doc.md
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/summary"
	"github.com/spf13/cobra"
)

// NewSummaryCmd builds the `summary` cobra subcommand.
func NewSummaryCmd() *cobra.Command {
	var evidence bool
	cmd := &cobra.Command{
		Use:   "summary <session-id>",
		Short: "Build a deterministic summary from the session ledger",
		Long: `sin-code summary reads the semantic ledger for a session and
produces a markdown summary plus an optional one-line evidence string. The
summary is deterministic and does not call an LLM. It includes the
verification status, tools used, and a one-liner.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := ledger.DefaultPath()
			if env := os.Getenv("SIN_CODE_LEDGER"); env != "" {
				path = env
			}
			store, err := ledger.Open(path)
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			sum, err := summary.Build(ctx, store, args[0])
			if err != nil {
				return err
			}
			if evidence {
				fmt.Println(summary.Evidence(sum))
				return nil
			}
			fmt.Print(summary.Format(sum))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&evidence, "evidence", "e", false, "Print one-line evidence string instead of markdown")
	return cmd
}

// _ keeps os import used.
var _ = os.Getenv
