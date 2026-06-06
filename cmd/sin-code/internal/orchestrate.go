// SPDX-License-Identifier: MIT
// Purpose: orchestrate — task management. Thin wrapper around standalone
// SIN-Code-Orchestrate-Tool.
package internal

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	orchAction string
	orchTitle  string
	orchTags   string
	orchID     string
	orchFormat string
)

var OrchestrateCmd = &cobra.Command{
	Use:   "orchestrate",
	Short: "Manage tasks with dependencies, parallel execution, and rollback plans",
	Long: `Manage tasks with dependencies, parallel execution plans, blocker
detection, and rollback plans. Delegates to standalone SIN-Code-Orchestrate-Tool.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		binary, err := lookupStandalone("orchestrate")
		if err != nil {
			return err
		}
		cArgs := []string{"-action", orchAction, "-format", orchFormat}
		if orchTitle != "" {
			cArgs = append(cArgs, "-title", orchTitle)
		}
		if orchTags != "" {
			cArgs = append(cArgs, "-tags", orchTags)
		}
		if orchID != "" {
			cArgs = append(cArgs, "-id", orchID)
		}
		c := exec.Command(binary, cArgs...)
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		return c.Run()
	},
}

func init() {
	OrchestrateCmd.Flags().StringVarP(&orchAction, "action", "a", "list", "Action: add|remove|list|status|complete")
	OrchestrateCmd.Flags().StringVarP(&orchTitle, "title", "t", "", "Task title")
	OrchestrateCmd.Flags().StringVarP(&orchTags, "tags", "", "", "Comma-separated tags")
	OrchestrateCmd.Flags().StringVarP(&orchID, "id", "i", "", "Task ID")
	OrchestrateCmd.Flags().StringVarP(&orchFormat, "format", "f", "text", "Output format: text|json")
}
