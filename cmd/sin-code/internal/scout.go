// SPDX-License-Identifier: MIT
// Purpose: scout — code search. Thin wrapper around standalone SIN-Code-Scout-Tool.
package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	scoutQuery   string
	scoutPath    string
	scoutType    string
	scoutFormat  string
	scoutMax     int
)

var ScoutCmd = &cobra.Command{
	Use:   "scout",
	Short: "Search code with regex, semantic, symbol, and usage search",
	Long: `Search code with regex, semantic, symbol, and usage search. Includes
dead-code detection. Delegates to standalone SIN-Code-Scout-Tool.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if scoutQuery == "" {
			return fmt.Errorf("--query is required")
		}
		absPath, err := filepath.Abs(scoutPath)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if _, err := os.Stat(absPath); err != nil {
			return fmt.Errorf("path not found: %w", err)
		}
		binary, err := lookupStandalone("scout")
		if err != nil {
			return err
		}
		cArgs := []string{
			"-query", scoutQuery,
			"-path", absPath,
			"-search_type", scoutType,
			"-format", scoutFormat,
			"-max_results", fmt.Sprintf("%d", scoutMax),
		}
		c := exec.Command(binary, cArgs...)
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		return c.Run()
	},
}

func init() {
	ScoutCmd.Flags().StringVarP(&scoutQuery, "query", "q", "", "Search query (regex or semantic)")
	_ = ScoutCmd.MarkFlagRequired("query")
	ScoutCmd.Flags().StringVarP(&scoutPath, "path", "p", ".", "Path to search")
	ScoutCmd.Flags().StringVarP(&scoutType, "search_type", "t", "regex", "Search type: regex|semantic|symbol|usage")
	ScoutCmd.Flags().StringVarP(&scoutFormat, "format", "f", "text", "Output format: text|json")
	ScoutCmd.Flags().IntVarP(&scoutMax, "max_results", "m", 50, "Max results")
}
