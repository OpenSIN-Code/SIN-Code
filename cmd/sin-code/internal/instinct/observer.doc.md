# instinct/observer.go — session buffer

The Observer is the **only** entry point the hook dispatcher uses.
Two methods:

## `Record(obs)`

Hot path. Appends one `Observation` to an in-memory slice. O(1) amortized
under the mutex. Never touches disk — that happens in `Flush`.

## `Flush(ctx)`

Cold path. Drains the buffer, runs the configured `Extractor`, and
folds each `Candidate` into the `Manager` (which may create, reinforce,
or contradict an instinct). Returns `(created, reinforced, err)` for
telemetry and CLI feedback.

## Thread safety

Single `sync.Mutex` guards the buffer. The buffer is replaced atomically
in `Flush` (slice header copy under lock), so a concurrent `Record` is
either fully before or fully after the drain — it never gets a half-
drained slice.

## Why not channel-based

Channels add back-pressure we don't want — a stalled `Flush` would
block `Record` and stall the whole tool dispatcher. The mutex +
slice pattern is back-pressure-free and matches the `agentloop`
model.
