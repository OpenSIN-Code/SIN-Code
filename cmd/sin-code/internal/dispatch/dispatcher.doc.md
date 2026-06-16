# dispatch/dispatcher.go — the public surface

`Dispatcher` is what the rest of SIN-Code calls. It owns three
references:

| Field | Role |
|---|---|
| `Reg` | the loaded asset registry (commands + agents) |
| `Prompts` | the main agent loop's prompt injection point |
| `Agents` | the orchestrator's subagent spawner |

## Three methods

| Method | Use |
|---|---|
| `Dispatch(ctx, line)` | handle a raw user input line (slash command expansion) |
| `DelegateToAgent(ctx, ctx, task)` | pick the best agent for a task context and run it |
| `RunNamedAgent(ctx, name, task)` | run a specific agent by name |

`Dispatch` returns `(true, nil)` for plain text (not a slash
command) — wait, no: it returns `(false, nil)` for plain text and
`(true, err)` for unknown commands. The caller checks `handled` to
decide whether the line was a command.

## Why two `Run` methods

`DelegateToAgent` uses the selector; `RunNamedAgent` is a backdoor
for callers that already know the agent (e.g. the orchestrator's
planner picked one explicitly). Both ultimately call
`SubagentRunner.RunSubagent`.

## Related files

- `command.go` — slash command resolution
- `agent.go` — agent resolution
- `internal/wiring/dispatch.go` — convenience builder
