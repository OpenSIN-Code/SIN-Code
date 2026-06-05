# Purpose: Doc companion for symbol_resolve.py — what it does and when to use it.
# Docs: symbol_resolve.py

# `symbol_resolve.py` — Unified code archaeology

## What it does

`SymbolResolver` fans out to **4 gitnexus primitives + 1 sin-context-bridge
query** in a single call, returning a unified graph view for any symbol
(function, class, module).

## Sub-sources

| Slice            | Source                       | CLI                                    |
|------------------|------------------------------|----------------------------------------|
| `callers`        | gitnexus `context`           | `gitnexus context <name> --json`       |
| `callees`        | gitnexus `context`           | (same call as callers)                 |
| `blast_radius`   | gitnexus `impact`            | `gitnexus impact '{"target": ...}'`    |
| `recent_changes` | gitnexus `detect-changes`    | `gitnexus detect-changes --json`       |
| `cross_source`   | sin-context-bridge `query`   | `sin-context-bridge query <name> ...`  |

The `sources_queried` list in the response makes it transparent which CLIs
actually responded.

## When to use

Before refactoring, before any edit to a public symbol, or whenever an agent
needs to answer "what does X touch?". Replaces the manual chain:

```python
gitnexus_query("validate_user")
gitnexus_context("validate_user")
gitnexus_impact("validate_user")
gitnexus_detect_changes()
```

with a single call:

```python
sin_symbol_resolve("validate_user", depth=2,
                   include="callers,callees,blast,recent")
# → {
#     "symbol": "validate_user",
#     "depth": 2,
#     "include": ["callers", "callees", "blast", "recent"],
#     "callers": [...],
#     "callees": [...],
#     "blast_radius": {"d1": [...], "d2": [...]},
#     "recent_changes": [...],
#     "cross_source": {},
#     "sources_queried": ["gitnexus:context", "gitnexus:impact", ...]
#   }
```

## Graceful degradation

Each sub-source is wrapped in its own `try/except` for `TimeoutExpired`,
`JSONDecodeError`, and generic `Exception`. A missing or failing CLI leaves
its slice empty but does not raise the whole call.

## Notes

- `depth` is currently advisory (1-3) — the gitnexus CLI controls the
  actual traversal depth, this parameter is forwarded via the request body
  where supported.
- `callers` and `callees` share a single CLI call (saves ~50 ms).
- `recent_changes` filters by substring match on the symbol name — fast
  but may produce false positives for short common names; refine the
  search with `file_path` / `kind` arguments on the underlying gitnexus
  call when needed.
