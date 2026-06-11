# index_store.go, index_cmd.go, scout_indexed.go

What: Persistent incremental code index — trigram + symbol table for instant lookups.

Who touches it: scout (production search), index CLI, MCP server (sin_index tool).

Key decisions:
- gob-persisted at `<root>/.sin-code/index.bin` (no external deps).
- Trigram FNV-1a hashing for content search pruning.
- Symbol table populated via `parseOutline()` (AST tiered engine).
- Auto-build on first search, auto-refresh on subsequent calls.
- Parallel worker pool (8 workers) for index build.
- `.gitignore` aware (uses same `walkScout` logic).
- Binary files indexed with empty trigrams.

Consumers:
- `scoutSearchAuto` — the production search entry point.
- `handleScout` MCP handler — calls `scoutSearchAuto` directly.
- `scout` CLI — calls `scoutSearchAuto` in its RunE.
- `sin_index` MCP tool — build/refresh/status/clear.

No CoDocs companion needed for `index_test.go` (test file).
