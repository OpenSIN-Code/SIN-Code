// SPDX-License-Identifier: MIT
// Purpose: execute — safe command execution. Thin wrapper around the standalone
// SIN-Code-Execute-Tool binary if installed.
package internal

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	execCommand  string
	execTimeout  int
	execFormat   string
	execStream   bool
)

var ExecuteCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute shell commands safely with secret redaction and timeout",
	Long: `Execute shell commands with safety checks, secret redaction, timeout
handling, and error analysis. Delegates to standalone SIN-Code-Execute-Tool.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if execCommand == "" {
			return fmt.Errorf("--command is required")
		}
		binary, err := lookupStandalone("execute")
		if err != nil {
			return err
		}
		cArgs := []string{
			"-command", execCommand,
			"-timeout", fmt.Sprintf("%d", execTimeout),
			"-format", execFormat,
		}
		if execStream {
			cArgs = append(cArgs, "-stream")
		}
		c := exec.Command(binary, cArgs...)
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		return c.Run()
	},
}

func init() {
	ExecuteCmd.Flags().StringVarP(&execCommand, "command", "c", "", "Command to execute")
	_ = ExecuteCmd.MarkFlagRequired("command")
	ExecuteCmd.Flags().IntVarP(&execTimeout, "timeout", "t", 60, "Timeout in seconds")
	ExecuteCmd.Flags().StringVarP(&execFormat, "format", "f", "text", "Output format: text|json")
	ExecuteCmd.Flags().BoolVarP(&execStream, "stream", "S", false, "Stream output in real-time")
}
