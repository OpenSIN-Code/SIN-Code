// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` slash command handlers for the interactive REPL.
// sin-debt: shrink, upgrade: when a second chat-run-related function is needed, merge
package main

import (
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentmode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
)

// handleSlashModel processes the /model command. Returns true if the
// line was a /model command (and was handled), false otherwise.
func handleSlashModel(line string, loop *agentloop.Loop, sinCfg internal.SinCodeConfig, agentCfg orchestrator.AgentConfig) bool {
	if line != "/model" && !strings.HasPrefix(line, "/model ") {
		return false
	}
	arg := strings.TrimSpace(strings.TrimPrefix(line, "/model"))
	if arg == "" {
		fmt.Fprintf(chatStdout, "Current model: %s\n", loop.GetModel())
		fmt.Fprintln(chatStdout, "Available models:")
		for _, m := range availableChatModels(sinCfg, agentCfg) {
			marker := "  "
			if m == loop.GetModel() {
				marker = "* "
			}
			fmt.Fprintf(chatStdout, "%s%s\n", marker, m)
		}
	} else {
		old := loop.GetModel()
		loop.SetModel(arg)
		fmt.Fprintf(chatStdout, "Switched model: %s → %s\n", old, loop.GetModel())
	}
	return true
}

// handleSlashMode processes the /mode command. Returns true if the
// line was a /mode command (and was handled), false otherwise.
func handleSlashMode(line string, loop *agentloop.Loop, mcpMgr *mcpclient.Manager, sinCfg internal.SinCodeConfig) bool {
	if line != "/mode" && !strings.HasPrefix(line, "/mode ") {
		return false
	}
	arg := strings.TrimSpace(strings.TrimPrefix(line, "/mode"))
	if arg == "" {
		fmt.Fprintf(chatStdout, "Current agent mode: %s\n", loop.AgentMode)
		fmt.Fprintln(chatStdout, "Available modes:")
		for _, m := range []string{"default", "architect", "debug", "code", "review"} {
			marker := "  "
			if m == loop.AgentMode {
				marker = "* "
			}
			fmt.Fprintf(chatStdout, "%s%s\n", marker, m)
		}
	} else {
		newMode, merr := agentmode.GetMode(arg)
		if merr != nil {
			fmt.Fprintf(chatStderr, "error: %v\n", merr)
			return true
		}
		old := loop.AgentMode
		loop.AgentMode = string(newMode)
		// Re-filter tools for the new mode (issue #485).
		loop.LocalSpec = newMode.FilterTools(combinedSpecs(mcpMgr))
		// Rebuild the system prompt with the new mode prefix.
		basePrompt := style.RenderSystemPrompt(sinCfg.LLMStyle)
		if modePrompt := newMode.SystemPrompt(); modePrompt != "" {
			loop.SystemPrompt = modePrompt + "\n\n" + basePrompt
		} else {
			loop.SystemPrompt = basePrompt
		}
		if newMode.IsRestricted() {
			fmt.Fprintf(chatStdout, "Switched agent mode: %s → %s (tools restricted)\n", old, newMode)
		} else {
			fmt.Fprintf(chatStdout, "Switched agent mode: %s → %s\n", old, newMode)
		}
	}
	return true
}
