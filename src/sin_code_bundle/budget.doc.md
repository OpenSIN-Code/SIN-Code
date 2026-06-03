# budget.py

Recursively trims MCP tool outputs so they don't blow the agent's context
window. Adds explicit `_truncated` markers instead of silent clipping so
the agent knows when data is missing.

## Dependencies

- stdlib only (`typing.Any`)

## Touched by

- `cli.py` — wraps every MCP tool response with `trim()` before returning
- Any new MCP tool handler should call `trim()` on its return value

## What it does

1. **Strings** — truncated to `MAX_STR` (default 2000) with a trailing
   ` ...[truncated]` marker.
2. **Lists** — capped to `MAX_LIST` (default 25) entries; an extra dict
   `{"_truncated": True, "_omitted": <n>}` is appended when clipping happens.
3. **Dicts** — recursively trimmed; keys are preserved.
4. **Other** (int, bool, None) — passed through unchanged.

## Important constants

- `MAX_LIST = 25` — empirical: 25 list items is roughly the boundary where
  a model starts ignoring later entries
- `MAX_STR = 2000` — ~500 tokens; safe for any modern context window

## Usage

```python
from sin_code_bundle.budget import trim

trim({"items": list(range(100))})
# → {"items": [0, 1, ..., 24, {"_truncated": True, "_omitted": 75}]}
```

## Known caveats

- The `_truncated` flag is a *convention* — tools that don't funnel their
  output through `trim()` will not be protected.
- `trim()` is a no-op for non-container scalars; do not pass a
  pre-truncated string back through it expecting different behavior.
