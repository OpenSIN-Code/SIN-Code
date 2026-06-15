# instinct/manager.go — public API

The single entry point. Talks to `Store` (disk) and optionally to a
`MemorySink` (mirror into `internal/memory`).

## Responsibilities

- Project detection (delegated to `DetectProject`)
- `Observe` — create or reinforce an instinct
- `Contradict` — apply a counter-signal
- `Active` — return what should influence the agent right now
- `EvolveAll` / `Prune` — periodic maintenance

## Threading

All public methods are safe to call from a single goroutine. The
Observer handles the buffering; the Manager handles the write-through.
The hook dispatcher (in `internal/learning/`) is the only caller in
production.

## Memory mirror

If a `MemorySink` is wired, every `Observe` mirrors the instinct as a
`Memory{Insight, Tags=[domain]}` in `internal/memory`. This is
opt-in: pass `nil` to disable. The adapter in
`internal/adapters/memory.go` is the reference implementation.
