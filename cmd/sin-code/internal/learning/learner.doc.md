# learning/learner.go — bridge to agentloop

The `Learner` is the single integration point between the new
subsystems (`instinct`, `hooklife`) and the existing
`agentloop.Loop`. It owns:

| Field | Role |
|---|---|
| `manager` | the Instinct system (load / save / score) |
| `obs` | the session-scoped Observer (buffer + flush) |
| `hooks` | the hooklife registry (built-in hooks) |
| `runner` | the hooklife runner (Pre/Post dispatch) |

## Lifecycle methods

| Method | When the loop calls it | What it does |
|---|---|---|
| `BeforeTurn` | before sending the system prompt to the LLM | prepends the active-instinct block |
| `BeforeTool` | before each tool execution | runs PreToolUse hooks; may veto |
| `AfterTool` | after each tool execution | runs PostToolUse hooks, feeds observer |
| `EndTurn` | turn finished | flushes the observer |
| `PreCompact` | before context compaction | flushes the observer, runs PreCompact hooks |

## Build-time options

`New(Options{...})` accepts whatever you have and silently
downgrades what you don't. `LLM: nil` → heuristic-only extractor.
`Memory: nil` → no mirror. `VerifyGate: nil` → no quality-gate
hook. This keeps the binary usable in tests and on small machines.

`Style string` (issue #167) is the verbosity mode fed into the
system prompt assembled by `BeforeTurn`. Allowed values:
`default`, `verbose`, `normal`, `terse`, `ultra`. Empty == default.
See `internal/style/style.doc.md` for the ruleset semantics; runtime
changes go through `Learner.SetStyle(level)` which is guarded by a
per-instance `sync.RWMutex` (mandate M7, race-free).

## Why no in-place mutation of `agentloop.Loop`

`agentloop.Loop` is constructed by the chat command and has its
own `Hooks *hooks.Engine`. The learner lives *beside* the loop and
is called by the chat command around the loop's main Run. This
keeps `agentloop` free of imports from the new packages — the
wiring is one-directional and reversible.

## Related files

- `internal/agentloop/loop.go` — the loop the learner lives beside
- `internal/instinct/` — the Instinct system
- `internal/hooklife/` — the hook registry + runner
- `internal/wiring/` — constructs the learner at startup
