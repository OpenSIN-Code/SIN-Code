# SIN-Code Hooks

Hooks are deterministic automation points fired by the CLI itself at
lifecycle events. They are **never** LLM-decided — when the event fires,
the hook runs. Configured under the `hooks` key in
`~/.config/sin/sin-code.toml` (user) or `./.sin-code/config.toml` (project,
deep-merged, project wins). See AGENTS.md §7 for the full config schema.

## Hook types

| Type | Behavior |
|---|---|
| `command` | Runs via `sh -c`. Event JSON on stdin. Env: `SIN_HOOK_EVENT`, `SIN_SESSION_ID`. Exit 0 = continue, exit 2 = **BLOCK** (stdout fed back to the agent), other = warning, continue. Default timeout 60s (`timeout_seconds`). |
| `webhook` | HTTP POST of the event JSON. Fire-and-forget, 15s timeout. Ideal for n8n. |
| `prompt` | Injects `text` as an additional user message into the next agent turn. |

## Blocking semantics

Only these events honor a block (command exit 2):

- `tool.pre`
- `verify.pre`
- `permission.ask`
- `commit.pre`
- `push.pre`
- `compaction.pre`

First blocking hook wins; remaining hooks for that event are skipped.
On all other events exit 2 degrades to a warning.

## Events (24 total — 3× Claude Code's surface)

| Event | Fired when | Payload `name` | Blockable |
|---|---|---|---|
| `session.start` | New session begins | — | no |
| `session.resume` | `--resume` succeeds | — | no |
| `session.end` | Session closes | — | no |
| `turn.start` / `turn.end` | Each model turn | — | no |
| `tool.pre` | Before any tool call | tool name | **yes** |
| `tool.post` | After successful tool call | tool name | no |
| `tool.denied` | Permission engine denied | tool name | no |
| `tool.error` | Tool returned an error | tool name | no |
| `permission.ask` | `ask` policy triggers | tool name | **yes** (block = deny) |
| `verify.pre` | Before the verification gate | — | **yes** |
| `verify.pass` / `verify.fail` | Gate result | — | no |
| `agent.spawn` / `agent.complete` | Orchestrator fan-out | agent name | no |
| `critic.reject` | Critic rejects a result | agent name | no |
| `adversary.finding` | Adversary finds an issue | agent name | no |
| `governor.block` | Governor stops an agent | agent name | no |
| `memory.write` / `memory.compact` | Memory operations | namespace | no |
| `commit.pre` / `commit.post` | Around git commit | — | pre: **yes** |
| `push.pre` | Before git push | — | **yes** |
| `task.complete` / `task.abort` | Task finished/aborted | — | no |
| `compaction.pre` | Before context compaction | — | **yes** |

`event` and `matcher` support globs (`tool.*`, `sin_*`). Empty matcher = all.

## Recipes

Auto-format after every edit:

```json
{ "event": "tool.post", "matcher": "sin_edit", "type": "command",
  "command": "gofmt -w $(jq -r '.data.args.path // empty') 2>/dev/null || true" }
```

Block pushes containing secrets:

```json
{ "event": "push.pre", "type": "command",
  "command": "sin-code secrets scan --staged --quiet || { echo 'secret detected'; exit 2; }" }
```

Notify n8n on every verified completion:

```json
{ "event": "task.complete", "type": "webhook",
  "url": "https://n8n.example.com/webhook/sin-task-done" }
```

Inject briefing text on every session start:

```json
{ "event": "session.start", "type": "prompt",
  "text": "Org mandates: n8n CI only, conventional commits, never reduce test coverage." }
```
