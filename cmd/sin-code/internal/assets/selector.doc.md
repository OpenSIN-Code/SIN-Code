# assets/selector.go — score & pick

The payoff of the asset loader. Given a `Context{Domain, Keywords}`,
the selector ranks every loaded asset of a given kind and returns the
top N matches.

## Scoring

| Match | Points |
|---|---|
| `asset.Domain == ctx.Domain` | +10 |
| `ctx.Domain` substring in `asset.Name` | +6 |
| keyword substring in `asset.Name` | +4 each |
| keyword substring in `asset.Description` | +2 each |

The weights are deliberately transparent — any operator should be able
to look at a result and explain why the asset won.

## Why no embeddings

The selector runs inline in the orchestrator's planning step. Calling
an embedding model on every asset for every plan would multiply cost.
A heuristic pass is more than enough for the 68-150 asset range; if
that grows to thousands, swap `score` for a vector lookup.

## Related files

- `loader.go` — fills the `Registry` from disk
- `registry.go` — provides `List(kind)`
- `validate.go` — runs first, so we never select an invalid asset
