# `src/simone_mcp/graphify_service.py` — Graphify CLI Integration

Partner file: `src/simone_mcp/graphify_service.py`

## Purpose
Wraps the external `graphify` CLI tool for knowledge graph operations. Discovers `graphify` binary via `which` or known paths. Provides update, query, explain, path, and summary operations.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `_find_graphify()` | function | Locate graphify binary (cached) |
| `_run_graphify()` | function | Execute graphify subprocess with timeout |
| `graphify_update()` | function | Run `graphify update <root>` |
| `graphify_query()` | function | Run `graphify query <question>` |
| `graphify_explain()` | function | Run `graphify explain <node>` |
| `graphify_path()` | function | Run `graphify path <source> <target>` |
| `graphify_install()` | function | Run `graphify install --platform <platform>` |
| `graphify_summary()` | function | Read graph.json and return stats (no CLI call) |
| `graphify_available()` | function | Check if graphify is installed |

## Graphify Binary Discovery
1. `/opt/homebrew/bin/graphify`
2. `/usr/local/bin/graphify`
3. `~/.local/bin/graphify`
4. `which graphify` fallback

## Relationship
- `src/simone_mcp/core.py` — calls graphify functions via `execute_simone_action()` for graphify actions
- `src/simone_mcp/protocol.py` — tasks may trigger graphify operations

## Dependencies
- Standard lib: `subprocess`, `os`, `json`, `tempfile`, `pathlib`
- External: `graphify` CLI (must be installed separately)

## Graph Path Convention
Knowledge graphs are stored at `<root>/graphify-out/graph.json`.

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
