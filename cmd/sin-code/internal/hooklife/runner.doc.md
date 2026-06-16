# hooklife/runner.go — dispatch

The hot path of the system. Every `PreToolUse`/`PostToolUse` event
goes through `Dispatch`.

## Per-hook isolation

Each hook runs in its own goroutine with `context.WithTimeout` (default
10s). If the hook returns, panics, or times out, the runner still
proceeds to the next hook. A misbehaving hook can never break a
session.

## Block semantics

For `PreToolUse` only, a `Block` verdict short-circuits the dispatch
and is returned immediately. The caller (the agent loop) is
responsible for not running the tool.

For every other phase, `Block` is folded into a warning — the runner
cannot actually stop a lifecycle point that has already happened.

## Warning aggregation

Warnings from all hooks of a phase are joined with `[hook-id] message`
format and surfaced in a single `Decision.Message`. The agent loop
appends this to the next user-facing turn.

## Related files

- `event.go` — the `Decision` contract
- `registry.go` — provides `Hooks(phase)`
- `builtin.go` — default hook set
