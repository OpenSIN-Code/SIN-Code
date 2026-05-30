# GitNexus Integration

[GitNexus](https://github.com/abhigyanpatwari/GitNexus) builds a queryable code
**knowledge graph** (symbols, call graph, imports, impact) from a repository.
SIN-Code-Bundle treats it as a **mandatory, always-on dependency** so coder
agents (OpenCode, Codex, Hermes, ...) never operate "blind" on a codebase.

## Licensing — why we bridge instead of vendor

GitNexus is distributed under the **PolyForm Noncommercial License 1.0.0**.
SIN-Code-Bundle is **MIT**. To keep the bundle permissively licensed we **do not
copy or vendor any GitNexus source code**. Instead the bridge invokes the
published npm package (`gitnexus`) via `npx` and reads the artifacts it
produces. GitNexus remains the upstream original and receives updates
independently; the bundle simply orchestrates it.

> If you intend to use this stack commercially, review GitNexus' license terms
> for your usage. The bridge does not change GitNexus' license.

## Requirements

- **Node.js >= 18** with `npx` on `PATH` (GitNexus is a Node/TypeScript tool).
- The `gitnexus` package is fetched and cached automatically on first use via
  `npx -y gitnexus@latest` — no manual install required.

## How it works

```
agent ──(MCP)──> gitnexus mcp ──> .gitnexus/ graph  <── sin gitnexus index
  │                                                  
  └── sin preflight  (auto-builds/refreshes the graph before any task)
```

1. **`sin gitnexus setup`** writes the GitNexus MCP server into each agent's
   own config so every CLI coder gets the graph tools individually:
   - OpenCode → `~/.config/opencode/opencode.json` (`mcp.gitnexus`)
   - Codex → `~/.codex/config.toml` (`[mcp_servers.gitnexus]`)
   - Hermes → `~/.hermes/mcp.json` (`mcpServers.gitnexus`)
   Existing config is preserved; the entry is idempotent.
2. **`sin preflight`** guarantees a fresh index before coding. A missing or
   stale (>24h) index is **auto-rebuilt** (`gitnexus analyze`). This is the
   "never code blind" guard.
3. The bundle's own MCP server (`sin serve`) also re-exposes
   `gitnexus_context`, `gitnexus_impact`, and `gitnexus_ai_context`, so agents
   pointed only at the bundle still get graph access.

## Commands

| Command | Purpose |
| --- | --- |
| `sin gitnexus setup [--agents opencode,codex,hermes]` | Wire the GitNexus MCP server into each agent's config. |
| `sin gitnexus index [root] [--force]` | Build/refresh the graph (auto unless `--force`). |
| `sin gitnexus status [root]` | Show on-disk index state (no GitNexus call). |
| `sin gitnexus doctor [root]` | Check Node/npx + index health. |
| `sin gitnexus context <symbol> [--root]` | Structural context for a symbol. |
| `sin gitnexus impact <symbol> [--root]` | Blast-radius impact analysis. |
| `sin gitnexus ai-context "<task>" [--root]` | Task-scoped context bundle. |
| `sin preflight [root] [--no-auto]` | Ensure graph is fresh before agent work. |

## Recommended setup (once per machine + per repo)

```bash
# 1. Machine: wire every coder agent to GitNexus
sin gitnexus setup

# 2. Per repo: make sure the graph exists before coding
sin preflight
```

Wire `sin preflight` into your task runner / pre-task hook so agents always
start with current graph context.
