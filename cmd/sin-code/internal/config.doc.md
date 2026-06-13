# `config.go` — Unified Configuration Management

## What this file does

Implements `sin-code config`, the user-facing configuration subsystem for the
unified `sin-code` binary. It supports user-level defaults
(`~/.config/sin/sin-code.toml`), project-level overrides
(`./.sin-code/config.toml`), deep merge, atomic writes, secret masking, and
validation.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `ConfigCmd` as a root subcommand.
- `cmd/sin-code/internal/config_test.go` — unit tests for all config logic.
- `cmd/sin-code/internal/loopbuilder/builder.go` *(future)* — may read
  merged config when constructing the agent loop.

## Important config values & limits

| Key | Type | Default | Valid values |
|---|---|---|---|
| `theme` | string | `dark` | `dark`, `light` |
| `default_timeout` | int | `60` | > 0 |
| `default_format` | string | `json` | `text`, `json` |
| `mcp_server_enabled` | bool | `true` | `true`, `false` |
| `llm.base_url` | string | `https://integrate.api.nvidia.com/v1` | any URL |
| `llm.api_key` | string | `""` | any string (masked in `show`) |
| `llm.model` | string | `""` | any string |
| `llm.max_tokens` | int | `8192` | > 0 |
| `llm.temperature` | float | `0.0` | `[0.0, 2.0]` |
| `agent.verify_mode` | string | `poc` | `off`, `poc`, `oracle` |
| `agent.max_turns` | int | `80` | > 0 |
| `agent.headless` | bool | `false` | `true`, `false` |
| `agent.yolo` | bool | `false` | `true`, `false` |
| `permissions.tools_allow` | []string | `[]` | comma-separated globs |
| `permissions.tools_deny` | []string | `[]` | comma-separated globs |
| `paths.mcp_config` | string | `~/.sin-code/mcp.json` | any path |
| `paths.skills_dir` | string | `""` | any path |

## Why certain decisions were made

- **Flat namespaced keys instead of TOML sections**: The parser is intentionally
  dependency-free (M2 — single static binary). Using `llm.base_url` keeps the
  file human-readable while avoiding a full TOML parser.
- **Deep merge with raw key maps**: Only keys actually present in a file override
  the parent level. This prevents zero-value booleans from silently disabling
  user defaults when a project config only changes an unrelated key.
- **Atomic writes via temp file + rename**: Readers never see a half-written
  config file during concurrent save operations.
- **Secret masking in `show`**: `llm.api_key` is the only secret today; it is
  masked as `sk-1...cdef` unless `--plain` is passed. This prevents accidental
  leakage in terminal logs.
- **Project config path**: `./.sin-code/config.toml` mirrors the user's
  `~/.config/sin/sin-code.toml` directory structure and is easy to add to
  `.gitignore`.

## Usage examples

```bash
# Create default config files
sin-code config init

# Set a value (validates the value and saves atomically)
sin-code config set theme light
sin-code config set llm.api_key sk-...

# Show merged config, masking secrets
sin-code config show

# Show with TOML or JSON output
sin-code config show --toml
sin-code config show --json --plain

# Validate merged config
sin-code config validate
```

## Known caveats or footguns

- The manual parser does **not** support TOML section headers (`[llm]`). Keys
  must be written flat (`llm.base_url = ...`).
- `config set` only writes to the user config file. Project-level values must
  be edited manually or by tooling that creates `./.sin-code/config.toml`.
- Empty string values are not distinguishable from "not set" for string fields
  during merge. Use a sentinel value or explicit override if that matters.
- The config file path is `~/.config/sin/sin-code.toml`, **not**
  `~/.config/sin-code/config.json` (which appears in older AGENTS.md drafts).
  This is preserved for backwards compatibility with the existing tests.
