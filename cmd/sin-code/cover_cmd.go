// SPDX-License-Identifier: MIT
// Purpose: `sin-code cover` — Coverage-Drohne entry point.
// Subcommands:
//
//	cover scan       # package coverage table
//	cover check      # CI gate with --min
//	cover gaps       # uncovered functions/blocks
//	cover generate   # AI test-generation request JSON
//
// Driver logic lives in cmd/sin-code/internal/coverdrohne.
// Docs: cover_cmd.doc.md
package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/coverdrohne"
)

// NewCoverCmd returns the `sin-code cover` subcommand.
func NewCoverCmd() *cobra.Command {
	// Wire the optional drain → autonomy queue callback so
	// `sin-code cover drain --enqueue` can enqueue test-gen goals.
	coverdrohne.EnqueueGoal = func(ctx context.Context, prompt, workspace string) error {
		q, err := autonomy.Open(autonomy.DefaultPath())
		if err != nil {
			return err
		}
		defer q.Close()
		id, err := q.Add(ctx, prompt, workspace, 0, 3)
		if err != nil {
			return err
		}
		fmt.Printf("goal %d enqueued\n", id)
		return nil
	}
	return coverdrohne.NewCommand()
}
