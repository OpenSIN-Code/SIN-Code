// SPDX-License-Identifier: MIT
// Purpose: convenience builder for Dispatcher — loads the standard
// asset layout and wires the three references the Dispatcher needs.
// Docs: dispatch.doc.md
package wiring

import (
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/assets"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dispatch"
)

// BuildDispatcher wires the asset registry to the prompt sink (main
// loop) and the subagent runner (orchestrator). Either sink may be
// nil during bring-up.
func BuildDispatcher(assetBase string, prompts dispatch.PromptSink, agents dispatch.SubagentRunner) (*dispatch.Dispatcher, error) {
	list, err := assets.LoadStandardLayout(assetBase)
	if err != nil {
		return nil, err
	}
	reg := assets.NewRegistry()
	reg.AddAll(list)
	return &dispatch.Dispatcher{Reg: reg, Prompts: prompts, Agents: agents}, nil
}
