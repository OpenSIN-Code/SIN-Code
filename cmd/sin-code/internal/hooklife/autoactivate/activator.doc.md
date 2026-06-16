# autoactivate/activator.go — per-session rule injection

Mirrors JuliusBrussee/caveman's "always-on" ruleset inside Go's
`hooklife` subsystem. Two Phase hooks register against any
`*hooklife.Registry`:

| Hook ID | Phase | Purpose |
|---|---|---|
| `autoactivate-session-start` | SessionStart | Initialise session state, emit rule body if AutoOn |
| `autoactivate-user-prompt` | UserPrompt | Trigger-phrase match + per-turn re-injection |

## Public API

| Method | Effect |
|---|---|
| `NewActivator(defaults)` | Construct; optional built-in defaults are exposed via the reserved `__builtins__` pseudo-session for introspection. |
| `OnSessionStart(sid, opts)` | Idempotent init. Replaces prior state for the same id. |
| `OnUserPrompt(sid, prompt)` | Trigger-phrase scan; returns `(RuleSet, ok=true)` when rules should be prepended this turn. |
| `Activate(sid, rule)` | Manually turn on a rule. Lazy-creates the session and sets `AutoOn=true` implicitly. |
| `Deactivate(sid, name)` | Remove a rule by name. |
| `SetAutoOn(sid, autoOn)` | Toggle the master switch for a session. |
| `Snapshot(sid)` | Defensive-copy state introspection. |
| `EndSession(sid)` | Drop session state. Privacy-first. |
| `Count()` | Active-session count (excluding built-ins pseudo-session). |
| `Register(reg)` | Wire both Phase hooks onto the given registry. |

## Concurrency

All mutating methods take `a.mu` write-lock; readers (`Snapshot`,
`Count`) take the read-lock. Tested under `go test -race -count=1 ./...`
with 16 parallel goroutines, 64 ops each. Mandate M7 invariant.

## Privacy

`EndSession` drops per-session state. The `__builtins__` pseudo-session
holds the small built-in defaults list and lives for the process
lifetime — it is never visible to user-facing APIs (`Snapshot("")`,
`Activate("__builtins__", ...)` are silent no-ops).

## Decision verb

Both hooks return `Warn` (not `Allow`) when emitting a rule body so the
runner surfaces the rendered text via its aggregation (the runner drops
`Allow`-verdict `Message` fields). The chat command consumes `d.Message`
and either prints it to stderr (today) or, once issue #176 phase 2
ships, prepends it to the model's system prompt.
