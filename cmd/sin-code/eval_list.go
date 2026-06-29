// SPDX-License-Identifier: MIT
// Purpose: `sin-code eval list` command — list available datasets.
// Extracted from eval_cmd.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: consolidate when eval is refactored
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
)

// ── eval list ───────────────────────────────────────────────────────

func newEvalListCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available datasets in a directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := dataset.ListDatasets(dir)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				fmt.Printf("no datasets found under %s\n", dir)
				return nil
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{"dir": dir, "datasets": files})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "evals", "Directory to scan for *.json datasets")
	return cmd
}
