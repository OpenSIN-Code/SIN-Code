// SPDX-License-Identifier: MIT
// Purpose: map — architecture analysis. Thin wrapper around standalone
// SIN-Code-Map-Tool binary if installed.
package internal

import (
	"fmt"
	"os"
	"os/exec"
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
and module-level analysis. Delegates to standalone SIN-Code-Map-Tool.`,
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
		binary, err := lookupStandalone("map")
		if err != nil {
			return err
		}
		cArgs := []string{"-path", absPath, "-action", mapAction, "-format", mapFormat}
		c := exec.Command(binary, cArgs...)
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		return c.Run()
	},
}

func init() {
	MapCmd.Flags().StringVarP(&mapPath, "path", "p", "", "Path to map (default: cwd)")
	MapCmd.Flags().StringVarP(&mapAction, "action", "a", "map", "Action: map|summary|graph|hotpaths")
	MapCmd.Flags().StringVarP(&mapFormat, "format", "f", "text", "Output format: text|json")
}
