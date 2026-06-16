# assets/registry.go — in-memory index

`Registry` is the lookup table the Selector and CLI work against.

## Keys

- `kind/name` — primary key. Overwriting an existing asset
  updates the pointer but does not change iteration order.
- `domain` — secondary, used by `ForDomain` (no index — O(n)).

## Iteration order

`List(kind)` returns assets sorted by `Name` ascending. Stable across
loads (the load order is preserved for assets added in batch via
`AddAll`).

## Related files

- `loader.go` — fills the registry from disk
- `selector.go` — ranks registry entries against a `Context`
- `cli.go` — the CLI consumer
