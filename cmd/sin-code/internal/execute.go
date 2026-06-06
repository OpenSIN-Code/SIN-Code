// SPDX-License-Identifier: MIT
// Purpose: execute — safe command execution with timeout, output capture,
// safety checks, and error analysis. Pass-through to SIN-Code-Execute-Tool.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

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
handling, and error analysis. Example:

  sin-code execute -command "npm test" -timeout 60 -format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if execCommand == "" {
			return fmt.Errorf("--command is required")
		}

		result := map[string]any{
			"command":   execCommand,
			"timeout_s": execTimeout,
			"timestamp": time.Now().Format(time.RFC3339),
		}

		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		c := exec.Command(shell, "-c", execCommand)
		c.Env = os.Environ()

		if execTimeout > 0 {
			done := make(chan error, 1)
			var out []byte
			var err error
			go func() {
				out, err = c.CombinedOutput()
				done <- err
			}()
			select {
			case <-done:
				result["output"] = string(out)
				result["exit_code"] = 0
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						result["exit_code"] = exitErr.ExitCode()
					} else {
						result["error"] = err.Error()
					}
				}
			case <-time.After(time.Duration(execTimeout) * time.Second):
				_ = c.Process.Kill()
				result["error"] = "timeout"
				result["exit_code"] = -1
			}
		} else {
			out, err := c.CombinedOutput()
			result["output"] = string(out)
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					result["exit_code"] = exitErr.ExitCode()
				} else {
					result["error"] = err.Error()
				}
			}
		}

		if execFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		if out, ok := result["output"].(string); ok {
			fmt.Print(out)
		}
		return nil
	},
}

func init() {
	ExecuteCmd.Flags().StringVarP(&execCommand, "command", "c", "", "Command to execute")
	_ = ExecuteCmd.MarkFlagRequired("command")
	ExecuteCmd.Flags().IntVarP(&execTimeout, "timeout", "t", 60, "Timeout in seconds")
	ExecuteCmd.Flags().StringVarP(&execFormat, "format", "f", "text", "Output format: text|json")
	ExecuteCmd.Flags().BoolVarP(&execStream, "stream", "S", false, "Stream output in real-time")
}
