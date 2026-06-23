# hooklife/registry.go — in-memory store

A trivial `map[Phase][]Hook` plus a stable-order sort on read. Determinism
matters: the order in which hooks fire must be the same on every run, so
the agent's behavior is reproducible and the audit log is meaningful.

## Iteration order

Hooks fire in ID-ascending order. This means a hook with ID
`a-quality-gate` always runs before `z-cost-tracker`, regardless of the
order in which they were registered. Plan your hook IDs accordingly.

## `All()` vs `Hooks(Phase)`

- `Hooks(phase)` — the subset to fire for a given event
- `All()` — every distinct hook (used by `sin hooks list`)

## Related files

- `runner.go` — consumes `Hooks(phase)`
- `builtin.go` — registers the default set
