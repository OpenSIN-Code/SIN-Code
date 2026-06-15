// SPDX-License-Identifier: MIT
// Purpose: the Dispatcher routes slash-commands to the prompt sink
// and agent requests to the subagent runner. It is the only public
// surface the rest of SIN-Code uses.
// Docs: dispatcher.doc.md
package dispatch

import (
	"context"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/assets"
)

// SubagentRunner is implemented by SIN-Code's orchestrator: it
// spawns a subagent with a system prompt + task and returns its
// final output.
type SubagentRunner interface {
	RunSubagent(ctx context.Context, inv AgentInvocation) (string, error)
}

// PromptSink is implemented by the main agent loop: it injects a
// resolved command prompt as if the user had typed the expanded
// instruction.
type PromptSink interface {
	SubmitPrompt(ctx context.Context, prompt string, allowedTools []string) error
}

// Dispatcher routes slash-commands to the prompt sink and agent
// requests to the subagent runner, using the asset registry as the
// source of definitions.
type Dispatcher struct {
	Reg     *assets.Registry
	Prompts PromptSink
	Agents  SubagentRunner
}

// Dispatch handles a raw user line. If it's a slash command, it
// resolves and submits the expanded prompt. Returns (handled, error).
func (d *Dispatcher) Dispatch(ctx context.Context, line string) (handled bool, err error) {
	name, rawArgs, isSlash := ParseSlash(line)
	if !isSlash {
		return false, nil
	}
	rc, err := ResolveCommand(d.Reg, name, rawArgs)
	if err != nil {
		return true, err
	}
	if d.Prompts == nil {
		return true, fmt.Errorf("no prompt sink configured")
	}
	return true, d.Prompts.SubmitPrompt(ctx, rc.Prompt, rc.AllowedTools)
}

// DelegateToAgent selects the best agent for a context and runs it
// as a subagent.
func (d *Dispatcher) DelegateToAgent(ctx context.Context, sel assets.Context, task string) (string, error) {
	if d.Agents == nil {
		return "", fmt.Errorf("no subagent runner configured")
	}
	inv, err := SelectAndResolveAgent(d.Reg, sel, task)
	if err != nil {
		return "", err
	}
	return d.Agents.RunSubagent(ctx, inv)
}

// RunNamedAgent runs a specific named agent (bypassing selection).
func (d *Dispatcher) RunNamedAgent(ctx context.Context, name, task string) (string, error) {
	if d.Agents == nil {
		return "", fmt.Errorf("no subagent runner configured")
	}
	inv, err := ResolveAgent(d.Reg, name, task)
	if err != nil {
		return "", err
	}
	return d.Agents.RunSubagent(ctx, inv)
}
