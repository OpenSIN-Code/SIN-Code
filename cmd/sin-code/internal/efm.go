// SPDX-License-Identifier: MIT
// Purpose: efm — Ephemeral Full-Stack Mocking. Delegates to the Python `efm`
// module in SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration (source of truth).
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

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
testing. Delegates to the Python ` + "`efm`" + ` module.

Examples:
  sin-code efm --action list
  sin-code efm --action up --stack docker-compose.yml --ttl 3600
  sin-code efm --action down
  sin-code efm --action status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if efmStack == "" && efmAction != "list" {
			return fmt.Errorf("--stack is required for actions other than 'list'")
		}

		pythonArgs := []string{"-m", "efm.cli"}
		switch efmAction {
		case "up":
			pythonArgs = append(pythonArgs, "up", efmStack, "--ttl", fmt.Sprintf("%d", efmTTL))
		case "down":
			pythonArgs = append(pythonArgs, "down")
		case "list":
			pythonArgs = append(pythonArgs, "status")
		case "status":
			pythonArgs = append(pythonArgs, "status")
		default:
			return fmt.Errorf("unknown action: %s (use up|down|list|status)", efmAction)
		}

		c := exec.Command("python3", pythonArgs...)
		c.Stderr = os.Stderr
		out, err := c.Output()
		if err != nil {
			return fmt.Errorf("efm execution failed: %w", err)
		}

		if efmFormat == "json" {
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
	EfmCmd.Flags().StringVarP(&efmAction, "action", "a", "list", "Action: up|down|list|status")
	EfmCmd.Flags().StringVarP(&efmStack, "stack", "s", "", "Stack definition (docker-compose.yml, k8s manifest, etc.)")
	EfmCmd.Flags().IntVarP(&efmTTL, "ttl", "t", 3600, "Time-to-live in seconds")
	EfmCmd.Flags().StringVarP(&efmFormat, "format", "f", "text", "Output format: text|json")
}
