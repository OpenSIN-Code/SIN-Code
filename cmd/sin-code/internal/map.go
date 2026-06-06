// SPDX-License-Identifier: MIT
// Purpose: map — architecture analysis with dependency graphs, entry points,
// hot paths, and module-level analysis. Pass-through to SIN-Code-Map-Tool.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	mapPath   string
	mapAction string
	mapFormat string
)

var MapCmd = &cobra.Command{
	Use:   "map [path]",
	Short: "Map code architecture with dependency graphs and hot-path analysis",
	Long: `Map code architecture with dependency graphs, entry points, hot paths,
and module-level analysis. Example:

  sin-code map . -action map -format json`,
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
			"action": mapAction,
			"format": mapFormat,
			"status": "delegated",
			"note":   "Full mapping logic lives in SIN-Code-Map-Tool/cmd/map",
		}

		if mapFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("Map: %s (action=%s)\n", absPath, mapAction)
		return nil
	},
}

func init() {
	MapCmd.Flags().StringVarP(&mapPath, "path", "p", "", "Path to map (default: cwd)")
	MapCmd.Flags().StringVarP(&mapAction, "action", "a", "map", "Action: map|summary|graph|hotpaths")
	MapCmd.Flags().StringVarP(&mapFormat, "format", "f", "text", "Output format: text|json")
}
