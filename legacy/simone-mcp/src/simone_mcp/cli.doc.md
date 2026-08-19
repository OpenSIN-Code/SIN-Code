# `cli.py` (simone_mcp) — CLI Implementation

What this file does: the real `simone` CLI implementation. The `src/cli.py` shim is a 1-line delegate to `main()` here. Supports `serve`, `serve-mcp`, `print-card`, `run-action`, `index`, `validate`, `integrate`, `tool-list`.

## Dependency map

- Imports: `core.TOOL_DEFINITIONS`, `core.build_agent_card`, `core.execute_simone_action`, `core.get_project_overview`, `mcp_stdio.serve_stdio`, plus stdlib.
- Imported by: `src/cli.py` (entry point), `simone_mcp/__init__.py` (re-exports `main`).

## Subcommands

| Subcommand    | Purpose                                                                                | Default port / path       |
|---------------|----------------------------------------------------------------------------------------|---------------------------|
| `serve`       | Start HTTP + A2A + MCP server (uvicorn)                                                | `127.0.0.1:8234`           |
| `serve-mcp`   | Start stdio MCP server (for OpenCode / Claude Desktop)                                | stdin/stdout               |
| `print-card`  | Print the A2A agent card as JSON                                                      | —                         |
| `run-action`  | Execute a single action from a JSON payload on stdin / argv                           | —                         |
| `index`       | Print `get_project_overview` for `PATH` (default cwd)                                  | `cwd`                      |
| `validate`    | Validate server config (QDRANT_URL, NEO4J_URI, etc.) and exit non-zero on issues         | —                         |
| `integrate`   | Patch `~/.config/opencode/opencode.json` to add the Simone MCP server + disable grep    | —                         |
| `tool-list`   | Print the MCP tool definitions as JSON                                                 | —                         |

## Important config / limits

- **`SIMONE_HOST`** env var: defaults to `0.0.0.0` for `serve` (exposes to network). Set to `127.0.0.1` for loopback only.
- **`SIMONE_BASE_URL`** env var: used to build the `url` field in the agent card. Defaults to `http://localhost:{port}`.
- **Default port: 8234** (configurable via `serve <port>`).
- **`integrate` patches OpenCode config**: it backs up `opencode.json` to `opencode.json.bak` once, then sets `mcp.sin-simone-mcp` and `permission.grep/glob` to `deny`. Idempotent.

## Design decisions

- **Why an `integrate` subcommand?** A discoverable one-liner for the most common setup task. Without it, users would have to hand-edit `opencode.json`.
- **Why `validate` exits non-zero on issues?** Lets CI catch misconfigured servers before they hit production.
- **Why lazy-import `uvicorn`?** Keeps `simone tool-list` and `simone validate` fast (no uvicorn import cost).

## Usage examples

```bash
# Start the server on the default port
simone serve

# Start on a custom port
simone serve 9000

# Run a single action from a JSON payload
simone run-action '{"action": "simone.mcp.health"}'

# Validate the server config
simone validate

# Wire into OpenCode (one-time)
simone integrate
```

## Caveats / footguns

- `integrate` modifies the user's `~/.config/opencode/opencode.json` in place (with backup). Inspect the diff before re-running.
- The `print-card` URL uses `SIMONE_BASE_URL` (or `localhost:8234`). If you're behind a reverse proxy, set this to the public URL.
- `validate` checks QDRANT/Neo4j connectivity by actually connecting. Don't run it in environments where outbound connections are blocked.
