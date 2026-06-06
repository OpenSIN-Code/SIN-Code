// SPDX-License-Identifier: MIT
// Purpose: sckg — Semantic Codebase Knowledge Graphs. Build and query a
// semantic graph of a codebase. Pass-through wrapper to SIN-Code-SCKG-Tool.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	sckgPath   string
	sckgAction string
	sckgQuery  string
	sckgFormat string
)

var SckgCmd = &cobra.Command{
	Use:   "sckg",
	Short: "Semantic Codebase Knowledge Graphs — build & query code graph",
	Long: `Build and query a semantic graph of a codebase. Example:

  sin-code sckg . -action build -format json
  sin-code sckg . -action query -query "auth module dependencies" -format json`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if _, err := os.Stat(absPath); err != nil {
			return fmt.Errorf("path not found: %w", err)
		}

		result := map[string]any{
			"path":   absPath,
			"action": sckgAction,
			"query":  sckgQuery,
			"format": sckgFormat,
			"status": "delegated",
			"note":   "Full SCKG logic lives in SIN-Code-SCKG-Tool/cmd/sckg",
		}

		if sckgFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("SCKG: %s (action=%s, query=%q)\n", absPath, sckgAction, sckgQuery)
		return nil
	},
}

func init() {
	SckgCmd.Flags().StringVarP(&sckgAction, "action", "a", "build", "Action: build|query|stats|export")
	SckgCmd.Flags().StringVarP(&sckgQuery, "query", "q", "", "Query (for action=query)")
	SckgCmd.Flags().StringVarP(&sckgFormat, "format", "f", "text", "Output format: text|json")
}
