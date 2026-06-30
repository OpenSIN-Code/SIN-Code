# Standards

## Bridged-External pattern
The vibe-notion MCP bridge wraps the globally-installed `vibe-notion` npm CLI
as a subprocess. It never vendors the CLI — it spawns it, parses JSON output,
and returns structured results. This matches SIN-Code's "Bridged-External,
never vendor" philosophy (M6).

## Read/Write separation
Read tools (`notion__notion_read_*`) are auto-allowed in the permission engine.
Write tools (`notion__notion_write_*`) require confirmation (`ask` policy).
In headless mode, `ask` resolves to `deny` unless `--yolo` (M4).

## Secret hygiene
The bridge redacts `token_v2` and `secret_*` patterns from all tool outputs.
Credentials are stored at `~/.config/vibe-notion/credentials.json` and must
never be committed to any repository.

## Fail-closed
If the `vibe-notion` binary is not on PATH, the bridge returns an explicit
error — it never silently succeeds with empty data.
