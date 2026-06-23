# instinct/tuning.go — package-wide tuning

`ApplyConfig(Config)` threads a `Config` into the package-level math.
`currentTuning()` is read lock-free (via `atomic.Value`) on every
`Reinforce`/`Contradict`/status transition.

## Why an atomic.Value

The hot path is the `PostToolUse` hook. Adding a mutex would be a
visible cost in profiling. `atomic.Value` gives lock-free reads and
exactly-once initialization for our usage.

## Default behavior preserved

Defaults loaded in `init()` match the constants in `types.go` exactly.
Behavior is unchanged until a `Config` is applied.

## Related files

- `types.go` — math calls `currentTuning()`
- `manager.go` — `NewManager` calls `ApplyConfig(LoadConfig())`
- `config.go` — `LoadConfig` reads env overrides
