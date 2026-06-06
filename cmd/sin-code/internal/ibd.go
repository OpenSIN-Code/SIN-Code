// SPDX-License-Identifier: MIT
// Purpose: ibd — Intent-Based Diffing. Compare two versions of code and
// determine if the changes match the stated intent. Pass-through wrapper.
package internal

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	ibdBefore   string
	ibdAfter    string
	ibdIntent   string
	ibdFormat   string
)

var IbdCmd = &cobra.Command{
	Use:   "ibd",
	Short: "Intent-Based Diffing — compare code changes against stated intent",
	Long: `Compare two versions of code and determine if the changes match the
stated intent. Example:

  sin-code ibd -before v1.0 -after HEAD -intent "add retry logic" -format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if ibdBefore == "" || ibdAfter == "" {
			return fmt.Errorf("--before and --after are required")
		}

		result := map[string]any{
			"before":  ibdBefore,
			"after":   ibdAfter,
			"intent":  ibdIntent,
			"format":  ibdFormat,
			"status":  "delegated",
			"note":    "Full IBD logic lives in SIN-Code-IBD-Tool/cmd/ibd",
			"matches_intent": nil,
		}

		if ibdFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("IBD: %s → %s (intent: %s)\n", ibdBefore, ibdAfter, ibdIntent)
		return nil
	},
}

func init() {
	IbdCmd.Flags().StringVarP(&ibdBefore, "before", "b", "", "Before version (ref, file, or commit)")
	IbdCmd.Flags().StringVarP(&ibdAfter, "after", "a", "", "After version (ref, file, or commit)")
	IbdCmd.Flags().StringVarP(&ibdIntent, "intent", "i", "", "Stated intent of the change")
	IbdCmd.Flags().StringVarP(&ibdFormat, "format", "f", "text", "Output format: text|json")
}
