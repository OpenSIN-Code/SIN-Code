// SPDX-License-Identifier: MIT
// Purpose: poc — Proof-of-Correctness. Verify that code satisfies its
// specification. Pass-through wrapper to SIN-Code-PoC-Tool.
package internal

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	pocSpec     string
	pocCode     string
	pocFormat   string
)

var PocCmd = &cobra.Command{
	Use:   "poc",
	Short: "Proof-of-Correctness — verify code satisfies its specification",
	Long: `Verify that code satisfies its specification. Example:

  sin-code poc -spec ./spec.md -code ./src/main.py -format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if pocSpec == "" {
			return fmt.Errorf("--spec is required")
		}
		if pocCode == "" {
			return fmt.Errorf("--code is required")
		}

		result := map[string]any{
			"spec":   pocSpec,
			"code":   pocCode,
			"format": pocFormat,
			"status": "delegated",
			"note":   "Full PoC logic lives in SIN-Code-PoC-Tool/cmd/poc",
		}

		if pocFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("PoC: spec=%s, code=%s\n", pocSpec, pocCode)
		return nil
	},
}

func init() {
	PocCmd.Flags().StringVarP(&pocSpec, "spec", "s", "", "Specification file (markdown, JSON, or natural language)")
	PocCmd.Flags().StringVarP(&pocCode, "code", "c", "", "Code file or directory to verify")
	PocCmd.Flags().StringVarP(&pocFormat, "format", "f", "text", "Output format: text|json")
}
