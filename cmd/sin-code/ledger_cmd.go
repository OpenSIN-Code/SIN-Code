// SPDX-License-Identifier: MIT
// Purpose: `sin-code ledger` — query the semantic session ledger.
// Docs: ledger_cmd.doc.md
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/spf13/cobra"
)

// NewLedgerCmd builds the `ledger` cobra subcommand.
func NewLedgerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ledger",
		Short: "Query the semantic session ledger",
		Long: `sin-code ledger reads the append-only session ledger that records
prompts, tool calls, verification results, and completions. Use it to audit
what the agent did in a session or to list recent sessions.`,
	}
	cmd.AddCommand(newLedgerListCmd())
	cmd.AddCommand(newLedgerShowCmd())
	return cmd
}

func ledgerStore() (*ledger.Store, error) {
	path := ledger.DefaultPath()
	if env := os.Getenv("SIN_CODE_LEDGER"); env != "" {
		path = env
	}
	return ledger.Open(path)
}

func newLedgerListCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "list",
		Short: "List recent sessions with ledger entries",
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := ledgerStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sessions, err := store.Sessions(ctx, limit)
			if err != nil {
				return err
			}
			if len(sessions) == 0 {
				fmt.Println("No sessions recorded.")
				return nil
			}
			for _, sid := range sessions {
				fmt.Println(sid)
			}
			return nil
		},
	}
	c.Flags().IntVarP(&limit, "limit", "n", 50, "Max sessions to show")
	return c
}

func newLedgerShowCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show ledger entries for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := ledgerStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			entries, err := store.List(ctx, args[0], limit)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No ledger entries for this session.")
				return nil
			}
			for _, e := range entries {
				fmt.Printf("%s  %-16s  %s\n", e.CreatedAt.Format(time.RFC3339), e.Type, e.Summary)
			}
			return nil
		},
	}
	c.Flags().IntVarP(&limit, "limit", "n", 100, "Max entries to show")
	return c
}

// _ keeps os import used.
var _ = strings.TrimSpace
