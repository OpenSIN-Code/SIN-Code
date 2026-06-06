// SPDX-License-Identifier: MIT
// Purpose: grasp — deep code understanding. Thin wrapper around standalone
// SIN-Code-Grasp-Tool binary if installed.
package internal

import (
	"fmt"
	"os"
	"os/exec"
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
usage, and related context. Delegates to standalone SIN-Code-Grasp-Tool.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if _, err := os.Stat(absPath); err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
		binary, err := lookupStandalone("grasp")
		if err != nil {
			return err
		}
		cArgs := []string{"-file", absPath, "-format", graspFormat}
		c := exec.Command(binary, cArgs...)
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		return c.Run()
	},
}

func init() {
	GraspCmd.Flags().StringVarP(&graspFormat, "format", "f", "text", "Output format: text|json")
}
