# dispatch/agent.go — agent resolution

`ResolveAgent` is the bridge from an agent name to a runnable
`AgentInvocation`. The orchestrator spawns a subagent by handing
it the system prompt + tool whitelist + task and waiting for
output.

## Two entry points

| Function | Use when |
|---|---|
| `ResolveAgent` | the caller knows the agent name |
| `SelectAndResolveAgent` | the caller has only a task `Context` and wants the best fit |

`SelectAndResolveAgent` wraps `assets.Selector.SelectAgents(ctx, 1)`
and is a no-op when nothing matches.

## `AllowedTools`

The whitelist is passed *to* the subagent, not enforced here. The
subagent runner is responsible for restricting its tool calls.

## Related files

- `command.go` — the parallel flow for slash commands
- `dispatcher.go` — the consumer
