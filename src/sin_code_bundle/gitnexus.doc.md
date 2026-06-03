# gitnexus.py

GitNexus bridge — invokes the upstream `gitnexus` npm package via `npx`
and wires the GitNexus MCP server into OpenCode / Codex / Hermes. Never
vendors or copies GitNexus source; MIT-licensed bundle stays clean.

## Dependencies

- stdlib: `json`, `os`, `shutil`, `subprocess`, `time`, `dataclasses`
- external: `npx` (Node.js ≥ 18) at runtime

## Touched by

- `cli.py` — `sin gitnexus doctor|setup|index|status|context|impact|ai-context`
- `install.sh` — `sin gitnexus setup` is invoked during bundle install

## What it does

1. **`detect_env()`** — locates `node` and `npx` on `PATH`; does not mutate.
2. **`analyze(root)`** — runs `npx -y gitnexus@latest analyze --path <root>`
   to build/refresh the per-repo `.gitnexus/` index.
3. **`ensure_index(root, auto=True)`** — rebuilds the index when missing or
   older than `DEFAULT_STALE_SECONDS` (24h). With `auto=False` the caller
   is told to index but nothing is mutated.
4. **`ai_context / query / context / impact`** — thin wrappers over
   the corresponding GitNexus CLI subcommands; return raw `stdout`.
5. **`doctor(root)`** — aggregate health report (runtime + index status).
6. **`setup_agents(agents)`** — wires the GitNexus MCP server into
   `~/.config/opencode/opencode.json` (JSON), `~/.codex/config.toml`
   (TOML), `~/.hermes/mcp.json` (JSON). Idempotent: existing entries
   are replaced, user edits elsewhere are preserved.

## Important config

- `GITNEXUS_PACKAGE = "gitnexus@latest"` — pinned to upstream; do not
  override in normal operation
- `DEFAULT_STALE_SECONDS = 24 * 60 * 60` — re-index threshold
- `INDEX_DIRNAME = ".gitnexus"` — per-repo index location
- `AGENTS = ("opencode", "codex", "hermes")` — the three supported coders

## Usage

```python
from sin_code_bundle import gitnexus

# Health check
print(gitnexus.doctor("."))

# Force a re-index
gitnexus.ensure_index(".", auto=True)

# Wire MCP into every supported agent
written = gitnexus.setup_agents()
print(written)  # {"opencode": "/Users/.../opencode.json", ...}
```

## Known caveats

- `_run` raises `GitNexusError` on missing `npx` or timeout (default
  900s, 1800s for `analyze`). Callers should catch and degrade gracefully.
- `_wire_opencode` / `_wire_codex` / `_wire_hermes` overwrite any
  existing `gitnexus` MCP entry on the same key — they do NOT merge
  per-field changes.
- `index_state()` uses *file mtime* as a proxy for "freshness"; if the
  user deletes a file inside `.gitnexus/`, the state can briefly appear
  stale.
