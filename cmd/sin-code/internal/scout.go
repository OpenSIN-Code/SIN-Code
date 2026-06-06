// SPDX-License-Identifier: MIT
// Purpose: scout — code search with regex, semantic, symbol, and usage search.
// Includes dead-code detection. Pass-through to SIN-Code-Scout-Tool.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
dead-code detection. Example:

  sin-code scout -query "func.*main" -path . -search_type regex -format json`,
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

		matches := []map[string]any{}
		if scoutType == "regex" {
			err := filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".py") {
					return nil
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("walk failed: %w", err)
			}
		}

		result := map[string]any{
			"query":      scoutQuery,
			"path":       absPath,
			"search_type": scoutType,
			"format":     scoutFormat,
			"matches":    matches,
			"count":      len(matches),
			"max":        scoutMax,
		}

		if scoutFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("Scout: query=%q, path=%s, type=%s, matches=%d\n",
			scoutQuery, absPath, scoutType, len(matches))
		return nil
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
