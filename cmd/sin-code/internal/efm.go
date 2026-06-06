// SPDX-License-Identifier: MIT
// Purpose: efm — Ephemeral Full-Stack Mocking. Spin up disposable full-stack
// environments (backend, DB, frontend) for testing. Pass-through wrapper.
package internal

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	efmStack   string
	efmAction  string
	efmTTL     int
	efmFormat  string
)

var EfmCmd = &cobra.Command{
	Use:   "efm",
	Short: "Ephemeral Full-Stack Mocking — spin up disposable test environments",
	Long: `Spin up disposable full-stack environments (backend, DB, frontend) for
testing. Example:

  sin-code efm -action up -stack ./docker-compose.yml -ttl 3600 -format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if efmStack == "" && efmAction != "list" {
			return fmt.Errorf("--stack is required for actions other than 'list'")
		}

		result := map[string]any{
			"action":  efmAction,
			"stack":   efmStack,
			"ttl_s":   efmTTL,
			"format":  efmFormat,
			"status":  "delegated",
			"note":    "Full EFM logic lives in SIN-Code-EFM-Tool/cmd/efm",
		}

		if efmFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("EFM: action=%s, stack=%s, ttl=%ds\n", efmAction, efmStack, efmTTL)
		return nil
	},
}

func init() {
	EfmCmd.Flags().StringVarP(&efmAction, "action", "a", "list", "Action: up|down|list|status")
	EfmCmd.Flags().StringVarP(&efmStack, "stack", "s", "", "Stack definition (docker-compose.yml, k8s manifest, etc.)")
	EfmCmd.Flags().IntVarP(&efmTTL, "ttl", "t", 3600, "Time-to-live in seconds")
	EfmCmd.Flags().StringVarP(&efmFormat, "format", "f", "text", "Output format: text|json")
}
