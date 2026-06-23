# evalharness/runner.go — execute

The runner is intentionally thin: a loop, a scorer, a progress
callback. All the interesting work is in the `Subject` and `Scorer`
implementations.

## Per-case isolation

Each case runs in its own context with its own `Timeout` (if set).
A case that hangs or panics cannot block the rest of the run —
the runner simply records a failing result and moves on.

## Why `Progress` is a callback

CLI and TUI consumers want different progress rendering. A callback
keeps the runner UI-agnostic; pass nil to suppress.

## Related files

- `types.go` — `Runner.Execute` returns `Run`
- `scorer.go` — `r.Scorer.Score(c, out)` per case
- `store.go` — `SaveRun` persists the result
