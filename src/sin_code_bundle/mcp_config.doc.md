# mcp_config.py

Generators and idempotent mergers for MCP client configurations for the
three supported coder agents:

- `opencode` → JSON (`opencode.json` with `mcp.<name>` key)
- `codex` → TOML (`[mcp_servers.<name>]` table)
- `hermes` → YAML (`mcp_servers.<name>` map)

Exposes both a single-server mode and a "full" mode that wires in all 15
individual SIN-Code tools (BR-3 / Issue #16).

## Dependencies

- stdlib: `json`, `pathlib`, `typing`
- optional: `pyyaml` (only when merging YAML / Hermes config)

## Touched by

- `cli.py` — `sin mcp-config` and `sin install` use these helpers
- `install.sh` — invoked during bundle install to write user configs

## What it does

1. **Generators** — `generate_opencode / codex / hermes` return the
   raw config string for the single `sin serve` entry.
2. **Full generators** — `generate_full_opencode / codex / hermes` return
   the raw config string with all 15 tools in `FULL_TOOLS`.
3. **Dispatchers** — `generate(client)` and `generate_full(client)` pick
   the right generator; raise `ValueError` for unknown clients.
4. **`default_path(client)`** — conventional config path for the client.
5. **`merge_into_file` / `merge_full_into_file`** — idempotent merger:
   loads existing file, sets the sin (or full) entry, writes back.
   Existing foreign entries are preserved.
6. **TOML helpers** — `_merge_codex_toml*` strips an existing
   `[mcp_servers.sin]` block (and sub-table `.env`) and appends a fresh
   one. Line-based, deliberately simple.
7. **`_strip_toml_table`** — removes a table and all sub-tables by
   header prefix match.

## Important config

- `SERVER_NAME = "sin"` — single-server entry name
- `COMMAND = "sin"`, `ARGS = ["serve"]` — the launch line
- `DEFAULT_ENV = {}` — empty env passthrough; callers can override
- `SUPPORTED_CLIENTS = ("opencode", "codex", "hermes")`
- `FULL_TOOLS` — 16 entries (7 Go binaries + 8 Python MCP servers + 1 SIN-Brain)

## Usage

```python
from pathlib import Path
from sin_code_bundle.mcp_config import generate, merge_into_file

print(generate("opencode"))  # → JSON string for the `sin` MCP server
merge_into_file("opencode", Path("opencode.json"))
# → "Merged 'sin' MCP server into opencode.json"
```

## Known caveats

- The TOML merger is **line-based** and does not parse TOML. It handles
  the format the bundle itself produces and tolerates typical foreign
  tables; malformed input may be mis-stripped.
- `merge_into_file` overwrites any existing `sin` (or `sin-*`) entry on
  the same key; it does not merge per-field changes.
- `FULL_TOOLS` points to `~/.local/bin/<name>`; if a user installs the
  Go tools elsewhere, they must edit the config post-merge.
