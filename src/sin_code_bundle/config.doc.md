# config.py

Layered config store for the SIN-Code CLI. Powers `sin config {show,get,set,unset,path}`.

## Source precedence (lowest → highest)

1. `~/.config/sin/config.toml`     — global TOML defaults
2. `~/.config/opencode/opencode.json` — pulls the `.sin` sub-object
3. `./sin.config.toml`             — project-local override
4. `SIN_*` environment variables   — runtime override (double-underscore = section separator)

## Dependencies

- stdlib: `tomllib`, `json`, `os`, `pathlib`, `dataclasses`
- external: `tomli_w` (lazy import; only needed for `set` / `unset`)

## Touched by

- `cli.py` — `config` `@app.command()` sub-commands
- Tests under `tests/test_config.py`

## Schema (top-level keys, all optional)

- `[tui]` — `theme`, `history_size`, `copy_command`
- `[opencode]` — `model`, `base_url`, `api_key` (redacted in `show`)
- `[paths]` — `bundle_repo`, `go_tools_dir`, `config_dir`
- `[update]` — `channel` (`stable`/`edge`), `auto_check` (bool)

Keys outside this schema are still *resolvable* via `sin config get <key>`
— the schema is documentary, not enforced. The redaction list
(`api_key`, `token`, `password`, `secret`, `private_key`) is hard-coded
in `REDACTED_SUBSTRINGS`.

## Sentinel

`MISSING` (singleton) is returned by `get()` when a key resolves through
no source. Distinct from `None` so callers can tell "never set" from
"explicitly cleared".

## Usage

```python
from sin_code_bundle import config

view = config.get("tui.theme")
print(view.value, "<-", view.source.label if view.source else "MISSING")

# Set + re-read
config.set_value("tui.theme", "light")
config.get("tui.theme")  # → ConfigView(value='light', source=project)
```

## Redaction

`redact(payload)` walks any nested dict/list and replaces values whose
key matches `REDACTED_SUBSTRINGS` (substring, case-insensitive) with
`"<redacted>"`. The default `merged()` call reda secrets; pass
`redact_secrets=False` only when the caller has confirmed the output is
going to a trusted sink (e.g. an authenticated dashboard).

## Env var convention

`SIN_<SECTION>__<LEAF>=value` → `payload[section][leaf] = value`.
Single-underscore keys become top-level entries. Values are always
strings — typed access is the caller's responsibility.

## Known caveats

- The opencode source reads `opencode.json[.sin]` only. Other top-level
  fields in opencode.json are *not* merged.
- `set_value` writes to **project** TOML only. There is no `--global`
  flag — use the `SIN_*` env var or edit the global file directly.
- `unset_value` returns `False` for top-level keys (use a dotted key)
  to match `set_value`'s restriction.
- `merged()` mutates a fresh dict on every call — safe to call from
  multiple threads as long as the underlying files don't change.
