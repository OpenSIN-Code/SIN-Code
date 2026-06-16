// SPDX-License-Identifier: MIT
// Purpose: `sin-code summary` — build a session summary from the ledger,
// augmented with token usage from internal/usage (issue #168). Docs:
// summary_cmd.doc.md
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/summary"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/usage"
)

// usageTokenSource adapts internal/usage.Store to summary.TokenSource so
// the summary package stays free of the usage import graph.
type usageTokenSource struct{ store *usage.Store }

func (u usageTokenSource) SessionTokens(ctx context.Context, sessionID string) (int, int, int, int, float64, error) {
	top, _, err := u.store.Aggregate(ctx, usage.Filter{SessionID: sessionID}, "")
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	if top == nil {
		return 0, 0, 0, 0, 0, nil
	}
	return top.InputTokens, top.OutputTokens, top.TotalTokens, top.EventCount, top.CostUSD, nil
}

// NewSummaryCmd builds the `summary` cobra subcommand.
func NewSummaryCmd() *cobra.Command {
	var evidence bool
	cmd := &cobra.Command{
		Use:   "summary <session-id>",
		Short: "Build a deterministic summary from the session ledger",
		Long: `sin-code summary reads the semantic ledger for a session and
produces a markdown summary plus an optional one-line evidence string. The
summary is deterministic and does not call an LLM. It includes the
verification status, tools used, and a one-liner. Since v3.17.0 (issue
#168) the summary also surfaces total tokens + estimated USD cost.`,
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

			// Issue #168: token usage is best-effort. A missing / unreachable
			// usage store leaves the summary tokenless, never an error.
			var src summary.TokenSource
			if uStore, uErr := usage.Open(usage.DefaultPath()); uErr == nil {
				defer func() { _ = uStore.Close() }()
				src = usageTokenSource{store: uStore}
			}

			sum, err := summary.BuildWithTokens(ctx, store, args[0], src)
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
