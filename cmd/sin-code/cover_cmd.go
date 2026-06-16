// SPDX-License-Identifier: MIT
// Purpose: `sin-code cover` — Coverage-Drohne entry point.
// Subcommands:
//   cover scan       # package coverage table
//   cover check      # CI gate with --min
//   cover gaps       # uncovered functions/blocks
//   cover generate   # AI test-generation request JSON
//
// Driver logic lives in cmd/sin-code/internal/coverdrohne.
// Docs: cover_cmd.doc.md
package main

import (
	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/coverdrohne"
)

// NewCoverCmd returns the `sin-code cover` subcommand.
func NewCoverCmd() *cobra.Command {
	return coverdrohne.NewCommand()
}
