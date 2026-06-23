// SPDX-License-Identifier: MIT
// Purpose: shared helpers for root-package command tests.
// Docs: dox.doc.md
package main

import (
	"io"

	"github.com/spf13/cobra"
)

// setOutAll recursively sets Out and Err writers on cmd and all subcommands.
// Cobra does not propagate SetOut from a parent to its children, so tests that
// execute subcommands must wire the output stream on every node in the tree.
func setOutAll(cmd *cobra.Command, w io.Writer) {
	cmd.SetOut(w)
	cmd.SetErr(w)
	for _, c := range cmd.Commands() {
		setOutAll(c, w)
	}
}
