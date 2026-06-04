# memory.py

Thin pass-through adapter to the external **`sin-brain`** package. The bundle
holds **no** memory logic itself — this module only:

1. detects whether `sin_brain` is importable (for `sin status`), and
2. exposes the five memory operations as MCP tools registered by `sin serve`.

## Architecture

```
+-----------------------------+
|  sin_code_bundle/memory.py  |   ← THIS module: pass-through adapter
|  (detect_env, register_tools)|
+-------------↓---------------+
              | importlib (lazy)
              v
+-----------------------------+
|  sin_brain.mcp_tools        |   ← REAL memory: SQLite + FTS5
|  (recall, remember, forget, |      1500+ LOC, MIT
|   pin, link_evidence)       |
+-----------------------------+
              |
              v
+-----------------------------+
|  sin_brain                  |   ← AGENTS.md inject, tiered recall,
|  (storage + retrieval core) |      evidence graph, ttl, pin
+-----------------------------+
```

## What this module exports

| Symbol | Purpose |
|---|---|
| `MemoryUnavailable` | Raised when `sin_brain` is not installed. |
| `MemoryEnv` | Dataclass for `detect_env()` output (available, db_path, tiers). |
| `detect_env()` | Cheap import-check; reads `sin_brain.stats()` if present. |
| `recall(query, scope, k)` | Tiered search. Returns JSON. |
| `remember(content, kind, ttl_days, scope)` | Persist. Returns JSON. |
| `forget(id)` | Remove. Returns JSON. |
| `pin(id)` | Protect from eviction. Returns JSON. |
| `link_evidence(entity, verdict, source)` | Attach subsystem verdict to code entity. |
| `inject()` | Return `sin_brain.inject()` AGENTS.md block (SB-4), or `""`. |
| `register_tools(mcp)` | Wire the five tools to an MCP server. Returns list of names. |
| `TOOL_NAMES` | Tuple `("recall", "remember", "forget", "pin", "link_evidence")`. |

## Dependencies

- stdlib: `importlib`, `json`, `dataclasses`, `typing`
- optional: **`sin_brain`** (separate package — `pip install sin-brain`,
  or `pip install sin-code-bundle[memory]`)

The `sin_brain` package is the **only** memory backend. There is no Honcho
integration in this bundle. If you want behavioral memory, install Honcho
separately and run your own server:

```bash
pip install honcho-ai
honcho serve    # default: http://localhost:8000
```

…then call Honcho from your own application code. The bundle does not
proxy to it.

## Touched by

- `cli.py` — no longer references `SINMemory` / `HonchoBackend`; the
  `sin memory` and `sin context` sub-commands were removed in the Honcho
  cleanup commit (see git log).
- `mcp_server.py` — calls `register_tools(mcp)` to wire the five tools.
- `agents_md.py` — calls `inject()` to embed the AGENTS.md block (SB-4).
- `tests/test_memory.py` — adapter-level unit tests with a fake `sin_brain`.

## What it does NOT do (deliberate)

- No SQLite access of its own — `sin_brain` owns the DB and FTS5 schema.
- No Honcho / behavioral-memory proxy.
- No CLI sub-commands — the `sin memory {retain,recall,reflect,stats,forget}`
  and `sin context query` commands referenced the old in-bundle `SINMemory`
  class. That class was moved to `sin-brain` during BR-1 (commit af69464).
  The bundle now exposes memory only as MCP tools, not as a CLI surface.
- No semantic search / vector embeddings — `sin_brain` decides the recall
  backend (currently LIKE + FTS5; vector recall is on the roadmap upstream).

## Usage

### Detect availability

```python
from sin_code_bundle import memory

env = memory.detect_env()
print(env.to_dict())
# → {"available": True, "db_path": "...", "tiers": {...}, "detail": "ok"}
# → {"available": False, "detail": "sin_brain package not importable"}
```

### Call the five operations directly (Python)

```python
from sin_code_bundle import memory

memory.remember("User prefers TypeScript over JavaScript",
                kind="preference", scope="user")
hits = memory.recall("typescript", scope="recall", k=5)
# All five operations return JSON strings.
```

### Register as MCP tools (what `sin serve` does)

```python
from sin_code_bundle import memory
from mcp.server.fastmcp import FastMCP

mcp = FastMCP("sin")
names = memory.register_tools(mcp)
# → ["recall", "remember", "forget", "pin", "link_evidence"]
```

## Important config

- `scope` in `recall` — one of `("recall", "archival", "graph")`. Invalid
  values raise `ValueError` (NOT `MemoryUnavailable`) — they are programming
  errors, not deployment errors.
- `kind` in `remember` — one of `("decision", "convention", "fix", "pitfall", "preference")`.
  These are the kinds AGENTS.md cares about; keep them stable.
- `scope` in `remember` — `("repo", "user")`. Repo-scoped facts survive
  across sessions for the project; user-scoped facts persist across repos.
- `ttl_days` in `remember` — `0` or `None` means no expiry. Otherwise the
  entry is eligible for eviction after N days.
- `source` in `link_evidence` — one of `("oracle", "poc", "ibd", "sckg", "adw")`.
  The five SIN-Code subsystem identifiers. Adding a new one needs a code
  change upstream.

## Known caveats

- The bundle does **not** implement a CLI surface for memory. The
  `sin_brain` package owns its own CLI (when installed) — use that for
  shell-level access.
- `inject()` returns `""` on **any** failure (import, call, exception) —
  by design, AGENTS.md injection must never crash the caller.
- `register_tools()` returns `[]` when `sin_brain` is missing. The MCP
  server is allowed to start with zero memory tools; the agent contract
  is that they degrade gracefully.

## See also

- `sin-brain` package — the real backend
- `src/sin_code_bundle/memory.py` — the adapter
- `tests/test_memory.py` — adapter tests with fake `sin_brain`
- `AGENTS.md` (this repo) — operational mandates around memory tools
