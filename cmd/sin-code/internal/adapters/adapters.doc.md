# adapters/ — concrete adapters to real SIN-Code packages

Each file in this package owns one adapter. Together they implement
the abstract interfaces declared by:

- `internal/hooklife` (`Verifier`, `Ledger`, `TypeChecker`)
- `internal/instinct` (`Completer`, `MemorySink`)

## Wiring pattern

The wiring layer (`internal/wiring`) constructs concrete adapters
and passes them into the systems that need them. Production code in
`cmd/sin-code` reads config / env, builds the LLM client, opens the
memory + ledger + verify stores, and assembles the whole graph
once at startup.

## Fail-soft contract

Adapters never panic. The instinct observer and the hook runner
both already fail soft: a bad adapter call falls back to
heuristics, an unconfigured adapter no-ops. This means a partially
wired binary still works — it just runs the cheaper path.

## Related files

- `internal/wiring/` — the builder
- `internal/hooklife/builtin.go` — the consumer of `Verifier` / `Ledger` / `TypeChecker`
- `internal/instinct/manager.go` — the consumer of `MemorySink`
