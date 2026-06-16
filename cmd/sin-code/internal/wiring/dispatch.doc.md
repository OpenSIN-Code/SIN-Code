# wiring/dispatch.go — Dispatcher builder

`BuildDispatcher(assetBase, prompts, agents)` is a one-call
constructor:

1. Loads the standard asset layout (`agents/`, `commands/`,
   `.agents/skills/`, `skills/`) from `assetBase`.
2. Fills a fresh `Registry`.
3. Wires it into a `Dispatcher` with the supplied sinks.

`prompts` and `agents` are interfaces, so the call site can pass
either real or test implementations. The Dispatcher is safe to
construct with one or both nil — calls that need the missing sink
return a clear error.

## Related files

- `dispatch/dispatcher.go` — the consumer
- `assets/loader.go` — `LoadStandardLayout`
- `assets/registry.go` — the in-memory index
