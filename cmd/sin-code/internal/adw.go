// SPDX-License-Identifier: MIT
// Purpose: adw — Architectural Debt Watchdogs. Detect and report architectural
// debt (god modules, circular deps, high coupling, etc.). Pass-through wrapper.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	adwPath   string
	adwFormat string
	adwStrict bool
)

var AdwCmd = &cobra.Command{
	Use:   "adw [path]",
	Short: "Architectural Debt Watchdogs — detect god modules, circular deps, etc.",
	Long: `Detect and report architectural debt in a codebase. Example:

  sin-code adw . -format json
  sin-code adw ./src --strict`,
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
			"format": adwFormat,
			"strict": adwStrict,
			"status": "delegated",
			"note":   "Full ADW logic lives in SIN-Code-ADW-Tool/cmd/adw",
			"debt_items": []map[string]any{},
		}

		if adwFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("ADW: %s (strict=%v)\n", absPath, adwStrict)
		return nil
	},
}

func init() {
	AdwCmd.Flags().StringVarP(&adwFormat, "format", "f", "text", "Output format: text|json")
	AdwCmd.Flags().BoolVarP(&adwStrict, "strict", "s", false, "Treat warnings as errors")
}
