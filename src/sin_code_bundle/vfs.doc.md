# vfs.py

URI-scheme based virtual filesystem for the SIN-Code v2 agent. Each scheme
maps a logical "address" to a real SIN-Code subsystem (SCKG, POC, IBD, ADW,
EFSM, Oracle, Git conflicts) and resolves it into a JSON-serializable dict
the MCP layer can stream back to the model.

## Dependencies

- stdlib only: `re`, `subprocess`, `pathlib`, `typing`
- Optional: each `sin_code_*` subsystem (lazy-imported, `ImportError` → graceful)

## Touched by

- Future MCP tool `sin://<scheme>/...` — wraps `SINVirtualFS.resolve()`
- Any agent that wants to expose semantic tools over a uniform URL interface
- The `vfs://` URI itself is a *meta* scheme: it doesn't resolve to a file,
  it resolves to structured data

## What it does

1. **`SINVirtualFS(repo_root)`** — owns the working repo path and a
   per-instance resolution cache (`_cache`).
2. **`resolve(uri)`** — parses `<scheme>://<path>`, looks up a `_resolve_<scheme>`
   method, runs it, caches the result. Returns `{"error": ...}` for bad URIs
   or unknown schemes (no exceptions leak out).
3. **`list_schemes()`** — returns the `URI_SCHEMES` dict so callers can
   discover what's available.

### Scheme → resolver mapping

| Scheme | Resolver | Real subsystem used |
|--------|----------|---------------------|
| `sckg://module/<n>/<q>` | `_resolve_sckg` | `sin_code_sckg.KnowledgeGraph` (`build_from_repo`, `get_neighbors`, `query`, `to_dict`) |
| `poc://strategy/<n>` | `_resolve_poc` | `sin_code_poc.list_properties`, `property_metadata` |
| `ibd://diff/<file>` | `_resolve_ibd` | `sin_code_ibd.ASTDiff` |
| `adw://smell/<n>` | `_resolve_adw` | `sin_code_adw.smells` (introspect analyzers) |
| `efsm://service/<n>` | `_resolve_efsm` | `sin_code_efsm.services` (introspect) |
| `oracle://strategy/<n>` | `_resolve_oracle` | `sin_code_oracle.verifier` (introspect) |
| `conflict://*` / `conflict://<N>` | `_resolve_conflict` | `git diff --name-only --diff-filter=U` |

## Important conventions

- **Lazy imports** — every `sin_code_*` import lives inside the resolver, so
  the module imports cleanly on systems where the subsystem isn't installed.
- **No side effects on import** — `build_from_repo()` is only called when a
  `sckg://` URI is actually resolved.
- **Caching** — every `resolve()` call is memoized per `(scheme, path)`. Pass
  a fresh `SINVirtualFS` if you need to bust the cache.

## Usage

```python
from pathlib import Path
from sin_code_bundle.vfs import SINVirtualFS, URI_SCHEMES

vfs = SINVirtualFS(Path("/Users/me/proj"))
print(URI_SCHEMES["sckg"])            # → "Semantic Codebase Knowledge Graph"
vfs.resolve("conflict://*")           # → {"type": "conflict_bulk", "files": [...], "count": 0}
vfs.resolve("sckg://module/auth/neighbors")
# → {"type": "sckg_module", "module": "auth", "query_type": "neighbors", "data": [...]}
vfs.resolve("nope://broken")          # → {"error": "Unknown scheme: nope"}
```

## Known caveats

- The SCKG resolver calls `kg.build_from_repo()` on every cache miss — this
  can be slow on large repos. Wrap with a higher-level cache if needed.
- The `ibd://diff/<path>` resolver does not parse a git ref; pass a
  *workspace-relative* path under `repo_root`.
- `conflict://` requires `git` on `PATH` and the repo to actually be a git
  worktree (a non-git dir returns `{"error": "git failed: ..."}`).
