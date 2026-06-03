# `graphify_service.py` — External `graphify` CLI Wrapper

What this file does: thin wrapper around the external `graphify` binary. Locates it, shells out for `update` / `query` / `explain` / `path` / `install`, and reads `graph.json` directly for the dashboard summary.

## Dependency map

- Imports: stdlib (`json`, `logging`, `os`, `subprocess`, `tempfile`, `pathlib`).
- External dep: the `graphify` binary (auto-located; not a Python import).
- Imported by: `core.py` (via `_graphify_*_impl` aliases).

## Public API

| Function                            | Purpose                                                          |
|-------------------------------------|------------------------------------------------------------------|
| `graphify_update(root)`             | Regenerate the knowledge graph                                   |
| `graphify_query(question, root, budget=2000, dfs=False)` | Ask a question about the graph              |
| `graphify_explain(node, root)`      | Plain-language explanation of a graph node                      |
| `graphify_path(source, target, root)` | Shortest path between two graph nodes                          |
| `graphify_install(platform="opencode")` | One-time install of the platform integration                |
| `graphify_summary(root)`            | Read `graph.json` directly (no CLI call) for the dashboard       |
| `graphify_available()`              | True iff the binary is on the system                             |

## Important config / limits

- **Binary lookup order**: `/opt/homebrew/bin/graphify` (Apple Silicon), `/usr/local/bin/graphify`, `~/.local/bin/graphify`, then `which graphify`.
- **Subprocess timeout: 120s.** Longer graphify runs (huge repos) may need a higher timeout.
- **`graphify-out/graph.json` is the canonical output path.** Every function in this module assumes that layout.
- **`graphify_summary` returns `has_graph: False`** if `graph.json` doesn't exist (a soft-fail, not a crash).

## Design decisions

- **Why shell out instead of importing?** `graphify` is a separate Go binary, not a Python library. Wrapping the CLI is the only way to integrate.
- **Why cache the binary path?** `which graphify` shells out to `/bin/sh`; doing it on every call would dominate latency. The cache is per-process.
- **Why read `graph.json` directly for the summary?** The CLI is slower and produces a different shape. The summary function only needs node/edge counts and a top-10 list, which the JSON has in O(1).

## Usage example

```python
from simone_mcp.graphify_service import graphify_update, graphify_query

# Build the graph (long-running; maybe once per workspace)
graphify_update("/path/to/repo")

# Ask questions
result = graphify_query("Where is auth handled?", "/path/to/repo", budget=2000)
print(result["output"])
```

## Caveats / footguns

- **`graphify_update` writes to `<root>/graphify-out/graph.json`.** Don't run it in read-only filesystems.
- **No lock around `graphify_update`** — two concurrent calls produce a corrupt graph.json. The orchestrator should serialize updates.
- **`graphify_query` returns `error: "No graph found"`** if the graph doesn't exist. Run `graphify_update` first.
