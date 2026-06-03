# lsp_backend.py

LSP-backed symbol resolution for the SCKG. Drives real language servers
(via multilspy: pyright, gopls, typescript-language-server, rust-analyzer,
jdtls, …) and falls back to a tree-sitter textual scan when no LSP is
available. Results are cached at `.sin/cache/`.

## Dependencies

- stdlib: `asyncio`, `dataclasses`, `pathlib`
- optional: `multilspy` (LSP backend)
- the function degrades to tree-sitter if multilspy is missing

## Touched by

- `cache.GraphCache` — wraps every `compute_impact()` result
- `cli.py` — `sin impact` calls `compute_impact()`
- `gitnexus.py` (indirectly) — symbol context for graph nodes

## What it does

1. **`_LANG_BY_EXT`** — file extension → LSP language mapping.
2. **`_lsp_impact(root, file, symbol, line, column)`** — async helper
   that boots a multilspy `LanguageServer`, requests `definition` and
   `references`, and returns a structured `ImpactResult`. Returns
   `None` if the language has no server or multilspy is not installed.
3. **`_treesitter_impact(root, symbol)`** — cheap textual fallback;
   scans the tree for the bare symbol name, classifies each hit as
   "definition" or "caller" based on a simple prefix heuristic.
4. **`compute_impact(root, symbol, file, line, column)`** — public entry
   point. Uses cache; prefers LSP; falls back to tree-sitter; flags
   the result with `source="lsp"` or `source="treesitter"`.

## Important config / scoring

- `_score_risk(callers, touches_tests, touches_api)`:
  - `high` — touches public API OR > 10 callers
  - `medium` — touches tests OR > 3 callers
  - `low` — otherwise
- `_is_test_path(p)` — heuristic: name contains `test`, path has
  `/tests/`, or ends in `_test.py`
- `_is_public_api_path(p)` — heuristic: name is `__init__.py`, `api.py`,
  `index.ts`, `index.js`, `mod.rs`, or `lib.rs`
- The 25-caller cap in callers list — keeps payload small

## Usage

```python
import asyncio
from sin_code_bundle.lsp_backend import compute_impact

result = compute_impact(".", "PoolManager", file="backend/pool_manager.py",
                        line=11, column=6)
print(result.risk, result.fan_in, result.source)
```

## Known caveats

- `_lsp_impact` is `async` and is invoked via `asyncio.run()` from the
  sync `compute_impact`. This will conflict if called from an existing
  event loop — see `lsp_bootstrap.py` for the language server catalog.
- The tree-sitter fallback is **textual**, not type-aware; expect
  false positives in dynamic languages (Python `getattr`, JS bracket
  access).
- `multilspy` API surface can shift between minor versions; pinned in
  `pyproject.toml`.
