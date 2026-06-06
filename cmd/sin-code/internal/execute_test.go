// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the execute subcommand.
package internal

import (
	"testing"
)

func TestExecuteCmd_RequiresCommand(t *testing.T) {
	execCommand = ""
	execFormat = "text"
	execTimeout = 1
	err := ExecuteCmd.RunE(ExecuteCmd, []string{})
	if err == nil {
		t.Error("expected error when --command is missing")
	}
}

func TestExecuteCmd_RunEcho(t *testing.T) {
	execCommand = "echo hello"
	execFormat = "text"
	execTimeout = 5
	if err := ExecuteCmd.RunE(ExecuteCmd, []string{}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
}

func TestExecuteCmd_RunFailingCommand(t *testing.T) {
	execCommand = "false"
	execFormat = "text"
	execTimeout = 5
	_ = ExecuteCmd.RunE(ExecuteCmd, []string{})
}
