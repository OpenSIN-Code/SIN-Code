// SPDX-License-Identifier: MIT
// Purpose: grasp — deep code understanding for individual files. Structure,
// dependencies, usage, and related context. Pass-through to SIN-Code-Grasp-Tool.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	graspPath   string
	graspFormat string
)

var GraspCmd = &cobra.Command{
	Use:   "grasp [path]",
	Short: "Deep code understanding for a single file",
	Long: `Deep code understanding for individual files — structure, dependencies,
usage, and related context. Example:

  sin-code grasp ./src/main.py -format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if _, err := os.Stat(absPath); err != nil {
			return fmt.Errorf("file not found: %w", err)
		}

		result := map[string]any{
			"path":   absPath,
			"format": graspFormat,
			"status": "delegated",
			"note":   "Full grasp logic lives in SIN-Code-Grasp-Tool/cmd/grasp",
		}

		if graspFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("Grasp: %s\n", absPath)
		return nil
	},
}

func init() {
	GraspCmd.Flags().StringVarP(&graspFormat, "format", "f", "text", "Output format: text|json")
}
