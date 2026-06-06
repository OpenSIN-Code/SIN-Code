// SPDX-License-Identifier: MIT
// Purpose: sckg — Semantic Codebase Knowledge Graphs. Delegates to the Python
// `sckg` module in SIN-Code-SCKG-Tool (source of truth).
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
	sckgPath   string
	sckgAction string
	sckgQuery  string
	sckgFormat string
)

var SckgCmd = &cobra.Command{
	Use:   "sckg",
	Short: "Semantic Codebase Knowledge Graphs — build & query code graph",
	Long: `Build and query a semantic graph of a codebase. Delegates to the Python
` + "`sckg`" + ` module.

Examples:
  sin-code sckg . --action build
  sin-code sckg . --action query --query "auth module dependencies"
  sin-code sckg . --action stats
  sin-code sckg . --action export --output graph.json`,
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

		pythonArgs := []string{"-m", "sckg.cli"}
		switch sckgAction {
		case "build":
			pythonArgs = append(pythonArgs, "build", absPath)
		case "query":
			if sckgQuery == "" {
				return fmt.Errorf("--query is required for action=query")
			}
			pythonArgs = append(pythonArgs, "query", sckgQuery)
		case "stats":
			pythonArgs = append(pythonArgs, "stats", absPath)
		case "export":
			pythonArgs = append(pythonArgs, "export", absPath)
		default:
			return fmt.Errorf("unknown action: %s (use build|query|stats|export)", sckgAction)
		}

		c := exec.Command("python3", pythonArgs...)
		c.Stderr = os.Stderr
		out, err := c.Output()
		if err != nil {
			return fmt.Errorf("sckg execution failed: %w", err)
		}

		if sckgFormat == "json" {
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
	SckgCmd.Flags().StringVarP(&sckgAction, "action", "a", "build", "Action: build|query|stats|export")
	SckgCmd.Flags().StringVarP(&sckgQuery, "query", "q", "", "Query (for action=query)")
	SckgCmd.Flags().StringVarP(&sckgFormat, "format", "f", "text", "Output format: text|json")
}
