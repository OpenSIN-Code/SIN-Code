// SPDX-License-Identifier: MIT
// Purpose: adw — Architectural Debt Watchdogs. Delegates to the Python `adw`
// module in SIN-Code-ADW-Tool (source of truth).
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	adwPath   string
	adwFormat string
	adwStrict bool
)

var AdwCmd = &cobra.Command{
	Use:   "adw",
	Short: "Architectural Debt Watchdogs — detect god modules, circular deps, etc.",
	Long: `Detect and report architectural debt in a codebase. Delegates to the Python
` + "`adw`" + ` module.

Examples:
  sin-code adw .
  sin-code adw ./src --strict
  sin-code adw . --format json`,
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

		pythonArgs := []string{"-m", "adw.cli", "scan", absPath}
		if adwStrict {
			pythonArgs = append(pythonArgs, "--strict")
		}

		c := exec.Command("python3", pythonArgs...)
		c.Stderr = os.Stderr
		out, err := c.Output()
		if err != nil {
			return fmt.Errorf("adw execution failed: %w", err)
		}

		if adwFormat == "json" {
			var pretty map[string]any
			if err := json.Unmarshal(out, &pretty); err == nil {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(pretty)
			}
		}
		fmt.Print(string(out))
		return nil
	},
}

func init() {
	AdwCmd.Flags().StringVarP(&adwFormat, "format", "f", "text", "Output format: text|json")
	AdwCmd.Flags().BoolVarP(&adwStrict, "strict", "s", false, "Treat warnings as errors")
}
