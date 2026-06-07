# `config.doc.md` — Configuration Management Subcommand

Manages sin-code configuration files stored in `~/.config/sin/`.

## What it does

- **Reads, writes, and lists** configuration values for the sin-code CLI and TUI.
- **Initializes default config files** with sensible defaults.
- **Validates values** on write (e.g., theme must be `dark` or `light`, format must be `text` or `json`).

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `ConfigCmd` and its subcommands into the root cobra command
- `cmd/sin-code/tui.go` — may read `theme` and `default_timeout` from the config file to customize TUI behavior
- `cmd/sin-code/internal/config_test.go` — unit tests for load/save/get/set logic

## Important config values & limits

| Key | Default | Valid values | Description |
|---|---|---|---|
| `theme` | `dark` | `dark`, `light` | TUI color theme |
| `default_timeout` | `60` | any positive integer | Default timeout for long-running commands (seconds) |
| `default_format` | `json` | `text`, `json` | Default output format for subcommands |
| `mcp_server_enabled` | `true` | `true`, `false` | Whether `serve` starts the MCP server by default |

## Config file location

```
~/.config/sin/sin-code.toml
```

## Usage examples

```bash
# Initialize default config files
sin-code config init

# Get a single value
sin-code config get theme

# Set a value (validated on write)
sin-code config set theme light
sin-code config set default_timeout 120

# List all current values
sin-code config list

# Show the config directory path
sin-code config path
```

## Known caveats / footguns

- **TOML-like parsing:** The config parser is a simple line-based `key = value` parser. It does NOT support nested TOML tables, arrays, or complex types. Stick to flat scalar values.
- **Missing config file:** If the file doesn't exist, `loadConfig` returns the default values silently. Use `config init` to create the file explicitly.
- **No automatic reload:** If you edit `sin-code.toml` manually while the TUI is running, changes won't be picked up until the next `sin-code` invocation.
- **Theme validation:** Setting an invalid theme string will error. Only `dark` and `light` are accepted.
