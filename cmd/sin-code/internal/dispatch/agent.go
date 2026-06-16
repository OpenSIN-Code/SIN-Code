// SPDX-License-Identifier: MIT
// Purpose: resolve an agent asset into a runnable AgentInvocation
// (system prompt + model hint + tool whitelist + task). Supports
// explicit-name and best-match selection.
// Docs: agent.doc.md
package dispatch

import (
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/assets"
)

// AgentInvocation is everything needed to spawn a subagent from an
// asset.
type AgentInvocation struct {
	Name         string
	SystemPrompt string   // the agent body
	Model        string   // model hint from frontmatter (may be empty)
	AllowedTools []string // tool whitelist (empty = inherit)
	Task         string   // the concrete task handed to the subagent
}

// ResolveAgent builds an invocation from an agent asset + a task
// description.
func ResolveAgent(reg *assets.Registry, name, task string) (AgentInvocation, error) {
	a, ok := reg.Get(assets.KindAgent, name)
	if !ok {
		return AgentInvocation{}, fmt.Errorf("unknown agent: %s", name)
	}
	return AgentInvocation{
		Name:         a.Name,
		SystemPrompt: a.Body,
		Model:        a.Model,
		AllowedTools: a.Tools,
		Task:         task,
	}, nil
}

// SelectAndResolveAgent picks the best-matching agent for a context
// and resolves it.
func SelectAndResolveAgent(reg *assets.Registry, ctx assets.Context, task string) (AgentInvocation, error) {
	sel := assets.NewSelector(reg)
	matches := sel.SelectAgents(ctx, 1)
	if len(matches) == 0 {
		return AgentInvocation{}, fmt.Errorf("no agent matched domain=%q keywords=%v", ctx.Domain, ctx.Keywords)
	}
	return ResolveAgent(reg, matches[0].Asset.Name, task)
}
