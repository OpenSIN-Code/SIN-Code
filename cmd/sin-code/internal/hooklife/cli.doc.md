# hooklife/cli.go — `sin hooks ...`

Two subcommands:

| Command | Purpose |
|---|---|
| `hooks list` | enumerate every registered hook with its phase set |
| `hooks test [phase]` | dispatch a synthetic event through the runner |

## Why a `test` subcommand

Operators need a way to validate that a hook set behaves as expected
without firing a real tool. The `test` command builds a synthetic
`Event` and runs the full runner pipeline against it.

## Related files

- `registry.go` — provides `All()`
- `runner.go` — the dispatch under test
