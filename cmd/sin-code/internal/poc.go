// SPDX-License-Identifier: MIT
// Purpose: poc — Proof-of-Correctness. Delegates to the Python `poc` module
// in SIN-Code-PoC-Tool (source of truth).
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	pocSpec   string
	pocCode   string
	pocFormat string
)

var PocCmd = &cobra.Command{
	Use:   "poc",
	Short: "Proof-of-Correctness — verify code satisfies its specification",
	Long: `Verify that code satisfies its specification. Delegates to the Python
` + "`poc`" + ` module.

Examples:
  sin-code poc --spec spec.md --code src/main.py
  sin-code poc --spec spec.json --code src/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		target := pocCode
		if target == "" {
			target = pocSpec
		}
		if target == "" {
			return fmt.Errorf("--code (or --spec for back-compat) is required")
		}

		pythonArgs := []string{"-m", "poc.cli", "verify", target}
		if pocSpec != "" && pocSpec != target {
			pythonArgs = append(pythonArgs, "--config", pocSpec)
		}

		c := exec.Command("python3", pythonArgs...)
		c.Stderr = os.Stderr
		out, err := c.Output()
		if err != nil {
			return fmt.Errorf("poc execution failed: %w", err)
		}

		if pocFormat == "json" {
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
	PocCmd.Flags().StringVarP(&pocSpec, "spec", "s", "", "Specification file (passed as --config)")
	PocCmd.Flags().StringVarP(&pocCode, "code", "c", "", "Code file or directory to verify (positional TARGET)")
	PocCmd.Flags().StringVarP(&pocFormat, "format", "f", "text", "Output format: text|json")
}
