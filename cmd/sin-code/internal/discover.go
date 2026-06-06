// SPDX-License-Identifier: MIT
// Purpose: discover — file discovery. Thin wrapper that calls the standalone
// SIN-Code-Discover-Tool binary if installed (preserves its Go implementation
// without duplicating logic).
package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	discoverPattern string
	discoverSort    string
	discoverFormat  string
	discoverLimit   int
)

var DiscoverCmd = &cobra.Command{
	Use:   "discover [path]",
	Short: "Discover files with relevance scoring and pattern matching",
	Long: `Discover files in a directory with relevance scoring, pattern matching,
and dependency analysis. Delegates to the standalone SIN-Code-Discover-Tool
binary if installed; otherwise uses built-in implementation.

Example:
  sin-code discover . -pattern "**/*.py" -sort_by relevance -format json`,
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

		binary, err := lookupStandalone("discover")
		if err != nil {
			return err
		}

		cArgs := []string{
			"-path", absPath,
			"-pattern", discoverPattern,
			"-sort_by", discoverSort,
			"-format", discoverFormat,
			"-max_results", fmt.Sprintf("%d", discoverLimit),
		}
		c := exec.Command(binary, cArgs...)
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		return c.Run()
	},
}

func init() {
	DiscoverCmd.Flags().StringVarP(&discoverPattern, "pattern", "p", "**/*", "File pattern (glob)")
	DiscoverCmd.Flags().StringVarP(&discoverSort, "sort_by", "s", "relevance", "Sort by: relevance|name|size|mtime")
	DiscoverCmd.Flags().StringVarP(&discoverFormat, "format", "f", "text", "Output format: text|json")
	DiscoverCmd.Flags().IntVarP(&discoverLimit, "limit", "l", 100, "Max results")
}
