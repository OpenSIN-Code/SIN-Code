# SIN-Code WebUI

The official web frontend is **[SIN-Code-WebUI-v2](https://github.com/OpenSIN-Code/SIN-Code-WebUI-v2)**.
The in-tree `internal/webui` is the legacy embedded UI and is in maintenance
mode — new UI features land in WebUI-v2 only.

## Architecture

WebUI-v2 talks to the SIN-Code backend over the same headless contract as
`sin-code chat -p --json`:

    {"session_id": "...", "summary": "...", "verified": true, "turns": 3}

## Running

    # backend
    sin-code serve --port 8080

    # frontend (separate checkout)
    cd SIN-Code-WebUI-v2 && pnpm install && pnpm dev

## Why two repos?

The predecessor (SIN-Code-Bundle-Web) was archived after drifting from the
backend. To avoid repeating that: every backend contract change (chat JSON,
session API, hook events) MUST be tested against WebUI-v2 in the same
milestone — see ECOSYSTEM.md sync rules.

## Verifying the v3.3.0 contract

```bash
# headless one-shot — must return the stable JSON
sin-code chat -p "add a health endpoint" --json
# {"session_id":"...","summary":"...","verified":true,"turns":3}

# MCP server exposes the same surface to external agents
sin-code serve --port 8080
# -> http://localhost:8080/mcp/v1 (44+ tools)
```
