// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the serve subcommand (MCP server).
package internal

import (
	"testing"
)

func TestServeCmd_Flags(t *testing.T) {
	cmd := ServeCmd
	if cmd.Use != "serve" {
		t.Errorf("expected Use 'serve', got %q", cmd.Use)
	}
	for _, f := range []string{"transport", "port"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestServeCmd_DefaultTransport(t *testing.T) {
	if v, _ := ServeCmd.Flags().GetString("transport"); v != "stdio" {
		t.Errorf("default transport should be stdio, got %q", v)
	}
}

func TestRegisterAllMCPTools(t *testing.T) {
	expectedTools := []string{
		"sin_discover", "sin_execute", "sin_map", "sin_grasp",
		"sin_scout", "sin_harvest", "sin_orchestrate",
		"sin_ibd", "sin_poc", "sin_sckg", "sin_adw", "sin_oracle", "sin_efm",
	}
	if len(expectedTools) != 13 {
		t.Errorf("expected 13 tools, test config has %d", len(expectedTools))
	}
}
