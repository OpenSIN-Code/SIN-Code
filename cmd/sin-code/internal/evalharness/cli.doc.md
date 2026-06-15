# evalharness/cli.go — `sin eval ...`

| Subcommand | Purpose |
|---|---|
| `eval run [set]` | execute the named set against the chosen subject |
| `eval list [set]` | list recorded runs (newest first) |
| `eval compare [baseline] [candidate]` | diff two runs; `--fail-on-regress` makes it a CI gate |

## Subject factory

The CLI does not know how to build a `Subject` — that is a wiring
concern. `wiring.EvalSubjectFactory` is the reference implementation
that uses the `verify` engine as the default subject.

## Why a factory, not a flag

A `Subject` is an interface with methods; cobra flags carry strings.
A factory closure lets the wiring layer inject a fully-built
subject graph (model client, verifier, etc.) without polluting the
CLI with type machinery.

## Related files

- `runner.go` — the actual execution
- `store.go` — load/save runs
- `regression.go` — the compare logic
